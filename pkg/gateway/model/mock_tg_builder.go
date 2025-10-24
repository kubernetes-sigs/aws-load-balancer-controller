package model

import (
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	elbv2modelk8s "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2/k8s"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type mockTargetGroupBuilder struct {
	tgs      []*elbv2model.TargetGroup
	buildErr error
}

func (m *mockTargetGroupBuilder) buildTargetGroup(stack core.Stack,
	gw *gwv1.Gateway, lbConfig elbv2gw.LoadBalancerConfiguration, lbIPType elbv2model.IPAddressType, routeDescriptor routeutils.RouteDescriptor, backend routeutils.Backend) (core.StringToken, error) {
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

var _ targetGroupBuilder = &mockTargetGroupBuilder{}

type mockTargetGroupBindingNetworkingBuilder struct {
	result *elbv2modelk8s.TargetGroupBindingNetworking
	err    error
}

func (m *mockTargetGroupBindingNetworkingBuilder) buildTargetGroupBindingNetworking(targetGroupSpec elbv2model.TargetGroupSpec, targetPort intstr.IntOrString) (*elbv2modelk8s.TargetGroupBindingNetworking, error) {
	return m.result, m.err
}

var _ targetGroupBindingNetworkBuilder = &mockTargetGroupBindingNetworkingBuilder{}
