#/bin/bash
pushd $(dirname $0)

GOARCH?=$(shell go env GOARCH)

make container

./out/multipass-autoscaler-$GOARCH \
    --config=masterkube/config/kubernetes-multipass-autoscaler.json \
    --save=masterkube/config/autoscaler-state.json \
    -v=9 \
    -logtostderr=true