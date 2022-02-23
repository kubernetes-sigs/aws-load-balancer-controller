package ec2

import (
	"context"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

var _ core.Resource = &ElasticIPAddress{}

// ElasticIPAddress represents a EC2 Elastic IP Address.
type ElasticIPAddress struct {
	core.ResourceMeta `json:"-"`

	//  desired state of SecurityGroup
	Spec ElasticIPAddressSpec `json:"spec"`

	// observed state of SecurityGroup
	Status *ElasticIPAddressStatus `json:"status,omitempty"`
}

// NewElasticIPAddress constructs new ElasticIPAddress resource.
func NewElasticIPAddress(stack core.Stack, id string, spec ElasticIPAddressSpec) *ElasticIPAddress {
	eip := &ElasticIPAddress{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::EC2::EIP", id),
		Spec:         spec,
		Status:       nil,
	}
	stack.AddResource(eip)
	return eip
}

// SetStatus sets the ElasticIPAddress's status
func (eip *ElasticIPAddress) SetStatus(status ElasticIPAddressStatus) {
	eip.Status = &status
}

// AllocationID returns a token for this ElasticIPAddress's allocationID.
func (eip *ElasticIPAddress) AllocationID() core.StringToken {
	return core.NewResourceFieldStringToken(eip, "status/allocationID",
		func(ctx context.Context, res core.Resource, fieldPath string) (s string, err error) {
			eip := res.(*ElasticIPAddress)
			if eip.Status == nil {
				return "", errors.Errorf("ElasticIPAddress is not fulfilled yet: %v", eip.ID())
			}
			return eip.Status.AllocationID, nil
		},
	)
}

// ElasticIPAddressSpec defines the desired state of ElasticIPAddress
type ElasticIPAddressSpec struct {
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// +optional
	PublicIPv4PoolID string `json:"publicIPv4PoolID,omitempty"`
}

// ElasticIPAddressStatus defines the observed state of ElasticIPAddress
type ElasticIPAddressStatus struct {
	// The ID of the Elastic IP Address.
	AllocationID string `json:"allocationID"`
}
