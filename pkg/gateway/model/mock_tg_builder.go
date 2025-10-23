package model

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2/k8s"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type MockTargetGroupBuilder struct {
	tgs      []*elbv2model.TargetGroup
	buildErr error
}

func (m *MockTargetGroupBuilder) buildTargetGroup(stack core.Stack, gw *gwv1.Gateway, lbConfig elbv2gw.LoadBalancerConfiguration, lbIPType elbv2model.IPAddressType, routeDescriptor routeutils.RouteDescriptor, backend routeutils.Backend, backendSGIDToken core.StringToken) (core.StringToken, error) {
	var tg *elbv2model.TargetGroup

	if len(m.tgs) > 0 {
		tg = m.tgs[0]
		m.tgs = m.tgs[1:]
	}

	var arn core.StringToken
	if tg != nil {
		arn = tg.TargetGroupARN()
	}
	return arn, m.buildErr
}

func (m *MockTargetGroupBuilder) buildTargetGroupSpec(gw *gwv1.Gateway, route routeutils.RouteDescriptor, lbConfig elbv2gw.LoadBalancerConfiguration, lbIPType elbv2model.IPAddressType, backend routeutils.Backend, targetGroupProps *elbv2gw.TargetGroupProps) (elbv2model.TargetGroupSpec, error) {
	//TODO implement me
	panic("implement me")
}

func (m *MockTargetGroupBuilder) buildTargetGroupBindingSpec(gw *gwv1.Gateway, tgProps *elbv2gw.TargetGroupProps, tgSpec elbv2model.TargetGroupSpec, nodeSelector *metav1.LabelSelector, backend routeutils.Backend, backendSGIDToken core.StringToken) k8s.TargetGroupBindingResourceSpec {
	//TODO implement me
	panic("implement me")
}

var _ targetGroupBuilder = &MockTargetGroupBuilder{}
