#/bin/bash

# This script create every thing to deploy a simple kubernetes autoscaled cluster with multipass.
# It will generate:
# Custom multipass image with every thing for kubernetes
# Config file to deploy the cluster autoscaler.

CURDIR=$(dirname $0)

export CUSTOM_IMAGE=YES
export SSH_KEY=$(cat ~/.ssh/id_rsa.pub)
export KUBERNETES_VERSION=v1.18.12
export KUBERNETES_PASSWORD=$(uuidgen)
export KUBECONFIG=$HOME/.kube/config
export CNI_VERSION="v0.8.6"
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
export OSDISTRO=$(uname -s)
export TRANSPORT="tcp"
export PORT=5200

TEMP=$(getopt -o ci:k:n:p:t:v: --long port:,transport:,no-custom-image,image:,ssh-key:,cni-version:,password:,kubernetes-version:,max-nodes-total:,cores-total:,memory-total:,max-autoprovisioned-node-group-count:,scale-down-enabled:,scale-down-delay-after-add:,scale-down-delay-after-delete:,scale-down-delay-after-failure:,scale-down-unneeded-time:,scale-down-unready-time:,unremovable-node-recheck-timeout: -n "$0" -- "$@")

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
	--port)
		PORT="$2"
		shift 2
		;;
	-p | --password)
		KUBERNETES_PASSWORD="$2"
		shift 2
		;;
	-t | --transport)
		TRANSPORT="$2"
		shift 2
		;;
	-v | --kubernetes-version)
		KUBERNETES_VERSION="$2"
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

function multipass_mount {
    echo -n "Mount point $1 to $2"
while :
do
	echo -n "."
	multipass mount $1 $2 > /dev/null 2>&1 && break
	sleep 1
done
    echo
}

pushd $CURDIR/../

if [ -z $TARGET_IMAGE ]; then
    mkdir -p ${PWD}/images/

    export TARGET_IMAGE=${PWD}/images/focal-k8s-$KUBERNETES_VERSION-amd64.img
fi

# GRPC network endpoint
if [ "$TRANSPORT" == "unix" ]; then
    LISTEN="${PWD}/config/grpc.sock"
    CONNECTTO="${TRANSPORT}:/etc/cluster-autoscaler/grpc.sock"
elif [ "$TRANSPORT" == "tcp" ]; then
    if [ "$OSDISTRO" == "Linux" ]; then
        NET_IF=$(ip route get 1 | awk '{print $5;exit}')
        IPADDR=$(ip addr show $NET_IF | grep "inet\s" | tr '/' ' ' | awk '{print $2}')
    else
        NET_IF=$(route get 1 | grep interface | awk '{print $2}')
        IPADDR=$(ifconfig $NET_IF | grep "inet\s" | sed -n 1p | awk '{print $2}')
    fi

    LISTEN="0.0.0.0:${PORT}"
    CONNECTTO="${IPADDR}:${PORT}"
else
    echo "Unknown transport: ${TRANSPORT}, should be unix or tcp"
    exit -1
fi

echo "Transport set to:${TRANSPORT}, listen endpoint at ${LISTEN}"


MOUNTPOINTS=$(cat <<EOF
    "$PWD/config": "/etc/cluster-autoscaler"
EOF
)

PACKAGE_UPGRADE="true"

RUN_CMD=$(
cat <<EOF
[
    "mkdir -p /opt/cni/bin",
    "mkdir -p /usr/local/bin",
    "mkdir -p /etc/systemd/system/kubelet.service.d",
    "echo 'export PATH=/usr/local/bin:/opt/bin:/opt/cni/bin:\$PATH' >> /etc/profile.d/apps-bin-path.sh",
    "curl https://get.docker.com | bash",
    "curl -L https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-amd64-${CNI_VERSION}.tgz | tar -C /opt/cni/bin -xz",
    "cd /usr/local/bin ; curl -L --remote-name-all https://storage.googleapis.com/kubernetes-release/release/${KUBERNETES_VERSION}/bin/linux/amd64/{kubeadm,kubelet,kubectl,kube-proxy}",
    "chmod +x /opt/bin/kube*",
    "echo \"KUBELET_EXTRA_ARGS='--fail-swap-on=false --read-only-port=10255'\" > /etc/default/kubelet",
    "curl -sSL \"https://raw.githubusercontent.com/kubernetes/kubernetes/v1.17.0/build/debs/kubelet.service\" | sed 's:/usr/bin:/opt/bin:g' > /etc/systemd/system/kubelet.service",
    "curl -sSL \"https://raw.githubusercontent.com/kubernetes/kubernetes/v1.17.0/build/debs/10-kubeadm.conf\" | sed 's:/usr/bin:/opt/bin:g' > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf",
    "systemctl enable kubelet",
    "systemctl restart kubelet",
    "apt autoremove -y",
    "echo '#!/bin/bash' > /usr/local/bin/kubeimage",
    "echo '/usr/local/bin/kubeadm config images pull --kubernetes-version=${KUBERNETES_VERSION}' >> /usr/local/bin/kubeimage",
    "chmod +x /usr/local/bin/kubeimage"
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

export DOMAIN_NAME=$(openssl x509 -noout -subject -in ./etc/ssl/cert.pem | awk -F= '{print $NF}' | sed -e 's/^[ \t]*//' | sed 's/\*\.//g')

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

	if [ "$OSDISTRO" == "Linux" ]; then
		echo "Create multipass preconfigured image"

		./bin/create-image.sh \
            --password=$KUBERNETES_PASSWORD \
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
            ],
            "write_files": [
                {
                    "encoding": "b64",
                    "content": "$(base64 -w 0 ~/.ssh/id_rsa)",
                    "path": "/usr/local/share/id_rsa",
                    "permissions": "0644"
                },
                {
                    "encoding": "b64",
                    "content": "$(base64 -w 0 ~/.ssh/id_rsa.pub)",
                    "path": "/usr/local/share/id_rsa.pub",
                    "permissions": "0644"
                }
            ]
        }
EOF
		echo "Create multipass VM to create the custom image $TARGET_IMAGE"

		multipass launch -n imagecreator -m 4096M -c 4 -d 20G --cloud-init=./config/imagecreator.yaml focal

		multipass_mount $PWD imagecreator:/masterkube

		echo "Create multipass preconfigured image (could take a long)"

		multipass exec imagecreator -- \
            /masterkube/bin/create-image.sh \
                --ssh-pub-key="/usr/local/share/id_rsa.pub" \
                --ssh-priv-key="/usr/local/share/id_rsa" \
                --password=$KUBERNETES_PASSWORD \
                --cni-version=$CNI_VERSION \
                --kubernetes-version=$KUBERNETES_VERSION \
                --custom-image=/home/ubuntu/focal-k8s-$KUBERNETES_VERSION-amd64.img

        multipass transfer \
            imagecreator:/home/ubuntu/focal-k8s-$KUBERNETES_VERSION-amd64.img \
            $TARGET_IMAGE

		multipass delete imagecreator -p
	fi
fi

./bin/delete-masterkube.sh

if [ "$CUSTOM_IMAGE" == "YES" ]; then
	echo "Launch custom masterkube instance with $TARGET_IMAGE"

	cat <<EOF | tee ./config/cloud-init-masterkube.json | python2 -c "import json,sys,yaml; print yaml.safe_dump(json.load(sys.stdin), width=500, indent=4, default_flow_style=False)" >./config/cloud-init-masterkube.yaml
    {
        "package_update": true,
        "package_upgrade": true,
        "users": $KUBERNETES_USER,
        "ssh_authorized_keys": [
            "$SSH_KEY"
        ],
        "runcmd": [
            "echo '#!/bin/bash' > /usr/local/bin/kubeimage",
            "echo '/usr/local/bin/kubeadm config images pull --kubernetes-version=${KUBERNETES_VERSION}' >> /usr/local/bin/kubeimage",
            "chmod +x /usr/local/bin/kubeimage"
        ],
        "group": [
            "kubernetes"
        ]
    }
EOF

	LAUNCH_IMAGE_URL=file://$TARGET_IMAGE

else
	echo "Launch standard masterkube instance"

	cat <<EOF | tee ./config/cloud-init-masterkube.json | python2 -c "import json,sys,yaml; print yaml.safe_dump(json.load(sys.stdin), width=500, indent=4, default_flow_style=False)" >./config/cloud-init-masterkube.yaml
    {
        "package_update": true,
        "package_upgrade": $PACKAGE_UPGRADE,
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

	LAUNCH_IMAGE_URL="focal"
fi

multipass launch -n masterkube -m 4096M -c 2 -d 10G --cloud-init=./config/cloud-init-masterkube.yaml $LAUNCH_IMAGE_URL

# Due bug in multipass MacOS, we need to reboot manually the VM after apt upgrade
if [ "$CUSTOM_IMAGE" != "YES" ] && [ "$OSDISTRO" != "Linux" ]; then
	multipass stop masterkube
	multipass start masterkube
fi

multipass_mount $PWD/bin masterkube:/masterkube/bin
multipass_mount $PWD/templates masterkube:/masterkube/templates
multipass_mount $PWD/etc masterkube:/masterkube/etc
multipass_mount $PWD/cluster masterkube:/etc/cluster
multipass_mount $PWD/kubernetes masterkube:/etc/kubernetes
multipass_mount $PWD/config masterkube:/etc/cluster-autoscaler

echo "Prepare masterkube instance"

multipass exec masterkube -- sudo usermod -aG docker ubuntu
multipass exec masterkube -- sudo usermod -aG docker kubernetes
multipass exec masterkube -- sudo /bin/bash -c /usr/local/bin/kubeimage

echo "Start kubernetes masterkube instance master node"

multipass exec masterkube -- sudo bash -c "export PATH=/opt/bin:/opt/cni/bin:/masterkube/bin:\$PATH; create-cluster.sh flannel ens3 '$KUBERNETES_VERSION' '$PROVIDERID'"

MASTER_IP=$(cat ./cluster/manager-ip)
TOKEN=$(cat ./cluster/token)
CACERT=$(cat ./cluster/ca.cert)

kubectl annotate node masterkube "cluster.autoscaler.nodegroup/name=ca-grpc-multipass" "cluster.autoscaler.nodegroup/node-index=0" "cluster.autoscaler.nodegroup/autoprovision=false" "cluster-autoscaler.kubernetes.io/scale-down-disabled=true" --overwrite --kubeconfig=./cluster/config
kubectl label nodes masterkube "cluster.autoscaler.nodegroup/name=ca-grpc-multipass" "master=true" --overwrite --kubeconfig=./cluster/config
kubectl create secret tls kube-system -n kube-system --key ./etc/ssl/privkey.pem --cert ./etc/ssl/fullchain.pem --kubeconfig=./cluster/config

./bin/kubeconfig-merge.sh masterkube cluster/config

echo "Write multipass cloud autoscaler provider config"

echo $(eval "cat <<EOF
$(<./templates/cluster/grpc-config.json)
EOF") | jq . >./config/grpc-config.json

if [ "$CUSTOM_IMAGE" = "YES" ]; then

	cat <<EOF | jq . > config/kubernetes-multipass-autoscaler.json
    {
        "network": "$TRANSPORT",
        "listen": "$LISTEN",
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
                "kubeadm config images pull --kubernetes-version=${KUBERNETES_VERSION}"
            ],
            "ssh_authorized_keys": [
                "$SSH_KEY"
            ],
            "group": [
                "kubernetes"
            ]
        },
        "mount-point": {
            $MOUNTPOINTS
        }
    }
EOF
else
	cat <<EOF | jq . > config/kubernetes-multipass-autoscaler.json
    {
        "network": "$TRANSPORT",
        "listen": "$LISTEN",
        "secret": "multipass",
        "minNode": $MINNODES,
        "maxNode": $MAXNODES,
        "nodePrice": 0.0,
        "podPrice": 0.0,
        "image": "focal",
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
            "package_upgrade": $PACKAGE_UPGRADE,
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
            $MOUNTPOINTS
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
