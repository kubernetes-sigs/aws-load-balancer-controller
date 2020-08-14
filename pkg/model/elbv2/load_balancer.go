package elbv2

import (
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
)

var _ core.Resource = &LoadBalancer{}

// LoadBalancer represents a ELBV2 LoadBalancer.
type LoadBalancer struct {
	// resource id
	id string

	// desired state of LoadBalancer
	spec LoadBalancerSpec `json:"spec"`

	// observed state of LoadBalancer
	// +optional
	status *LoadBalancerStatus `json:"status,omitempty"`
}

// NewLoadBalancer constructs new LoadBalancer resource.
func NewLoadBalancer(stack core.Stack, id string, spec LoadBalancerSpec) *LoadBalancer {
	lb := &LoadBalancer{
		id:     id,
		spec:   spec,
		status: nil,
	}
	stack.AddResource(lb)
	lb.registerDependencies(stack)
	return lb
}

// ID returns resource's ID within stack.
func (lb *LoadBalancer) ID() string {
	return lb.id
}

func (lb *LoadBalancer) registerDependencies(stack core.Stack) {
	for _, sgToken := range lb.spec.SecurityGroups {
		for _, dep := range sgToken.Dependencies() {
			stack.AddDependency(lb, dep)
		}
	}
}

type LoadBalancerType string

const (
	LoadBalancerTypeApplication = "application"
	LoadBalancerTypeNetwork     = "network"
)

type IPAddressType string

const (
	IPAddressTypeIPV4      IPAddressType = "ipv4"
	IPAddressTypeDualStack               = "dualstack"
)

type LoadBalancerScheme string

const (
	LoadBalancerSchemeInternal       LoadBalancerScheme = "internal"
	LoadBalancerSchemeInternetFacing                    = "internet-facing"
)

// Information about a subnet mapping.
type SubnetMapping struct {
	// [Network Load Balancers] The allocation ID of the Elastic IP address for
	// an internet-facing load balancer.
	AllocationID *string `json:"allocationID,omitempty"`

	// [Network Load Balancers] The private IPv4 address for an internal load balancer.
	PrivateIPv4Address *string `json:"privateIPv4Address,omitempty"`

	// The ID of the subnet.
	SubnetID *string `json:"subnetID,omitempty"`
}

// LoadBalancerSpec defines the desired state of LoadBalancer
type LoadBalancerSpec struct {
	// The name of the load balancer.
	LoadBalancerName string `json:"loadBalancerName"`

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
	SubnetMappings []SubnetMapping `json:"scheme,omitempty"`

	// [Application Load Balancers] The IDs of the security groups for the load balancer.
	// +optional
	SecurityGroups []core.StringToken `json:"securityGroups,omitempty"`

	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// LoadBalancerStatus defines the observed state of LoadBalancer
type LoadBalancerStatus struct {
	// +optional
	LoadBalancerARN *string `json:"loadBalancerARN,omitempty"`

	// +optional
	DNSName *string `json:"dnsName,omitempty"`
}
