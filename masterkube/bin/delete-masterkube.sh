#!/bin/bash
CURDIR=$(dirname $0)

echo "Delete masterkube previous instance"

pushd $CURDIR/../

if [ -f ./cluster/config ]; then
    for vm in $(kubectl get node -o json --kubeconfig ./cluster/config | jq '.items| .[] | .metadata.labels["kubernetes.io/hostname"]')
    do
        vm=$(echo -n $vm | tr -d '"')
        echo "Delete multipass VM: $vm"
        multipass delete $vm -p &> /dev/null
    done
fi

./bin/kubeconfig-delete.sh masterkube &> /dev/null

if [ -f config/multipass-autoscaler.pid ]; then
    kill $(cat config/multipass-autoscaler.pid)
fi

rm -rf cluster/*
rm -rf config/*
rm -rf kubernetes/*

popd