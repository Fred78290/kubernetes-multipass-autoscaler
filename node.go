package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	apiv1 "k8s.io/api/core/v1"

	"github.com/golang/glog"
)

type nodeState int32

const (
	nodeStateNotCreated nodeState = 0
	nodeStateRunning    nodeState = 1
	nodeStateStopped    nodeState = 2
	nodeStateDeleted    nodeState = 3
	nodeStateUndefined  nodeState = 4
)

// Describe a multipass VM
type multipassNode struct {
	nodeName string
	memory   int
	cpu      int
	disk     int
	address  []string
	state    nodeState
}

// VMDiskInfo describe VM disk usage
type VMDiskInfo struct {
	Total string `json:"total"`
	Used  string `json:"used"`
}

// VMMemoryInfo describe VM mem infos
type VMMemoryInfo struct {
	Total int `json:"total"`
	Used  int `json:"used"`
}

// VMMountInfos describe VM mounts point between host and guest
type VMMountInfos struct {
	GIDMappings []string `json:"gid_mappings"`
	UIDMappings []string `json:"uid_mappings"`
	SourcePath  string   `json:"source_path"`
}

// VMInfos describe VM global infos
type VMInfos struct {
	Disks        map[string]*VMDiskInfo `json:"disks"`
	ImageHash    string                 `json:"image_hash"`
	ImageRelease string                 `json:"image_release"`
	Ipv4         []string               `json:"ipv4"`
	Load         []float64              `json:"load"`
	Memory       *VMMemoryInfo          `json:"memory"`
	Mounts       map[string]string      `json:"mounts"`
	Release      string                 `json:"release"`
	State        string                 `json:"state"`
}

// MultipassVMInfos describe all about VMs
type MultipassVMInfos struct {
	Errors []interface{}       `json:"errors"`
	Info   map[string]*VMInfos `json:"info"`
}

func pipe(args ...string) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	glog.V(5).Infof("Shell:%v", args)

	cmd := exec.Command(args[0], args[1:]...)

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		s := strings.TrimSpace(stderr.String())

		return s, fmt.Errorf("%s, %s", err.Error(), s)
	}

	return strings.TrimSpace(stdout.String()), nil
}

func shell(args ...string) error {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	glog.V(5).Infof("Shell:%v", args)

	cmd := exec.Command(args[0], args[1:]...)

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s, %s", err.Error(), strings.TrimSpace(stderr.String()))
	}

	return nil
}

func (vm *multipassNode) waitReady(kubeconfig string) error {
	glog.V(5).Infof("multipassNode::waitReady, node:%s", vm.nodeName)

	// Max 60s
	for index := 0; index < 12; index++ {
		var out string
		var err error
		var arg = []string{
			"kubectl",
			"get",
			"nodes",
			vm.nodeName,
			"--output",
			"json",
			"--kubeconfig",
			kubeconfig,
		}

		if out, err = pipe(arg...); err != nil {
			return err
		}

		var nodeInfo apiv1.Node

		if err := json.Unmarshal([]byte(out), &nodeInfo); err != nil {
			return fmt.Errorf(errKubeCtlIgnoredError, vm.nodeName, err)
		}

		glog.V(5).Infof("multipassNode::waitReady, %v", nodeInfo)

		for _, status := range nodeInfo.Status.Conditions {
			if status.Type == "Ready" {
				glog.V(5).Infof("multipassNode::waitReady, (%i) found Ready for %s", index, vm.nodeName)
				if b, e := strconv.ParseBool(string(status.Status)); e == nil {
					if b {
						glog.V(5).Infof("multipassNode::waitReady, (%i) VM %s is Ready", index, vm.nodeName)
						return nil
					}
				}
			}
		}

		glog.V(5).Infof("multipassNode::waitReady, (%d) node:%s not ready", index, vm.nodeName)

		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf(errNodeIsNotReady, vm.nodeName)
}

func (vm *multipassNode) launchVM(extras *nodeCreationExtra) error {
	glog.V(5).Infof("multipassNode::launchVM, node:%s", vm.nodeName)

	var cloudInitFile *os.File
	var err error
	var status nodeState

	if vm.state != nodeStateNotCreated {
		return fmt.Errorf(errVMAlreadyCreated, vm.nodeName)
	}

	if extras.cloudInit != nil && len(extras.cloudInit) > 0 {
		var b []byte

		fName := fmt.Sprintf("%s/cloud-init-%s.yaml", os.TempDir(), vm.nodeName)
		cloudInitFile, err = os.Create(fName)
		if err != nil {
			glog.Errorf(errTempFile, err)
			return err
		}

		defer os.Remove(cloudInitFile.Name())

		b, err = yaml.Marshal(extras.cloudInit)

		if err != nil {
			glog.Errorf(errCloudInitMarshallError, err)
			return err
		}

		if _, err = cloudInitFile.Write(b); err != nil {
			glog.Errorf(errCloudInitWriteError, err)
			return err
		}
	}

	var args = []string{
		"multipass",
		"launch",
		"--name",
		vm.nodeName,
	}

	/*
		Append VM attributes Memory,cpus, hard drive size....
	*/
	if vm.memory > 0 {
		args = append(args, fmt.Sprintf("--mem=%dM", vm.memory))
	}

	if vm.cpu > 0 {
		args = append(args, fmt.Sprintf("--cpus=%d", vm.cpu))
	}

	if vm.disk > 0 {
		args = append(args, fmt.Sprintf("--disk=%dM", vm.disk))
	}

	// If cloud-init file is present
	if cloudInitFile != nil {
		args = append(args, fmt.Sprintf("--cloud-init=%s", cloudInitFile.Name()))
	}

	// If an image/url image
	if len(extras.image) > 0 {
		args = append(args, extras.image)
	}

	// Launch the VM and wait until finish launched
	if err = shell(args...); err != nil {
		glog.Errorf(errUnableToLaunchVM, vm.nodeName, err)
		return err
	}

	// Add mount point
	if extras.mountPoints != nil && len(extras.mountPoints) > 0 {
		for hostPath, guestPath := range extras.mountPoints {
			if err = shell("multipass", "mount", hostPath, fmt.Sprintf("%s:%s", vm.nodeName, guestPath)); err != nil {
				glog.Warningf(errUnableToMountPath, hostPath, guestPath, vm.nodeName, err)
			}
		}
	}

	status, err = vm.statusVM()

	if err != nil {
		glog.Error(err.Error())
		return err
	}

	// If the VM is running call kubeadm join
	if extras.autoprovision {
		if status != nodeStateRunning {
			return fmt.Errorf(errKubeAdmJoinNotRunning, vm.nodeName)
		}

		args = []string{
			"multipass",
			"exec",
			vm.nodeName,
			"--",
			"sudo",
			"kubeadm",
			"join",
			extras.kubeHost,
			"--token",
			extras.kubeToken,
			"--discovery-token-ca-cert-hash",
			extras.kubeCACert,
		}

		// Append extras arguments
		if len(extras.kubeExtraArgs) > 0 {
			args = append(args, extras.kubeExtraArgs...)
		}

		if err := shell(args...); err != nil {
			glog.Errorf(errKubeAdmJoinFailed, vm.nodeName, err)
			return fmt.Errorf(errKubeAdmJoinFailed, vm.nodeName, err)
		}

		if err := vm.waitReady(extras.kubeConfig); err != nil {
			return err
		}

		if len(extras.nodeLabels)+len(extras.systemLabels) > 0 {

			args = []string{
				"kubectl",
				"label",
				"nodes",
				vm.nodeName,
			}

			// Append extras arguments
			for k, v := range extras.nodeLabels {
				args = append(args, fmt.Sprintf("%s=%s", k, v))
			}

			for k, v := range extras.systemLabels {
				args = append(args, fmt.Sprintf("%s=%s", k, v))
			}

			args = append(args, "--kubeconfig")
			args = append(args, extras.kubeConfig)

			if err := shell(args...); err != nil {
				glog.Errorf(errKubeCtlIgnoredError, vm.nodeName, err)
				return fmt.Errorf(errKubeCtlIgnoredError, vm.nodeName, err)
			}
		}
	}

	return nil
}

func (vm *multipassNode) startVM(kubeconfig string) error {
	glog.V(5).Infof("multipassNode::startVM, node:%s", vm.nodeName)

	var err error
	var state nodeState

	state, err = vm.statusVM()

	if err != nil {
		return err
	}

	if state == nodeStateStopped {
		if err = shell("multipass", "start", vm.nodeName); err != nil {
			glog.Errorf(errStartVMFailed, vm.nodeName, err)
			return fmt.Errorf(errStartVMFailed, vm.nodeName, err)
		}

		args := []string{
			"kubectl",
			"uncordon",
			vm.nodeName,
			"--kubeconfig",
			kubeconfig,
		}

		if err = shell(args...); err != nil {
			glog.Errorf(errKubeCtlIgnoredError, vm.nodeName, err)
		}

		vm.state = nodeStateRunning
	} else if state != nodeStateRunning {
		glog.Errorf(errVMNotFound, vm.nodeName)
		return fmt.Errorf(errVMNotFound, vm.nodeName)
	}

	return nil
}

func (vm *multipassNode) stopVM(kubeconfig string) error {
	glog.V(5).Infof("multipassNode::stopVM, node:%s", vm.nodeName)

	var err error
	var state nodeState

	state, err = vm.statusVM()

	if err != nil {
		return err
	}

	if state == nodeStateRunning {
		args := []string{
			"kubectl",
			"cordon",
			vm.nodeName,
			"--kubeconfig",
			kubeconfig,
		}

		if err = shell(args...); err != nil {
			glog.Errorf(errKubeCtlIgnoredError, vm.nodeName, err)
		}

		if err = shell("multipass", "stop", vm.nodeName); err != nil {
			glog.Errorf(errStopVMFailed, vm.nodeName, err)
			return fmt.Errorf(errStopVMFailed, vm.nodeName, err)
		}

		vm.state = nodeStateStopped
	} else if state != nodeStateStopped {
		glog.Errorf(errVMNotFound, vm.nodeName)
		return fmt.Errorf(errVMNotFound, vm.nodeName)
	}

	return nil
}

func (vm *multipassNode) deleteVM(kubeconfig string) error {
	glog.V(5).Infof("multipassNode::deleteVM, node:%s", vm.nodeName)

	var err error
	var state nodeState

	state, err = vm.statusVM()

	if err != nil {
		return err
	}

	args := []string{
		"kubectl",
		"drain",
		vm.nodeName,
		"--delete-local-data",
		"--force",
		"--ignore-daemonsets",
		"--kubeconfig",
		kubeconfig,
	}

	if err = shell(args...); err != nil {
		glog.Errorf(errKubeCtlIgnoredError, vm.nodeName, err)
	}

	args = []string{
		"kubectl",
		"delete",
		"node",
		vm.nodeName,
		"--kubeconfig",
		kubeconfig,
	}

	if err = shell(args...); err != nil {
		glog.Errorf(errKubeCtlIgnoredError, vm.nodeName, err)
	}

	if state == nodeStateRunning {
		if err = shell("multipass", "stop", vm.nodeName); err != nil {
			glog.Errorf(errStopVMFailed, vm.nodeName, err)
			return fmt.Errorf(errStopVMFailed, vm.nodeName, err)
		}
	}

	if err = shell("multipass", "delete", "--purge", vm.nodeName); err != nil {
		glog.Errorf(errDeleteVMFailed, vm.nodeName, err)
		return fmt.Errorf(errDeleteVMFailed, vm.nodeName, err)
	}

	vm.state = nodeStateDeleted

	return nil
}

func (vm *multipassNode) statusVM() (nodeState, error) {
	glog.V(5).Infof("multipassNode::statusVM, node:%s", vm.nodeName)

	// Get VM infos
	var out string
	var err error
	var vmInfos MultipassVMInfos

	if out, err = pipe("multipass", "info", vm.nodeName, "--format=json"); err != nil {
		glog.Errorf(errGetVMInfoFailed, vm.nodeName, err)
		return nodeStateUndefined, err
	}

	if err = json.Unmarshal([]byte(out), &vmInfos); err != nil {
		glog.Errorf(errGetVMInfoFailed, vm.nodeName, err)
		return nodeStateUndefined, err
	}

	if vmInfo := vmInfos.Info[vm.nodeName]; vmInfo != nil {
		vm.address = vmInfo.Ipv4

		switch vmInfo.State {
		case "RUNNING":
			vm.state = nodeStateRunning
		case "STOPPED":
			vm.state = nodeStateStopped
		case "DELETED":
			vm.state = nodeStateDeleted
		default:
			vm.state = nodeStateUndefined
		}

		return vm.state, nil
	}

	return nodeStateUndefined, fmt.Errorf(errMultiPassInfoNotFound, vm.nodeName)
}
