package nlb

import (
	"context"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/networking"
)

type mockSubnetsReoslver struct {
	subnets []string
}

var _ networking.SubnetsResolver = &mockSubnetsReoslver{}

func (m *mockSubnetsReoslver) DiscoverSubnets(ctx context.Context, scheme elbv2.LoadBalancerScheme) ([]string, error) {
	return m.subnets, nil
}

func NewMockSubnetsResolver(subnets []string) networking.SubnetsResolver {
	return &mockSubnetsReoslver{subnets: subnets}
}
