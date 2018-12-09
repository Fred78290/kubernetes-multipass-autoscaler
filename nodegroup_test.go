package main

import (
	"encoding/json"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
	name          string
	memory        int
	cpu           int
	disk          int
	address       []string
	autoprovision bool
}

type nodeTest struct {
	name    string
	wantErr bool
	vm      vm
}

var kubeHost = "10.196.85.98:6443"
var kubeToken = "zsbvhy.qziur5dugk7vca0b"
var kubeCACert = "sha256:a2ecf06587da1c02624457e67e37849d5d7c01d74a8239b72c879511c00de008"
var kubeExtraArgs = []string{
	"--ignore-preflight-errors=All",
}
var mountPoints = &map[string]string{
	"~/.minikube/config": "~/.kube/config",
	"~/Vagrant/data":     "/data",
}

var testNode = []nodeTest{
	nodeTest{
		name:    "Test Node VM",
		wantErr: false,
		vm: vm{
			name:          testNodeName,
			memory:        2048,
			cpu:           2,
			disk:          5120,
			autoprovision: true,
			address: []string{
				"127.0.0.1",
			},
		},
	},
}

func newTestCloudInitConfig() (map[string]interface{}, error) {
	var cloudInit map[string]interface{}

	cloudInitStr, err := ioutil.ReadFile("./masterkube/config/cloud-init.json")

	err = json.Unmarshal(cloudInitStr, &cloudInit)

	if err != nil {
		return cloudInit, err
	}

	bKubeHost, err := ioutil.ReadFile("./masterkube/cluster/manager-ip")

	if err != nil {
		return cloudInit, err
	}

	kubeHost = strings.TrimSpace(string(bKubeHost))

	bKubeToken, err := ioutil.ReadFile("./masterkube/cluster/token")

	if err != nil {
		return cloudInit, err
	}

	kubeToken = strings.TrimSpace(string(bKubeToken))

	bKubeCACert, err := ioutil.ReadFile("./masterkube/cluster/ca.cert")

	kubeCACert = strings.TrimSpace(string(bKubeCACert))

	if err != nil {
		return cloudInit, err
	}

	return cloudInit, nil
}

func Test_multipassNode_launchVM(t *testing.T) {
	cloudInit, err := newTestCloudInitConfig()

	assert.NoError(t, err)

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
				kubeHost,
				kubeToken,
				kubeCACert,
				kubeExtraArgs,
				"",
				&cloudInit,
				mountPoints,
				tt.vm.autoprovision,
			}

			if err := vm.launchVM(extras); (err != nil) != tt.wantErr {
				t.Errorf("multipassNode.launchVM() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
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
			if err := vm.startVM(); (err != nil) != tt.wantErr {
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
			if err := vm.stopVM(); (err != nil) != tt.wantErr {
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
			if err := vm.deleteVM(); (err != nil) != tt.wantErr {
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
	cloudInit, err := newTestCloudInitConfig()

	assert.NoError(t, err)

	extras := &nodeCreationExtra{
		kubeHost,
		kubeToken,
		kubeCACert,
		kubeExtraArgs,
		"",
		&cloudInit,
		mountPoints,
		true,
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

func Test_multipassNodeGroup_deleteNode(t *testing.T) {
	ng := &multipassNodeGroup{
		identifier: testGroupID,
		machine: &machineCharacteristic{
			Memory: 4096,
			Vcpu:   4,
			Disk:   5120,
		},
		created: false,
		minSize: 0,
		maxSize: 3,
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
			if err := tt.ng.deleteNodeByName(tt.nodeName); (err != nil) != tt.wantErr {
				t.Errorf("multipassNodeGroup.deleteNode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_multipassNodeGroup_deleteNodeGroup(t *testing.T) {
	ng := &multipassNodeGroup{
		identifier: testGroupID,
		machine: &machineCharacteristic{
			Memory: 4096,
			Vcpu:   4,
			Disk:   5120,
		},
		created: false,
		minSize: 0,
		maxSize: 3,
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
			if err := tt.ng.deleteNodeGroup(); (err != nil) != tt.wantErr {
				t.Errorf("multipassNodeGroup.deleteNodeGroup() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
