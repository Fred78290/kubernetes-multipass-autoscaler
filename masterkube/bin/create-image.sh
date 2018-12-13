#/bin/bash

# This script customize bionic-server-cloudimg-amd64.img to include docker+kubernetes
# Before running this script, you must install some elements with the command below
# sudo apt install qemu-kvm libvirt-clients libvirt-daemon-system bridge-utils virt-manager
# This process disable netplan and use old /etc/network/interfaces because I don't now why each VM instance running the customized image
# have the same IP with different mac address.

[ -z $1 ] && TARGET_IMAGE=$HOME/.local/multipass/cache/bionic-kubernetes-amd64.img || TARGET_IMAGE=$1
[ -z $2 ] && KUBERNETES_VERSION=$(curl -sSL https://dl.k8s.io/release/stable.txt) || KUBERNETES_VERSION=$2

# Hack because virt-customize doesn't recopy the good /etc/resolv.conf due dnsmasq
if [ -f /run/systemd/resolve/resolv.conf ]; then
    RESOLVCONF=/run/systemd/resolve/resolv.conf
else
    RESOLVCONF=/etc/resolv.conf
fi

# Grab nameserver/domainname
NAMESERVER=$(grep nameserver $RESOLVCONF | awk '{print $2}')
DOMAINNAME=$(grep search $RESOLVCONF | awk '{print $2}')
CNI_VERSION=v0.7.1

cat > /tmp/prepare-k8s-bionic.sh <<EOF
#/bin/bash

echo "nameserver $NAMESERVER" > /etc/resolv.conf 
echo "search $DOMAINNAME" >> /etc/resolv.conf 

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get upgrade -y

apt-get install jq -y

curl https://get.docker.com | bash

mkdir -p /opt/cni/bin
curl -L "https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-amd64-${CNI_VERSION}.tgz" | tar -C /opt/cni/bin -xz

sed -i 's/PasswordAuthentication/#PasswordAuthentication/g' /etc/ssh/sshd_config 

mkdir -p /usr/local/bin
cd /usr/local/bin
curl -L --remote-name-all https://storage.googleapis.com/kubernetes-release/release/${KUBERNETES_VERSION}/bin/linux/amd64/{kubeadm,kubelet,kubectl}
chmod +x /usr/local/bin/kube*

curl -sSL "https://raw.githubusercontent.com/kubernetes/kubernetes/${KUBERNETES_VERSION}/build/debs/kubelet.service" | sed "s:/usr/bin:/usr/local/bin:g" > /etc/systemd/system/kubelet.service
mkdir -p /etc/systemd/system/kubelet.service.d
curl -sSL "https://raw.githubusercontent.com/kubernetes/kubernetes/${KUBERNETES_VERSION}/build/debs/10-kubeadm.conf" | sed "s:/usr/bin:/usr/local/bin:g" > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf

systemctl enable kubelet

echo 'export PATH=/opt/cni/bin:\$PATH' >> /etc/profile.d/apps-bin-path.sh
export PATH=/opt/cni/bin:/usr/local/bin:\$PATH

kubeadm config images pull

apt-get autoremove -y

apt-get install ifupdown

echo "network: {config: disabled}" > /etc/cloud/cloud.cfg.d/99-disable-network-config.cfg

echo "auto lo" >> /etc/network/interfaces
echo "iface lo inet loopback" >> /etc/network/interfaces
echo >> /etc/network/interfaces
echo "auto ens3" >> /etc/network/interfaces
echo "iface ens3 inet dhcp" >> /etc/network/interfaces

#dpkg-reconfigure cloud-init
EOF

chmod +x /tmp/prepare-k8s-bionic.sh

if [ ! -f ~/.local/multipass/cache/bionic-server-cloudimg-amd64.img ]; then
    mkdir -p ~/.local/multipass/cache

    wget https://cloud-images.ubuntu.com/bionic/current/bionic-server-cloudimg-amd64.img -O ~/.local/multipass/cache/bionic-server-cloudimg-amd64.img
fi

cp ~/.local/multipass/cache/bionic-server-cloudimg-amd64.img $TARGET_IMAGE

qemu-img resize $TARGET_IMAGE 5G
sudo virt-customize --network -a $TARGET_IMAGE --timezone Europe/Paris
sudo virt-customize --network -a $TARGET_IMAGE --root-password password:$(uuidgen)
sudo virt-customize --network -a $TARGET_IMAGE --copy-in  /tmp/prepare-k8s-bionic.sh:/usr/local/bin
sudo virt-customize --network -a $TARGET_IMAGE  --run-command /usr/local/bin/prepare-k8s-bionic.sh
sudo virt-customize --network -a $TARGET_IMAGE  --run-command "/bin/rm /etc/machine-id"
sudo virt-customize --network -a $TARGET_IMAGE  --firstboot-command "/bin/rm /etc/machine-id; /bin/systemd-machine-id-setup"
sudo virt-sysprep -a $TARGET_IMAGE

rm /tmp/prepare-k8s-bionic.sh

echo "Created image $TARGET_IMAGE with kubernetes version $KUBERNETES_VERSION"
