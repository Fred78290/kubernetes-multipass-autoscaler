package main

import (
	"encoding/json"
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

var cloudInitStr = `{
	"package_update": true,
	"package_upgrade": true,
	"runcmd": [
		"export CNI_VERSION=v0.7.1",
		"export RELEASE=v1.12.3",
		"curl https://get.docker.com | bash",
		"mkdir -p /opt/cni/bin",
		"curl -L https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-amd64-${CNI_VERSION}.tgz | tar -C /opt/cni/bin -xz",
		"mkdir -p /usr/local/bin",
		"cd /usr/local/bin ; curl -L --remote-name-all https://storage.googleapis.com/kubernetes-release/release/${RELEASE}/bin/linux/amd64/{kubeadm,kubelet,kubectl}",
		"chmod +x /usr/local/bin/kube*",
		"echo \"KUBELET_EXTRA_ARGS='--fail-swap-on=false --read-only-port=10255 --feature-gates=VolumeSubpathEnvExpansion=true'\" > /etc/default/kubelet",
		"curl -sSL \"https://raw.githubusercontent.com/kubernetes/kubernetes/${RELEASE}/build/debs/kubelet.service\" | sed 's:/usr/bin:/usr/local/bin:g' > /etc/systemd/system/kubelet.service",
		"mkdir -p /etc/systemd/system/kubelet.service.d",
		"curl -sSL \"https://raw.githubusercontent.com/kubernetes/kubernetes/${RELEASE}/build/debs/10-kubeadm.conf\" | sed 's:/usr/bin:/usr/local/bin:g' > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf",
		"systemctl enable kubelet && systemctl restart kubelet",
		"echo 'export PATH=/opt/cni/bin:$PATH' >> /etc/profile.d/apps-bin-path.sh",
		"kubeadm config images pull --kubernetes-version=$RELEASE",
		"apt autoremove"
	],
	"ssh_authorized_keys": [
		"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDhniyEBZs0t7aQZhn8gWfYrFacYJKQQx9x6pckZvMJIceLsQPB/J9CbqARtcCKZkK47yDzlH/zZNwt/AJvOawKZp6LDIWMOMF6TGicVhA+0RD3dOuqKRT0uJmaSo3Cz0GAaanTJXkhsEDZzaPkyLWXYaf6LxGAuMKCxv69j4H9ffGhRxNZ+62bs7DY+SH12hlcObZaz9GRydvEI/PUDghKJ4h1QKgvCKM1Mre1vQ2DHOuSifQC0Qbh0zK/JiJpHyBgFWRvKz72e2ya6+RW0ZuDGa6Qc3Zt8FIfH6eoiX+WOG7BUsXRN3n5gcWSXyYA9kxzBlNdMyYtD0fRlyb3+HgL"
	],
	"users": [
		{
			"name": "kubernetes",
			"primary_group": "kubernetes",
			"groups": [
				"adm",
				"users"
			],
			"lock_passwd": false,
			"passwd": "a9c81dff-3e23-43cf-b755-67c940e4cbbc",
			"sudo": "ALL=(ALL) NOPASSWD:ALL",
			"shell": "/bin/bash",
			"ssh_authorized_keys": [
				"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDhniyEBZs0t7aQZhn8gWfYrFacYJKQQx9x6pckZvMJIceLsQPB/J9CbqARtcCKZkK47yDzlH/zZNwt/AJvOawKZp6LDIWMOMF6TGicVhA+0RD3dOuqKRT0uJmaSo3Cz0GAaanTJXkhsEDZzaPkyLWXYaf6LxGAuMKCxv69j4H9ffGhRxNZ+62bs7DY+SH12hlcObZaz9GRydvEI/PUDghKJ4h1QKgvCKM1Mre1vQ2DHOuSifQC0Qbh0zK/JiJpHyBgFWRvKz72e2ya6+RW0ZuDGa6Qc3Zt8FIfH6eoiX+WOG7BUsXRN3n5gcWSXyYA9kxzBlNdMyYtD0fRlyb3+HgL"
			]
		}
	],
	"group": [
		"kubernetes"
	],
	"power_state": {
		"mode": "reboot",
		"message": "Reboot VM due upgrade",
		"condition": true
	}
}`

var tests = []nodeTest{
	nodeTest{
		name:    "Test Node VM",
		wantErr: false,
		vm: vm{
			name:          testNodeName,
			memory:        2048,
			cpu:           2,
			disk:          5120,
			autoprovision: false,
			address: []string{
				"127.0.0.1",
			},
		},
	},
}

func newTestCloudInitConfig() (map[string]interface{}, error) {
	var cloudInit map[string]interface{}

	err := json.Unmarshal([]byte(cloudInitStr), &cloudInit)

	return cloudInit, err
}
func Test_multipassNode_launchVM(t *testing.T) {
	cloudInit, err := newTestCloudInitConfig()

	assert.NoError(t, err)

	for _, tt := range tests {
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
	for _, tt := range tests {
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
	for _, tt := range tests {
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
	for _, tt := range tests {
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
	for _, tt := range tests {
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
