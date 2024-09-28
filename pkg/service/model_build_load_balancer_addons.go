package service

import (
	"context"

	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	shieldmodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/shield"
)

func (t *defaultModelBuildTask) buildLoadBalancerAddOns(ctx context.Context, lbARN core.StringToken) error {
	if _, err := t.buildShieldProtection(ctx, lbARN); err != nil {
		return err
	}
	return nil
}

func (t *defaultModelBuildTask) buildShieldProtection(_ context.Context, lbARN core.StringToken) (*shieldmodel.Protection, error) {
	explicitEnableProtections := make(map[bool]struct{})
	rawEnableProtection := false
	exists, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixShieldAdvancedProtection, &rawEnableProtection, t.service.Annotations)
	if err != nil {
		return nil, err
	}
	if exists {
		explicitEnableProtections[rawEnableProtection] = struct{}{}
	}
	if len(explicitEnableProtections) == 0 {
		return nil, nil
	}
	_, enableProtection := explicitEnableProtections[true]
	protection := shieldmodel.NewProtection(t.stack, resourceIDLoadBalancer, shieldmodel.ProtectionSpec{
		Enabled:     enableProtection,
		ResourceARN: lbARN,
	})
	return protection, nil
}
