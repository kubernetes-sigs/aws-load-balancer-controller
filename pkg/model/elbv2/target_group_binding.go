package elbv2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

var _ core.Resource = &TargetGroupBindingResource{}

// TargetGroupBindingResource represents an TargetGroupBinding Custom Resource.
type TargetGroupBindingResource struct {
	core.ResourceMeta `json:"-"`

	// desired state of TargetGroupBindingResource
	Spec TargetGroupBindingResourceSpec `json:"spec"`

	// observed state of TargetGroupBindingResource
	// +optional
	Status *TargetGroupBindingResourceStatus `json:"status,omitempty"`
}

// NewTargetGroupBindingResource constructs new TargetGroupBindingResource resource.
func NewTargetGroupBindingResource(stack core.Stack, id string, spec TargetGroupBindingResourceSpec) *TargetGroupBindingResource {
	tgb := &TargetGroupBindingResource{
		ResourceMeta: core.NewResourceMeta(stack, "K8S::ElasticLoadBalancingV2::TargetGroupBinding", id),
		Spec:         spec,
		Status:       nil,
	}
	stack.AddResource(tgb)
	tgb.registerDependencies(stack)
	return tgb
}

// SetStatus sets the TargetGroup's status
func (tgb *TargetGroupBindingResource) SetStatus(status TargetGroupBindingResourceStatus) {
	tgb.Status = &status
}

// register dependencies for TargetGroupBindingResource.
func (tgb *TargetGroupBindingResource) registerDependencies(stack core.Stack) {
	for _, dep := range tgb.Spec.Template.Spec.TargetGroupARN.Dependencies() {
		stack.AddDependency(dep, tgb)
	}
}

// SecurityGroup defines reference to an AWS EC2 SecurityGroup.
type SecurityGroup struct {
	// GroupID is the EC2 SecurityGroupID.
	GroupID core.StringToken `json:"groupID"`
}

// NetworkingPeer defines the source/destination peer for networking rules.
type NetworkingPeer struct {
	// IPBlock defines an IPBlock peer.
	// If specified, none of the other fields can be set.
	// +optional
	IPBlock *elbv2api.IPBlock `json:"ipBlock,omitempty"`

	// SecurityGroup defines a SecurityGroup peer.
	// If specified, none of the other fields can be set.
	// +optional
	SecurityGroup *SecurityGroup `json:"securityGroup,omitempty"`
}

type NetworkingIngressRule struct {
	// List of peers which should be able to access the targets in TargetGroup.
	// At least one NetworkingPeer should be specified.
	From []NetworkingPeer `json:"from"`

	// List of ports which should be made accessible on the targets in TargetGroup.
	// At least one NetworkingPort should be specified.
	Ports []elbv2api.NetworkingPort `json:"ports"`
}

type TargetGroupBindingNetworking struct {
	// List of ingress rules to allow ELBV2 LoadBalancer to access targets in TargetGroup.
	// +optional
	Ingress []NetworkingIngressRule `json:"ingress,omitempty"`
}

// TargetGroupBindingSpec defines the desired state of TargetGroupBinding
type TargetGroupBindingSpec struct {
	// targetGroupARN is the Amazon Resource Name (ARN) for the TargetGroup.
	TargetGroupARN core.StringToken `json:"targetGroupARN"`

	// targetType is the TargetType of TargetGroup. If unspecified, it will be automatically inferred.
	// +optional
	TargetType *elbv2api.TargetType `json:"targetType,omitempty"`

	// serviceRef is a reference to a Kubernetes Service and ServicePort.
	ServiceRef elbv2api.ServiceReference `json:"serviceRef"`

	// networking provides the networking setup for ELBV2 LoadBalancer to access targets in TargetGroup.
	// +optional
	Networking *TargetGroupBindingNetworking `json:"networking,omitempty"`

	// node selector for instance type target groups to only register certain nodes
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`

	// ipAddressType specifies whether the target group is of type IPv4 or IPv6. If unspecified, it will be automatically inferred.
	// +optional
	IPAddressType *elbv2api.TargetGroupIPAddressType `json:"ipAddressType,omitempty"`
}

// Template for TargetGroupBinding Custom Resource.
type TargetGroupBindingTemplate struct {
	// Standard object's metadata.
	metav1.ObjectMeta `json:"metadata"`

	// Specification of TargetGroupBinding Custom Resource.
	Spec TargetGroupBindingSpec `json:"spec"`
}

// desired state of TargetGroupBindingResource
type TargetGroupBindingResourceSpec struct {
	// Describes the TargetGroupBinding Custom Resource that will be created when synthesize this TargetGroupBindingResource.
	Template TargetGroupBindingTemplate `json:"template"`
}

// observed state of TargetGroupBindingResource
type TargetGroupBindingResourceStatus struct {
	// reference to the TargetGroupBinding Custom Resource.
	TargetGroupBindingRef corev1.ObjectReference `json:"targetGroupBindingRef"`
}
