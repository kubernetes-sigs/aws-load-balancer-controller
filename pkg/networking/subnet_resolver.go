package networking

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sort"
)

const (
	TagKeySubnetInternalELB = "kubernetes.io/role/internal-elb"
	TagKeySubnetPublicELB   = "kubernetes.io/role/elb"
)

type SubnetsResolver interface {
	DiscoverSubnets(ctx context.Context, scheme elbv2.LoadBalancerScheme) ([]*ec2.Subnet, error)
}

type subnetsResolver struct {
	ec2Client   services.EC2
	vpcID       string
	logger      logr.Logger
	clusterName string
}

var _ SubnetsResolver = &subnetsResolver{}

func NewSubnetsResolver(ec2Client services.EC2, vpcID string, clusterName string, logger logr.Logger) SubnetsResolver {
	return &subnetsResolver{
		clusterName: clusterName,
		ec2Client:   ec2Client,
		vpcID:       vpcID,
		logger:      logger,
	}
}

func (r *subnetsResolver) DiscoverSubnets(ctx context.Context, scheme elbv2.LoadBalancerScheme) ([]*ec2.Subnet, error) {
	subnetRoleTagKey := ""
	switch scheme {
	case elbv2.LoadBalancerSchemeInternal:
		subnetRoleTagKey = TagKeySubnetInternalELB
	case elbv2.LoadBalancerSchemeInternetFacing:
		subnetRoleTagKey = TagKeySubnetPublicELB
	}
	clusterResourceTagKey := fmt.Sprintf("kubernetes.io/cluster/%s", r.clusterName)

	req := &ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{
		{
			Name:   aws.String("tag:" + clusterResourceTagKey),
			Values: aws.StringSlice([]string{"owned", "shared"}),
		},
		{
			Name:   aws.String("tag:" + subnetRoleTagKey),
			Values: aws.StringSlice([]string{"", "1"}),
		},
		{
			Name:   aws.String("vpc-id"),
			Values: aws.StringSlice([]string{r.vpcID}),
		},
	}}
	subnets, err := r.ec2Client.DescribeSubnetsAsList(ctx, req)
	if err != nil {
		return nil, err
	}
	subnetsByAZ := make(map[string][]*ec2.Subnet)
	for _, subnet := range subnets {
		subnetAZ := aws.StringValue(subnet.AvailabilityZone)
		subnetsByAZ[subnetAZ] = append(subnetsByAZ[subnetAZ], subnet)
	}
	chosenSubnets := make([]*ec2.Subnet, 0, len(subnetsByAZ))
	for az, subnets := range subnetsByAZ {
		if len(subnets) == 1 {
			chosenSubnets = append(chosenSubnets, subnets[0])
		} else if len(subnets) > 1 {
			sort.Slice(subnets, func(i, j int) bool {
				return aws.StringValue(subnets[i].SubnetId) < aws.StringValue(subnets[j].SubnetId)
			})
			r.logger.Info("multiple subnet in the same AvailabilityZone", "AvailabilityZone", az,
				"chosen", subnets[0].SubnetId, "ignored", subnets[1:])
			chosenSubnets = append(chosenSubnets, subnets[0])
		}
	}
	return chosenSubnets, nil
}
