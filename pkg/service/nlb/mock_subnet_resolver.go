package nlb

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/networking"
)

type mockSubnetsReoslver struct {
	subnets []string
	cirds   []string
}

var _ networking.SubnetsResolver = &mockSubnetsReoslver{}

func (m *mockSubnetsReoslver) DiscoverSubnets(ctx context.Context, scheme elbv2.LoadBalancerScheme) ([]*ec2.Subnet, error) {
	subnets := make([]*ec2.Subnet, 0, len(m.subnets))
	for idx, sn := range m.subnets {
		ec2Subnet := &ec2.Subnet{
			SubnetId:  aws.String(sn),
			CidrBlock: aws.String(m.cirds[idx]),
		}
		subnets = append(subnets, ec2Subnet)
	}
	return subnets, nil
}

func NewMockSubnetsResolver(subnets []string, cidrs []string) networking.SubnetsResolver {
	return &mockSubnetsReoslver{
		subnets: subnets,
		cirds:   cidrs,
	}
}
