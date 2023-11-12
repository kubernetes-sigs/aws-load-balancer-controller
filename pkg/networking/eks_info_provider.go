package networking

import (
	"context"
	"net/netip"

	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/eks"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

// EKSInfoResolver is reponsible for returning information about the EKS cluster
type EKSInfoResolver interface {
	// ListCIDRs returns a list of CIDRS assocaited to a cluster
	ListCIDRs(ctx context.Context) ([]netip.Prefix, error)

	// ListSubnetIDs returns a list of subnet IDs associated with a cluster
	ListSubnetIDs(ctx context.Context) ([]*string, error)
}

func NewDefaultEKSInfoResolver(eksClient services.EKS, ec2Client services.EC2, clusterName string) *defaultEKSInfoResolver {
	return &defaultEKSInfoResolver{
		eksClient:   eksClient,
		ec2Client:   ec2Client,
		clusterName: clusterName,
	}
}

type defaultEKSInfoResolver struct {
	eksClient   services.EKS
	ec2Client   services.EC2
	clusterName string
}

func (c *defaultEKSInfoResolver) ListSubnetIDs(ctx context.Context) ([]*string, error) {
	input := &eks.DescribeClusterInput{
		Name: awssdk.String(c.clusterName),
	}
	result, err := c.eksClient.DescribeClusterWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	subnetIDs := result.Cluster.ResourcesVpcConfig.SubnetIds
	return subnetIDs, nil
}

func (c *defaultEKSInfoResolver) ListCIDRs(ctx context.Context) ([]netip.Prefix, error) {
	subnetIDs, err := c.ListSubnetIDs(ctx)
	if err != nil {
		return nil, err
	}
	input := &ec2sdk.DescribeSubnetsInput{
		SubnetIds: subnetIDs,
	}
	output, err := c.ec2Client.DescribeSubnets(input)
	if err != nil {
		return nil, err
	}
	var CIDRStrings []string
	for _, subnet := range output.Subnets {
		CIDRStrings = append(CIDRStrings, *subnet.CidrBlock)
	}
	CIDRs, err := ParseCIDRs(CIDRStrings)
	if err != nil {
		return nil, err
	}
	return CIDRs, nil
}
