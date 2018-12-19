#!/bin/bash
PROVIDERID=$1
KUBEHOST=$2
KUBETOKEN=$3
KUBECACERT=$4


. /etc/default/kubelet

KUBELET_EXTRA_ARGS="$KUBELET_EXTRA_ARGS --provider-id=$PROVIDERID"

echo "KUBELET_EXTRA_ARGS='$KUBELET_EXTRA_ARGS'" > /etc/default/kubelet

systemctl enable kubelet && systemctl restart kubelet

kubeadm join $KUBEHOST --token $KUBETOKEN --discovery-token-ca-cert-hash $KUBECACERT