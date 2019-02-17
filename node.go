package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	apiv1 "k8s.io/api/core/v1"

	"github.com/golang/glog"
)

// MultipassNodeState VM state
type MultipassNodeState int32

const (
	// MultipassNodeStateNotCreated not created state
	MultipassNodeStateNotCreated MultipassNodeState = 0

	// MultipassNodeStateRunning running state
	MultipassNodeStateRunning MultipassNodeState = 1

	// MultipassNodeStateStopped stopped state
	MultipassNodeStateStopped MultipassNodeState = 2

	// MultipassNodeStateDeleted deleted state
	MultipassNodeStateDeleted MultipassNodeState = 3

	// MultipassNodeStateUndefined undefined state
	MultipassNodeStateUndefined MultipassNodeState = 4
)

// MultipassNode Describe a multipass VM
type MultipassNode struct {
	ProviderID       string             `json:"providerID"`
	NodeName         string             `json:"name"`
	NodeIndex        int                `json:"index"`
	Memory           int                `json:"memory"`
	CPU              int                `json:"cpu"`
	Disk             int                `json:"disk"`
	Addresses        []string           `json:"addresses"`
	State            MultipassNodeState `json:"state"`
	AutoProvisionned bool               `json:"auto"`
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
	Disks        map[string]*VMDiskInfo  `json:"disks"`
	ImageHash    string                  `json:"image_hash"`
	ImageRelease string                  `json:"image_release"`
	Ipv4         []string                `json:"ipv4"`
	Load         []float64               `json:"load"`
	Memory       *VMMemoryInfo           `json:"memory"`
	Mounts       map[string]VMMountInfos `json:"mounts"`
	Release      string                  `json:"release"`
	State        string                  `json:"state"`
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

func (vm *MultipassNode) prepareKubelet() error {
	var out string
	var err error
	var fName = fmt.Sprintf("/tmp/set-kubelet-default-%s.sh", vm.NodeName)

	kubeletDefault := []string{
		"#!/bin/bash",
		". /etc/default/kubelet",
		fmt.Sprintf("echo \"KUBELET_EXTRA_ARGS=\\\"$KUBELET_EXTRA_ARGS --provider-id=%s\\\"\" > /etc/default/kubelet", vm.ProviderID),
		"systemctl restart kubelet",
	}

	if err = ioutil.WriteFile(fName, []byte(strings.Join(kubeletDefault, "\n")), 0755); err != nil {
		return fmt.Errorf(errKubeletNotConfigured, vm.NodeName, out, err)
	}

	defer os.Remove(fName)

	if out, err = pipe("multipass", "copy-files", fName, vm.NodeName+":"+fName); err != nil {
		return fmt.Errorf(errKubeletNotConfigured, vm.NodeName, out, err)
	}

	if out, err = pipe("multipass", "exec", vm.NodeName, "--", "sudo", "bash", fName); err != nil {
		return fmt.Errorf(errKubeletNotConfigured, vm.NodeName, out, err)
	}

	return nil
}

func (vm *MultipassNode) waitReady(kubeconfig string) error {
	glog.V(5).Infof("multipassNode::waitReady, node:%s", vm.NodeName)

	// Max 60s
	for index := 0; index < 12; index++ {
		var out string
		var err error
		var arg = []string{
			"kubectl",
			"get",
			"nodes",
			vm.NodeName,
			"--output",
			"json",
			"--kubeconfig",
			kubeconfig,
		}

		if out, err = pipe(arg...); err != nil {
			return err
		}

		var nodeInfo apiv1.Node

		if err = json.Unmarshal([]byte(out), &nodeInfo); err != nil {
			return fmt.Errorf(errUnmarshallingError, vm.NodeName, err)
		}

		for _, status := range nodeInfo.Status.Conditions {
			if status.Type == "Ready" {
				if b, e := strconv.ParseBool(string(status.Status)); e == nil {
					if b {
						glog.Infof("The kubernetes node %s is Ready", vm.NodeName)
						return nil
					}
				}
			}
		}

		glog.Infof("The kubernetes node:%s is not ready", vm.NodeName)

		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf(errNodeIsNotReady, vm.NodeName)
}

func (vm *MultipassNode) kubeAdmJoin(extras *nodeCreationExtra) error {
	args := []string{
		"multipass",
		"exec",
		vm.NodeName,
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
		return fmt.Errorf(errKubeAdmJoinFailed, vm.NodeName, err)
	}

	return nil
}

func (vm *MultipassNode) setNodeLabels(extras *nodeCreationExtra) error {
	if len(extras.nodeLabels)+len(extras.systemLabels) > 0 {

		args := []string{
			"kubectl",
			"label",
			"nodes",
			vm.NodeName,
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
			return fmt.Errorf(errKubeCtlIgnoredError, vm.NodeName, err)
		}
	}

	args := []string{
		"kubectl",
		"annotate",
		"node",
		vm.NodeName,
		fmt.Sprintf("%s=%s", nodeLabelGroupName, extras.nodegroupID),
		fmt.Sprintf("%s=%s", annotationNodeAutoProvisionned, strconv.FormatBool(vm.AutoProvisionned)),
		fmt.Sprintf("%s=%d", annotationNodeIndex, vm.NodeIndex),
		"--overwrite",
		"--kubeconfig",
		extras.kubeConfig,
	}

	if err := shell(args...); err != nil {
		return fmt.Errorf(errKubeCtlIgnoredError, vm.NodeName, err)
	}

	return nil
}

func (vm *MultipassNode) mountPoints(extras *nodeCreationExtra) {
	if extras.mountPoints != nil && len(extras.mountPoints) > 0 {
		for hostPath, guestPath := range extras.mountPoints {
			if err := shell("multipass", "mount", hostPath, fmt.Sprintf("%s:%s", vm.NodeName, guestPath)); err != nil {
				glog.Warningf(errUnableToMountPath, hostPath, guestPath, vm.NodeName, err)
			}
		}
	}
}

func (vm *MultipassNode) writeCloudFile(extras *nodeCreationExtra) (*os.File, error) {
	var cloudInitFile *os.File
	var err error
	var b []byte

	if extras.cloudInit != nil && len(extras.cloudInit) > 0 {
		fName := fmt.Sprintf("%s/cloud-init-%s.yaml", os.TempDir(), vm.NodeName)
		cloudInitFile, err = os.Create(fName)

		if err == nil {
			defer os.Remove(fName)

			if b, err = yaml.Marshal(extras.cloudInit); err == nil {
				if _, err = cloudInitFile.Write(b); err != nil {
					err = fmt.Errorf(errCloudInitWriteError, err)
				}
			} else {
				err = fmt.Errorf(errCloudInitMarshallError, err)
			}
		} else {
			err = fmt.Errorf(errTempFile, err)
		}
	}

	return cloudInitFile, err
}

func (vm *MultipassNode) launchVM(extras *nodeCreationExtra) error {
	glog.V(5).Infof("multipassNode::launchVM, node:%s", vm.NodeName)

	var cloudInitFile *os.File
	var err error
	var status MultipassNodeState

	glog.Infof("Launch VM:%s for nodegroup: %s", vm.NodeName, extras.nodegroupID)

	if vm.AutoProvisionned == false {
		err = fmt.Errorf(errVMNotProvisionnedByMe, vm.NodeName)
	} else {

		if vm.State != MultipassNodeStateNotCreated {
			err = fmt.Errorf(errVMAlreadyCreated, vm.NodeName)
		} else if cloudInitFile, err = vm.writeCloudFile(extras); err == nil {
			var args = []string{
				"multipass",
				"launch",
				"--name",
				vm.NodeName,
			}

			/*
				Append VM attributes Memory,cpus, hard drive size....
			*/
			if vm.Memory > 0 {
				args = append(args, fmt.Sprintf("--mem=%dM", vm.Memory))
			}

			if vm.CPU > 0 {
				args = append(args, fmt.Sprintf("--cpus=%d", vm.CPU))
			}

			if vm.Disk > 0 {
				args = append(args, fmt.Sprintf("--disk=%dM", vm.Disk))
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
				err = fmt.Errorf(errUnableToLaunchVM, vm.NodeName, err)
			} else {
				// Add mount point
				vm.mountPoints(extras)

				if status, err = vm.statusVM(); err != nil {
					glog.Error(err.Error())
				} else if status == MultipassNodeStateRunning {
					// If the VM is running call kubeadm join
					if extras.vmprovision {
						if err = vm.prepareKubelet(); err == nil {
							if err = vm.kubeAdmJoin(extras); err == nil {
								if err = vm.waitReady(extras.kubeConfig); err == nil {
									err = vm.setNodeLabels(extras)
								}
							}
						}
					}
				} else {
					err = fmt.Errorf(errKubeAdmJoinNotRunning, vm.NodeName)
				}
			}
		}
	}

	if err == nil {
		glog.Infof("Launched VM:%s for nodegroup: %s", vm.NodeName, extras.nodegroupID)
	} else {
		glog.Errorf("Unable to launch VM:%s for nodegroup: %s. Reason: %v", vm.NodeName, extras.nodegroupID, err.Error())
	}

	return err
}

func (vm *MultipassNode) startVM(kubeconfig string) error {
	glog.V(5).Infof("multipassNode::startVM, node:%s", vm.NodeName)

	var err error
	var state MultipassNodeState

	glog.Infof("Start VM:%s", vm.NodeName)

	if vm.AutoProvisionned == false {
		err = fmt.Errorf(errVMNotProvisionnedByMe, vm.NodeName)
	} else {
		state, err = vm.statusVM()

		if err == nil {
			if state == MultipassNodeStateStopped {
				if err = shell("multipass", "start", vm.NodeName); err != nil {
					args := []string{
						"kubectl",
						"uncordon",
						vm.NodeName,
						"--kubeconfig",
						kubeconfig,
					}

					if err = shell(args...); err != nil {
						glog.Errorf(errKubeCtlIgnoredError, vm.NodeName, err)

						err = nil
					}

					vm.State = MultipassNodeStateRunning
				} else {
					err = fmt.Errorf(errStartVMFailed, vm.NodeName, err)
				}
			} else if state != MultipassNodeStateRunning {
				err = fmt.Errorf(errStartVMFailed, vm.NodeName, fmt.Sprintf("Unexpected state: %d", state))
			}
		}
	}

	if err == nil {
		glog.Infof("Started VM:%s", vm.NodeName)
	} else {
		glog.Errorf("Unable to start VM:%s. Reason: %v", vm.NodeName, err)
	}

	return nil
}

func (vm *MultipassNode) stopVM(kubeconfig string) error {
	glog.V(5).Infof("multipassNode::stopVM, node:%s", vm.NodeName)

	var err error
	var state MultipassNodeState

	glog.Infof("Stop VM:%s", vm.NodeName)

	if vm.AutoProvisionned == false {
		err = fmt.Errorf(errVMNotProvisionnedByMe, vm.NodeName)
	} else {
		state, err = vm.statusVM()

		if err == nil {

			if state == MultipassNodeStateRunning {
				args := []string{
					"kubectl",
					"cordon",
					vm.NodeName,
					"--kubeconfig",
					kubeconfig,
				}

				if err = shell(args...); err != nil {
					glog.Errorf(errKubeCtlIgnoredError, vm.NodeName, err)
				}

				if err = shell("multipass", "stop", vm.NodeName); err == nil {
					vm.State = MultipassNodeStateStopped
				} else {
					err = fmt.Errorf(errStopVMFailed, vm.NodeName, err)
				}
			} else if state != MultipassNodeStateStopped {
				err = fmt.Errorf(errStopVMFailed, vm.NodeName, fmt.Sprintf("Unexpected state: %d", state))
			}
		}
	}

	if err == nil {
		glog.Infof("Stopped VM:%s", vm.NodeName)
	} else {
		glog.Errorf("Could not stop VM:%s. Reason: %s", vm.NodeName, err)
	}

	return err
}

func (vm *MultipassNode) deleteVM(kubeconfig string) error {
	glog.V(5).Infof("multipassNode::deleteVM, node:%s", vm.NodeName)

	var err error
	var state MultipassNodeState

	if vm.AutoProvisionned == false {
		err = fmt.Errorf(errVMNotProvisionnedByMe, vm.NodeName)
	} else {
		state, err = vm.statusVM()

		if err == nil {

			args := []string{
				"kubectl",
				"drain",
				vm.NodeName,
				"--delete-local-data",
				"--force",
				"--ignore-daemonsets",
				"--kubeconfig",
				kubeconfig,
			}

			if err = shell(args...); err != nil {
				glog.Errorf(errKubeCtlIgnoredError, vm.NodeName, err)
			}

			args = []string{
				"kubectl",
				"delete",
				"node",
				vm.NodeName,
				"--kubeconfig",
				kubeconfig,
			}

			if err = shell(args...); err != nil {
				glog.Errorf(errKubeCtlIgnoredError, vm.NodeName, err)
			}

			if state == MultipassNodeStateRunning {
				if err = shell("multipass", "stop", vm.NodeName); err == nil {
					vm.State = MultipassNodeStateStopped

					if err = shell("multipass", "delete", "--purge", vm.NodeName); err == nil {
						vm.State = MultipassNodeStateDeleted
					} else {
						err = fmt.Errorf(errDeleteVMFailed, vm.NodeName, err)
					}
				} else {
					err = fmt.Errorf(errStopVMFailed, vm.NodeName, err)
				}
			} else if err = shell("multipass", "delete", "--purge", vm.NodeName); err == nil {
				vm.State = MultipassNodeStateDeleted
			} else {
				err = fmt.Errorf(errDeleteVMFailed, vm.NodeName, err)
			}
		}
	}

	if err == nil {
		glog.Infof("Deleted VM:%s", vm.NodeName)
	} else {
		glog.Errorf("Could not delete VM:%s. Reason: %s", vm.NodeName, err)
	}

	return err
}

func (vm *MultipassNode) statusVM() (MultipassNodeState, error) {
	glog.V(5).Infof("multipassNode::statusVM, node:%s", vm.NodeName)

	// Get VM infos
	var out string
	var err error
	var vmInfos MultipassVMInfos

	if out, err = pipe("multipass", "info", vm.NodeName, "--format=json"); err != nil {
		glog.Errorf(errGetVMInfoFailed, vm.NodeName, err)
		return MultipassNodeStateUndefined, err
	}

	if err = json.Unmarshal([]byte(out), &vmInfos); err != nil {
		glog.Errorf(errGetVMInfoFailed, vm.NodeName, err)
		return MultipassNodeStateUndefined, err
	}

	if vmInfo := vmInfos.Info[vm.NodeName]; vmInfo != nil {
		vm.Addresses = vmInfo.Ipv4

		switch vmInfo.State {
		case "RUNNING":
			vm.State = MultipassNodeStateRunning
		case "STOPPED":
			vm.State = MultipassNodeStateStopped
		case "DELETED":
			vm.State = MultipassNodeStateDeleted
		default:
			vm.State = MultipassNodeStateUndefined
		}

		vm.Addresses = vmInfo.Ipv4

		return vm.State, nil
	}

	return MultipassNodeStateUndefined, fmt.Errorf(errMultiPassInfoNotFound, vm.NodeName)
}
