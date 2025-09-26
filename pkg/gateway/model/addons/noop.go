package addons

import "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"

type noop struct{}

func (n *noop) AddToStack(_ core.Stack, _ core.StringToken) {
	// no-op
}

var _ PreStackAddon = &noop{}

func makeNoOpPrestack() PreStackAddon {
	return &noop{}
}
