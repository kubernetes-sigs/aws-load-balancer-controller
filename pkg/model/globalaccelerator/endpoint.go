package globalaccelerator

import "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"

type Endpoint struct {
	core.ResourceMeta `json:"-"`

	// desired state of Endpoint
	Spec EndpointSpec `json:"spec"`
}

// NewEndpoint constructs new Endpoint resource.
func NewEndpoint(stack core.Stack, id string, spec EndpointSpec) *Endpoint {
	e := &Endpoint{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::GlobalAccelerator::Endpoint", id),
		Spec:         spec,
	}
	stack.AddResource(e)
	e.registerDependencies(stack)
	return e
}

// register dependencies for Endpoint.
func (p *Endpoint) registerDependencies(stack core.Stack) {
	for _, dep := range p.Spec.ResourceARN.Dependencies() {
		stack.AddDependency(dep, p)
	}
}

// EndpointSpec defines the desired state of Endpoint.
type EndpointSpec struct {
	EndpointGroupARN string           `json:"endpointGroupARN"`
	ResourceARN      core.StringToken `json:"resourceARN"`
	Create           bool             `json:"create"`
}
