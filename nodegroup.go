package main

import (
	"fmt"
	"sync"

	"github.com/golang/glog"
)

type nodegroupState int32

const (
	nodegroupNotCreated nodegroupState = 0
	nodegroupCreated    nodegroupState = 1
	nodegroupDeleting   nodegroupState = 2
	nodegroupDeleted    nodegroupState = 3
)

// Group all multipass VM created inside a NodeGroup
// Each node have name like <node group name>-vm-<vm index>
type multipassNodeGroup struct {
	sync.Mutex
	identifier     string
	machine        *MachineCharacteristic
	status         nodegroupState
	minSize        int
	maxSize        int
	nodes          map[string]*multipassNode
	pendingNodes   map[string]*multipassNode
	pendingNodesWG sync.WaitGroup
	nodeLabels     map[string]string
	systemLabels   map[string]string
}

type nodeCreationExtra struct {
	kubeHost      string
	kubeToken     string
	kubeCACert    string
	kubeExtraArgs []string
	kubeConfig    string
	image         string
	cloudInit     map[string]interface{}
	mountPoints   map[string]string
	nodeLabels    map[string]string
	systemLabels  map[string]string
	autoprovision bool
}

func (g *multipassNodeGroup) cleanup(kubeconfig string) error {
	glog.V(5).Infof("multipassNodeGroup::cleanup, nodeGroupID:%s", g.identifier)

	var lastError error

	g.status = nodegroupDeleting

	g.pendingNodesWG.Wait()

	glog.V(5).Infof("multipassNodeGroup::cleanup, nodeGroupID:%s, iterate node to delete", g.identifier)

	for _, node := range g.nodes {
		if lastError = node.deleteVM(kubeconfig); lastError != nil {
			glog.Errorf(errNodeGroupCleanupFailOnVM, g.identifier, node.nodeName, lastError)
		}
	}

	g.nodes = make(map[string]*multipassNode)
	g.pendingNodes = make(map[string]*multipassNode)
	g.status = nodegroupDeleted

	return lastError
}

func (g *multipassNodeGroup) targetSize() int {
	glog.V(5).Infof("multipassNodeGroup::targetSize, nodeGroupID:%s", g.identifier)

	return len(g.pendingNodes) + len(g.nodes)
}

func (g *multipassNodeGroup) setNodeGroupSize(newSize int, extras *nodeCreationExtra) error {
	glog.V(5).Infof("multipassNodeGroup::setNodeGroupSize, nodeGroupID:%s", g.identifier)

	var err error

	g.Lock()

	delta := newSize - g.targetSize()

	if delta < 0 {
		err = g.deleteNodes(delta, extras)
	} else if delta > 0 {
		err = g.addNodes(delta, extras)
	}

	g.Unlock()

	return err
}

func (g *multipassNodeGroup) refresh() {
	glog.V(5).Infof("multipassNodeGroup::refresh, nodeGroupID:%s", g.identifier)

	for _, node := range g.nodes {
		node.statusVM()
	}
}

// delta must be negative!!!!
func (g *multipassNodeGroup) deleteNodes(delta int, extras *nodeCreationExtra) error {
	glog.V(5).Infof("multipassNodeGroup::deleteNodes, nodeGroupID:%s", g.identifier)

	startIndex := len(g.nodes) - 1
	endIndex := startIndex + delta
	tempNodes := make([]*multipassNode, 0, -delta)

	for nodeIndex := startIndex; nodeIndex >= endIndex; nodeIndex-- {
		nodeName := g.nodeName(nodeIndex)

		if node := g.nodes[nodeName]; node != nil {
			if err := node.deleteVM(extras.kubeConfig); err != nil {
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
	glog.V(5).Infof("multipassNodeGroup::addNodes, nodeGroupID:%s", g.identifier)

	startIndex := g.targetSize()
	endIndex := startIndex + delta
	tempNodes := make([]*multipassNode, 0, delta)

	g.pendingNodesWG.Add(delta)

	for nodeIndex := startIndex; nodeIndex < endIndex; nodeIndex++ {
		if g.status != nodegroupCreated {
			glog.V(5).Infof("multipassNodeGroup::addNodes, nodeGroupID:%s -> g.status != nodegroupCreated", g.identifier)
			break
		}

		node := &multipassNode{
			nodeName: g.nodeName(nodeIndex),
			memory:   g.machine.Memory,
			cpu:      g.machine.Vcpu,
			disk:     g.machine.Disk,
		}

		tempNodes = append(tempNodes, node)

		g.pendingNodes[node.nodeName] = node
	}

	for _, node := range tempNodes {
		if g.status != nodegroupCreated {
			glog.V(5).Infof("multipassNodeGroup::addNodes, nodeGroupID:%s -> g.status != nodegroupCreated", g.identifier)
			break
		}

		if err := node.launchVM(extras); err != nil {
			glog.Errorf(errUnableToLaunchVM, node.nodeName, err)

			for _, node := range tempNodes {
				delete(g.pendingNodes, node.nodeName)

				if status, _ := node.statusVM(); status == nodeStateRunning {
					if err := node.deleteVM(extras.kubeConfig); err != nil {
						glog.Errorf(errUnableToDeleteVM, node.nodeName)
					}
				}

				g.pendingNodesWG.Done()
			}

			return err
		}

		delete(g.pendingNodes, node.nodeName)

		g.nodes[node.nodeName] = node
		g.pendingNodesWG.Done()
	}

	return nil
}

func (g *multipassNodeGroup) deleteNodeByName(kubeconfig, nodeName string) error {
	glog.V(5).Infof("multipassNodeGroup::deleteNodeByName, nodeGroupID:%s, nodeName:%s", g.identifier, nodeName)

	if node := g.nodes[nodeName]; node != nil {

		if err := node.deleteVM(kubeconfig); err != nil {
			glog.Errorf(errUnableToDeleteVM, node.nodeName)
			return err
		}

		delete(g.nodes, nodeName)

		return nil
	}

	return fmt.Errorf(errNodeNotFoundInNodeGroup, nodeName, g.identifier)
}

func (g *multipassNodeGroup) deleteNodeGroup(kubeConfig string) error {
	glog.V(5).Infof("multipassNodeGroup::deleteNodeGroup, nodeGroupID:%s", g.identifier)

	return g.cleanup(kubeConfig)
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
