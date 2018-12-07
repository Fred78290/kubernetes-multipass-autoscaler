package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"gopkg.in/yaml.v2"

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

func (vm *multipassNode) launchVM(extras *nodeCreationExtra) error {
	var cloudInitFile *os.File
	var err error
	var output []byte
	var status nodeState
	var cmd *exec.Cmd

	if vm.state != nodeStateNotCreated {
		return fmt.Errorf(errVMAlreadyCreated, vm.nodeName)
	}

	if extras.cloudInit != nil && len(*extras.cloudInit) > 0 {
		fName := fmt.Sprintf("%s/cloud-init-%s.yaml", os.TempDir(), vm.nodeName)
		cloudInitFile, err = os.Create(fName)
		if err != nil {
			glog.Errorf(errTempFile, err)
			return err
		}

		//defer os.Remove(cloudInitFile.Name())

		output, err = yaml.Marshal(*extras.cloudInit)

		if err != nil {
			glog.Errorf(errCloudInitMarshallError, err)
			return err
		}

		if _, err = cloudInitFile.Write(output); err != nil {
			glog.Errorf(errCloudInitWriteError, err)
			return err
		}
	}

	var args = []string{
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
	cmd = exec.Command("multipass", args...)

	glog.Errorf("Execute:%v", args)

	if output, err = cmd.Output(); err != nil {
		glog.Errorf(errUnableToLaunchVM, vm.nodeName, string(output), err)
		return err
	}

	// Add mount point
	if extras.mountPoints != nil && len(*extras.mountPoints) > 0 {
		for hostPath, guestPath := range *extras.mountPoints {
			cmd = exec.Command("multipass", "mount", hostPath, fmt.Sprintf("%s:%s", vm.nodeName, guestPath))

			if output, err = cmd.Output(); err != nil {
				glog.Warningf(errUnableToMountPath, hostPath, guestPath, vm.nodeName, string(output), err)
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
			"join",
			extras.kubeHost,
			fmt.Sprintf("--token=%s", extras.kubeToken),
			fmt.Sprintf("--discovery-token-ca-cert-hash=%s", extras.kubeCACert),
		}

		// Append extras arguments
		if len(extras.kubeExtraArgs) > 0 {
			args = append(args, extras.kubeExtraArgs...)
		}

		cmd = exec.Command("kubeadm", args...)

		if out, err := cmd.Output(); err != nil {
			glog.Errorf(errKubeAdmJoinFailed, vm.nodeName, string(out), err)
			return fmt.Errorf(errKubeAdmJoinFailed, vm.nodeName, string(out), err)
		}
	}

	return nil
}

func (vm *multipassNode) startVM() error {
	var err error
	var out []byte
	var state nodeState
	var cmd *exec.Cmd

	state, err = vm.statusVM()

	if err != nil {
		return err
	}

	if state == nodeStateStopped {
		cmd = exec.Command("multipass", "start", vm.nodeName)

		if out, err = cmd.Output(); err != nil {
			glog.Errorf(errStartVMFailed, vm.nodeName, string(out), err)
			return fmt.Errorf(errStartVMFailed, vm.nodeName, string(out), err)
		}

		vm.state = nodeStateRunning
	} else if state != nodeStateRunning {
		glog.Errorf(errVMNotFound, vm.nodeName)
		return fmt.Errorf(errVMNotFound, vm.nodeName)
	}

	return nil
}

func (vm *multipassNode) stopVM() error {
	var err error
	var out []byte
	var state nodeState
	var cmd *exec.Cmd

	state, err = vm.statusVM()

	if err != nil {
		return err
	}

	if state == nodeStateRunning {
		cmd = exec.Command("multipass", "stop", vm.nodeName)

		if out, err = cmd.Output(); err != nil {
			glog.Errorf(errStopVMFailed, vm.nodeName, string(out), err)
			return fmt.Errorf(errStopVMFailed, vm.nodeName, string(out), err)
		}

		vm.state = nodeStateStopped
	} else if state != nodeStateStopped {
		glog.Errorf(errVMNotFound, vm.nodeName)
		return fmt.Errorf(errVMNotFound, vm.nodeName)
	}

	return nil
}

func (vm *multipassNode) deleteVM() error {
	var err error
	var out []byte
	var state nodeState
	var cmd *exec.Cmd

	state, err = vm.statusVM()

	if err != nil {
		return err
	}

	if state == nodeStateRunning {
		cmd = exec.Command("multipass", "stop", vm.nodeName)

		if out, err = cmd.Output(); err != nil {
			glog.Errorf(errStopVMFailed, vm.nodeName, string(out), err)
			return fmt.Errorf(errStopVMFailed, vm.nodeName, string(out), err)
		}
	}

	cmd = exec.Command("multipass", "delete", "--purge", vm.nodeName)

	if out, err = cmd.Output(); err != nil {
		glog.Errorf(errDeleteVMFailed, vm.nodeName, string(out), err)
		return fmt.Errorf(errDeleteVMFailed, vm.nodeName, string(out), err)
	}

	vm.state = nodeStateDeleted

	return nil
}

func (vm *multipassNode) statusVM() (nodeState, error) {
	// Get VM infos
	var out []byte
	var err error
	var cmd *exec.Cmd

	cmd = exec.Command("multipass", "info", vm.nodeName, "--format=json")

	var vmInfos MultipassVMInfos

	if out, err = cmd.Output(); err != nil {
		glog.Errorf(errGetVMInfoFailed, vm.nodeName, string(out), err)
		return nodeStateUndefined, err
	}

	if err = json.Unmarshal(out, &vmInfos); err != nil {
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
