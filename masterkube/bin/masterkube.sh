#/bin/bash
KUBERNETES_VERSION=v1.12.3
CURDIR=$(dirname $0)

pushd $CURDIR

export PATH=$PWD:$PATH

rm -rf $PWD/../cluster/*
rm -rf $PWD/../kubernetes/*

echo "Delete masterkube previous instance"
multipass delete masterkube -p &> /dev/null

kubeconfig-delete.sh masterkube

echo "Launch masterkube instance"

multipass launch -n masterkube -m 4096 -c 2

multipass mount $PWD masterkube:/masterkube/bin
multipass mount $PWD/../cluster masterkube:/etc/cluster
multipass mount $PWD/../kubernetes masterkube:/etc/kubernetes

echo "Prepare masterkube instance"

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

echo "Restart masterkube instance"

multipass restart masterkube

echo "Start kubernetes masterkube instance master node"

multipass shell masterkube <<EOF
sudo bash -c "export PATH=/opt/bin:/opt/cni/bin:/masterkube/bin:$PATH; create-cluster.sh flannel ens3 $KUBERNETES_VERSION"
exit
EOF

MASTER_IP=$(cat $PWD/../cluster/manager-ip)
TOKEN=$(cat $PWD/../cluster/token)
CACERT=$(cat $PWD/../cluster/ca.cert)
SSH_KEY=$(cat ~/.ssh/id_rsa.pub)

kubeconfig-merge.sh masterkube $PWD/../cluster/config

echo "Write multipass cloudautoscaler provider config"

cat > $PWD/../config/config.json <<EOF
{
    "listen": "127.0.0.1:5200",
    "secret": "multipass",
    "minNode": 0,
    "maxNode": 5,
    "nodePrice": 0.0,
    "podPrice": 0.0,
    "image": "bionic",
    "auto-provision": true,
    "kubeconfig": "/etc/kubernetes/config",
    "optionals": {
        "pricing": false,
        "getAvailableMachineTypes": false,
        "newNodeGroup": false,
        "templateNodeInfo": false,
        "createNodeGroup": false,
        "deleteNodeGroup": false
    },
    "kubeadm": {
        "address": "$MASTER_IP",
        "token": "$TOKEN",
        "ca": "sha256:$CACERT",
        "extras-args": [
            "--ignore-preflight-errors=All"
        ]
    },
    "machines": {
        "tiny": {
            "memsize": 2048,
            "vcpus": 2,
            "disksize": 5120
        },
        "medium": {
            "memsize": 4096,
            "vcpus": 2,
            "disksize": 10240
        },
        "large": {
            "memsize": 8192,
            "vcpus": 4,
            "disksize": 20480
        },
        "extra-large": {
            "memsize": 16384,
            "vcpus": 8,
            "disksize": 51200
        }
    },
    "cloud-init": {
        "package_update": true,
        "package_upgrade": true,
        "runcmd": [
            "export CNI_VERSION=v0.7.1",
            "export RELEASE=v1.12.3",
            "curl https://get.docker.com | bash",
            "mkdir -p /opt/cni/bin",
            "curl -L https://github.com/containernetworking/plugins/releases/download/\${CNI_VERSION}/cni-plugins-amd64-\${CNI_VERSION}.tgz | tar -C /opt/cni/bin -xz",
            "mkdir -p /usr/local/bin",
            "cd /usr/local/bin ; curl -L --remote-name-all https://storage.googleapis.com/kubernetes-release/release/\${RELEASE}/bin/linux/amd64/{kubeadm,kubelet,kubectl}",
            "chmod +x /usr/local/bin/kube*",
            "echo \"KUBELET_EXTRA_ARGS='--fail-swap-on=false --read-only-port=10255 --feature-gates=VolumeSubpathEnvExpansion=true'\" > /etc/default/kubelet",
            "curl -sSL \"https://raw.githubusercontent.com/kubernetes/kubernetes/\${RELEASE}/build/debs/kubelet.service\" | sed 's:/usr/bin:/usr/local/bin:g' > /etc/systemd/system/kubelet.service",
            "mkdir -p /etc/systemd/system/kubelet.service.d",
            "curl -sSL \"https://raw.githubusercontent.com/kubernetes/kubernetes/\${RELEASE}/build/debs/10-kubeadm.conf\" | sed 's:/usr/bin:/usr/local/bin:g' > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf",
            "systemctl enable kubelet && systemctl restart kubelet",
            "echo 'export PATH=/opt/cni/bin:\$PATH' >> /etc/profile.d/apps-bin-path.sh",
            "kubeadm config images pull",
            "apt autoremove"
        ],
        "ssh_authorized_keys": [
            "$SSH_KEY"
        ],
        "users": [
            {
                "name": "kubernetes",
                "primary_group": "kubernetes",
                "groups": [
                    "adm",
                    "users"
                ],
                "lock_passwd": false,
                "passwd": "a9c81dff-3e23-43cf-b755-67c940e4cbbc",
                "sudo": "ALL=(ALL) NOPASSWD:ALL",
                "shell": "/bin/bash",
                "ssh_authorized_keys": [
                    "$SSH_KEY"
                ]
            }
        ],
        "group": [
            "kubernetes"
        ],
        "power_state": {
            "mode": "reboot",
            "message": "Reboot VM due upgrade",
            "condition": true
        }
    },
    "mount-point": {
        "~/Vagrant/data": "/data"
    }
}
EOF

popd