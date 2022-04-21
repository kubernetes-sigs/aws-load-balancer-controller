package ec2

import (
	"context"

	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

var _ core.Resource = &VPCEndpointService{}

// VPCEndpointService represents a VPC Endpoint Service.
type VPCEndpointService struct {
	core.ResourceMeta `json:"-"`

	//  desired state of VPCEndpointService
	Spec VPCEndpointServiceSpec `json:"spec"`

	// observed state of VPCEndpointService
	Status *VPCEndpointServiceStatus `json:"status,omitempty"`
}

// NewVPCEndpointService constructs new VPCEndpointService resource.
func NewVPCEndpointService(stack core.Stack, id string, spec VPCEndpointServiceSpec) *VPCEndpointService {
	es := &VPCEndpointService{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::EC2::VPCEndpointService", id),
		Spec:         spec,
		Status:       nil,
	}
	stack.AddResource(es)
	return es
}

// SetStatus sets the VPCEndpointService's status
func (es *VPCEndpointService) SetStatus(status VPCEndpointServiceStatus) {
	es.Status = &status
}

// ServiceID returns a token for this VPCEndpointService's serviceID.
func (es *VPCEndpointService) ServiceID() core.StringToken {
	return core.NewResourceFieldStringToken(es, "status/serviceID",
		func(ctx context.Context, res core.Resource, fieldPath string) (s string, err error) {
			es := res.(*VPCEndpointService)
			if es.Status == nil {
				return "", errors.Errorf("VPCEndpointService is not fulfilled yet: %v", es.ID())
			}
			return es.Status.ServiceID, nil
		},
	)
}

// VPCEndpointServiceSpec defines the desired state of VPCEndpointService
type VPCEndpointServiceSpec struct {
	// whether requests from service consumers to create an endpoint to the service must be accepted
	AcceptanceRequired *bool `json:"acceptanceRequired"`

	NetworkLoadBalancerArns []core.StringToken `json:"networkLoadBalancerArns"`

	PrivateDNSName *string `json:"privateDnsName"`

	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// VPCEndpointServiceStatus defines the observed state of VPCEndpointService
type VPCEndpointServiceStatus struct {
	// The ID of the endpoint service.
	ServiceID string `json:"serviceID"`

	BaseEndpointDnsNames []string `json:"baseEndpointDnsNames"`
}

type VPCEndpointServicePermissions struct {
	core.ResourceMeta `json:"-"`
	Spec              VPCEndpointServicePermissionsSpec `json:"spec"`
}

// NewVPCEndpointService constructs new VPCEndpointServicePermissions resource.
func NewVPCEndpointServicePermissions(stack core.Stack, id string, spec VPCEndpointServicePermissionsSpec) *VPCEndpointServicePermissions {
	esPermissions := &VPCEndpointServicePermissions{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::EC2::VPCEndpointServicePermissions", id),
		Spec:         spec,
	}
	stack.AddResource(esPermissions)
	return esPermissions
}

type VPCEndpointServicePermissionsSpec struct {
	AllowedPrinciples []string         `json:"allowedPrinciples"`
	ServiceId         core.StringToken `json:"serviceID"`
}
