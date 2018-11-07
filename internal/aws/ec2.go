package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"

	"github.com/aws/aws-sdk-go/service/ec2"
)

const (
	ManagedByKey   = "ManagedBy"
	ManagedByValue = "alb-ingress"

	TagNameCluster = "kubernetes.io/cluster"

	TagNameSubnetInternalELB = "kubernetes.io/role/internal-elb"
	TagNameSubnetPublicELB   = "kubernetes.io/role/elb"
)

// EC2API is our wrapper EC2 API interface
type EC2API interface {
	GetSubnetsByNameOrID(context.Context, []string) ([]*ec2.Subnet, error)

	// StatusEC2 validates EC2 connectivity
	StatusEC2() func() error

	// IsNodeHealthy returns true if the node is ready
	IsNodeHealthy(string) (bool, error)

	// GetInstancesByIDs retrieves ec2 instances by slice of instanceID
	GetInstancesByIDs([]string) ([]*ec2.Instance, error)

	// GetSecurityGroupByID retrieves securityGroup by securityGroupID
	GetSecurityGroupByID(string) (*ec2.SecurityGroup, error)

	// GetSecurityGroupByName retrieves securityGroup by securityGroupName(SecurityGroup names within vpc are unique)
	GetSecurityGroupByName(string) (*ec2.SecurityGroup, error)

	// GetSecurityGroupsByName retrieves securityGroups by securityGroupName(SecurityGroup names within vpc are unique)
	GetSecurityGroupsByName(context.Context, []string) ([]*ec2.SecurityGroup, error)

	// DeleteSecurityGroupByID delete securityGroup by securityGroupID
	DeleteSecurityGroupByID(string) error

	ModifyNetworkInterfaceAttributeWithContext(context.Context, *ec2.ModifyNetworkInterfaceAttributeInput) (*ec2.ModifyNetworkInterfaceAttributeOutput, error)
	CreateSecurityGroupWithContext(context.Context, *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error)
	AuthorizeSecurityGroupIngressWithContext(context.Context, *ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
	CreateTagsWithContext(context.Context, *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error)
	RevokeSecurityGroupIngressWithContext(context.Context, *ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error)
}

func (c *Cloud) ModifyNetworkInterfaceAttributeWithContext(ctx context.Context, i *ec2.ModifyNetworkInterfaceAttributeInput) (*ec2.ModifyNetworkInterfaceAttributeOutput, error) {
	return c.ec2.ModifyNetworkInterfaceAttributeWithContext(ctx, i)
}
func (c *Cloud) CreateSecurityGroupWithContext(ctx context.Context, i *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
	if i.VpcId == nil {
		i.VpcId = aws.String(c.vpcID)
	}
	return c.ec2.CreateSecurityGroupWithContext(ctx, i)
}

func (c *Cloud) AuthorizeSecurityGroupIngressWithContext(ctx context.Context, i *ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	return c.ec2.AuthorizeSecurityGroupIngressWithContext(ctx, i)
}
func (c *Cloud) CreateTagsWithContext(ctx context.Context, i *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
	return c.ec2.CreateTagsWithContext(ctx, i)
}
func (c *Cloud) RevokeSecurityGroupIngressWithContext(ctx context.Context, i *ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	return c.ec2.RevokeSecurityGroupIngressWithContext(ctx, i)
}

func (c *Cloud) GetSubnetsByNameOrID(ctx context.Context, nameOrIDs []string) (subnets []*ec2.Subnet, err error) {
	var filters [][]*ec2.Filter
	var names []string
	var ids []string

	for _, s := range nameOrIDs {
		if strings.HasPrefix(s, "subnet-") {
			ids = append(ids, s)
		} else {
			names = append(names, s)
		}
	}

	if len(ids) > 0 {
		filters = append(filters, []*ec2.Filter{
			{
				Name:   aws.String("subnet-id"),
				Values: aws.StringSlice(ids),
			},
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(c.vpcID)},
			},
		})
	}
	if len(names) > 0 {
		filters = append(filters, []*ec2.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: aws.StringSlice(names),
			},
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(c.vpcID)},
			},
		})
	}

	for _, in := range filters {
		describeSubnetsOutput, err := c.ec2.DescribeSubnetsWithContext(ctx, &ec2.DescribeSubnetsInput{Filters: in})
		if err != nil {
			return subnets, fmt.Errorf("unable to fetch subnets due to %v", err)
		}

		subnets = append(subnets, describeSubnetsOutput.Subnets...)
	}

	return
}

func (c *Cloud) GetSecurityGroupsByName(ctx context.Context, names []string) (groups []*ec2.SecurityGroup, err error) {
	in := &ec2.DescribeSecurityGroupsInput{Filters: []*ec2.Filter{
		{
			Name:   aws.String("tag:Name"),
			Values: aws.StringSlice(names),
		},
		{
			Name:   aws.String("vpc-id"),
			Values: []*string{aws.String(c.vpcID)},
		},
	}}

	describeSecurityGroupsOutput, err := c.ec2.DescribeSecurityGroupsWithContext(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch security groups %v due to %v", in.Filters, err)
	}

	if describeSecurityGroupsOutput == nil {
		return nil, nil
	}

	return describeSecurityGroupsOutput.SecurityGroups, nil
}

func (c *Cloud) GetInstancesByIDs(instanceIDs []string) ([]*ec2.Instance, error) {
	reservations, err := c.describeInstancesHelper(&ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice(instanceIDs),
	})
	if err != nil {
		return nil, err
	}
	var result []*ec2.Instance
	for _, reservation := range reservations {
		result = append(result, reservation.Instances...)
	}
	return result, nil
}

func (c *Cloud) GetSecurityGroupByID(groupID string) (*ec2.SecurityGroup, error) {
	securityGroups, err := c.describeSecurityGroupsHelper(&ec2.DescribeSecurityGroupsInput{
		GroupIds: []*string{aws.String(groupID)},
	})
	if err != nil {
		return nil, err
	}
	if len(securityGroups) == 0 {
		return nil, nil
	}
	return securityGroups[0], nil
}

func (c *Cloud) GetSecurityGroupByName(groupName string) (*ec2.SecurityGroup, error) {
	securityGroups, err := c.describeSecurityGroupsHelper(&ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(c.vpcID)},
			},
			{
				Name:   aws.String("group-name"),
				Values: []*string{aws.String(groupName)},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(securityGroups) == 0 {
		return nil, nil
	}
	return securityGroups[0], nil
}

func (c *Cloud) DeleteSecurityGroupByID(groupID string) error {
	input := &ec2.DeleteSecurityGroupInput{
		GroupId: aws.String(groupID),
	}

	retryOption := func(req *request.Request) {
		req.Retryer = &deleteSecurityGroupRetryer{
			req.Retryer,
		}
	}
	if _, err := c.ec2.DeleteSecurityGroupWithContext(aws.BackgroundContext(), input, retryOption); err != nil {
		return err
	}
	return nil
}

// describeSecurityGroups is an helper to handle pagination for DescribeSecurityGroups API call
func (c *Cloud) describeSecurityGroupsHelper(params *ec2.DescribeSecurityGroupsInput) (results []*ec2.SecurityGroup, err error) {
	p := request.Pagination{
		EndPageOnSameToken: true,
		NewRequest: func() (*request.Request, error) {
			req, _ := c.ec2.DescribeSecurityGroupsRequest(params)
			return req, nil
		},
	}
	for p.Next() {
		page := p.Page().(*ec2.DescribeSecurityGroupsOutput)
		results = append(results, page.SecurityGroups...)
	}
	err = p.Err()
	return results, err
}

func (c *Cloud) describeInstancesHelper(params *ec2.DescribeInstancesInput) (result []*ec2.Reservation, err error) {
	err = c.ec2.DescribeInstancesPages(params, func(output *ec2.DescribeInstancesOutput, _ bool) bool {
		result = append(result, output.Reservations...)
		return true
	})
	return result, err
}

// StatusEC2 validates EC2 connectivity
func (c *Cloud) StatusEC2() func() error {
	return func() error {
		in := &ec2.DescribeTagsInput{MaxResults: aws.Int64(1)}

		if _, err := c.ec2.DescribeTagsWithContext(context.TODO(), in); err != nil {
			return fmt.Errorf("[ec2.DescribeTagsWithContext]: %v", err)
		}
		return nil
	}
}

// IsNodeHealthy returns true if the node is ready
func (c *Cloud) IsNodeHealthy(instanceid string) (bool, error) {
	in := &ec2.DescribeInstanceStatusInput{
		InstanceIds: []*string{aws.String(instanceid)},
	}
	o, err := c.ec2.DescribeInstanceStatus(in)
	if err != nil {
		return false, fmt.Errorf("Unable to DescribeInstanceStatus on %v: %v", instanceid, err.Error())
	}

	for _, instanceStatus := range o.InstanceStatuses {
		if *instanceStatus.InstanceId != instanceid {
			continue
		}
		if *instanceStatus.InstanceState.Code == 16 { // running
			return true, nil
		}
		return false, nil
	}

	return false, nil
}

type deleteSecurityGroupRetryer struct {
	request.Retryer
}

func (r *deleteSecurityGroupRetryer) ShouldRetry(req *request.Request) bool {
	if awsErr, ok := req.Error.(awserr.Error); ok {
		if awsErr.Code() == "DependencyViolation" {
			return true
		}
	}
	// Fallback to built in retry rules
	return r.Retryer.ShouldRetry(req)
}

func (r *deleteSecurityGroupRetryer) MaxRetries() int {
	return 20
}
