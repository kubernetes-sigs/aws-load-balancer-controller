package elbv2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2api "sigs.k8s.io/aws-alb-ingress-controller/apis/elbv2/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
)

var _ core.Resource = &TargetGroupBindingResource{}

// TargetGroupBindingResource represents an TargetGroupBinding Custom Resource.
type TargetGroupBindingResource struct {
	// resource id
	id string

	// desired state of TargetGroupBindingResource
	Spec TargetGroupBindingResourceSpec `json:"spec"`

	// observed state of TargetGroupBindingResource
	// +optional
	Status *TargetGroupBindingResourceStatus `json:"status,omitempty"`
}

// NewTargetGroupBindingResource constructs new TargetGroupBindingResource resource.
func NewTargetGroupBindingResource(stack core.Stack, id string, spec TargetGroupBindingResourceSpec) *TargetGroupBindingResource {
	tgb := &TargetGroupBindingResource{
		id:     id,
		Spec:   spec,
		Status: nil,
	}
	stack.AddResource(tgb)
	tgb.registerDependencies(stack)
	return tgb
}

// Type returns resource's Type.
func (tgb *TargetGroupBindingResource) Type() string {
	return "K8S::ElasticLoadBalancingV2::TargetGroupBinding"
}

// ID returns resource's ID within stack.
func (tgb *TargetGroupBindingResource) ID() string {
	return tgb.id
}

// SetStatus sets the TargetGroup's status
func (tgb *TargetGroupBindingResource) SetStatus(status TargetGroupBindingResourceStatus) {
	tgb.Status = &status
}

// register dependencies for TargetGroupBindingResource.
func (tgb *TargetGroupBindingResource) registerDependencies(stack core.Stack) {
	for _, dep := range tgb.Spec.TargetGroupARN.Dependencies() {
		stack.AddDependency(dep, tgb)
	}
}

// Template for TargetGroupBinding Custom Resource.
type TargetGroupBindingTemplate struct {
	// Standard object's metadata.
	metav1.ObjectMeta `json:"metadata"`

	// Specification of TargetGroupBinding Custom Resource.
	Spec elbv2api.TargetGroupBindingSpec `json:"spec"`
}

// desired state of TargetGroupBindingResource
type TargetGroupBindingResourceSpec struct {
	// TargetGroupARN is the Amazon Resource Name (ARN) for the TargetGroup.
	TargetGroupARN core.StringToken `json:"targetGroupARN"`

	// Describes the TargetGroupBinding Custom Resource that will be created when synthesize this TargetGroupBindingResource.
	Template TargetGroupBindingTemplate `json:"template"`
}

// observed state of TargetGroupBindingResource
type TargetGroupBindingResourceStatus struct {
	// reference to the TargetGroupBinding Custom Resource.
	TargetGroupBindingRef corev1.ObjectReference `json:"targetGroupBindingRef"`
}
