#/bin/bash
pushd $(dirname $0)

make container

[ $(uname -s) = "Darwin" ] && GOOS=darwin || GOOS=linux

./out/multipass-autoscaler-$GOOS-amd64 \
    --config=./masterkube/config/kubernetes-multipass-autoscaler.json \
    -v=9 \
    -logtostderr=true