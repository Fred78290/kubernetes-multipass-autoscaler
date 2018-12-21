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
	"log"
	"net"
	"os"

	"github.com/golang/glog"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"

	apigrc "github.com/Fred78290/kubernetes-multipass-autoscaler/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var phVersion = "v0.0.0-unset"
var phBuildDate = ""
var phSavedState = ""
var phMultipassServer *MultipassServer
var phSaveState bool

func main() {
	var config MultipassServerConfig

	versionPtr := flag.Bool("version", false, "Give the version")
	savePtr := flag.String("save", "", "The file to persists the server")
	configPtr := flag.String("config", "/etc/default/multipass-cluster-autoscaler.json", "The config for the server")
	flag.Parse()

	if *versionPtr {
		log.Printf("The current version is:%s, build at:%s", phVersion, phBuildDate)
	} else {
		if len(*savePtr) > 0 {
			phSavedState = *savePtr
			phSaveState = true
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

		if config.Optionals == nil {
			config.Optionals = &MultipassServerOptionals{
				Pricing:                  false,
				GetAvailableMachineTypes: false,
				NewNodeGroup:             false,
				TemplateNodeInfo:         false,
				Create:                   false,
				Delete:                   false,
			}
		}

		kubeAdmConfig := &apigrc.KubeAdmConfig{
			KubeAdmAddress:        config.KubeAdm.Address,
			KubeAdmToken:          config.KubeAdm.Token,
			KubeAdmCACert:         config.KubeAdm.CACert,
			KubeAdmExtraArguments: config.KubeAdm.ExtraArguments,
		}

		if phSaveState == false || fileExists(phSavedState) == false {
			phMultipassServer = &MultipassServer{
				ResourceLimiter: &ResourceLimiter{
					map[string]int64{cloudprovider.ResourceNameCores: 1, cloudprovider.ResourceNameMemory: 10000000},
					map[string]int64{cloudprovider.ResourceNameCores: 5, cloudprovider.ResourceNameMemory: 100000000},
				},
				Configuration:        config,
				Groups:               make(map[string]*MultipassNodeGroup),
				KubeAdmConfiguration: kubeAdmConfig,
			}

			if phSaveState {
				if err := phMultipassServer.save(phSavedState); err != nil {
					log.Fatalf(errFailedToSaveServerState, err)
				}
			}
		} else {
			phMultipassServer = &MultipassServer{}

			if err := phMultipassServer.load(phSavedState); err != nil {
				log.Fatalf(errFailedToLoadServerState, err)
			}
		}

		glog.V(2).Infof("Start listening server on %s", config.Listen)

		lis, err := net.Listen("tcp", config.Listen)

		if err != nil {
			glog.Fatalf("failed to listen: %v", err)
		}

		server := grpc.NewServer()

		apigrc.RegisterCloudProviderServiceServer(server, phMultipassServer)
		apigrc.RegisterNodeGroupServiceServer(server, phMultipassServer)
		apigrc.RegisterPricingModelServiceServer(server, phMultipassServer)

		reflection.Register(server)

		if err := server.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}

		glog.V(2).Infof("End listening server")
		glog.Flush()
	}
}
