#!/bin/bash
KUBERNETES_VERSION=$1
CNI_VERSION="v0.8.5"

curl -s https://get.docker.com | bash

# On lxd container remove overlay mod test
if [ -f /lib/systemd/system/containerd.service ]; then
	sed -i  's/ExecStartPre=/#ExecStartPre=/g' /lib/systemd/system/containerd.service
	systemctl daemon-reload
	systemctl restart containerd.service
	systemctl restart docker
fi

cat >> /etc/dhcp/dhclient.conf << EOF
interface "eth0" {
}
EOF

if [ "x$KUBERNETES_VERSION" == "x" ]; then
	RELEASE="v1.18.2"
else
	RELEASE=$KUBERNETES_VERSION
fi

echo "Prepare kubernetes version $RELEASE"

mkdir -p /opt/cni/bin
curl -L "https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-linux-amd64-${CNI_VERSION}.tgz" | tar -C /opt/cni/bin -xz

mkdir -p /usr/local/bin
cd /usr/local/bin
curl -L --remote-name-all https://storage.googleapis.com/kubernetes-release/release/${RELEASE}/bin/linux/amd64/{kubeadm,kubelet,kubectl,kube-proxy}
chmod +x {kubeadm,kubelet,kubectl}

if [ -f /run/systemd/resolve/resolv.conf ]; then
	echo "KUBELET_EXTRA_ARGS='--resolv-conf=/run/systemd/resolve/resolv.conf --fail-swap-on=false --read-only-port=10255 --feature-gates=VolumeSubpathEnvExpansion=true'" > /etc/default/kubelet
else
	echo "KUBELET_EXTRA_ARGS='--fail-swap-on=false --read-only-port=10255 --feature-gates=VolumeSubpathEnvExpansion=true'" > /etc/default/kubelet
fi

mkdir -p /etc/systemd/system/kubelet.service.d

cat > /etc/systemd/system/kubelet.service <<EOF
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
EOF

cat > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf <<"EOF"
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
ExecStart=/usr/local/bin/kubelet $KUBELET_KUBECONFIG_ARGS $KUBELET_CONFIG_ARGS $KUBELET_KUBEADM_ARGS $KUBELET_EXTRA_ARGS
EOF

systemctl enable kubelet && systemctl restart kubelet

# Clean all image
for img in $(docker images --format "{{.Repository}}:{{.Tag}}")
do
	echo "Delete docker image:$img"
	docker rmi $img
done

echo 'export PATH=/opt/cni/bin:$PATH' >> /etc/bash.bashrc
#echo 'export PATH=/usr/local/bin:/opt/cni/bin:$PATH' >> /etc/profile.d/apps-bin-path.sh

kubeadm config images pull --kubernetes-version=$RELEASE
