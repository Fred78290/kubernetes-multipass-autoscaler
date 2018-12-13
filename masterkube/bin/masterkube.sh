#/bin/bash
KUBERNETES_VERSION=v1.12.3
TARGET_IMAGE=$HOME/.local/multipass/cache/bionic-kubernetes-amd64.img
CURDIR=$(dirname $0)
SSH_KEY=$(cat ~/.ssh/id_rsa.pub)
KUBERNETES_PASSWORD=$(uuidgen)

pushd $CURDIR

export PATH=$PWD:$PATH

rm -rf $PWD/../cluster/*
rm -rf $PWD/../kubernetes/*

if [ ! -f $TARGET_IMAGE ]; then
    echo "Create multipass preconfigured image"
    mkdir -p $HOME/.local/multipass/cache/
    create-image.sh $TARGET_IMAGE $KUBERNETES_VERSION
fi

echo "Delete masterkube previous instance"
multipass delete masterkube -p &> /dev/null

kubeconfig-delete.sh masterkube &> /dev/null

echo "Launch masterkube instance"

cat > /tmp/cloud-init-masterkube.json <<EOF
{
    "package_update": true,
    "package_upgrade": true,
    "runcmd": [
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
            "passwd": "$KUBERNETES_PASSWORD",
            "sudo": "ALL=(ALL) NOPASSWD:ALL",
            "shell": "/bin/bash",
            "ssh_authorized_keys": [
                "$SSH_KEY"
            ]
        }
    ],
    "group": [
        "kubernetes"
    ]
}
EOF

multipass launch -n masterkube -m 4096 -c 2 --cloud-init=/tmp/cloud-init-masterkube.json file://$TARGET_IMAGE

multipass mount $PWD masterkube:/masterkube/bin
multipass mount $PWD/../cluster masterkube:/etc/cluster
multipass mount $PWD/../kubernetes masterkube:/etc/kubernetes

echo "Prepare masterkube instance"

multipass shell masterkube <<EOF
echo "Pull kubernetes images"
sudo kubeadm config images pull
sudo usermod -aG docker multipass
echo "Start kubernetes masterkube instance master node"
sudo bash -c "export PATH=/opt/bin:/opt/cni/bin:/masterkube/bin:$PATH; create-cluster.sh flannel ens3 $KUBERNETES_VERSION"
exit
EOF

MASTER_IP=$(cat $PWD/../cluster/manager-ip)
TOKEN=$(cat $PWD/../cluster/token)
CACERT=$(cat $PWD/../cluster/ca.cert)

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
    "image": "file://$TARGET_IMAGE",
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