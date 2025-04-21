package model

import elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"

type mockTagHelper struct {
	tags map[string]string
	err  error
}

func (m *mockTagHelper) getGatewayTags(lbConf *elbv2gw.LoadBalancerConfiguration) (map[string]string, error) {
	return m.tags, m.err
}

var _ tagHelper = &mockTagHelper{}
