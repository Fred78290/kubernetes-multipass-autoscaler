package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
