package wafregional

import "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"

// WebACLAssociation represents a waf-region web-acl association.
type WebACLAssociation struct {
	core.ResourceMeta `json:"-"`

	// desired state of WebACLAssociation
	Spec WebACLAssociationSpec `json:"spec"`
}

// NewWebACLAssociation constructs new WebACLAssociation resource.
func NewWebACLAssociation(stack core.Stack, id string, spec WebACLAssociationSpec) *WebACLAssociation {
	a := &WebACLAssociation{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::WAFRegional::WebACLAssociation", id),
		Spec:         spec,
	}
	stack.AddResource(a)
	a.registerDependencies(stack)
	return a
}

// register dependencies for WebACLAssociation.
func (a *WebACLAssociation) registerDependencies(stack core.Stack) {
	for _, dep := range a.Spec.ResourceARN.Dependencies() {
		stack.AddDependency(dep, a)
	}
}

// WebACLAssociationSpec defines the desired state of LoadBalancer
type WebACLAssociationSpec struct {
	WebACLID    string           `json:"webACLID"`
	ResourceARN core.StringToken `json:"resourceARN"`
}
