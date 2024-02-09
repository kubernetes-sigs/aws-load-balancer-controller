package services

import (
	"context"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
)

type EC2 interface {
	ec2iface.EC2API

	// wrapper to DescribeInstancesPagesWithContext API, which aggregates paged results into list.
	DescribeInstancesAsList(ctx context.Context, input *ec2.DescribeInstancesInput) ([]*ec2.Instance, error)

	// wrapper to DescribeNetworkInterfacesPagesWithContext API, which aggregates paged results into list.
	DescribeNetworkInterfacesAsList(ctx context.Context, input *ec2.DescribeNetworkInterfacesInput) ([]*ec2.NetworkInterface, error)

	// wrapper to DescribeSecurityGroupsPagesWithContext API, which aggregates paged results into list.
	DescribeSecurityGroupsAsList(ctx context.Context, input *ec2.DescribeSecurityGroupsInput) ([]*ec2.SecurityGroup, error)

	// wrapper to DescribeSubnetsPagesWithContext API, which aggregates paged results into list.
	DescribeSubnetsAsList(ctx context.Context, input *ec2.DescribeSubnetsInput) ([]*ec2.Subnet, error)
}

// NewEC2 constructs new EC2 implementation.
func NewEC2(session *session.Session) EC2 {
	return &defaultEC2{
		EC2API: ec2.New(session),
	}
}

type defaultEC2 struct {
	ec2iface.EC2API
}

func (c *defaultEC2) DescribeInstancesAsList(ctx context.Context, input *ec2.DescribeInstancesInput) ([]*ec2.Instance, error) {
	var result []*ec2.Instance
	if err := c.DescribeInstancesPagesWithContext(ctx, input, func(output *ec2.DescribeInstancesOutput, _ bool) bool {
		for _, reservation := range output.Reservations {
			result = append(result, reservation.Instances...)
		}
		return true
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *defaultEC2) DescribeNetworkInterfacesAsList(ctx context.Context, input *ec2.DescribeNetworkInterfacesInput) ([]*ec2.NetworkInterface, error) {
	var result []*ec2.NetworkInterface
	if err := c.DescribeNetworkInterfacesPagesWithContext(ctx, input, func(output *ec2.DescribeNetworkInterfacesOutput, _ bool) bool {
		result = append(result, output.NetworkInterfaces...)
		return true
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *defaultEC2) DescribeSecurityGroupsAsList(ctx context.Context, input *ec2.DescribeSecurityGroupsInput) ([]*ec2.SecurityGroup, error) {
	var result []*ec2.SecurityGroup
	if err := c.DescribeSecurityGroupsPagesWithContext(ctx, input, func(output *ec2.DescribeSecurityGroupsOutput, _ bool) bool {
		result = append(result, output.SecurityGroups...)
		return true
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *defaultEC2) DescribeSubnetsAsList(ctx context.Context, input *ec2.DescribeSubnetsInput) ([]*ec2.Subnet, error) {
	var result []*ec2.Subnet
	if err := c.DescribeSubnetsPagesWithContext(ctx, input, func(output *ec2.DescribeSubnetsOutput, _ bool) bool {
		result = append(result, output.Subnets...)
		return true
	}); err != nil {
		return nil, err
	}
	return result, nil
}
