package subnet

import (
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

type Mutator interface {
	Mutate(elbSubnets []*elbv2model.SubnetMapping, ec2Subnets []ec2types.Subnet, subnetConfig []elbv2gw.SubnetConfiguration) error
}
