#/bin/bash

# This script create every thing to deploy a simple kubernetes autoscaled cluster with multipass.
# It will generate:
# Custom multipass image with every thing for kubernetes
# Config file to deploy the cluster autoscaler.

CURDIR=$(dirname $0)

export CUSTOM_IMAGE=YES
export SSH_KEY=$(cat ~/.ssh/id_rsa.pub)
export KUBERNETES_VERSION=$(curl -sSL https://dl.k8s.io/release/stable.txt)
export KUBERNETES_VERSION=v1.12.4
export KUBERNETES_PASSWORD=$(uuidgen)
export KUBECONFIG=$HOME/.kube/config
export TARGET_IMAGE=$HOME/.local/multipass/cache/bionic-k8s-$KUBERNETES_VERSION-amd64.img
export CNI_VERSION="v0.7.1"
export PROVIDERID="multipass://ca-grpc-multipass/object?type=node&name=masterkube"
export MINNODES=0
export MAXNODES=5
export MAXTOTALNODES=$MAXNODES
export CORESTOTAL="0:16"
export MEMORYTOTAL="0:24"
export MAXAUTOPROVISIONNEDNODEGROUPCOUNT="1"
export SCALEDOWNENABLED="true"
export SCALEDOWNDELAYAFTERADD="1m"
export SCALEDOWNDELAYAFTERDELETE="1m"
export SCALEDOWNDELAYAFTERFAILURE="1m"
export SCALEDOWNUNEEDEDTIME="1m"
export SCALEDOWNUNREADYTIME="1m"
export DEFAULT_MACHINE="medium"
export UNREMOVABLENODERECHECKTIMEOUT="1m"
export OSDISTRO=$(uname -a)

TEMP=$(getopt -o ci:k:n:p:v: --long no-custom-image,image:,ssh-key:,cni-version:,password:,kubernetes-version:,max-nodes-total:,cores-total:,memory-total:,max-autoprovisioned-node-group-count:,scale-down-enabled:,scale-down-delay-after-add:,scale-down-delay-after-delete:,scale-down-delay-after-failure:,scale-down-unneeded-time:,scale-down-unready-time:,unremovable-node-recheck-timeout: -n "$0" -- "$@")

eval set -- "$TEMP"

# extract options and their arguments into variables.
while true; do
	case "$1" in
	-c | --no-custom-image)
		CUSTOM_IMAGE="NO"
		shift 1
		;;
	-d | --default-machine)
		DEFAULT_MACHINE="$2"
		shift 2
		;;
	-i | --image)
		TARGET_IMAGE="$2"
		shift 2
		;;
	-k | --ssh-key)
		SSH_KEY="$2"
		shift 2
		;;
	-n | --cni-version)
		CNI_VERSION="$2"
		shift 2
		;;
	-p | --password)
		KUBERNETES_PASSWORD="$2"
		shift 2
		;;
	-v | --kubernetes-version)
		KUBERNETES_VERSION="$2"
		TARGET_IMAGE="$HOME/.local/multipass/cache/bionic-k8s-$KUBERNETES_VERSION-amd64.img"
		shift 2
		;;
	--max-nodes-total)
		MAXTOTALNODES="$2"
		shift 2
		;;
	--cores-total)
		CORESTOTAL="$2"
		shift 2
		;;
	--memory-total)
		MEMORYTOTAL="$2"
		shift 2
		;;
	--max-autoprovisioned-node-group-count)
		MAXAUTOPROVISIONNEDNODEGROUPCOUNT="$2"
		shift 2
		;;
	--scale-down-enabled)
		SCALEDOWNENABLED="$2"
		shift 2
		;;
	--scale-down-delay-after-add)
		SCALEDOWNDELAYAFTERADD="$2"
		shift 2
		;;
	--scale-down-delay-after-delete)
		SCALEDOWNDELAYAFTERDELETE="$2"
		shift 2
		;;
	--scale-down-delay-after-failure)
		SCALEDOWNDELAYAFTERFAILURE="$2"
		shift 2
		;;
	--scale-down-unneeded-time)
		SCALEDOWNUNEEDEDTIME="$2"
		shift 2
		;;
	--scale-down-unready-time)
		SCALEDOWNUNREADYTIME="$2"
		shift 2
		;;
	--unremovable-node-recheck-timeout)
		UNREMOVABLENODERECHECKTIMEOUT="$2"
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

RUN_CMD=$(
	cat <<EOF
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
    "systemctl enable kubelet",
    "systemctl restart kubelet",
    "echo 'export PATH=/opt/cni/bin:\$PATH' >> /etc/profile.d/apps-bin-path.sh",
    "kubeadm config images pull --kubernetes-version=${KUBERNETES_VERSION}",
    "apt autoremove -y"
]
EOF
)

KUBERNETES_USER=$(
	cat <<EOF
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

MACHINE_DEFS=$(
	cat <<EOF
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

[ -d config ] || mkdir -p config
[ -d cluster ] || mkdir -p cluster
[ -d kubernetes ] || mkdir -p kubernetes

export PATH=$CURDIR:$PATH

if [ ! -f ./etc/ssl/privkey.pem ]; then
	mkdir -p ./etc/ssl/
	openssl genrsa 2048 >./etc/ssl/privkey.pem
	openssl req -new -x509 -nodes -sha1 -days 3650 -key ./etc/ssl/privkey.pem >./etc/ssl/cert.pem
	cat ./etc/ssl/cert.pem ./etc/ssl/privkey.pem >./etc/ssl/fullchain.pem
	chmod 644 ./etc/ssl/*
fi

export DOMAIN_NAME=$(openssl x509 -noout -fingerprint -text <./etc/ssl/cert.pem | grep 'Subject: CN' | tr '=' ' ' | awk '{print $3}' | sed 's/\*\.//g')

# Because multipass on MacOS doesn't support local image, we can't use custom image
if [ "$OSDISTRO" == "Linux" ]; then
	POWERSTATE=$(
		cat <<EOF
        , "power_state": {
            "mode": "reboot",
            "message": "Reboot VM due upgrade",
            "condition": true
        }
EOF
	)
else
	CUSTOM_IMAGE="NO"
	POWERSTATE=
fi

if [ "$CUSTOM_IMAGE" == "YES" ] && [ ! -f $TARGET_IMAGE ]; then

	[ -d "$HOME/.local/multipass/cache/" ] || mkdir -p $HOME/.local/multipass/cache/

	if [ "$OSDISTRO" == "Linux" ]; then
		echo "Create multipass preconfigured image"

		create-image.sh --password=$KUBERNETES_PASSWORD \
			--cni-version=$CNI_VERSION \
			--custom-image=$TARGET_IMAGE \
			--kubernetes-version=$KUBERNETES_VERSION
	else
		cat <<-EOF | python2 -c "import json,sys,yaml; print yaml.safe_dump(json.load(sys.stdin), width=500, indent=4, default_flow_style=False)" >./config/imagecreator.yaml
        {
            "package_update": true,
            "package_upgrade": false,
            "packages" : [
                "libguestfs-tools"
            ],
            "ssh_authorized_keys": [
                "$SSH_KEY"
            ]
        }
EOF
		echo "Create multipass VM to create the custom image"

		multipass launch -n imagecreator -m 4096 -c 4 --cloud-init=./config/imagecreator.yaml bionic

		ROOT_IMAGE=$(dirname $TARGET_IMAGE)

		multipass mount $PWD/bin imagecreator:/masterkube/bin
		multipass mount $HOME/.local/multipass/cache/ imagecreator:/home/multipass/.local/multipass/cache/
		multipass mount $ROOT_IMAGE imagecreator:$ROOT_IMAGE

		echo "Create multipass preconfigured image (could take a long)"

		multipass shell imagecreator <<EOF
            /masterkube/bin/create-image.sh --password=$KUBERNETES_PASSWORD \
                --cni-version=$CNI_VERSION \
                --custom-image=$TARGET_IMAGE \
                --kubernetes-version=$KUBERNETES_VERSION
            exit
EOF
		multipass delete imagecreator -p
	fi
fi

./bin/delete-masterkube.sh

if [ "$CUSTOM_IMAGE" = "YES" ]; then
	echo "Launch custom masterkube instance with $TARGET_IMAGE"

	cat <<EOF | python2 -c "import json,sys,yaml; print yaml.safe_dump(json.load(sys.stdin), width=500, indent=4, default_flow_style=False)" >./config/cloud-init-masterkube.yaml
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

	LAUNCH_IMAGE_URL=file://$TARGET_IMAGE

else
	echo "Launch standard masterkube instance"

	cat <<EOF | python2 -c "import json,sys,yaml; print yaml.safe_dump(json.load(sys.stdin), width=500, indent=4, default_flow_style=False)" >./config/cloud-init-masterkube.yaml
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
        $POWERSTATE
    }
EOF

	LAUNCH_IMAGE_URL="bionic"
fi

multipass launch -n masterkube -m 4096 -c 2 --cloud-init=./config/cloud-init-masterkube.yaml $LAUNCH_IMAGE_URL

# Due bug in multipass MacOS, we need to reboot manually the VM after apt upgrade
if [ "$CUSTOM_IMAGE" != "YES" ] && [ "$OSDISTRO" != "Linux" ]; then
	multipass stop masterkube
	multipass start masterkube
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
sudo usermod -aG docker kubernetes
echo "Start kubernetes masterkube instance master node"
sudo bash -c "export PATH=/opt/bin:/opt/cni/bin:/masterkube/bin:\$PATH; kubeadm config images pull; create-cluster.sh flannel ens3 '$KUBERNETES_VERSION' '$PROVIDERID'"
exit
EOF

MASTER_IP=$(cat ./cluster/manager-ip)
TOKEN=$(cat ./cluster/token)
CACERT=$(cat ./cluster/ca.cert)

if [ "$OSDISTRO" == "Linux" ]; then
	NET_IF=$(ip route get 1 | awk '{print $5;exit}')
	IPADDR=$(ip addr show $NET_IF | grep "inet\s" | tr '/' ' ' | awk '{print $2}')
else
	NET_IF=$(route get 1 | grep interface | awk '{print $2}')
	IPADDR=$(ifconfig $NET_IF | grep "inet\s" | sed -n 1p | awk '{print $2}')
fi

kubectl annotate node masterkube "cluster.autoscaler.nodegroup/name=ca-grpc-multipass" "cluster.autoscaler.nodegroup/node-index=0" "cluster.autoscaler.nodegroup/autoprovision=false" "cluster-autoscaler.kubernetes.io/scale-down-disabled=true" --overwrite --kubeconfig=./cluster/config
kubectl label nodes masterkube "cluster.autoscaler.nodegroup/name=ca-grpc-multipass" "master=true" --overwrite --kubeconfig=./cluster/config
kubectl create secret tls kube-system -n kube-system --key ./etc/ssl/privkey.pem --cert ./etc/ssl/fullchain.pem --kubeconfig=./cluster/config

./bin/kubeconfig-merge.sh masterkube cluster/config

echo "Write multipass cloud autoscaler provider config"

echo $(eval "cat <<EOF
$(<./templates/cluster/grpc-config.json)
EOF") | jq . >./config/grpc-config.json

if [ "$CUSTOM_IMAGE" = "YES" ]; then

	cat <<EOF | jq . >config/kubernetes-multipass-autoscaler.json
    {
        "listen": "$IPADDR:5200",
        "secret": "multipass",
        "minNode": $MINNODES,
        "maxNode": $MAXNODES,
        "nodePrice": 0.0,
        "podPrice": 0.0,
        "image": "$LAUNCH_IMAGE_URL",
        "vm-provision": true,
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
        "default-machine": "$DEFAULT_MACHINE",
        "machines": $MACHINE_DEFS,
        "cloud-init": {
            "package_update": false,
            "package_upgrade": false,
            "users": $KUBERNETES_USER,
            "runcmd": [
                "kubeadm config images pull --kubernetes-version=${KUBERNETES_VERION}"
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
	cat <<EOF | jq . >config/kubernetes-multipass-autoscaler.json
    {
        "listen": "$IPADDR:5200",
        "secret": "multipass",
        "minNode": $MINNODES,
        "maxNode": $MAXNODES,
        "nodePrice": 0.0,
        "podPrice": 0.0,
        "image": "bionic",
        "vm-provision": true,
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
        "default-machine": "$DEFAULT_MACHINE",
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
            ]
            $POWERSTATE
        },
        "mount-point": {
            "$PWD/config": "/etc/cluster-autoscaler"
        }
    }
EOF

fi

HOSTS_DEF=$(multipass info masterkube | grep IPv4 | awk "{print \$2 \"    masterkube.$DOMAIN_NAME masterkube-dashboard.$DOMAIN_NAME\"}")

if [ "$OSDISTRO" == "Linux" ]; then
	sudo sed -i '/masterkube/d' /etc/hosts
	sudo bash -c "echo '$HOSTS_DEF' >> /etc/hosts"
else
	sudo sed -i '' '/masterkube/d' /etc/hosts
	sudo bash -c "echo '$HOSTS_DEF' >> /etc/hosts"
fi

./bin/create-ingress-controller.sh
./bin/create-dashboard.sh
./bin/create-autoscaler.sh
./bin/create-helloworld.sh

popd
