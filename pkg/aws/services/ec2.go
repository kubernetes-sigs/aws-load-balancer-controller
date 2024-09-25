package services

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/endpoints"
)

type EC2 interface {

	// DescribeInstancesAsList wraps the DescribeInstancesPagesWithContext API, which aggregates paged results into list.
	DescribeInstancesAsList(ctx context.Context, input *ec2.DescribeInstancesInput) ([]types.Instance, error)

	// DescribeNetworkInterfacesAsList wraps the DescribeNetworkInterfacesPagesWithContext API, which aggregates paged results into list.
	DescribeNetworkInterfacesAsList(ctx context.Context, input *ec2.DescribeNetworkInterfacesInput) ([]types.NetworkInterface, error)

	// DescribeSecurityGroupsAsList wraps the DescribeSecurityGroupsPagesWithContext API, which aggregates paged results into list.
	DescribeSecurityGroupsAsList(ctx context.Context, input *ec2.DescribeSecurityGroupsInput) ([]types.SecurityGroup, error)

	// DescribeSubnetsAsList wraps the DescribeSubnetsPagesWithContext API, which aggregates paged results into list.
	DescribeSubnetsAsList(ctx context.Context, input *ec2.DescribeSubnetsInput) ([]types.Subnet, error)

	// DescribeVPCsAsList wraps the DescribeVpcsPagesWithContext API, which aggregates paged results into list.
	DescribeVPCsAsList(ctx context.Context, input *ec2.DescribeVpcsInput) ([]types.Vpc, error)

	CreateTagsWithContext(ctx context.Context, input *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error)
	DeleteTagsWithContext(ctx context.Context, input *ec2.DeleteTagsInput) (*ec2.DeleteTagsOutput, error)
	CreateSecurityGroupWithContext(ctx context.Context, input *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error)
	DeleteSecurityGroupWithContext(ctx context.Context, input *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error)
	AuthorizeSecurityGroupIngressWithContext(ctx context.Context, input *ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
	RevokeSecurityGroupIngressWithContext(ctx context.Context, input *ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error)
	DescribeAvailabilityZonesWithContext(ctx context.Context, input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error)
	DescribeVpcsWithContext(ctx context.Context, input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error)
	DescribeInstancesWithContext(ctx context.Context, input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error)
}

// NewEC2 constructs new EC2 implementation.
func NewEC2(cfg aws.Config, endpointsResolver *endpoints.Resolver) EC2 {
	customEndpoint := endpointsResolver.EndpointFor(ec2.ServiceID)
	return &ec2Client{
		ec2Client: ec2.NewFromConfig(cfg, func(o *ec2.Options) {
			if customEndpoint != nil {
				o.BaseEndpoint = customEndpoint
			}
		}),
	}
}

type ec2Client struct {
	ec2Client *ec2.Client
}

func (c *ec2Client) DescribeInstancesWithContext(ctx context.Context, input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	return c.ec2Client.DescribeInstances(ctx, input)
}

func (c *ec2Client) DescribeInstancesAsList(ctx context.Context, input *ec2.DescribeInstancesInput) ([]types.Instance, error) {
	var result []types.Instance
	paginator := ec2.NewDescribeInstancesPaginator(c.ec2Client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, reservation := range output.Reservations {
			result = append(result, reservation.Instances...)
		}
	}
	return result, nil
}

func (c *ec2Client) DescribeNetworkInterfacesAsList(ctx context.Context, input *ec2.DescribeNetworkInterfacesInput) ([]types.NetworkInterface, error) {
	var result []types.NetworkInterface
	paginator := ec2.NewDescribeNetworkInterfacesPaginator(c.ec2Client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.NetworkInterfaces...)
	}
	return result, nil
}

func (c *ec2Client) DescribeSecurityGroupsAsList(ctx context.Context, input *ec2.DescribeSecurityGroupsInput) ([]types.SecurityGroup, error) {
	var result []types.SecurityGroup
	paginator := ec2.NewDescribeSecurityGroupsPaginator(c.ec2Client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.SecurityGroups...)
	}
	return result, nil
}

func (c *ec2Client) DescribeSubnetsAsList(ctx context.Context, input *ec2.DescribeSubnetsInput) ([]types.Subnet, error) {
	var result []types.Subnet
	paginator := ec2.NewDescribeSubnetsPaginator(c.ec2Client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.Subnets...)
	}
	return result, nil
}

func (c *ec2Client) DescribeVPCsAsList(ctx context.Context, input *ec2.DescribeVpcsInput) ([]types.Vpc, error) {
	var result []types.Vpc
	paginator := ec2.NewDescribeVpcsPaginator(c.ec2Client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.Vpcs...)
	}
	return result, nil
}

func (c *ec2Client) CreateTagsWithContext(ctx context.Context, input *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
	return c.ec2Client.CreateTags(ctx, input)
}

func (c *ec2Client) DeleteTagsWithContext(ctx context.Context, input *ec2.DeleteTagsInput) (*ec2.DeleteTagsOutput, error) {
	return c.ec2Client.DeleteTags(ctx, input)
}

func (c *ec2Client) CreateSecurityGroupWithContext(ctx context.Context, input *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
	return c.ec2Client.CreateSecurityGroup(ctx, input)
}

func (c *ec2Client) DeleteSecurityGroupWithContext(ctx context.Context, input *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
	return c.ec2Client.DeleteSecurityGroup(ctx, input)
}

func (c *ec2Client) AuthorizeSecurityGroupIngressWithContext(ctx context.Context, input *ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	return c.ec2Client.AuthorizeSecurityGroupIngress(ctx, input)
}

func (c *ec2Client) RevokeSecurityGroupIngressWithContext(ctx context.Context, input *ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	return c.ec2Client.RevokeSecurityGroupIngress(ctx, input)
}

func (c *ec2Client) DescribeAvailabilityZonesWithContext(ctx context.Context, input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
	return c.ec2Client.DescribeAvailabilityZones(ctx, input)
}

func (c *ec2Client) DescribeVpcsWithContext(ctx context.Context, input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	return c.ec2Client.DescribeVpcs(ctx, input)
}
