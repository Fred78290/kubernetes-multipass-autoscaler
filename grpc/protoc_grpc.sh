#/bin/bash

pushd $(dirname $0)
protoc -I . -I vendor grpc/grpc.proto --go_out=plugins=grpc:.
popd