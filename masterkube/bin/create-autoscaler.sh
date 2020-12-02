#/bin/bash
CURDIR=$(dirname $0)

[ $(uname -s) = "Darwin" ] && GOOS=darwin || GOOS=linux

pushd $CURDIR/../

MASTER_IP=$(cat ./cluster/manager-ip)
TOKEN=$(cat ./cluster/token)
CACERT=$(cat ./cluster/ca.cert)
GOARCH=$(go env GOARCH)

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

nohup ../out/multipass-autoscaler-$GOARCH \
    --cache-dir=$PWD/config \
    --config=$PWD/config/kubernetes-multipass-autoscaler.json \
    --save=$PWD/config/autoscaler-state.json \
    -v=1 \
    -logtostderr=true 1>>config/multipass-autoscaler.log 2>&1 &
pid="$!"

echo -n "$pid" > config/multipass-autoscaler.pid

echo "multipass-autoscaler-$GOARCH running with PID:$pid"

deploy service-account
deploy cluster-role
deploy role
deploy cluster-role-binding
deploy role-binding
deploy deployment

popd
