package service

import (
	"context"

	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	shieldmodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/shield"
)

func (t *defaultModelBuildTask) buildLoadBalancerAddOns(ctx context.Context) error {
	if _, err := t.buildShieldProtection(ctx); err != nil {
		return err
	}
	return nil
}

func (t *defaultModelBuildTask) buildShieldProtection(_ context.Context) (*shieldmodel.Protection, error) {
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
	if len(explicitEnableProtections) > 1 {
		return nil, errors.New("conflicting enable shield advanced protection")
	}
	if _, enableProtection := explicitEnableProtections[true]; enableProtection {
		protection := shieldmodel.NewProtection(t.stack, resourceIDLoadBalancer, shieldmodel.ProtectionSpec{
			ResourceARN: t.loadBalancer.LoadBalancerARN(),
		})
		return protection, nil
	}
	return nil, nil
}
