package elbv2

import (
	"context"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

var _ core.Resource = &LoadBalancer{}

// LoadBalancer represents a ELBV2 LoadBalancer.
type LoadBalancer struct {
	core.ResourceMeta `json:"-"`

	// desired state of LoadBalancer
	Spec LoadBalancerSpec `json:"spec"`

	// observed state of LoadBalancer
	// +optional
	Status *LoadBalancerStatus `json:"status,omitempty"`
}

// NewLoadBalancer constructs new LoadBalancer resource.
func NewLoadBalancer(stack core.Stack, id string, spec LoadBalancerSpec) *LoadBalancer {
	lb := &LoadBalancer{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", id),
		Spec:         spec,
		Status:       nil,
	}
	stack.AddResource(lb)
	lb.registerDependencies(stack)
	return lb
}

// SetStatus sets the LoadBalancer's status
func (lb *LoadBalancer) SetStatus(status LoadBalancerStatus) {
	lb.Status = &status
}

// LoadBalancerARN returns The Amazon Resource Name (ARN) of the load balancer.
func (lb *LoadBalancer) LoadBalancerARN() core.StringToken {
	return core.NewResourceFieldStringToken(lb, "status/loadBalancerARN",
		func(ctx context.Context, res core.Resource, fieldPath string) (s string, err error) {
			lb := res.(*LoadBalancer)
			if lb.Status == nil {
				return "", errors.Errorf("LoadBalancer is not fulfilled yet: %v", lb.ID())
			}
			return lb.Status.LoadBalancerARN, nil
		},
	)
}

// DNSName returns The public DNS name of the load balancer.
func (lb *LoadBalancer) DNSName() core.StringToken {
	return core.NewResourceFieldStringToken(lb, "status/dnsName",
		func(ctx context.Context, res core.Resource, fieldPath string) (s string, err error) {
			lb := res.(*LoadBalancer)
			if lb.Status == nil {
				return "", errors.Errorf("LoadBalancer is not fulfilled yet: %v", lb.ID())
			}
			return lb.Status.DNSName, nil
		},
	)
}

// register dependencies for LoadBalancer.
func (lb *LoadBalancer) registerDependencies(stack core.Stack) {
	for _, sgToken := range lb.Spec.SecurityGroups {
		for _, dep := range sgToken.Dependencies() {
			stack.AddDependency(dep, lb)
		}
	}
}

type LoadBalancerType string

const (
	LoadBalancerTypeApplication LoadBalancerType = "application"
	LoadBalancerTypeNetwork     LoadBalancerType = "network"
)

type IPAddressType string

const (
	IPAddressTypeIPV4      IPAddressType = "ipv4"
	IPAddressTypeDualStack IPAddressType = "dualstack"
)

type LoadBalancerScheme string

const (
	LoadBalancerSchemeInternal       LoadBalancerScheme = "internal"
	LoadBalancerSchemeInternetFacing LoadBalancerScheme = "internet-facing"
)

// Information about a subnet mapping.
type SubnetMapping struct {
	// [Network Load Balancers] The allocation ID of the Elastic IP address for
	// an internet-facing load balancer.
	AllocationID *string `json:"allocationID,omitempty"`

	// [Network Load Balancers] The private IPv4 address for an internal load balancer.
	PrivateIPv4Address *string `json:"privateIPv4Address,omitempty"`

	// The ID of the subnet.
	SubnetID string `json:"subnetID"`
}

// Information about a load balancer attribute.
type LoadBalancerAttribute struct {
	// The name of the attribute.
	Key string `json:"key"`

	// The value of the attribute.
	Value string `json:"value"`
}

// LoadBalancerSpec defines the desired state of LoadBalancer
type LoadBalancerSpec struct {
	// The name of the load balancer.
	Name string `json:"name"`

	// The type of load balancer.
	Type LoadBalancerType `json:"type"`

	// The nodes of an Internet-facing load balancer have public IP addresses.
	// The nodes of an internal load balancer have only private IP addresses.
	// +optional
	Scheme *LoadBalancerScheme `json:"scheme,omitempty"`

	// The type of IP addresses used by the subnets for your load balancer.
	// +optional
	IPAddressType *IPAddressType `json:"ipAddressType,omitempty"`

	// The IDs of the public subnets. You can specify only one subnet per Availability Zone.
	// +optional
	SubnetMappings []SubnetMapping `json:"subnetMapping,omitempty"`

	// [Application Load Balancers] The IDs of the security groups for the load balancer.
	// +optional
	SecurityGroups []core.StringToken `json:"securityGroups,omitempty"`

	// [Application Load Balancers on Outposts] The ID of the customer-owned address pool (CoIP pool).
	// +optional
	CustomerOwnedIPv4Pool *string `json:"customerOwnedIPv4Pool,omitempty"`

	// The load balancer attributes.
	// +optional
	LoadBalancerAttributes []LoadBalancerAttribute `json:"loadBalancerAttributes,omitempty"`

	// The tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// LoadBalancerStatus defines the observed state of LoadBalancer
type LoadBalancerStatus struct {
	// The Amazon Resource Name (ARN) of the load balancer.
	LoadBalancerARN string `json:"loadBalancerARN"`

	// The public DNS name of the load balancer.
	DNSName string `json:"dnsName"`
}
