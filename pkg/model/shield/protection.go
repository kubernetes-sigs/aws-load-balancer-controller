package shield

import "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"

type Protection struct {
	core.ResourceMeta `json:"-"`

	// desired state of Protection
	Spec ProtectionSpec `json:"spec"`
}

// NewProtection constructs new Protection resource.
func NewProtection(stack core.Stack, id string, spec ProtectionSpec) *Protection {
	p := &Protection{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::Shield::Protection", id),
		Spec:         spec,
	}
	stack.AddResource(p)
	p.registerDependencies(stack)
	return p
}

// register dependencies for Protection.
func (p *Protection) registerDependencies(stack core.Stack) {
	for _, dep := range p.Spec.ResourceARN.Dependencies() {
		stack.AddDependency(dep, p)
	}
}

// ProtectionSpec defines the desired state of Protection.
type ProtectionSpec struct {
	ResourceARN core.StringToken `json:"resourceARN"`
}
