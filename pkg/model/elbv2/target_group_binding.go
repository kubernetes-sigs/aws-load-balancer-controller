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
	spec TargetGroupBindingResourceSpec `json:"spec"`

	// observed state of TargetGroupBindingResource
	// +optional
	status *TargetGroupBindingResourceStatus `json:"status,omitempty"`
}

// NewTargetGroupBindingResource constructs new TargetGroupBindingResource resource.
func NewTargetGroupBindingResource(stack core.Stack, id string, spec TargetGroupBindingResourceSpec) *TargetGroupBindingResource {
	tgb := &TargetGroupBindingResource{
		id:     id,
		spec:   spec,
		status: nil,
	}
	stack.AddResource(tgb)
	tgb.registerDependencies(stack)
	return tgb
}

// ID returns resource's ID within stack.
func (tgb *TargetGroupBindingResource) ID() string {
	return tgb.id
}

// register dependencies for TargetGroupBindingResource.
func (tgb *TargetGroupBindingResource) registerDependencies(stack core.Stack) {
	for _, dep := range tgb.spec.TargetGroupARN.Dependencies() {
		stack.AddDependency(tgb, dep)
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
	// +optional
	TargetGroupBindingRef *corev1.ObjectReference `json:"targetGroupBindingRef,omitempty"`
}
