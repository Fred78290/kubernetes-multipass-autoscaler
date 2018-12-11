package main

import (
	"encoding/json"
	"io/ioutil"
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
				vm := &multipassNode{
					nodeName: tt.vm.name,
					memory:   tt.vm.memory,
					cpu:      tt.vm.cpu,
					disk:     tt.vm.disk,
					address:  tt.vm.address,
					state:    nodeStateNotCreated,
				}

				extras := &nodeCreationExtra{
					config.KubeAdm.Address,
					config.KubeAdm.Token,
					config.KubeAdm.CACert,
					config.KubeAdm.ExtraArguments,
					config.KubeCtlConfig,
					config.Image,
					&config.CloudInit,
					&config.MountPoints,
					config.AutoProvision,
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
			vm := &multipassNode{
				nodeName: tt.vm.name,
				memory:   tt.vm.memory,
				cpu:      tt.vm.cpu,
				disk:     tt.vm.disk,
				address:  tt.vm.address,
				state:    nodeStateNotCreated,
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
			vm := &multipassNode{
				nodeName: tt.vm.name,
				memory:   tt.vm.memory,
				cpu:      tt.vm.cpu,
				disk:     tt.vm.disk,
				address:  tt.vm.address,
				state:    nodeStateNotCreated,
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
			vm := &multipassNode{
				nodeName: tt.vm.name,
				memory:   tt.vm.memory,
				cpu:      tt.vm.cpu,
				disk:     tt.vm.disk,
				address:  tt.vm.address,
				state:    nodeStateNotCreated,
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
			vm := &multipassNode{
				nodeName: tt.vm.name,
				memory:   tt.vm.memory,
				cpu:      tt.vm.cpu,
				disk:     tt.vm.disk,
				address:  tt.vm.address,
				state:    nodeStateNotCreated,
			}
			got, err := vm.statusVM()
			if (err != nil) != tt.wantErr {
				t.Errorf("multipassNode.statusVM() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != nodeStateRunning {
				t.Errorf("multipassNode.statusVM() = %v, want %v", got, nodeStateRunning)
			}
		})
	}
}

func Test_multipassNodeGroup_addNode(t *testing.T) {
	config, err := newTestConfig()

	if assert.NoError(t, err) {
		extras := &nodeCreationExtra{
			config.KubeAdm.Address,
			config.KubeAdm.Token,
			config.KubeAdm.CACert,
			config.KubeAdm.ExtraArguments,
			config.KubeCtlConfig,
			config.Image,
			&config.CloudInit,
			&config.MountPoints,
			config.AutoProvision,
		}

		tests := []struct {
			name    string
			delta   int
			ng      *multipassNodeGroup
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
					t.Errorf("multipassNodeGroup.addNode() error = %v, wantErr %v", err, tt.wantErr)
				}
			})
		}
	}
}

func Test_multipassNodeGroup_deleteNode(t *testing.T) {
	ng := &multipassNodeGroup{
		identifier: testGroupID,
		machine: &MachineCharacteristic{
			Memory: 4096,
			Vcpu:   4,
			Disk:   5120,
		},
		status:       nodegroupNotCreated,
		minSize:      0,
		maxSize:      5,
		pendingNodes: make(map[string]*multipassNode),
		nodes: map[string]*multipassNode{
			testNodeName: &multipassNode{
				nodeName: testNodeName,
				memory:   4096,
				cpu:      4,
				disk:     5120,
				address:  []string{},
				state:    nodeStateNotCreated,
			},
		},
	}

	tests := []struct {
		name     string
		delta    int
		nodeName string
		ng       *multipassNodeGroup
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
				t.Errorf("multipassNodeGroup.deleteNode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_multipassNodeGroup_deleteNodeGroup(t *testing.T) {
	ng := &multipassNodeGroup{
		identifier: testGroupID,
		machine: &MachineCharacteristic{
			Memory: 4096,
			Vcpu:   4,
			Disk:   5120,
		},
		status:       nodegroupNotCreated,
		minSize:      0,
		maxSize:      5,
		pendingNodes: make(map[string]*multipassNode),
		nodes: map[string]*multipassNode{
			testNodeName: &multipassNode{
				nodeName: testNodeName,
				memory:   4096,
				cpu:      4,
				disk:     5120,
				address:  []string{},
				state:    nodeStateNotCreated,
			},
		},
	}

	tests := []struct {
		name     string
		delta    int
		nodeName string
		ng       *multipassNodeGroup
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
				t.Errorf("multipassNodeGroup.deleteNodeGroup() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
