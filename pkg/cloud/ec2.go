package cloud

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"strings"
)

// EC2 is an wrapper around original EC2API with additional convenient APIs.
type EC2 interface {
	ec2iface.EC2API

	GetSubnetsByNameOrID(ctx context.Context, nameOrIDs []string) ([]*ec2.Subnet, error)
	DescribeSecurityGroupsAsList(ctx context.Context, input *ec2.DescribeSecurityGroupsInput) ([]*ec2.SecurityGroup, error)
	DescribeInstancesAsList(ctx context.Context, input *ec2.DescribeInstancesInput) ([]*ec2.Instance, error)
}

func NewEC2(session *session.Session) EC2 {
	return &defaultEC2{
		ec2.New(session),
	}
}

var _ EC2 = (*defaultEC2)(nil)

type defaultEC2 struct {
	ec2iface.EC2API
}

func (c *defaultEC2) GetSubnetsByNameOrID(ctx context.Context, nameOrIDs []string) ([]*ec2.Subnet, error) {
	var names []string
	var ids []string
	for _, s := range nameOrIDs {
		if strings.HasPrefix(s, "subnet-") {
			ids = append(ids, s)
		} else {
			names = append(names, s)
		}
	}

	var filters [][]*ec2.Filter
	if len(ids) > 0 {
		filters = append(filters, []*ec2.Filter{
			{
				Name:   aws.String("subnet-id"),
				Values: aws.StringSlice(ids),
			},
		})
	}
	if len(names) > 0 {
		filters = append(filters, []*ec2.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: aws.StringSlice(names),
			},
		})
	}

	var subnets []*ec2.Subnet
	for _, in := range filters {
		describeSubnetsOutput, err := c.DescribeSubnetsWithContext(ctx, &ec2.DescribeSubnetsInput{Filters: in})
		if err != nil {
			return nil, err
		}
		subnets = append(subnets, describeSubnetsOutput.Subnets...)
	}

	return subnets, nil
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

func (c *defaultEC2) DescribeInstancesAsList(ctx context.Context, input *ec2.DescribeInstancesInput) ([]*ec2.Instance, error) {
	var result []*ec2.Instance
	if err := c.DescribeInstancesPagesWithContext(ctx, input, func(output *ec2.DescribeInstancesOutput, _ bool) bool {
		for _, item := range output.Reservations {
			result = append(result, item.Instances...)
		}
		return true
	}); err != nil {
		return nil, err
	}
	return result, nil
}
