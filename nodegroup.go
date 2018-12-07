package main

import (
	"fmt"
	"sync"

	"github.com/golang/glog"
)

// Group all multipass VM created inside a NodeGroup
// Each node have name like <node group name>-vm-<vm index>
type multipassNodeGroup struct {
	sync.Mutex
	identifier string
	machine    *machineCharacteristic
	created    bool
	minSize    int
	maxSize    int
	queueSize  int
	nodes      map[string]*multipassNode
}

type nodeCreationExtra struct {
	kubeHost      string
	kubeToken     string
	kubeCACert    string
	kubeExtraArgs []string
	image         string
	cloudInit     *map[string]interface{}
	mountPoints   *map[string]string
	autoprovision bool
}

func (g *multipassNodeGroup) cleanup() error {
	for _, node := range g.nodes {
		if err := node.deleteVM(); err != nil {
			glog.Errorf(errNodeGroupCleanupFailOnVM, g.identifier, node.nodeName, err)
		}
	}

	g.nodes = make(map[string]*multipassNode)

	return nil
}

func (g *multipassNodeGroup) targetSize() int {
	return g.queueSize + len(g.nodes)
}

func (g *multipassNodeGroup) setNodeGroupSize(newSize int, extras *nodeCreationExtra) error {

	var err error

	g.Lock()

	delta := newSize - g.targetSize()

	if delta < 0 {
		err = g.deleteNodes(delta)
	} else if delta > 0 {
		err = g.addNodes(delta, extras)
	}

	g.Unlock()

	return err
}

// delta must be negative!!!!
func (g *multipassNodeGroup) deleteNodes(delta int) error {

	startIndex := len(g.nodes) - 1
	endIndex := startIndex + delta
	tempNodes := make([]*multipassNode, 0, -delta)

	for nodeIndex := startIndex; nodeIndex >= endIndex; nodeIndex-- {
		nodeName := g.nodeName(nodeIndex)

		if node := g.nodes[nodeName]; node != nil {
			if err := node.deleteVM(); err != nil {
				glog.Errorf(errUnableToDeleteVM, node.nodeName)
				return err
			}

			tempNodes = append(tempNodes, node)
		}
	}

	for _, node := range tempNodes {
		delete(g.nodes, node.nodeName)
	}

	return nil
}

func (g *multipassNodeGroup) addNodes(delta int, extras *nodeCreationExtra) error {

	g.queueSize = delta

	startIndex := len(g.nodes)
	endIndex := startIndex + delta
	tempNodes := make([]*multipassNode, 0, delta)

	for nodeIndex := startIndex; nodeIndex < endIndex; nodeIndex++ {
		node := &multipassNode{
			nodeName: g.nodeName(nodeIndex),
			memory:   g.machine.Memory,
			cpu:      g.machine.Vcpu,
			disk:     g.machine.Disk,
		}

		tempNodes = append(tempNodes, node)
	}

	for _, node := range tempNodes {
		g.queueSize--

		if err := node.launchVM(extras); err != nil {
			glog.Errorf(errUnableToLaunchVM, node.nodeName)

			for _, node := range tempNodes {
				if node.state == nodeStateRunning {
					if err := node.deleteVM(); err != nil {
						glog.Errorf(errUnableToDeleteVM, node.nodeName)
					}
				}
			}

			return err
		}
	}

	for _, node := range tempNodes {
		g.nodes[node.nodeName] = node
	}

	return nil
}

func (g *multipassNodeGroup) deleteNodeByName(nodeName string) error {

	if node := g.nodes[nodeName]; node != nil {

		if err := node.deleteVM(); err != nil {
			glog.Errorf(errUnableToDeleteVM, node.nodeName)
			return err
		}

		delete(g.nodes, nodeName)

		return nil
	}

	return fmt.Errorf(errNodeNotFoundInNodeGroup, nodeName, g.identifier)
}

func (g *multipassNodeGroup) deleteNodeGroup() error {
	return g.cleanup()
}

func (g *multipassNodeGroup) nodeName(vmIndex int) string {
	return fmt.Sprintf("%s-vm-%02d", g.identifier, vmIndex)
}

func (g *multipassNodeGroup) providerID(providerID string) string {
	return fmt.Sprintf("%s://%s/object?type=group", providerID, g.identifier)
}

func (g *multipassNodeGroup) providerIDForNode(providerID string, nodeName string) string {
	return fmt.Sprintf("%s://%s/object?type=node&name=%s", providerID, g.identifier, nodeName)
}
