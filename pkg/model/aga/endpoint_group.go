package aga

import (
	"context"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

const (
	// ResourceTypeEndpointGroup is the resource type for Global Accelerator EndpointGroup
	ResourceTypeEndpointGroup = "AWS::GlobalAccelerator::EndpointGroup"
)

var _ core.Resource = &EndpointGroup{}

// EndpointGroup represents an AWS Global Accelerator EndpointGroup.
type EndpointGroup struct {
	core.ResourceMeta `json:"-"`

	// desired state of EndpointGroup
	Spec EndpointGroupSpec `json:"spec"`

	// observed state of EndpointGroup
	// +optional
	Status *EndpointGroupStatus `json:"status,omitempty"`

	// reference to Listener resource
	Listener *Listener `json:"-"`
}

// NewEndpointGroup constructs new EndpointGroup resource.
func NewEndpointGroup(stack core.Stack, id string, spec EndpointGroupSpec, listener *Listener) *EndpointGroup {
	endpointGroup := &EndpointGroup{
		ResourceMeta: core.NewResourceMeta(stack, ResourceTypeEndpointGroup, id),
		Spec:         spec,
		Status:       nil,
		Listener:     listener,
	}
	stack.AddResource(endpointGroup)
	endpointGroup.registerDependencies(stack)
	return endpointGroup
}

// SetStatus sets the EndpointGroup's status
func (eg *EndpointGroup) SetStatus(status EndpointGroupStatus) {
	eg.Status = &status
}

// EndpointGroupARN returns The Amazon Resource Name (ARN) of the endpoint group.
func (eg *EndpointGroup) EndpointGroupARN() core.StringToken {
	return core.NewResourceFieldStringToken(eg, "status/endpointGroupARN",
		func(ctx context.Context, res core.Resource, fieldPath string) (s string, err error) {
			endpointGroup := res.(*EndpointGroup)
			if endpointGroup.Status == nil {
				return "", errors.Errorf("EndpointGroup is not fulfilled yet: %v", endpointGroup.ID())
			}
			return endpointGroup.Status.EndpointGroupARN, nil
		},
	)
}

// register dependencies for EndpointGroup.
func (eg *EndpointGroup) registerDependencies(stack core.Stack) {
	// EndpointGroup depends on its Listener
	stack.AddDependency(eg, eg.Listener)
}

// PortOverride defines the port override for Global Accelerator endpoint groups.
type PortOverride struct {
	// ListenerPort is the listener port that you want to map to a specific endpoint port.
	ListenerPort int32 `json:"listenerPort"`

	// EndpointPort is the endpoint port that you want traffic to be routed to.
	EndpointPort int32 `json:"endpointPort"`
}

// EndpointGroupSpec defines the desired state of EndpointGroup
type EndpointGroupSpec struct {
	// ListenerARN is the ARN of the listener for the endpoint group
	ListenerARN core.StringToken `json:"listenerARN"`

	// Region is the AWS Region where the endpoint group is located.
	Region string `json:"region"`

	// TrafficDialPercentage is the percentage of traffic to send to an AWS Region.
	// +optional
	TrafficDialPercentage *int32 `json:"trafficDialPercentage,omitempty"`

	// PortOverrides is a list of endpoint port overrides.
	// +optional
	PortOverrides []PortOverride `json:"portOverrides,omitempty"`

	// EndpointConfigurations is a list of endpoint configurations for the endpoint group.
	// +optional
	// This field is not implemented in the initial version as it will be part of a separate endpoint builder.
}

// EndpointGroupStatus defines the observed state of EndpointGroup
type EndpointGroupStatus struct {
	// EndpointGroupARN is the Amazon Resource Name (ARN) of the endpoint group.
	EndpointGroupARN string `json:"endpointGroupARN"`
}
