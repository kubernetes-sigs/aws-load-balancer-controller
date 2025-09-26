package addons

import (
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	wafv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/wafv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
)

type waf struct {
	webACLARN string
}

func (w *waf) AddToStack(stack core.Stack, lbARN core.StringToken) {
	wafv2model.NewWebACLAssociation(stack, shared_constants.ResourceIDLoadBalancer, wafv2model.WebACLAssociationSpec{
		WebACLARN:   w.webACLARN,
		ResourceARN: lbARN,
	})
}

var _ PreStackAddon = &waf{}

func makeWAFPrestack(webACLARN string) PreStackAddon {
	return &waf{
		webACLARN: webACLARN,
	}
}
