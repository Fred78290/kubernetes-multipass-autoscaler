/*
Copyright 2018 Fred78290. https://github.com/Fred78290/

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/golang/glog"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"

	"github.com/Fred78290/kubernetes-multipass-autoscaler/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	var config MultipassServerConfig

	glog.V(2).Infof("Start listening server")

	configPtr := flag.String("config", "/etc/default/multipass-cluster-autoscaler.json", "The config for the server")
	tokenPtr := flag.String("token", "", "Token to use with kubeadm join")
	joinPtr := flag.String("host", "", "Address:port to use with kubeadm join")
	caPtr := flag.String("discovery-token-ca-cert-hash", "", "CA cert")
	flag.Parse()

	kubeAdmExtras := flag.Args()

	if tokenPtr == nil || len(*tokenPtr) == 0 {
		glog.Fatalf("Kubeadm join token is not defined")
	}

	if joinPtr == nil || len(*joinPtr) == 0 {
		glog.Fatalf("Kubeadm join address is not defined")
	}

	if caPtr == nil || len(*caPtr) == 0 {
		glog.Fatalf("Kubeadm join discovery-token-ca-cert-hash is not defined")
	}

	file, err := os.Open(*configPtr)
	if err != nil {
		glog.Fatalf("failed to open config file:%s, error:%v", *configPtr, err)
	}

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		glog.Fatalf("failed to decode config file:%s, error:%v", *configPtr, err)
	}

	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", config.Address, config.Port))

	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}

	server := grpc.NewServer()
	multipassserver := &MultipassServer{
		resourceLimiter: &resourceLimiter{
			map[string]int64{cloudprovider.ResourceNameCores: 1, cloudprovider.ResourceNameMemory: 10000000},
			map[string]int64{cloudprovider.ResourceNameCores: 5, cloudprovider.ResourceNameMemory: 100000000},
		},
		config:         config,
		nodeGroups:     make(map[string]*multipassNodeGroup),
		kubeAdmAddress: *joinPtr,
		kubeAdmToken:   *tokenPtr,
		kubeAdmCA:      *caPtr,
		kubeAdmExtras:  kubeAdmExtras,
	}

	grpccloudprovider.RegisterCloudProviderServiceServer(server, multipassserver)
	grpccloudprovider.RegisterNodeGroupServiceServer(server, multipassserver)
	grpccloudprovider.RegisterPricingModelServiceServer(server, multipassserver)

	reflection.Register(server)

	if err := server.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}

	glog.V(2).Infof("End listening server")
	glog.Flush()
}
