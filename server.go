package main

import (
	"context"
	"fmt"
	"time"

	apigrpc "github.com/Fred78290/kubernetes-multipass-autoscaler/grpc"
	"github.com/golang/glog"
	apiv1 "k8s.io/api/core/v1"
)

type resourceLimiter struct {
	minLimits map[string]int64
	maxLimits map[string]int64
}

type machineCharacteristic struct {
	Memory int `json:"memsize"`  // VM Memory size in megabytes
	Vcpu   int `json:"vcpus"`    // VM number of cpus
	Disk   int `json:"disksize"` // VM disk size in megabytes
}

// MultipassServerConfig is contains configuration
type MultipassServerConfig struct {
	Address       string                            `default:"0.0.0.0" json:"address"`             // Mandatory, Address to listen
	Port          int                               `default:"5200" json:"port"`                   // Mandatory, Port to listen
	ProviderID    string                            `json:"secret"`                                // Mandatory, secret Identifier, client must match this
	MinNode       int                               `json:"minNode"`                               // Mandatory, Min Multipass VM
	MaxNode       int                               `json:"maxNode"`                               // Mandatory, Max Multipass VM
	NodePrice     float64                           `json:"nodePrice"`                             // Optional, The VM price
	PodPrice      float64                           `json:"podPrice"`                              // Optional, The pod price
	Image         string                            `json:"image"`                                 // Optional, URL to multipass image or image name
	Machines      map[string]*machineCharacteristic `default:"{\"standard\": {}}" json:"machines"` // Mandatory, Available machines
	CloudInit     map[string]interface{}            `json:"cloud-init"`                            // Optional, The cloud init conf file
	MountPoints   map[string]string                 `json:"mount-points"`                          // Optional, mount point between host and guest
	AutoProvision bool                              `default:"true" json:"auto-provision"`
}

// MultipassServer declare multipass grpc server
type MultipassServer struct {
	resourceLimiter *resourceLimiter
	nodeGroups      map[string]*multipassNodeGroup
	config          MultipassServerConfig
	kubeAdmConfig   *apigrpc.KubeAdmConfig
}

func (s *MultipassServer) generateNodeGroupName() string {
	return fmt.Sprintf("ng-%d", time.Now().Unix())
}

func (s *MultipassServer) newNodeGroup(nodeGroupID string, machineType string) (*multipassNodeGroup, error) {

	machine := s.config.Machines[machineType]

	if machine == nil {
		return nil, fmt.Errorf(errMachineTypeNotFound, machineType)
	}

	if nodeGroup := s.nodeGroups[nodeGroupID]; nodeGroup != nil {
		glog.Errorf(errNodeGroupAlreadyExists, nodeGroupID)
		return nil, fmt.Errorf(errNodeGroupAlreadyExists, nodeGroupID)
	}

	nodeGroup := &multipassNodeGroup{
		identifier: nodeGroupID,
		machine:    machine,
		nodes:      make(map[string]*multipassNode),
		minSize:    s.config.MinNode,
		maxSize:    s.config.MaxNode,
	}

	s.nodeGroups[nodeGroupID] = nodeGroup

	return nodeGroup, nil
}

func (s *MultipassServer) deleteNodeGroup(nodeGroupID string) error {
	nodeGroup := s.nodeGroups[nodeGroupID]

	if nodeGroup == nil {
		glog.Errorf(errNodeGroupNotFound, nodeGroupID)
		return fmt.Errorf(errNodeGroupNotFound, nodeGroupID)
	}

	if err := nodeGroup.deleteNodeGroup(); err != nil {
		glog.Errorf(errUnableToDeleteNodeGroup, nodeGroupID, err)
		return err
	}

	delete(s.nodeGroups, nodeGroupID)

	return nil
}

func (s *MultipassServer) createNodeGroup(nodeGroupID string) (*multipassNodeGroup, error) {
	nodeGroup := s.nodeGroups[nodeGroupID]

	if nodeGroup == nil {
		glog.Errorf(errNodeGroupNotFound, nodeGroupID)
		return nil, fmt.Errorf(errNodeGroupNotFound, nodeGroupID)
	}

	if nodeGroup.created == false {
		// Must launch minNode VM
		if nodeGroup.minSize > 0 {

			extras := &nodeCreationExtra{
				s.kubeAdmConfig.KubeAdmAddress,
				s.kubeAdmConfig.KubeAdmToken,
				s.kubeAdmConfig.KubeAdmCACert,
				s.kubeAdmConfig.KubeAdmExtraArguments,
				s.config.Image,
				&s.config.CloudInit,
				&s.config.MountPoints,
				s.config.AutoProvision,
			}

			if err := nodeGroup.addNodes(nodeGroup.minSize, extras); err != nil {
				glog.Errorf(err.Error())
				return nil, err
			}
		}

		nodeGroup.created = true
	}

	return nodeGroup, nil
}

// Connect allows client to connect
func (s *MultipassServer) Connect(ctx context.Context, request *apigrpc.ConnectRequest) (*apigrpc.ConnectReply, error) {
	glog.V(2).Infof("Call server Connect: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	if request.GetResourceLimiter() != nil {
		s.resourceLimiter = &resourceLimiter{
			minLimits: request.ResourceLimiter.MinLimits,
			maxLimits: request.ResourceLimiter.MaxLimits,
		}
	}

	if request.GetKubeAdmConfiguration() != nil {
		s.kubeAdmConfig = request.GetKubeAdmConfiguration()
	}

	return &apigrpc.ConnectReply{
		Response: &apigrpc.ConnectReply_Connected{
			Connected: true,
		},
	}, nil
}

// Name returns name of the cloud provider.
func (s *MultipassServer) Name(ctx context.Context, request *apigrpc.CloudProviderServiceRequest) (*apigrpc.NameReply, error) {
	glog.V(2).Infof("Call server Name: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	return &apigrpc.NameReply{
		Name: providerName,
	}, nil
}

// NodeGroups returns all node groups configured for this cloud provider.
func (s *MultipassServer) NodeGroups(ctx context.Context, request *apigrpc.CloudProviderServiceRequest) (*apigrpc.NodeGroupsReply, error) {
	glog.V(2).Infof("Call server NodeGroups: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	nodeGroups := make([]*apigrpc.NodeGroup, 0, len(s.nodeGroups))

	for n := range s.nodeGroups {
		nodeGroups = append(nodeGroups, &apigrpc.NodeGroup{
			Id: n,
		})
	}

	return &apigrpc.NodeGroupsReply{
		NodeGroups: nodeGroups,
	}, nil
}

func (s *MultipassServer) nodeGroupForNode(providerID string) (*multipassNodeGroup, error) {
	nodeGroupID, err := nodeGroupIDFromProviderID(s.config.ProviderID, providerID)

	if err != nil {
		glog.Errorf(errCantDecodeNodeIDWithReason, providerID, err)

		return nil, fmt.Errorf(errCantDecodeNodeIDWithReason, providerID, err)
	}

	if len(nodeGroupID) == 0 {
		glog.Errorf(errCantDecodeNodeID, providerID)

		return nil, fmt.Errorf(errCantDecodeNodeID, providerID)
	}

	nodeGroup := s.nodeGroups[nodeGroupID]

	if nodeGroup == nil {
		glog.Errorf(errNodeGroupForNodeNotFound, nodeGroupID, providerID)

		return nil, fmt.Errorf(errNodeGroupForNodeNotFound, nodeGroupID, providerID)
	}

	return nodeGroup, err
}

// NodeGroupForNode returns the node group for the given node, nil if the node
// should not be processed by cluster autoscaler, or non-nil error if such
// occurred. Must be implemented.
func (s *MultipassServer) NodeGroupForNode(ctx context.Context, request *apigrpc.NodeGroupForNodeRequest) (*apigrpc.NodeGroupForNodeReply, error) {
	glog.V(2).Infof("Call server NodeGroupForNode: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	node, err := nodeFromJSON(request.GetNode())

	if err != nil {
		glog.Errorf(errCantUnmarshallNodeWithReason, request.GetNode(), err)

		return &apigrpc.NodeGroupForNodeReply{
			Response: &apigrpc.NodeGroupForNodeReply_Error{
				Error: &apigrpc.Error{
					Code:   cloudProviderError,
					Reason: err.Error(),
				},
			},
		}, nil
	}

	nodeGroup, err := s.nodeGroupForNode(node.Spec.ProviderID)

	if err != nil {
		return &apigrpc.NodeGroupForNodeReply{
			Response: &apigrpc.NodeGroupForNodeReply_Error{
				Error: &apigrpc.Error{
					Code:   cloudProviderError,
					Reason: err.Error(),
				},
			},
		}, nil
	}

	return &apigrpc.NodeGroupForNodeReply{
		Response: &apigrpc.NodeGroupForNodeReply_NodeGroup{
			NodeGroup: &apigrpc.NodeGroup{
				Id: nodeGroup.identifier,
			},
		},
	}, nil
}

// Pricing returns pricing model for this cloud provider or error if not available.
// Implementation optional.
func (s *MultipassServer) Pricing(ctx context.Context, request *apigrpc.CloudProviderServiceRequest) (*apigrpc.PricingModelReply, error) {
	glog.V(2).Infof("Call server Pricing: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	return &apigrpc.PricingModelReply{
		Response: &apigrpc.PricingModelReply_PriceModel{
			PriceModel: &apigrpc.PricingModel{
				Id: s.config.ProviderID,
			},
		},
	}, nil
}

// GetAvailableMachineTypes get all machine types that can be requested from the cloud provider.
// Implementation optional.
func (s *MultipassServer) GetAvailableMachineTypes(ctx context.Context, request *apigrpc.CloudProviderServiceRequest) (*apigrpc.AvailableMachineTypesReply, error) {
	glog.V(2).Infof("Call server GetAvailableMachineTypes: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	machineTypes := make([]string, 0, len(s.config.Machines))

	for n := range s.config.Machines {
		machineTypes = append(machineTypes, n)
	}

	return &apigrpc.AvailableMachineTypesReply{
		Response: &apigrpc.AvailableMachineTypesReply_AvailableMachineTypes{
			AvailableMachineTypes: &apigrpc.AvailableMachineTypes{
				MachineType: machineTypes,
			},
		},
	}, nil
}

// NewNodeGroup builds a theoretical node group based on the node definition provided. The node group is not automatically
// created on the cloud provider side. The node group is not returned by NodeGroups() until it is created.
// Implementation optional.
func (s *MultipassServer) NewNodeGroup(ctx context.Context, request *apigrpc.NewNodeGroupRequest) (*apigrpc.NewNodeGroupReply, error) {
	glog.V(2).Infof("Call server NewNodeGroup: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	machineType := s.config.Machines[request.GetMachineType()]

	if machineType == nil {
		glog.Errorf(errMachineTypeNotFound, request.GetMachineType())

		return &apigrpc.NewNodeGroupReply{
			Response: &apigrpc.NewNodeGroupReply_Error{
				Error: &apigrpc.Error{
					Code:   cloudProviderError,
					Reason: fmt.Sprintf(errMachineTypeNotFound, request.GetMachineType()),
				},
			},
		}, nil
	}

	nodeGroupIdentifier := s.generateNodeGroupName()
	nodeGroup, err := s.newNodeGroup(nodeGroupIdentifier, request.GetMachineType())

	if err != nil {
		glog.Errorf(errUnableToCreateNodeGroup, nodeGroupIdentifier, err)

		return &apigrpc.NewNodeGroupReply{
			Response: &apigrpc.NewNodeGroupReply_Error{
				Error: &apigrpc.Error{
					Code:   cloudProviderError,
					Reason: fmt.Sprintf(errUnableToCreateNodeGroup, nodeGroupIdentifier, err),
				},
			},
		}, nil
	}

	return &apigrpc.NewNodeGroupReply{
		Response: &apigrpc.NewNodeGroupReply_NodeGroup{
			NodeGroup: &apigrpc.NodeGroup{
				Id: nodeGroup.identifier,
			},
		},
	}, nil
}

// GetResourceLimiter returns struct containing limits (max, min) for resources (cores, memory etc.).
func (s *MultipassServer) GetResourceLimiter(ctx context.Context, request *apigrpc.CloudProviderServiceRequest) (*apigrpc.ResourceLimiterReply, error) {
	glog.V(2).Infof("Call server GetResourceLimiter: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	return &apigrpc.ResourceLimiterReply{
		Response: &apigrpc.ResourceLimiterReply_ResourceLimiter{
			ResourceLimiter: &apigrpc.ResourceLimiter{
				MinLimits: s.resourceLimiter.minLimits,
				MaxLimits: s.resourceLimiter.maxLimits,
			},
		},
	}, nil
}

// Cleanup cleans up open resources before the cloud provider is destroyed, i.e. go routines etc.
func (s *MultipassServer) Cleanup(ctx context.Context, request *apigrpc.CloudProviderServiceRequest) (*apigrpc.CleanupReply, error) {
	glog.V(2).Infof("Call server Cleanup: %v", request)

	var lastError *apigrpc.Error

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	for _, nodeGroup := range s.nodeGroups {
		if err := nodeGroup.cleanup(); err != nil {
			lastError = &apigrpc.Error{
				Code:   cloudProviderError,
				Reason: err.Error(),
			}
		}
	}

	return &apigrpc.CleanupReply{
		Error: lastError,
	}, nil
}

// Refresh is called before every main loop and can be used to dynamically update cloud provider state.
// In particular the list of node groups returned by NodeGroups can change as a result of CloudProvider.Refresh().
func (s *MultipassServer) Refresh(ctx context.Context, request *apigrpc.CloudProviderServiceRequest) (*apigrpc.RefreshReply, error) {
	glog.V(2).Infof("Call server Refresh: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	return &apigrpc.RefreshReply{
		Error: nil,
	}, nil
}

// MaxSize returns maximum size of the node group.
func (s *MultipassServer) MaxSize(ctx context.Context, request *apigrpc.NodeGroupServiceRequest) (*apigrpc.MaxSizeReply, error) {
	glog.V(2).Infof("Call server MaxSize: %v", request)

	var maxSize int

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	nodeGroup := s.nodeGroups[request.GetNodeGroupID()]

	if nodeGroup != nil {
		glog.Errorf(errNodeGroupNotFound, request.GetNodeGroupID())

		maxSize = nodeGroup.maxSize
	}

	return &apigrpc.MaxSizeReply{
		MaxSize: int32(maxSize),
	}, nil
}

// MinSize returns minimum size of the node group.
func (s *MultipassServer) MinSize(ctx context.Context, request *apigrpc.NodeGroupServiceRequest) (*apigrpc.MinSizeReply, error) {
	glog.V(2).Infof("Call server MinSize: %v", request)

	var minSize int

	if request.GetProviderID() != s.config.ProviderID {
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	nodeGroup := s.nodeGroups[request.GetNodeGroupID()]

	if nodeGroup != nil {
		glog.Errorf(errNodeGroupNotFound, request.GetNodeGroupID())

		minSize = nodeGroup.minSize
	}

	return &apigrpc.MinSizeReply{
		MinSize: int32(minSize),
	}, nil
}

// TargetSize returns the current target size of the node group. It is possible that the
// number of nodes in Kubernetes is different at the moment but should be equal
// to Size() once everything stabilizes (new nodes finish startup and registration or
// removed nodes are deleted completely). Implementation required.
func (s *MultipassServer) TargetSize(ctx context.Context, request *apigrpc.NodeGroupServiceRequest) (*apigrpc.TargetSizeReply, error) {
	glog.V(2).Infof("Call server TargetSize: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	nodeGroup := s.nodeGroups[request.GetNodeGroupID()]

	if nodeGroup == nil {
		glog.Errorf(errNodeGroupNotFound, request.GetNodeGroupID())

		return &apigrpc.TargetSizeReply{
			Response: &apigrpc.TargetSizeReply_Error{
				Error: &apigrpc.Error{
					Code:   cloudProviderError,
					Reason: fmt.Sprintf(errNodeGroupNotFound, request.GetNodeGroupID()),
				},
			},
		}, nil
	}

	return &apigrpc.TargetSizeReply{
		Response: &apigrpc.TargetSizeReply_TargetSize{
			TargetSize: int32(nodeGroup.targetSize()),
		},
	}, nil
}

// IncreaseSize increases the size of the node group. To delete a node you need
// to explicitly name it and use DeleteNode. This function should wait until
// node group size is updated. Implementation required.
func (s *MultipassServer) IncreaseSize(ctx context.Context, request *apigrpc.IncreaseSizeRequest) (*apigrpc.IncreaseSizeReply, error) {
	glog.V(2).Infof("Call server IncreaseSize: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	nodeGroup := s.nodeGroups[request.GetNodeGroupID()]

	if nodeGroup == nil {
		glog.Errorf(errNodeGroupNotFound, request.GetNodeGroupID())

		return &apigrpc.IncreaseSizeReply{
			Error: &apigrpc.Error{
				Code:   cloudProviderError,
				Reason: fmt.Sprintf(errNodeGroupNotFound, request.GetNodeGroupID()),
			},
		}, nil
	}

	if request.GetDelta() <= 0 {
		glog.Errorf(errIncreaseSizeMustBePositive)

		return &apigrpc.IncreaseSizeReply{
			Error: &apigrpc.Error{
				Code:   cloudProviderError,
				Reason: errIncreaseSizeMustBePositive,
			},
		}, nil
	}

	newSize := len(nodeGroup.nodes) + int(request.GetDelta())

	if newSize > nodeGroup.maxSize {
		glog.Errorf(errIncreaseSizeTooLarge, newSize, nodeGroup.maxSize)

		return &apigrpc.IncreaseSizeReply{
			Error: &apigrpc.Error{
				Code:   cloudProviderError,
				Reason: fmt.Sprintf(errIncreaseSizeTooLarge, newSize, nodeGroup.maxSize),
			},
		}, nil
	}

	extras := &nodeCreationExtra{
		s.kubeAdmConfig.KubeAdmAddress,
		s.kubeAdmConfig.KubeAdmToken,
		s.kubeAdmConfig.KubeAdmCACert,
		s.kubeAdmConfig.KubeAdmExtraArguments,
		s.config.Image,
		&s.config.CloudInit,
		&s.config.MountPoints,
		s.config.AutoProvision,
	}

	err := nodeGroup.setNodeGroupSize(newSize, extras)

	if err != nil {
		return &apigrpc.IncreaseSizeReply{
			Error: &apigrpc.Error{
				Code:   cloudProviderError,
				Reason: err.Error(),
			},
		}, nil
	}

	return &apigrpc.IncreaseSizeReply{
		Error: nil,
	}, nil
}

// DeleteNodes deletes nodes from this node group. Error is returned either on
// failure or if the given node doesn't belong to this node group. This function
// should wait until node group size is updated. Implementation required.
func (s *MultipassServer) DeleteNodes(ctx context.Context, request *apigrpc.DeleteNodesRequest) (*apigrpc.DeleteNodesReply, error) {
	glog.V(2).Infof("Call server DeleteNodes: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	nodeGroup := s.nodeGroups[request.GetNodeGroupID()]

	if nodeGroup == nil {
		glog.Errorf(errNodeGroupNotFound, request.GetNodeGroupID())

		return &apigrpc.DeleteNodesReply{
			Error: &apigrpc.Error{
				Code:   cloudProviderError,
				Reason: fmt.Sprintf(errNodeGroupNotFound, request.GetNodeGroupID()),
			},
		}, nil
	}

	if nodeGroup.targetSize() < nodeGroup.minSize {
		return &apigrpc.DeleteNodesReply{
			Error: &apigrpc.Error{
				Code:   cloudProviderError,
				Reason: fmt.Sprintf(errMinSizeReached, request.GetNodeGroupID()),
			},
		}, nil
	}

	// Iterate over each requested node to delete
	for idx, sNode := range request.GetNode() {
		node, err := nodeFromJSON(sNode)

		// Can't deserialize
		if node == nil || err != nil {
			glog.Errorf(errCantUnmarshallNodeWithReason, sNode, err)

			return &apigrpc.DeleteNodesReply{
				Error: &apigrpc.Error{
					Code:   cloudProviderError,
					Reason: fmt.Sprintf(errCantUnmarshallNode, idx, request.GetNodeGroupID()),
				},
			}, nil
		}

		// Check node group owner
		nodeName := node.Spec.ProviderID
		nodeGroupForNode, err := s.nodeGroupForNode(nodeName)

		// Node group not found
		if err != nil {
			glog.Errorf(errNodeGroupNotFound, nodeName)

			return &apigrpc.DeleteNodesReply{
				Error: &apigrpc.Error{
					Code:   cloudProviderError,
					Reason: err.Error(),
				},
			}, nil
		}

		// Not in the same group
		if nodeGroupForNode.identifier != nodeGroup.identifier {
			return &apigrpc.DeleteNodesReply{
				Error: &apigrpc.Error{
					Code:   cloudProviderError,
					Reason: fmt.Sprintf(errUnableToDeleteNode, nodeName, nodeGroup.identifier),
				},
			}, nil
		}

		// Delete the node in the group
		nodeName, err = nodeNameFromProviderID(s.config.ProviderID, nodeName)

		err = nodeGroup.deleteNodeByName(nodeName)

		if err != nil {
			return &apigrpc.DeleteNodesReply{
				Error: &apigrpc.Error{
					Code:   cloudProviderError,
					Reason: err.Error(),
				},
			}, nil
		}
	}

	return &apigrpc.DeleteNodesReply{
		Error: nil,
	}, nil
}

// DecreaseTargetSize decreases the target size of the node group. This function
// doesn't permit to delete any existing node and can be used only to reduce the
// request for new nodes that have not been yet fulfilled. Delta should be negative.
// It is assumed that cloud provider will not delete the existing nodes when there
// is an option to just decrease the target. Implementation required.
func (s *MultipassServer) DecreaseTargetSize(ctx context.Context, request *apigrpc.DecreaseTargetSizeRequest) (*apigrpc.DecreaseTargetSizeReply, error) {
	glog.V(2).Infof("Call server DecreaseTargetSize: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	nodeGroup := s.nodeGroups[request.GetNodeGroupID()]

	if nodeGroup == nil {
		glog.Errorf(errNodeGroupNotFound, request.GetNodeGroupID())

		return &apigrpc.DecreaseTargetSizeReply{
			Error: &apigrpc.Error{
				Code:   cloudProviderError,
				Reason: fmt.Sprintf(errNodeGroupNotFound, request.GetNodeGroupID()),
			},
		}, nil
	}

	if request.GetDelta() >= 0 {
		glog.Errorf(errDecreaseSizeMustBeNegative)

		return &apigrpc.DecreaseTargetSizeReply{
			Error: &apigrpc.Error{
				Code:   cloudProviderError,
				Reason: errDecreaseSizeMustBeNegative,
			},
		}, nil
	}

	newSize := nodeGroup.targetSize() + int(request.GetDelta())

	if newSize < len(nodeGroup.nodes) {
		glog.Errorf(errDecreaseSizeAttemptDeleteNodes, nodeGroup.targetSize(), request.GetDelta(), newSize)

		return &apigrpc.DecreaseTargetSizeReply{
			Error: &apigrpc.Error{
				Code:   cloudProviderError,
				Reason: fmt.Sprintf(errDecreaseSizeAttemptDeleteNodes, nodeGroup.targetSize(), request.GetDelta(), newSize),
			},
		}, nil
	}

	extras := &nodeCreationExtra{
		s.kubeAdmConfig.KubeAdmAddress,
		s.kubeAdmConfig.KubeAdmToken,
		s.kubeAdmConfig.KubeAdmCACert,
		s.kubeAdmConfig.KubeAdmExtraArguments,
		s.config.Image,
		&s.config.CloudInit,
		&s.config.MountPoints,
		s.config.AutoProvision,
	}

	err := nodeGroup.setNodeGroupSize(newSize, extras)

	if err != nil {
		return &apigrpc.DecreaseTargetSizeReply{
			Error: &apigrpc.Error{
				Code:   cloudProviderError,
				Reason: err.Error(),
			},
		}, nil
	}

	return &apigrpc.DecreaseTargetSizeReply{
		Error: nil,
	}, nil
}

// Id returns an unique identifier of the node group.
func (s *MultipassServer) Id(ctx context.Context, request *apigrpc.NodeGroupServiceRequest) (*apigrpc.IdReply, error) {
	glog.V(2).Infof("Call server Id: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	nodeGroup := s.nodeGroups[request.GetNodeGroupID()]

	if nodeGroup == nil {
		glog.Errorf(errNodeGroupNotFound, request.GetNodeGroupID())

		return nil, fmt.Errorf(errNodeGroupNotFound, request.GetNodeGroupID())
	}

	return &apigrpc.IdReply{
		Response: nodeGroup.identifier,
	}, nil
}

// Debug returns a string containing all information regarding this node group.
func (s *MultipassServer) Debug(ctx context.Context, request *apigrpc.NodeGroupServiceRequest) (*apigrpc.DebugReply, error) {
	glog.V(2).Infof("Call server Debug: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	nodeGroup := s.nodeGroups[request.GetNodeGroupID()]

	if nodeGroup == nil {
		glog.Errorf(errNodeGroupNotFound, request.GetNodeGroupID())

		return nil, fmt.Errorf(errNodeGroupNotFound, request.GetNodeGroupID())
	}

	return &apigrpc.DebugReply{
		Response: fmt.Sprintf("%s-%s", request.GetProviderID(), nodeGroup.identifier),
	}, nil
}

// Nodes returns a list of all nodes that belong to this node group.
// It is required that Instance objects returned by this method have Id field set.
// Other fields are optional.
func (s *MultipassServer) Nodes(ctx context.Context, request *apigrpc.NodeGroupServiceRequest) (*apigrpc.NodesReply, error) {
	glog.V(2).Infof("Call server Nodes: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	nodeGroup := s.nodeGroups[request.GetNodeGroupID()]

	if nodeGroup == nil {
		glog.Errorf(errNodeGroupNotFound, request.GetNodeGroupID())

		return &apigrpc.NodesReply{
			Response: &apigrpc.NodesReply_Error{
				Error: &apigrpc.Error{
					Code:   cloudProviderError,
					Reason: fmt.Sprintf(errNodeGroupNotFound, request.GetNodeGroupID()),
				},
			},
		}, nil
	}

	instances := make([]*apigrpc.Instance, 0, len(nodeGroup.nodes))

	for nodeName, node := range nodeGroup.nodes {
		instances = append(instances, &apigrpc.Instance{
			Id: nodeGroup.providerIDForNode(s.config.ProviderID, nodeName),
			Status: &apigrpc.InstanceStatus{
				State:     apigrpc.InstanceState(node.state),
				ErrorInfo: nil,
			},
		})
	}

	return &apigrpc.NodesReply{
		Response: &apigrpc.NodesReply_Instances{
			Instances: &apigrpc.Instances{
				Items: instances,
			},
		},
	}, nil
}

// TemplateNodeInfo returns a schedulercache.NodeInfo structure of an empty
// (as if just started) node. This will be used in scale-up simulations to
// predict what would a new node look like if a node group was expanded. The returned
// NodeInfo is expected to have a fully populated Node object, with all of the labels,
// capacity and allocatable information as well as all pods that are started on
// the node by default, using manifest (most likely only kube-proxy). Implementation optional.
func (s *MultipassServer) TemplateNodeInfo(ctx context.Context, request *apigrpc.NodeGroupServiceRequest) (*apigrpc.TemplateNodeInfoReply, error) {
	glog.V(2).Infof("Call server TemplateNodeInfo: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	nodeGroup := s.nodeGroups[request.GetNodeGroupID()]

	if nodeGroup == nil {
		glog.Errorf(errNodeGroupNotFound, request.GetNodeGroupID())

		return &apigrpc.TemplateNodeInfoReply{
			Response: &apigrpc.TemplateNodeInfoReply_Error{
				Error: &apigrpc.Error{
					Code:   cloudProviderError,
					Reason: fmt.Sprintf(errNodeGroupNotFound, request.GetNodeGroupID()),
				},
			},
		}, nil
	}

	node := &apiv1.Node{
		Spec: apiv1.NodeSpec{
			ProviderID:    nodeGroup.providerID(s.config.ProviderID),
			Unschedulable: false,
		},
	}

	return &apigrpc.TemplateNodeInfoReply{
		Response: &apigrpc.TemplateNodeInfoReply_NodeInfo{NodeInfo: &apigrpc.NodeInfo{
			Node: toJSON(node),
		}},
	}, nil
}

// Exist checks if the node group really exists on the cloud provider side. Allows to tell the
// theoretical node group from the real one. Implementation required.
func (s *MultipassServer) Exist(ctx context.Context, request *apigrpc.NodeGroupServiceRequest) (*apigrpc.ExistReply, error) {
	glog.V(2).Infof("Call server Exist: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	nodeGroup := s.nodeGroups[request.GetNodeGroupID()]

	return &apigrpc.ExistReply{
		Exists: nodeGroup != nil,
	}, nil
}

// Create creates the node group on the cloud provider side. Implementation optional.
func (s *MultipassServer) Create(ctx context.Context, request *apigrpc.NodeGroupServiceRequest) (*apigrpc.CreateReply, error) {
	glog.V(2).Infof("Call server Create: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	nodeGroup, err := s.createNodeGroup(request.GetNodeGroupID())

	if err != nil {
		glog.Errorf(errUnableToCreateNodeGroup, request.GetNodeGroupID(), err)

		return &apigrpc.CreateReply{
			Response: &apigrpc.CreateReply_Error{
				Error: &apigrpc.Error{
					Code:   cloudProviderError,
					Reason: err.Error(),
				},
			},
		}, nil
	}

	return &apigrpc.CreateReply{
		Response: &apigrpc.CreateReply_NodeGroup{
			NodeGroup: &apigrpc.NodeGroup{
				Id: nodeGroup.identifier,
			},
		},
	}, nil
}

// Delete deletes the node group on the cloud provider side.
// This will be executed only for autoprovisioned node groups, once their size drops to 0.
// Implementation optional.
func (s *MultipassServer) Delete(ctx context.Context, request *apigrpc.NodeGroupServiceRequest) (*apigrpc.DeleteReply, error) {
	glog.V(2).Infof("Call server Delete: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	err := s.deleteNodeGroup(request.GetNodeGroupID())

	if err != nil {
		glog.Errorf(errUnableToDeleteNodeGroup, request.GetNodeGroupID(), err)
		return &apigrpc.DeleteReply{
			Error: &apigrpc.Error{
				Code:   cloudProviderError,
				Reason: err.Error(),
			},
		}, nil
	}

	return &apigrpc.DeleteReply{
		Error: nil,
	}, nil
}

// Autoprovisioned returns true if the node group is autoprovisioned. An autoprovisioned group
// was created by CA and can be deleted when scaled to 0.
func (s *MultipassServer) Autoprovisioned(ctx context.Context, request *apigrpc.NodeGroupServiceRequest) (*apigrpc.AutoprovisionedReply, error) {
	glog.V(2).Infof("Call server Autoprovisioned: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	return &apigrpc.AutoprovisionedReply{
		Autoprovisioned: true,
	}, nil
}

// Belongs returns true if the given node belongs to the NodeGroup.
func (s *MultipassServer) Belongs(ctx context.Context, request *apigrpc.BelongsRequest) (*apigrpc.BelongsReply, error) {
	glog.V(2).Infof("Call server Belongs: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	node, err := nodeFromJSON(request.GetNode())

	if err != nil {
		glog.Errorf(errCantUnmarshallNodeWithReason, request.GetNode(), err)

		return &apigrpc.BelongsReply{
			Response: &apigrpc.BelongsReply_Error{
				Error: &apigrpc.Error{
					Code:   cloudProviderError,
					Reason: err.Error(),
				},
			},
		}, nil
	}

	nodeGroup, err := s.nodeGroupForNode(node.Spec.ProviderID)

	var belong bool

	if nodeGroup != nil {
		if nodeGroup.identifier == request.GetNodeGroupID() {
			nodeName, err := nodeNameFromProviderID(s.config.ProviderID, node.Spec.ProviderID)

			if err != nil {
				return &apigrpc.BelongsReply{
					Response: &apigrpc.BelongsReply_Error{
						Error: &apigrpc.Error{
							Code:   cloudProviderError,
							Reason: err.Error(),
						},
					},
				}, nil
			}

			belong = nodeGroup.nodes[nodeName] != nil
		}
	}

	return &apigrpc.BelongsReply{
		Response: &apigrpc.BelongsReply_Belongs{
			Belongs: belong,
		},
	}, nil
}

// NodePrice returns a price of running the given node for a given period of time.
// All prices returned by the structure should be in the same currency.
func (s *MultipassServer) NodePrice(ctx context.Context, request *apigrpc.NodePriceRequest) (*apigrpc.NodePriceReply, error) {
	glog.V(2).Infof("Call server NodePrice: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	return &apigrpc.NodePriceReply{
		Response: &apigrpc.NodePriceReply_Price{
			Price: s.config.NodePrice,
		},
	}, nil
}

// PodPrice returns a theoretical minimum price of running a pod for a given
// period of time on a perfectly matching machine.
func (s *MultipassServer) PodPrice(ctx context.Context, request *apigrpc.PodPriceRequest) (*apigrpc.PodPriceReply, error) {
	glog.V(2).Infof("Call server PodPrice: %v", request)

	if request.GetProviderID() != s.config.ProviderID {
		glog.Errorf(errMismatchingProvider)
		return nil, fmt.Errorf(errMismatchingProvider)
	}

	return &apigrpc.PodPriceReply{
		Response: &apigrpc.PodPriceReply_Price{
			Price: s.config.PodPrice,
		},
	}, nil
}
