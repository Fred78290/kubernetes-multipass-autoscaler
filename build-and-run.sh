#/bin/bash
pushd $(dirname $0)

make build
make container

./kubernetes-multipass-autoscaler --config masterkube/config/config.json -v=9 -logtostderr=true