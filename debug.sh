#!/bin/bash

CURDIR=$(dirname $0)

$CURDIR/out/multipass-autoscaler-amd64 \
    --config=$CURDIR/masterkube/config/kubernetes-multipass-autoscaler.json \
    --save=$CURDIR/masterkube/config/autoscaler-state.json \
    -v=1 \
    -logtostderr=true
