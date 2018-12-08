#/bin/bash
KUBERNETES_VERSION=v1.12.3
CURDIR=$(dirname $0)

pushd $CURDIR

export PATH=$PWD:$PATH

multipass delete masterkube -p &> /dev/null

kubeconfig-delete.sh masterkube

multipass launch -n masterkube -m 4096 -c 2

multipass mount $PWD masterkube:/masterkube/bin
multipass mount $PWD/../cluster masterkube:/etc/cluster
multipass mount $PWD/../kubernetes masterkube:/etc/kubernetes

multipass shell masterkube <<EOF
echo "Install update"
sudo bash -c "export DEBIAN_FRONTEND=noninteractive ; apt-get update ; apt-get upgrade -y"
echo "Install jq"
sudo apt-get install jq -y
echo "Install kubernetes"
sudo bash -c "export PATH=/masterkube/bin:$PATH; install-kubernetes.sh $KUBERNETES_VERSION"
sudo usermod -aG docker multipass
exit
EOF

multipass restart masterkube

multipass shell masterkube <<EOF
sudo bash -c "export PATH=/opt/bin:/opt/cni/bin:/masterkube/bin:$PATH; create-cluster.sh flannel ens3 $KUBERNETES_VERSION"
TOKEN=$(cat /etc/cluster/token)
CACERT=$(cat /etc/cluster/ca.cert)
echo "kubeadm token=$TOKEN"
echo "kubeadm ca.cert=sha256:$CACERT"
exit
EOF

kubeconfig-merge.sh masterkube $PWD/../etc/config

popd