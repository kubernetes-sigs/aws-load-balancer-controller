package aga

import (
	"context"
	"github.com/pkg/errors"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

var _ core.Resource = &Accelerator{}

// Accelerator represents an AWS Global Accelerator.
type Accelerator struct {
	core.ResourceMeta `json:"-"`

	// desired state of Accelerator
	Spec AcceleratorSpec `json:"spec"`

	// observed state of Accelerator
	// +optional
	Status *AcceleratorStatus `json:"status,omitempty"`

	// Reference to the CRD for accessing status
	crd agaapi.GlobalAccelerator `json:"-"`
}

// NewAccelerator constructs new Accelerator resource.
func NewAccelerator(stack core.Stack, id string, spec AcceleratorSpec, crd *agaapi.GlobalAccelerator) *Accelerator {
	accelerator := &Accelerator{
		ResourceMeta: core.NewResourceMeta(stack, ResourceTypeAccelerator, id),
		Spec:         spec,
		Status:       nil,
		crd:          *crd,
	}
	stack.AddResource(accelerator)
	accelerator.registerDependencies(stack)
	return accelerator
}

// GetARNFromCRDStatus returns the ARN from the CRD status if available.
func (a *Accelerator) GetARNFromCRDStatus() string {
	if a.crd.Status.AcceleratorARN != nil {
		return *a.crd.Status.AcceleratorARN
	}
	return ""
}

// GetCRDUID returns the UID of the CRD for use as idempotency token.
func (a *Accelerator) GetCRDUID() string {
	return string(a.crd.UID)
}

// SetStatus sets the Accelerator's status
func (a *Accelerator) SetStatus(status AcceleratorStatus) {
	a.Status = &status
}

// AcceleratorARN returns The Amazon Resource Name (ARN) of the accelerator.
func (a *Accelerator) AcceleratorARN() core.StringToken {
	return core.NewResourceFieldStringToken(a, "status/acceleratorARN",
		func(ctx context.Context, res core.Resource, fieldPath string) (s string, err error) {
			accelerator := res.(*Accelerator)
			if accelerator.Status == nil {
				return "", errors.Errorf("Accelerator is not fulfilled yet: %v", accelerator.ID())
			}
			return accelerator.Status.AcceleratorARN, nil
		},
	)
}

// DNSName returns The Domain Name System (DNS) name that Global Accelerator creates.
func (a *Accelerator) DNSName() core.StringToken {
	return core.NewResourceFieldStringToken(a, "status/dnsName",
		func(ctx context.Context, res core.Resource, fieldPath string) (s string, err error) {
			accelerator := res.(*Accelerator)
			if accelerator.Status == nil {
				return "", errors.Errorf("Accelerator is not fulfilled yet: %v", accelerator.ID())
			}
			return accelerator.Status.DNSName, nil
		},
	)
}

// DualStackDNSName returns The dual-stack DNS name that Global Accelerator creates.
func (a *Accelerator) DualStackDNSName() core.StringToken {
	return core.NewResourceFieldStringToken(a, "status/dualStackDnsName",
		func(ctx context.Context, res core.Resource, fieldPath string) (s string, err error) {
			accelerator := res.(*Accelerator)
			if accelerator.Status == nil {
				return "", errors.Errorf("Accelerator is not fulfilled yet: %v", accelerator.ID())
			}
			return accelerator.Status.DualStackDNSName, nil
		},
	)
}

// register dependencies for Accelerator.
func (a *Accelerator) registerDependencies(stack core.Stack) {
	// No dependencies for accelerator itself
}

type IPAddressType string

const (
	IPAddressTypeIPV4      IPAddressType = "IPV4"
	IPAddressTypeDualStack IPAddressType = "DUAL_STACK"
)

// IPSet represents the static IP addresses that Global Accelerator associates with the accelerator.
type IPSet struct {
	// IpAddresses is the array of IP addresses in the IP address set.
	IpAddresses []string `json:"ipAddresses,omitempty"`

	// IpAddressFamily is the types of IP addresses included in this IP set.
	IpAddressFamily string `json:"ipAddressFamily,omitempty"`
}

// AcceleratorSpec defines the desired state of Accelerator
type AcceleratorSpec struct {
	// Name is the name of the Global Accelerator.
	Name string `json:"name"`

	// IpAddresses optionally specifies the IP addresses from your own IP address pool (BYOIP).
	// +optional
	IpAddresses []string `json:"ipAddresses,omitempty"`

	// IPAddressType is the value for the address type.
	// +optional
	IPAddressType IPAddressType `json:"ipAddressType,omitempty"`

	// Enabled indicates whether the accelerator is enabled.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Tags defines list of Tags on the Global Accelerator.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// AcceleratorStatus defines the observed state of Accelerator
type AcceleratorStatus struct {
	// AcceleratorARN is the Amazon Resource Name (ARN) of the accelerator.
	AcceleratorARN string `json:"acceleratorARN"`

	// DNSName The Domain Name System (DNS) name that Global Accelerator creates.
	DNSName string `json:"dnsName"`

	// DualStackDNSName is the Domain Name System (DNS) name for dual-stack accelerator.
	DualStackDNSName string `json:"dualStackDnsName,omitempty"`

	// IPSets is the static IP addresses that Global Accelerator associates with the accelerator.
	IPSets []IPSet `json:"ipSets,omitempty"`

	// Status is the current status of the accelerator.
	Status string `json:"status,omitempty"`
}
