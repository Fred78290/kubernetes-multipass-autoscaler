package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	kubeconfig = "/etc/kubernetes/config"
)

type arguments struct {
	kubeHost      string
	kubeToken     string
	kubeCACert    string
	kubeExtraArgs []string
	image         string
	cloudInit     *map[string]interface{}
	mountPoints   *map[string]string
}

type vm struct {
	name    string
	memory  int
	cpu     int
	disk    int
	address []string
}

type nodeTest struct {
	name    string
	wantErr bool
	vm      vm
}

var testNode = []nodeTest{
	nodeTest{
		name:    "Test Node VM",
		wantErr: false,
		vm: vm{
			name:   testNodeName,
			memory: 2048,
			cpu:    2,
			disk:   5120,
			address: []string{
				"127.0.0.1",
			},
		},
	},
}

func newTestConfig() (*MultipassServerConfig, error) {
	var config MultipassServerConfig

	configStr, err := ioutil.ReadFile("./masterkube/config/config.json")
	err = json.Unmarshal(configStr, &config)

	if err != nil {
		return nil, err
	}

	return &config, nil
}

func Test_multipassNode_launchVM(t *testing.T) {
	config, err := newTestConfig()

	if assert.NoError(t, err) {
		for _, tt := range testNode {
			t.Run(tt.name, func(t *testing.T) {
				vm := &MultipassNode{
					NodeName:         tt.vm.name,
					Memory:           tt.vm.memory,
					CPU:              tt.vm.cpu,
					Disk:             tt.vm.disk,
					Addresses:        tt.vm.address,
					State:            MultipassNodeStateNotCreated,
					AutoProvisionned: true,
				}

				nodeLabels := map[string]string{
					"monitor":  "true",
					"database": "true",
				}

				extras := &nodeCreationExtra{
					kubeHost:      config.KubeAdm.Address,
					kubeToken:     config.KubeAdm.Token,
					kubeCACert:    config.KubeAdm.CACert,
					kubeExtraArgs: config.KubeAdm.ExtraArguments,
					kubeConfig:    config.KubeCtlConfig,
					image:         config.Image,
					cloudInit:     config.CloudInit,
					mountPoints:   config.MountPoints,
					nodegroupID:   testGroupID,
					nodeLabels:    nodeLabels,
					systemLabels:  make(map[string]string),
					vmprovision:   config.VMProvision,
					cacheDir:      os.UserCacheDir(),
				}

				if err := vm.launchVM(extras); (err != nil) != tt.wantErr {
					t.Errorf("multipassNode.launchVM() error = %v, wantErr %v", err, tt.wantErr)
				}
			})
		}
	}
}

func Test_multipassNode_startVM(t *testing.T) {
	for _, tt := range testNode {
		t.Run(tt.name, func(t *testing.T) {
			vm := &MultipassNode{
				NodeName:         tt.vm.name,
				Memory:           tt.vm.memory,
				CPU:              tt.vm.cpu,
				Disk:             tt.vm.disk,
				Addresses:        tt.vm.address,
				State:            MultipassNodeStateNotCreated,
				AutoProvisionned: true,
			}
			if err := vm.startVM(kubeconfig); (err != nil) != tt.wantErr {
				t.Errorf("multipassNode.startVM() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_multipassNode_stopVM(t *testing.T) {
	for _, tt := range testNode {
		t.Run(tt.name, func(t *testing.T) {
			vm := &MultipassNode{
				NodeName:         tt.vm.name,
				Memory:           tt.vm.memory,
				CPU:              tt.vm.cpu,
				Disk:             tt.vm.disk,
				Addresses:        tt.vm.address,
				State:            MultipassNodeStateNotCreated,
				AutoProvisionned: true,
			}
			if err := vm.stopVM(kubeconfig); (err != nil) != tt.wantErr {
				t.Errorf("multipassNode.stopVM() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_multipassNode_deleteVM(t *testing.T) {
	for _, tt := range testNode {
		t.Run(tt.name, func(t *testing.T) {
			vm := &MultipassNode{
				NodeName:         tt.vm.name,
				Memory:           tt.vm.memory,
				CPU:              tt.vm.cpu,
				Disk:             tt.vm.disk,
				Addresses:        tt.vm.address,
				State:            MultipassNodeStateNotCreated,
				AutoProvisionned: true,
			}
			if err := vm.deleteVM(kubeconfig); (err != nil) != tt.wantErr {
				t.Errorf("multipassNode.deleteVM() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_multipassNode_statusVM(t *testing.T) {
	for _, tt := range testNode {
		t.Run(tt.name, func(t *testing.T) {
			vm := &MultipassNode{
				NodeName:         tt.vm.name,
				Memory:           tt.vm.memory,
				CPU:              tt.vm.cpu,
				Disk:             tt.vm.disk,
				Addresses:        tt.vm.address,
				State:            MultipassNodeStateNotCreated,
				AutoProvisionned: true,
			}
			got, err := vm.statusVM()
			if (err != nil) != tt.wantErr {
				t.Errorf("multipassNode.statusVM() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != MultipassNodeStateRunning {
				t.Errorf("multipassNode.statusVM() = %v, want %v", got, MultipassNodeStateRunning)
			}
		})
	}
}

func Test_multipassNodeGroup_addNode(t *testing.T) {
	config, err := newTestConfig()

	if assert.NoError(t, err) {
		extras := &nodeCreationExtra{
			kubeHost:      config.KubeAdm.Address,
			kubeToken:     config.KubeAdm.Token,
			kubeCACert:    config.KubeAdm.CACert,
			kubeExtraArgs: config.KubeAdm.ExtraArguments,
			kubeConfig:    config.KubeCtlConfig,
			image:         config.Image,
			cloudInit:     config.CloudInit,
			mountPoints:   config.MountPoints,
			nodegroupID:   testGroupID,
			nodeLabels:    testNodeGroup.NodeLabels,
			systemLabels:  testNodeGroup.SystemLabels,
			vmprovision:   config.VMProvision,
			cacheDir:      os.UserCacheDir(),
		}

		tests := []struct {
			name    string
			delta   int
			ng      *MultipassNodeGroup
			wantErr bool
		}{
			{
				name:    "addNode",
				delta:   1,
				wantErr: false,
				ng:      &testNodeGroup,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if err := tt.ng.addNodes(tt.delta, extras); (err != nil) != tt.wantErr {
					t.Errorf("MultipassNodeGroup.addNode() error = %v, wantErr %v", err, tt.wantErr)
				}
			})
		}
	}
}

func Test_multipassNodeGroup_deleteNode(t *testing.T) {
	ng := &MultipassNodeGroup{
		ServiceIdentifier:   testProviderID,
		NodeGroupIdentifier: testGroupID,
		Machine: &MachineCharacteristic{
			Memory: 4096,
			Vcpu:   4,
			Disk:   5120,
		},
		Status:       NodegroupNotCreated,
		MinNodeSize:  0,
		MaxNodeSize:  5,
		PendingNodes: make(map[string]*MultipassNode),
		Nodes: map[string]*MultipassNode{
			testNodeName: &MultipassNode{
				NodeName:         testNodeName,
				Memory:           4096,
				CPU:              4,
				Disk:             5120,
				Addresses:        []string{},
				State:            MultipassNodeStateNotCreated,
				AutoProvisionned: true,
			},
		},
	}

	tests := []struct {
		name     string
		delta    int
		nodeName string
		ng       *MultipassNodeGroup
		wantErr  bool
	}{
		{
			name:     "deleteNode",
			delta:    1,
			wantErr:  false,
			nodeName: testNodeName,
			ng:       ng,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.ng.deleteNodeByName(kubeconfig, tt.nodeName); (err != nil) != tt.wantErr {
				t.Errorf("MultipassNodeGroup.deleteNode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_multipassNodeGroup_deleteNodeGroup(t *testing.T) {
	ng := &MultipassNodeGroup{
		ServiceIdentifier:   testProviderID,
		NodeGroupIdentifier: testGroupID,
		Machine: &MachineCharacteristic{
			Memory: 4096,
			Vcpu:   4,
			Disk:   5120,
		},
		Status:       NodegroupNotCreated,
		MinNodeSize:  0,
		MaxNodeSize:  5,
		PendingNodes: make(map[string]*MultipassNode),
		Nodes: map[string]*MultipassNode{
			testNodeName: &MultipassNode{
				NodeName:         testNodeName,
				Memory:           4096,
				CPU:              4,
				Disk:             5120,
				Addresses:        []string{},
				State:            MultipassNodeStateNotCreated,
				AutoProvisionned: true,
			},
		},
	}

	tests := []struct {
		name     string
		delta    int
		nodeName string
		ng       *MultipassNodeGroup
		wantErr  bool
	}{
		{
			name:    "deleteNodeGroup",
			delta:   1,
			wantErr: false,
			ng:      ng,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.ng.deleteNodeGroup(kubeconfig); (err != nil) != tt.wantErr {
				t.Errorf("MultipassNodeGroup.deleteNodeGroup() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
