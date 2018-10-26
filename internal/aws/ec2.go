package aws

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/request"

	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"

	"github.com/aws/aws-sdk-go/service/ec2"

	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

const (
	instSpecifierTag = "instance"
	ManagedByKey     = "ManagedBy"
	ManagedByValue   = "alb-ingress"

	tagNameCluster = "kubernetes.io/cluster"

	tagNameSubnetInternalELB = "kubernetes.io/role/internal-elb"
	tagNameSubnetPublicELB   = "kubernetes.io/role/elb"
)

// EC2API is our wrapper EC2 API interface
type EC2API interface {
	GetSubnets([]*string) ([]*string, error)

	GetSecurityGroups(context.Context, []*string) ([]*string, error)

	// GetVPCID returns the VPC of the instance the controller is currently running on.
	// This is achieved by getting the identity document of the EC2 instance and using
	// the DescribeInstances call to determine its VPC ID.
	GetVPCID() (*string, error)

	GetVPC(*string) (*ec2.Vpc, error)

	// StatusEC2 validates EC2 connectivity
	StatusEC2() func() error

	// IsNodeHealthy returns true if the node is ready
	IsNodeHealthy(string) (bool, error)

	// GetInstancesByIDs retrieves ec2 instances by slice of instanceID
	GetInstancesByIDs([]string) ([]*ec2.Instance, error)

	// GetSecurityGroupByID retrieves securityGroup by securityGroupID
	GetSecurityGroupByID(string) (*ec2.SecurityGroup, error)

	// GetSecurityGroupByName retrives securityGroup by vpcID and securityGroupName(SecurityGroup names within vpc are unique)
	GetSecurityGroupByName(string, string) (*ec2.SecurityGroup, error)

	// DeleteSecurityGroupByID delete securityGroup by securityGroupID
	DeleteSecurityGroupByID(string) error

	// ClusterSubnets returns the subnets that are tagged for the cluster
	ClusterSubnets(scheme string) ([]string, error)

	// ResolveSecurityGroupNames returns the security group ids for a list of security group names and ids
	ResolveSecurityGroupNames(context.Context, []string) ([]string, error)

	// ResolveSubnets returns the subnets for a list of subnet names and ids
	ResolveSubnets(context.Context, string, []string) ([]string, error)

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

func (c *Cloud) GetSubnets(names []*string) (subnets []*string, err error) {
	vpcID, err := c.GetVPCID()
	if err != nil {
		return
	}

	in := &ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{
		{
			Name:   aws.String("tag:Name"),
			Values: names,
		},
		{
			Name:   aws.String("vpc-id"),
			Values: []*string{vpcID},
		},
	}}

	describeSubnetsOutput, err := c.ec2.DescribeSubnets(in)
	if err != nil {
		return subnets, fmt.Errorf("Unable to fetch subnets %v: %v", in.Filters, err)
	}

	for _, subnet := range describeSubnetsOutput.Subnets {
		_, ok := util.EC2Tags(subnet.Tags).Get("Name")
		if ok {
			subnets = append(subnets, subnet.SubnetId)
		}
	}
	return
}

func (c *Cloud) GetSecurityGroups(ctx context.Context, names []*string) (sgs []*string, err error) {
	vpcID, err := c.GetVPCID()
	if err != nil {
		return
	}

	in := &ec2.DescribeSecurityGroupsInput{Filters: []*ec2.Filter{
		{
			Name:   aws.String("tag:Name"),
			Values: names,
		},
		{
			Name:   aws.String("vpc-id"),
			Values: []*string{vpcID},
		},
	}}

	describeSecurityGroupsOutput, err := c.ec2.DescribeSecurityGroupsWithContext(ctx, in)
	if err != nil {
		return sgs, fmt.Errorf("Unable to fetch security groups %v: %v", in.Filters, err)
	}

	if describeSecurityGroupsOutput == nil {
		return nil, nil
	}

	for _, sg := range describeSecurityGroupsOutput.SecurityGroups {
		sgs = append(sgs, sg.GroupId)
	}

	return
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

func (c *Cloud) ResolveSecurityGroupNames(ctx context.Context, in []string) ([]string, error) {
	var names []string
	var output []string

	for _, sg := range in {
		if strings.HasPrefix(sg, "sg-") {
			output = append(output, sg)
			continue
		}

		names = append(names, sg)
	}

	if len(names) > 0 {
		groups, err := c.GetSecurityGroups(ctx, aws.StringSlice(names))
		if err != nil {
			return output, err
		}

		output = append(output, aws.StringValueSlice(groups)...)
	}

	if len(output) != len(in) {
		return output, fmt.Errorf("not all security groups were resolvable, (%v != %v)", strings.Join(in, ","), strings.Join(output, ","))
	}

	return output, nil
}

func (c *Cloud) ResolveSubnets(ctx context.Context, scheme string, in []string) ([]string, error) {
	if len(in) == 0 {
		subnets, err := c.ClusterSubnets(scheme)
		return subnets, err

	}

	var names []string
	var subnets []string

	for _, subnet := range in {
		if strings.HasPrefix(subnet, "subnet-") {
			subnets = append(subnets, subnet)
			continue
		}
		names = append(names, subnet)
	}

	if len(names) > 0 {
		nets, err := c.GetSubnets(aws.StringSlice(names))
		if err != nil {
			return subnets, err
		}

		subnets = append(subnets, aws.StringValueSlice(nets)...)
	}

	sort.Strings(subnets)
	if len(subnets) != len(in) {
		return subnets, fmt.Errorf("not all subnets were resolvable, (%v != %v)", strings.Join(in, ","), strings.Join(subnets, ","))
	}

	return subnets, nil
}

func (c *Cloud) GetSecurityGroupByName(vpcID string, groupName string) (*ec2.SecurityGroup, error) {
	securityGroups, err := c.describeSecurityGroupsHelper(&ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(vpcID)},
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

// GetVPCID returns the VPC of the instance the controller is currently running on.
// This is achieved by getting the identity document of the EC2 instance and using
// the DescribeInstances call to determine its VPC ID.
func (c *Cloud) GetVPCID() (*string, error) {
	var vpc *string

	if v := os.Getenv("AWS_VPC_ID"); v != "" {
		return &v, nil
	}

	identityDoc, err := c.ec2metadata.GetInstanceIdentityDocument()
	if err != nil {
		return nil, err
	}

	// capture instance ID for lookup in DescribeInstances
	descInstancesInput := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(identityDoc.InstanceID)},
	}

	// capture description of this instance for later capture of VpcId
	descInstancesOutput, err := c.ec2.DescribeInstances(descInstancesInput)
	if err != nil {
		return nil, err
	}

	// Before attempting to return VpcId of instance, ensure at least 1 reservation and instance
	// (in that reservation) was found.
	if err = instanceVPCIsValid(descInstancesOutput); err != nil {
		return nil, err
	}

	vpc = descInstancesOutput.Reservations[0].Instances[0].VpcId
	return vpc, nil
}

func (c *Cloud) GetVPC(id *string) (*ec2.Vpc, error) {
	o, err := c.ec2.DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []*string{id},
	})
	if err != nil {
		return nil, err
	}
	if len(o.Vpcs) != 1 {
		return nil, fmt.Errorf("Invalid amount of VPCs %d returned for %s", len(o.Vpcs), *id)
	}

	return o.Vpcs[0], nil
}

// instanceVPCIsValid ensures returned instance data has a valid VPC ID in the output
func instanceVPCIsValid(o *ec2.DescribeInstancesOutput) error {
	if len(o.Reservations) < 1 {
		return fmt.Errorf("When looking up VPC ID could not identify instance. Found %d reservations"+
			" in AWS call. Should have found atleast 1.", len(o.Reservations))
	}
	if len(o.Reservations[0].Instances) < 1 {
		return fmt.Errorf("When looking up VPC ID could not identify instance. Found %d instances"+
			" in AWS call. Should have found atleast 1.", len(o.Reservations))
	}
	if o.Reservations[0].Instances[0].VpcId == nil {
		return fmt.Errorf("When looking up VPC ID could not instance returned had a nil value for VPC.")
	}
	if *o.Reservations[0].Instances[0].VpcId == "" {
		return fmt.Errorf("When looking up VPC ID could not instance returned had an empty value for VPC.")
	}

	return nil
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

// ClusterSubnets returns the subnets that are tagged for the cluster
func (c *Cloud) ClusterSubnets(scheme string) ([]string, error) {
	var useableSubnets []*ec2.Subnet
	var out []string
	var key string

	if scheme == elbv2.LoadBalancerSchemeEnumInternal {
		key = tagNameSubnetInternalELB
	} else if scheme == elbv2.LoadBalancerSchemeEnumInternetFacing {
		key = tagNameSubnetPublicELB
	} else {
		return nil, fmt.Errorf("invalid scheme [%s]", scheme)
	}

	clusterSubnets, err := c.GetClusterSubnets()
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS tags. Error: %s", err.Error())
	}

	var filterValues []*string
	for arn, subnetTags := range clusterSubnets {
		for _, tag := range subnetTags {
			if aws.StringValue(tag.Key) == key {
				p := strings.Split(arn, "/")
				subnetID := &p[len(p)-1]
				filterValues = append(filterValues, subnetID)
			}
		}
	}

	if len(filterValues) == 0 {
		sort.Strings(out)
		return out, nil
	}

	input := &ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("subnet-id"),
				Values: filterValues,
			},
		},
	}
	o, err := c.ec2.DescribeSubnets(input)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch subnets %v: %v", awsutil.Prettify(input.Filters), err)
	}

	for _, subnet := range o.Subnets {
		if subnetIsUsable(subnet, useableSubnets) {
			useableSubnets = append(useableSubnets, subnet)
			out = append(out, aws.StringValue(subnet.SubnetId))
		}
	}

	// if len(subnets) == 0 {
	// 	return nil, errors.NewInvalidAnnotationContentReason(`No subnets defined or were discoverable`)
	// }

	if len(out) < 2 {
		return nil, fmt.Errorf("Retrieval of subnets failed to resolve 2 qualified subnets. Subnets must "+
			"contain the %s/<cluster name> tag with a value of shared or owned and the %s tag signifying it should be used for ALBs "+
			"Additionally, there must be at least 2 subnets with unique availability zones as required by "+
			"ALBs. Either tag subnets to meet this requirement or use the subnets annotation on the "+
			"ingress resource to explicitly call out what subnets to use for ALB creation. The subnets "+
			"that did resolve were %v.", tagNameCluster, tagNameSubnetInternalELB,
			log.Prettify(out))
	}

	sort.Strings(out)
	return out, nil
}

// subnetIsUsable determines if the subnet shares the same availablity zone as a subnet in the
// existing list. If it does, false is returned as you cannot have albs provisioned to 2 subnets in
// the same availability zone.
func subnetIsUsable(new *ec2.Subnet, existing []*ec2.Subnet) bool {
	for _, subnet := range existing {
		if *new.AvailabilityZone == *subnet.AvailabilityZone {
			return false
		}
	}
	return true
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
