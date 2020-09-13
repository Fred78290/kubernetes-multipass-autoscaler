#/bin/bash

# This script customize bionic-server-cloudimg-amd64.img to include docker+kubernetes
# Before running this script, you must install some elements with the command below
# sudo apt install qemu-kvm libvirt-clients libvirt-daemon-system bridge-utils virt-manager
# This process disable netplan and use old /etc/network/interfaces because I don't now why each VM instance running the customized image
# have the same IP with different mac address.

# /usr/lib/python3/dist-packages/cloudinit/net/netplan.py
CURDIR=$(dirname $0)
KUBERNETES_VERSION=$(curl -sSL https://dl.k8s.io/release/stable.txt)
KUBERNETES_PASSWORD=$(uuidgen)
SSH_KEY=$(cat ~/.ssh/id_rsa.pub)
SSH_PRIV_KEY="~/.ssh/id_rsa"
CNI_VERSION=v0.8.6
CACHE=~/.cache
TEMP=$(getopt -o i:k:n:p:v: --long ssh-pub-key:,ssh-priv-key:,custom-image:,cni-version:,password:,kubernetes-version: -n "$0" -- "$@")
eval set -- "$TEMP"

PACKER_LOG=0

# extract options and their arguments into variables.
while true; do
    #echo "1:$1"
    case "$1" in
    --ssh-pub-key)
        SSH_KEY="$(cat $2)"
        shift 2
        ;;
    --ssh-priv-key)
        SSH_PRIV_KEY="$2"
        shift 2
        ;;
    -i | --custom-image)
        TARGET_IMAGE="$2"
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

if [ -z $TARGET_IMAGE ]; then
    TARGET_IMAGE=$CURDIR/../images/bionic-k8s-$KUBERNETES_VERSION-amd64.img
fi

# Grab nameserver/domainname
INIT_SCRIPT=/tmp/prepare-k8s-bionic.sh
KUBERNETES_MINOR_RELEASE=$(echo -n $KUBERNETES_VERSION | tr '.' ' ' | awk '{ print $2 }')

sudo apt install qemu qemu-kvm -y

if [ -z $(command -v packer) ]; then
    curl -fsSL https://apt.releases.hashicorp.com/gpg | sudo apt-key add -
    sudo apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main"
    sudo apt-get update
    sudo apt-get install packer -y
fi

cat > $INIT_SCRIPT <<EOF
#/bin/bash

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get upgrade -y
apt-get dist-upgrade -y
apt-get install jq socat conntrack -y

apt-get autoremove -y

# Setup daemon.
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
systemctl enable docker
systemctl restart docker

mkdir -p /opt/cni/bin
curl -L "https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-linux-amd64-${CNI_VERSION}.tgz" | tar -C /opt/cni/bin -xz

sed -i 's/PasswordAuthentication/#PasswordAuthentication/g' /etc/ssh/sshd_config 

mkdir -p /usr/local/bin
cd /usr/local/bin
curl -L --remote-name-all https://storage.googleapis.com/kubernetes-release/release/${KUBERNETES_VERSION}/bin/linux/amd64/{kubeadm,kubelet,kubectl,kube-proxy}
chmod +x /usr/local/bin/kube*

mkdir -p /etc/systemd/system/kubelet.service.d

echo "KUBELET_EXTRA_ARGS='--fail-swap-on=false --read-only-port=10255'" > /etc/default/kubelet

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

/usr/local/bin/kubeadm config images pull --kubernetes-version=${KUBERNETES_VERSION}

echo "kubeadm installed"
exit 0
EOF

chmod +x /tmp/prepare-k8s-bionic.sh

mkdir -p $CACHE/packer/cloud-data

echo -n > $CACHE/packer/cloud-data/meta-data
cat >  $CACHE/packer/cloud-data/user-data <<EOF
#cloud-config
ssh_pwauth: true
users:
  - name: packer
    sudo: ALL=(ALL) NOPASSWD:ALL
    groups: users, admin
    ssh_authorized_keys:
      - $SSH_KEY
    lock_passwd: true
apt:
    preserve_sources_list: true
package_update: false
EOF

ISO_CHECKSUM=$(curl -s "http://cloud-images.ubuntu.com/releases/bionic/release/MD5SUMS" | grep "ubuntu-18.04-server-cloudimg-amd64.img" | awk '{print $1}')
cp $CURDIR/../templates/packer/template.json $CACHE/packer/template.json
pushd $CACHE/packer
packer build -var SSH_PRIV_KEY="$SSH_PRIV_KEY" -var ISO_CHECKSUM="md5:$ISO_CHECKSUM" -var INIT_SCRIPT="$INIT_SCRIPT" -var KUBERNETES_PASSWORD="$KUBERNETES_PASSWORD" template.json
mv output-qemu/packer-qemu $TARGET_IMAGE
popd

rm -rf $CACHE/packer

echo "Created image $TARGET_IMAGE with kubernetes version $KUBERNETES_VERSION"
