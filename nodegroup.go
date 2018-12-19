package main

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/golang/glog"
	apiv1 "k8s.io/api/core/v1"
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
	identifier      string
	cloudProviderID string
	machine         *MachineCharacteristic
	status          nodegroupState
	minSize         int
	maxSize         int
	nodes           map[string]*multipassNode
	pendingNodes    map[string]*multipassNode
	pendingNodesWG  sync.WaitGroup
	nodeLabels      map[string]string
	systemLabels    map[string]string
	autoProvision   bool
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
	vmprovision   bool
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

		nodeName := g.nodeName(nodeIndex)

		node := &multipassNode{
			providerID:       g.providerIDForNode(nodeName),
			nodeName:         nodeName,
			memory:           g.machine.Memory,
			cpu:              g.machine.Vcpu,
			disk:             g.machine.Disk,
			autoprovisionned: true,
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

func (g *multipassNodeGroup) autoDiscoveryNodes(kubeconfig string) error {
	var nodeInfos apiv1.NodeList
	var out string
	var err error
	var arg = []string{
		"kubectl",
		"get",
		"nodes",
		"--output",
		"json",
		"--kubeconfig",
		kubeconfig,
	}

	if out, err = pipe(arg...); err != nil {
		return err
	}

	if err = json.Unmarshal([]byte(out), &nodeInfos); err != nil {
		return fmt.Errorf(errUnmarshallingError, "multipassNodeGroup::autoDiscoveryNodes", err)
	}

	for _, nodeInfo := range nodeInfos.Items {
		var providerID = nodeInfo.Spec.ProviderID
		var nodeID = ""

		if len(providerID) > 0 {
			out, err = nodeGroupIDFromProviderID(g.cloudProviderID, providerID)

			if out == g.identifier {
				glog.V(2).Infof("Discover node:%s matching nodegroup:%s", providerID, g.identifier)

				if nodeID, err = nodeNameFromProviderID(g.cloudProviderID, providerID); err == nil {
					runningIP := ""

					for _, address := range nodeInfo.Status.Addresses {
						if address.Type == apiv1.NodeInternalIP {
							runningIP = address.Address
							break
						}
					}

					glog.V(2).Infof("Add node:%s with IP:%s to nodegroup:%s", nodeID, runningIP, g.identifier)

					node := &multipassNode{
						providerID:       providerID,
						nodeName:         nodeID,
						state:            nodeStateRunning,
						autoprovisionned: false,
						address: []string{
							runningIP,
						},
					}

					g.nodes[nodeID] = node
				}
			}
		}
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

func (g *multipassNodeGroup) providerID() string {
	return fmt.Sprintf("%s://%s/object?type=group", g.cloudProviderID, g.identifier)
}

func (g *multipassNodeGroup) providerIDForNode(nodeName string) string {
	return fmt.Sprintf("%s://%s/object?type=node&name=%s", g.cloudProviderID, g.identifier, nodeName)
}
