package gateway

import (
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
)

type TargetGroupConfigConstructor interface {
	ConstructTargetGroupConfigForRoute(tgConfig *elbv2gw.TargetGroupConfiguration, name, namespace, kind string) *elbv2gw.TargetGroupProps
}

type targetGroupConfigConstructorImpl struct {
}

func (t targetGroupConfigConstructorImpl) ConstructTargetGroupConfigForRoute(tgConfig *elbv2gw.TargetGroupConfiguration, name, namespace, kind string) *elbv2gw.TargetGroupProps {
	if tgConfig == nil {
		return nil
	}
	return &tgConfig.Spec.DefaultConfiguration
}

func NewTargetGroupConfigConstructor() TargetGroupConfigConstructor {
	return &targetGroupConfigConstructorImpl{}
}
