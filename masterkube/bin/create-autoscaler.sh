#/bin/bash
CURDIR=$(dirname $0)

[ $(uname -s) = "Darwin" ] && GOOS=darwin || GOOS=linux

pushd $CURDIR/../

MASTER_IP=$(cat ./cluster/manager-ip)
TOKEN=$(cat ./cluster/token)
CACERT=$(cat ./cluster/ca.cert)

function deploy {
    echo "Create ./config/$1.json"
echo $(eval "cat <<EOF
$(<./autoscaler/$1.json)
EOF") | jq . > ./config/$1.json

kubectl apply -f ./config/$1.json --kubeconfig=./cluster/config
}

#nohup ./out/multipass-autoscaler-$GOOS-amd64 --config ./config/kubernetes-multipass-autoscaler.json -v=9 -logtostderr=true &> kubernetes-multipass-autoscaler.log &

deploy service-account
deploy cluster-role
deploy role
deploy cluster-role-binding
deploy role-binding
deploy deployment

popd
