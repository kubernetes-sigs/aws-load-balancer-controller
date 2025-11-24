package aga

import (
	"context"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

const (
	// ResourceTypeListener is the resource type for Global Accelerator Listener
	ResourceTypeListener = "AWS::GlobalAccelerator::Listener"
)

var _ core.Resource = &Listener{}

// Listener represents an AWS Global Accelerator Listener.
type Listener struct {
	core.ResourceMeta `json:"-"`

	// desired state of Listener
	Spec ListenerSpec `json:"spec"`

	// observed state of Listener
	// +optional
	Status *ListenerStatus `json:"status,omitempty"`

	// reference to Accelerator resource
	Accelerator *Accelerator `json:"-"`
}

// NewListener constructs new Listener resource.
func NewListener(stack core.Stack, id string, spec ListenerSpec, accelerator *Accelerator) *Listener {
	listener := &Listener{
		ResourceMeta: core.NewResourceMeta(stack, ResourceTypeListener, id),
		Spec:         spec,
		Status:       nil,
		Accelerator:  accelerator,
	}
	stack.AddResource(listener)
	listener.registerDependencies(stack)
	return listener
}

// SetStatus sets the Listener's status
func (l *Listener) SetStatus(status ListenerStatus) {
	l.Status = &status
}

// ListenerARN returns The Amazon Resource Name (ARN) of the listener.
func (l *Listener) ListenerARN() core.StringToken {
	return core.NewResourceFieldStringToken(l, "status/listenerARN",
		func(ctx context.Context, res core.Resource, fieldPath string) (s string, err error) {
			listener := res.(*Listener)
			if listener.Status == nil {
				return "", errors.Errorf("Listener is not fulfilled yet: %v", listener.ID())
			}
			return listener.Status.ListenerARN, nil
		},
	)
}

// register dependencies for Listener.
func (l *Listener) registerDependencies(stack core.Stack) {
	// Listener depends on its Accelerator
	stack.AddDependency(l, l.Accelerator)
}

type Protocol string

const (
	ProtocolTCP Protocol = "TCP"
	ProtocolUDP Protocol = "UDP"
)

type ClientAffinity string

const (
	ClientAffinitySourceIP ClientAffinity = "SOURCE_IP"
	ClientAffinityNone     ClientAffinity = "NONE"
)

// PortRange defines the port range for Global Accelerator listeners.
type PortRange struct {
	// FromPort is the first port in the range of ports, inclusive.
	FromPort int32 `json:"fromPort"`

	// ToPort is the last port in the range of ports, inclusive.
	ToPort int32 `json:"toPort"`
}

// ListenerSpec defines the desired state of Listener
type ListenerSpec struct {
	// AcceleratorARN is the ARN of the accelerator to which the listener belongs
	AcceleratorARN core.StringToken `json:"acceleratorARN"`

	// Protocol is the protocol for the connections from clients to the accelerator.
	Protocol Protocol `json:"protocol"`

	// PortRanges is the list of port ranges for the connections from clients to the accelerator.
	PortRanges []PortRange `json:"portRanges"`

	// ClientAffinity determines how to direct all requests from a specific client to the same endpoint
	// +optional
	ClientAffinity ClientAffinity `json:"clientAffinity,omitempty"`
}

// ListenerStatus defines the observed state of Listener
type ListenerStatus struct {
	// ListenerARN is the Amazon Resource Name (ARN) of the listener.
	ListenerARN string `json:"listenerARN"`
}
