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
	var tmpDir string
	var err error
	var file *os.File
	var cacheStats os.FileInfo

	if tmpDir, err = os.UserCacheDir(); err != nil {
		glog.Fatalf("Unable to find user cache", err)
	}

	versionPtr := flag.Bool("version", false, "Give the version")
	savePtr := flag.String("save", "", "The file to persists the server")
	configPtr := flag.String("config", "/etc/default/multipass-cluster-autoscaler.json", "The config for the server")
	cachePtr := flag.String("cache-dir", tmpDir, "The cache directory")

	flag.Parse()

	if *versionPtr {
		log.Printf("The current version is:%s, build at:%s", phVersion, phBuildDate)
	} else {
		if len(*savePtr) > 0 {
			phSavedState = *savePtr
			phSaveState = true
		}

		if cacheStats, err = os.Lstat(*cachePtr); err != nil {
			glog.Fatalf("failed to find cache dir:%s, error:%v", *cachePtr, err)
		}

		if cacheStats.IsDir() == false {
			glog.Fatalf("declared cache dir:%s, is not a directory", *cachePtr)
		}

		file, err = os.Open(*configPtr)
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

		if !phSaveState || !fileExists(phSavedState) {
			phMultipassServer = &MultipassServer{
				ResourceLimiter: &ResourceLimiter{
					map[string]int64{ResourceNameCores: 1, ResourceNameMemory: 10000000},
					map[string]int64{ResourceNameCores: 5, ResourceNameMemory: 100000000},
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

		phMultipassServer.CacheDir = *cachePtr

		glog.Infof("Start listening server %s on %s", phVersion, config.Listen)

		lis, err := net.Listen(config.Network, config.Listen)

		if err != nil {
			glog.Fatalf("failed to listen: %v", err)
		}

		server := grpc.NewServer()

		defer server.Stop()

		apigrc.RegisterCloudProviderServiceServer(server, phMultipassServer)
		apigrc.RegisterNodeGroupServiceServer(server, phMultipassServer)
		apigrc.RegisterPricingModelServiceServer(server, phMultipassServer)

		reflection.Register(server)

		if err := server.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}

		glog.Infof("End listening server")
		glog.Flush()
	}
}
