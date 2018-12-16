#/bin/bash
CURDIR=$(dirname $0)

CUSTOM_IMAGE=YES
SSH_KEY=$(cat ~/.ssh/id_rsa.pub)
#KUBERNETES_VERSION=$(curl -sSL https://dl.k8s.io/release/stable.txt)
KUBERNETES_VERSION=v1.12.3
KUBERNETES_PASSWORD=$(uuidgen)
KUBECONFIG=$HOME/.kube/config
TARGET_IMAGE=$HOME/.local/multipass/cache/bionic-k8s-$KUBERNETES_VERSION-amd64.img
CNI_VERSION="v0.7.1"

TEMP=`getopt -o ci:k:n:p:v: --long no-custom-image,custom-image:,ssh-key:,cni-version:,password:,kubernetes-version: -n "$0" -- "$@"`
eval set -- "$TEMP"

# extract options and their arguments into variables.
while true ; do
    case "$1" in
        -c|--no-custom-image)
            CUSTOM_IMAGE="NO"
            shift 1
            ;;
        -i|--custom-image)
            TARGET_IMAGE="$2"
            shift 2
            ;;
        -k|--ssh-key)
            SSH_KEY=$2
            shift 2
            ;;
        -n|--cni-version)
            CNI_VERSION=$2
            shift 2
            ;;
        -p|--password)
            KUBERNETES_PASSWORD=$2
            shift 2
            ;;
        -v|--kubernetes-version)
            KUBERNETES_VERSION=$2
            TARGET_IMAGE=$HOME/.local/multipass/cache/bionic-k8s-$KUBERNETES_VERSION-amd64.img
            shift 2
            ;;
        --)
            shift
            break
            ;;
        *) echo "$1 - Internal error!" ; exit 1 ;;
    esac
done

RUN_CMD=$(cat <<EOF
[
    "curl https://get.docker.com | bash",
    "mkdir -p /opt/cni/bin",
    "curl -L https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-amd64-${CNI_VERSION}.tgz | tar -C /opt/cni/bin -xz",
    "mkdir -p /usr/local/bin",
    "cd /usr/local/bin ; curl -L --remote-name-all https://storage.googleapis.com/kubernetes-release/release/${KUBERNETES_VERSION}/bin/linux/amd64/{kubeadm,kubelet,kubectl}",
    "chmod +x /usr/local/bin/kube*",
    "echo \"KUBELET_EXTRA_ARGS='--fail-swap-on=false --read-only-port=10255 --feature-gates=VolumeSubpathEnvExpansion=true'\" > /etc/default/kubelet",
    "curl -sSL \"https://raw.githubusercontent.com/kubernetes/kubernetes/${KUBERNETES_VERSION}/build/debs/kubelet.service\" | sed 's:/usr/bin:/usr/local/bin:g' > /etc/systemd/system/kubelet.service",
    "mkdir -p /etc/systemd/system/kubelet.service.d",
    "curl -sSL \"https://raw.githubusercontent.com/kubernetes/kubernetes/${KUBERNETES_VERSION}/build/debs/10-kubeadm.conf\" | sed 's:/usr/bin:/usr/local/bin:g' > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf",
    "systemctl enable kubelet && systemctl restart kubelet",
    "echo 'export PATH=/opt/cni/bin:\$PATH' >> /etc/profile.d/apps-bin-path.sh",
    "kubeadm config images pull",
    "apt autoremove"
]
EOF
)

KUBERNETES_USER=$(cat <<EOF
[
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
]
EOF
)

MACHINE_DEFS=$(cat <<EOF
{
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
}
EOF
)

pushd $CURDIR/../

export PATH=$CURDIR:$PATH

if [ ! -f ./etc/ssl/privkey.pem ]; then
    mkdir -p ./etc/ssl/
    openssl genrsa 2048 > ./etc/ssl/privkey.pem 
    openssl req -new -x509 -nodes -sha1 -days 3650 -key ./etc/ssl/privkey.pem  > ./etc/ssl/cert.pem 
    cat ./etc/ssl/cert.pem ./etc/ssl/privkey.pem > ./etc/ssl/fullchain.pem
    chmod 644 ./etc/ssl/*
fi

export DOMAIN_NAME=$(openssl x509 -noout -fingerprint -text < ./etc/ssl/cert.pem | grep 'Subject: CN =' | awk '{print $4}' | sed 's/\*\.//g')

rm -rf cluster/*
rm -rf config/*
rm -rf kubernetes/*

if [ "$CUSTOM_IMAGE" == "YES" ] && [ ! -f $TARGET_IMAGE ]; then
    echo "Create multipass preconfigured image"
    mkdir -p $HOME/.local/multipass/cache/
    create-image.sh --password=$KUBERNETES_PASSWORD \
        --cni-version=$CNI_VERSION \
        --custom-image=$TARGET_IMAGE \
        --kubernetes-version=$KUBERNETES_VERSION
fi

echo "Delete masterkube previous instance"
multipass delete masterkube -p &> /dev/null

kubeconfig-delete.sh masterkube &> /dev/null

if [ "$CUSTOM_IMAGE" = "YES" ]; then
    echo "Launch custom masterkube instance"

    cat > ./config/cloud-init-masterkube.json <<-EOF
    {
        "package_update": false,
        "package_upgrade": false,
        "users": $KUBERNETES_USER,
        "ssh_authorized_keys": [
            "$SSH_KEY"
        ],
        "group": [
            "kubernetes"
        ]
    }
EOF

    multipass launch -n masterkube -m 4096 -c 2 --cloud-init=./config/cloud-init-masterkube.json file://$TARGET_IMAGE

else
    echo "Launch standard masterkube instance"

    cat > ./config/cloud-init-masterkube.json <<-EOF
    {
        "package_update": true,
        "package_upgrade": true,
        "runcmd": $RUN_CMD,
        "users": $KUBERNETES_USER,
        "ssh_authorized_keys": [
            "$SSH_KEY"
        ],
        "group": [
            "kubernetes"
        ]
    }
EOF

    multipass launch -n masterkube -m 4096 -c 2 --cloud-init=./config/cloud-init-masterkube.json file://$TARGET_IMAGE
fi


multipass mount $PWD/bin masterkube:/masterkube/bin
multipass mount $PWD/templates masterkube:/masterkube/templates
multipass mount $PWD/etc masterkube:/masterkube/etc
multipass mount $PWD/cluster masterkube:/etc/cluster
multipass mount $PWD/kubernetes masterkube:/etc/kubernetes
multipass mount $PWD/config masterkube:/etc/cluster-autoscaler

echo "Prepare masterkube instance"

multipass shell masterkube <<EOF
sudo usermod -aG docker multipass
echo "Start kubernetes masterkube instance master node"
sudo bash -c "export PATH=/opt/bin:/opt/cni/bin:/masterkube/bin:$PATH; kubeadm config images pull; create-cluster.sh flannel ens3 $KUBERNETES_VERSION"
exit
EOF

MASTER_IP=$(cat ./cluster/manager-ip)
TOKEN=$(cat ./cluster/token)
CACERT=$(cat ./cluster/ca.cert)
NET_IF=$(ip route get 1|awk '{print $5;exit}')
IPADDR=$(ip addr show $NET_IF | grep "inet\s" | tr '/' ' ' | awk '{print $2}')

kubectl label nodes masterkube master=true --kubeconfig=./cluster/config
kubectl create secret tls kube-system -n kube-system --key ./etc/ssl/privkey.pem --cert ./etc/ssl/fullchain.pem --kubeconfig=./cluster/config

kubeconfig-merge.sh masterkube cluster/config

echo "Write multipass cloud autoscaler provider config"

echo $(eval "cat <<EOF
$(<./templates/autoscaler/grpc-config.json)
EOF") | jq . > ./config/grpc-config.json

if [ "$CUSTOM_IMAGE" = "YES" ]; then

    cat > ./config/kubernetes-multipass-autoscaler.json <<-EOF
    {
        "listen": "$IPADDR:5200",
        "secret": "multipass",
        "minNode": 0,
        "maxNode": 5,
        "nodePrice": 0.0,
        "podPrice": 0.0,
        "image": "file://$TARGET_IMAGE",
        "auto-provision": true,
        "kubeconfig": "$KUBECONFIG",
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
        "machines": $MACHINE_DEFS,
        "cloud-init": {
            "package_update": false,
            "package_upgrade": false,
            "users": $KUBERNETES_USER,
            "runcmd": [
                "kubeadm config images pull"
            ],
            "ssh_authorized_keys": [
                "$SSH_KEY"
            ],
            "group": [
                "kubernetes"
            ]
        },
        "mount-point": {
            "$PWD/config": "/etc/cluster-autoscaler"
        }
    }
EOF
else
    cat > config/kubernetes-multipass-autoscaler.json <<-EOF
    {
        "listen": "$IPADDR:5200",
        "secret": "multipass",
        "minNode": 0,
        "maxNode": 5,
        "nodePrice": 0.0,
        "podPrice": 0.0,
        "image": "bionic",
        "auto-provision": true,
        "kubeconfig": "$KUBECONFIG",
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
        "machines": $MACHINE_DEFS,
        "cloud-init": {
            "package_update": true,
            "package_upgrade": true,
            "runcmd": $RUN_CMD,
            "users": $KUBERNETES_USER,
            "ssh_authorized_keys": [
                "$SSH_KEY"
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
            "$PWD/config": "/etc/cluster-autoscaler"
        }
    }
EOF

fi

HOSTS_DEF=$(multipass info masterkube|grep IPv4|awk "{print \$2 \"    masterkube.$DOMAIN_NAME masterkube-dashboard.$DOMAIN_NAME\"}")
sudo sed -i '/masterkube/d' /etc/hosts
sudo bash -c "echo '$HOSTS_DEF' >> /etc/hosts"

echo "Create docker registry secret"
kubectl create secret docker-registry $GITLAB_REGISTRY \
    --docker-username=$GITLAB_UID \
    --docker-password=$GITLAB_PWD \
    --docker-server=$GITLAB_REGISTRY \
    --docker-email=$GITLAB_EMAIL \
    --kubeconfig=./cluster/config \
	-n kube-system

./bin/create-ingress-controller.sh
./bin/create-dashboard.sh

popd