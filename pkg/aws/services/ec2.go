package services

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/provider"
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
func NewEC2(awsClientsProvider provider.AWSClientsProvider) EC2 {
	return &ec2Client{
		awsClientsProvider: awsClientsProvider,
	}
}

type ec2Client struct {
	awsClientsProvider provider.AWSClientsProvider
}

func (c *ec2Client) DescribeInstancesWithContext(ctx context.Context, input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	client, err := c.awsClientsProvider.GetEC2Client(ctx, "DescribeInstances")
	if err != nil {
		return nil, err
	}
	return client.DescribeInstances(ctx, input)
}

func (c *ec2Client) DescribeInstancesAsList(ctx context.Context, input *ec2.DescribeInstancesInput) ([]types.Instance, error) {
	var result []types.Instance
	client, err := c.awsClientsProvider.GetEC2Client(ctx, "DescribeInstances")
	if err != nil {
		return nil, err
	}
	paginator := ec2.NewDescribeInstancesPaginator(client, input)
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
	client, err := c.awsClientsProvider.GetEC2Client(ctx, "DescribeNetworkInterfaces")
	if err != nil {
		return nil, err
	}
	paginator := ec2.NewDescribeNetworkInterfacesPaginator(client, input)
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
	client, err := c.awsClientsProvider.GetEC2Client(ctx, "DescribeSecurityGroups")
	if err != nil {
		return nil, err
	}
	paginator := ec2.NewDescribeSecurityGroupsPaginator(client, input)
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
	client, err := c.awsClientsProvider.GetEC2Client(ctx, "DescribeSubnets")
	if err != nil {
		return nil, err
	}
	paginator := ec2.NewDescribeSubnetsPaginator(client, input)
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
	client, err := c.awsClientsProvider.GetEC2Client(ctx, "DescribeVpcs")
	if err != nil {
		return nil, err
	}
	paginator := ec2.NewDescribeVpcsPaginator(client, input)
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
	client, err := c.awsClientsProvider.GetEC2Client(ctx, "CreateTags")
	if err != nil {
		return nil, err
	}
	return client.CreateTags(ctx, input)
}

func (c *ec2Client) DeleteTagsWithContext(ctx context.Context, input *ec2.DeleteTagsInput) (*ec2.DeleteTagsOutput, error) {
	client, err := c.awsClientsProvider.GetEC2Client(ctx, "DeleteTags")
	if err != nil {
		return nil, err
	}
	return client.DeleteTags(ctx, input)
}

func (c *ec2Client) CreateSecurityGroupWithContext(ctx context.Context, input *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
	client, err := c.awsClientsProvider.GetEC2Client(ctx, "CreateSecurityGroup")
	if err != nil {
		return nil, err
	}
	return client.CreateSecurityGroup(ctx, input)
}

func (c *ec2Client) DeleteSecurityGroupWithContext(ctx context.Context, input *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
	client, err := c.awsClientsProvider.GetEC2Client(ctx, "DeleteSecurityGroup")
	if err != nil {
		return nil, err
	}
	return client.DeleteSecurityGroup(ctx, input)
}

func (c *ec2Client) AuthorizeSecurityGroupIngressWithContext(ctx context.Context, input *ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	client, err := c.awsClientsProvider.GetEC2Client(ctx, "AuthorizeSecurityGroupIngress")
	if err != nil {
		return nil, err
	}
	return client.AuthorizeSecurityGroupIngress(ctx, input)
}

func (c *ec2Client) RevokeSecurityGroupIngressWithContext(ctx context.Context, input *ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	client, err := c.awsClientsProvider.GetEC2Client(ctx, "RevokeSecurityGroupIngress")
	if err != nil {
		return nil, err
	}
	return client.RevokeSecurityGroupIngress(ctx, input)
}

func (c *ec2Client) DescribeAvailabilityZonesWithContext(ctx context.Context, input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
	client, err := c.awsClientsProvider.GetEC2Client(ctx, "DescribeAvailabilityZones")
	if err != nil {
		return nil, err
	}
	return client.DescribeAvailabilityZones(ctx, input)
}

func (c *ec2Client) DescribeVpcsWithContext(ctx context.Context, input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	client, err := c.awsClientsProvider.GetEC2Client(ctx, "DescribeVpcs")
	if err != nil {
		return nil, err
	}
	return client.DescribeVpcs(ctx, input)
}
