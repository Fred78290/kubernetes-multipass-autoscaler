#/bin/bash
CURDIR=$(dirname $0)

[ $(uname -s) = "Darwin" ] && GOOS=darwin || GOOS=linux

pushd $CURDIR/../

MASTER_IP=$(cat ./cluster/manager-ip)
TOKEN=$(cat ./cluster/token)
CACERT=$(cat ./cluster/ca.cert)

export K8NAMESPACE=kube-system
export ETC_DIR=./config/deployment/autoscaler
export KUBERNETES_TEMPLATE=./templates/autoscaler

mkdir -p $ETC_DIR

function deploy {
    echo "Create $ETC_DIR/$1.json"
echo $(eval "cat <<EOF
$(<$KUBERNETES_TEMPLATE/$1.json)
EOF") | jq . > $ETC_DIR/$1.json

kubectl apply -f $ETC_DIR/$1.json --kubeconfig=./cluster/config
}

#nohup ./out/multipass-autoscaler-$GOOS-amd64 --config ./config/kubernetes-multipass-autoscaler.json -v=9 -logtostderr=true &> kubernetes-multipass-autoscaler.log &

deploy service-account
deploy cluster-role
deploy role
deploy cluster-role-binding
deploy role-binding
deploy deployment

popd
