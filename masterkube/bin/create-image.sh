#/bin/bash

# This script customize bionic-server-cloudimg-amd64.img to include docker+kubernetes
# Before running this script, you must install some elements with the command below
# sudo apt install qemu-kvm libvirt-clients libvirt-daemon-system bridge-utils virt-manager
# This process disable netplan and use old /etc/network/interfaces because I don't now why each VM instance running the customized image
# have the same IP with different mac address.

# /usr/lib/python3/dist-packages/cloudinit/net/netplan.py

KUBERNETES_VERSION=$(curl -sSL https://dl.k8s.io/release/stable.txt)
KUBERNETES_PASSWORD=$(uuidgen)
TARGET_IMAGE=$HOME/.local/multipass/cache/bionic-k8s-$KUBERNETES_VERSION-amd64.img
CNI_VERSION=v0.8.5
CACHE=~/.local/multipass/cache
TEMP=$(getopt -o i:k:n:p:v: --long custom-image:,ssh-key:,cni-version:,password:,kubernetes-version: -n "$0" -- "$@")
eval set -- "$TEMP"

# extract options and their arguments into variables.
while true; do
    #echo "1:$1"
    case "$1" in
    -i | --custom-image)
        TARGET_IMAGE="$2"
        shift 2
        ;;
    -k | --ssh-key)
        SSH_KEY=$2
        shift 2
        ;;
    -n | --cni-version)
        CNI_VERSION=$2
        shift 2
        ;;
    -p | --password)
        KUBERNETES_PASSWORD=$2
        shift 2
        ;;
    -v | --kubernetes-version)
        KUBERNETES_VERSION=$2
        TARGET_IMAGE=$HOME/.local/multipass/cache/bionic-k8s-$KUBERNETES_VERSION-amd64.img
        shift 2
        ;;
    --)
        shift
        break
        ;;
    *)
        echo "$1 - Internal error!"
        exit 1
        ;;
    esac
done

# Hack because virt-customize doesn't recopy the good /etc/resolv.conf due systemd-resolved.service
if [ -f /run/systemd/resolve/resolv.conf ]; then
    RESOLVCONF=/run/systemd/resolve/resolv.conf
else
    RESOLVCONF=/etc/resolv.conf
fi

# Grab nameserver/domainname
NAMESERVER=$(grep nameserver $RESOLVCONF | awk '{print $2}')
DOMAINNAME=$(grep search $RESOLVCONF | awk '{print $2}')
INIT_SCRIPT=/tmp/prepare-k8s-bionic.sh
KUBERNETES_MINOR_RELEASE=$(echo -n $KUBERNETES_VERSION | tr '.' ' ' | awk '{ print $2 }')

cat > $INIT_SCRIPT <<EOF
#/bin/bash

echo "nameserver $NAMESERVER" > /etc/resolv.conf
echo "search $DOMAINNAME" >> /etc/resolv.conf 

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get upgrade -y
apt-get dist-upgrade -y
apt-get install jq socat conntrack -y

apt-get autoremove -y

# Setup daemon.
if [ $KUBERNETES_MINOR_RELEASE -ge 14 ]; then
    mkdir -p /etc/docker

    cat > /etc/docker/daemon.json <<SHELL
{
    "exec-opts": [
        "native.cgroupdriver=systemd"
    ],
    "log-driver": "json-file",
    "log-opts": {
        "max-size": "100m"
    },
    "storage-driver": "overlay2"
}
SHELL

    curl https://get.docker.com | bash

    mkdir -p /etc/systemd/system/docker.service.d

    # Restart docker.
    systemctl daemon-reload
    systemctl restart docker
else
    curl https://get.docker.com | bash
fi

# Setup Kube DNS resolver
#mkdir -p /etc/systemd
#cat > /etc/systemd/resolved.conf <<SHELL
#  This file is part of systemd.
#
#  systemd is free software; you can redistribute it and/or modify it
#  under the terms of the GNU Lesser General Public License as published by
#  the Free Software Foundation; either version 2.1 of the License, or
#  (at your option) any later version.
#
# Entries in this file show the compile time defaults.
# You can change settings by editing this file.
# Defaults can be restored by simply deleting this file.
#
# See resolved.conf(5) for details

#[Resolve]
#DNS=10.96.0.10
#FallbackDNS=
#Domains=~cluster.local
#LLMNR=no
#MulticastDNS=no
#DNSSEC=no
#Cache=yes
#DNSStubListener=yes
#SHELL

mkdir -p /opt/cni/bin
curl -L "https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-linux-amd64-${CNI_VERSION}.tgz" | tar -C /opt/cni/bin -xz

sed -i 's/PasswordAuthentication/#PasswordAuthentication/g' /etc/ssh/sshd_config 

mkdir -p /usr/local/bin
cd /usr/local/bin
curl -L --remote-name-all https://storage.googleapis.com/kubernetes-release/release/${KUBERNETES_VERSION}/bin/linux/amd64/{kubeadm,kubelet,kubectl,kube-proxy}
chmod +x /usr/local/bin/kube*

mkdir -p /etc/systemd/system/kubelet.service.d

echo "KUBELET_EXTRA_ARGS='--fail-swap-on=false --read-only-port=10255 --feature-gates=VolumeSubpathEnvExpansion=true'" > /etc/default/kubelet

cat > /etc/systemd/system/kubelet.service <<SHELL
[Unit]
Description=kubelet: The Kubernetes Node Agent
Documentation=http://kubernetes.io/docs/

[Service]
ExecStart=/usr/local/bin/kubelet
Restart=always
StartLimitInterval=0
RestartSec=10

[Install]
WantedBy=multi-user.target
SHELL

cat > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf <<"SHELL"
# Note: This dropin only works with kubeadm and kubelet v1.11+
[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml"
# This is a file that "kubeadm init" and "kubeadm join" generate at runtime, populating the KUBELET_KUBEADM_ARGS variable dynamically
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
# This is a file that the user can use for overrides of the kubelet args as a last resort. Preferably, the user should use
# the .NodeRegistration.KubeletExtraArgs object in the configuration files instead. KUBELET_EXTRA_ARGS should be sourced from this file.
EnvironmentFile=-/etc/default/kubelet
ExecStart=
ExecStart=/usr/local/bin/kubelet \$KUBELET_KUBECONFIG_ARGS \$KUBELET_CONFIG_ARGS \$KUBELET_KUBEADM_ARGS \$KUBELET_EXTRA_ARGS
SHELL

systemctl enable kubelet

echo 'export PATH=/opt/cni/bin:\$PATH' >> /etc/profile.d/apps-bin-path.sh
export PATH=/opt/cni/bin:\$PATH

/usr/local/bin/kubeadm config images pull --kubernetes-version=${KUBERNETES_VERION}

[ -f /etc/cloud/cloud.cfg.d/50-curtin-networking.cfg ] && rm /etc/cloud/cloud.cfg.d/50-curtin-networking.cfg
rm /etc/netplan/*
rm /etc/machine-id
cloud-init clean
rm /var/log/cloud-ini*
rm /var/log/syslog

cat > /lib/systemd/system/systemd-machine-id.service <<SHELL
#  This file is part of systemd.
#
#  systemd is free software; you can redistribute it and/or modify it
#  under the terms of the GNU Lesser General Public License as published by
#  the Free Software Foundation; either version 2.1 of the License, or
#  (at your option) any later version.

[Unit]
Description=Regenerate machine-id if missing
Documentation=man:systemd-machine-id(1)
DefaultDependencies=no
Conflicts=shutdown.target
After=systemd-remount-fs.service
Before=systemd-sysusers.service sysinit.target shutdown.target
ConditionPathIsReadWrite=/etc
ConditionFirstBoot=yes

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/systemd-machine-id-setup
StandardOutput=tty
StandardInput=tty
StandardError=tty

[Install]
WantedBy=sysinit.target
SHELL

chown root:root /lib/systemd/system/systemd-machine-id.service

systemctl enable systemd-machine-id.service

exit 0
EOF

chmod +x /tmp/prepare-k8s-bionic.sh

[ -d $CACHE ] || mkdir -p $CACHE

if [ ! -f $CACHE/bionic-server-cloudimg-amd64.img ]; then
    wget https://cloud-images.ubuntu.com/bionic/current/bionic-server-cloudimg-amd64.img -O $CACHE/bionic-server-cloudimg-amd64.img
fi

cp $CACHE/bionic-server-cloudimg-amd64.img $TARGET_IMAGE

qemu-img resize $TARGET_IMAGE 5G
sudo virt-sysprep --network -a $TARGET_IMAGE --timezone Europe/Paris --root-password password:$KUBERNETES_PASSWORD --copy-in $INIT_SCRIPT:/tmp --run-command $INIT_SCRIPT

#rm /tmp/prepare-k8s-bionic.sh

echo "Created image $TARGET_IMAGE with kubernetes version $KUBERNETES_VERSION"
