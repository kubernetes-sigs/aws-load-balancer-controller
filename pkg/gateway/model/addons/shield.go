package addons

import (
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	shieldmodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/shield"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
)

type shield struct {
	enabled bool
}

func (w *shield) AddToStack(stack core.Stack, lbARN core.StringToken) {
	shieldmodel.NewProtection(stack, shared_constants.ResourceIDLoadBalancer, shieldmodel.ProtectionSpec{
		Enabled:     w.enabled,
		ResourceARN: lbARN,
	})
}

var _ PreStackAddon = &shield{}

func makeShieldPrestack(enabled bool) PreStackAddon {
	return &shield{
		enabled: enabled,
	}
}
