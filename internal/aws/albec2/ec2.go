package albec2

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albcache"

	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

const (
	instSpecifierTag = "instance"
	ManagedByKey     = "ManagedBy"
	ManagedByValue   = "alb-ingress"

	tagNameCluster = "kubernetes.io/cluster"

	tagNameSubnetInternalELB = "kubernetes.io/role/internal-elb"
	tagNameSubnetPublicELB   = "kubernetes.io/role/elb"

	GetSecurityGroupsCacheTTL = time.Minute * 60
	GetSubnetsCacheTTL        = time.Minute * 60

	IsNodeHealthyCacheTTL = time.Minute * 5
)

// EC2svc is a pointer to the awsutil EC2 service
var EC2svc *EC2

// EC2Metadatasvc is a pointer to the awsutil EC2metadata service
var EC2Metadatasvc *EC2MData

// EC2 is our extension to AWS's ec2.EC2
type EC2 struct {
	ec2iface.EC2API
}

// EC2MData is our extension to AWS's ec2metadata.EC2Metadata
// cache is not required for this struct as we only use it to lookup
// instance metadata when the cache for the EC2 struct is expired.
type EC2MData struct {
	*ec2metadata.EC2Metadata
}

// NewEC2 returns an awsutil EC2 service
func NewEC2(awsSession *session.Session) {
	EC2svc = &EC2{
		ec2.New(awsSession),
	}
}

// NewEC2Metadata returns an awsutil EC2Metadata service
func NewEC2Metadata(awsSession *session.Session) {
	EC2Metadatasvc = &EC2MData{
		ec2metadata.New(awsSession),
	}
}

// DescribeSGByPermissionGroup Finds an SG that the passed SG has permission to.
func (e *EC2) DescribeSGByPermissionGroup(sg *string) (*string, error) {
	cacheName := "EC2.DescribeSGByPermissionGroup"
	item := albcache.Get(cacheName, *sg)

	if item != nil {
		groupid := item.Value().(*string)
		return groupid, nil
	}

	in := &ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("ip-permission.group-id"),
				Values: []*string{sg},
			},
		},
	}
	o, err := e.DescribeSecurityGroups(in)
	if err != nil {
		return nil, err
	}

	if len(o.SecurityGroups) != 1 {
		return nil, fmt.Errorf("Didn't find exactly 1 matching (managed) instance SG. Found %d", len(o.SecurityGroups))
	}

	albcache.Set(cacheName, *sg, o.SecurityGroups[0].GroupId, time.Minute*5)
	return o.SecurityGroups[0].GroupId, nil
}

// DescribeSGPorts returns the ports associated with a SG.
func (e *EC2) DescribeSGPorts(sgID *string) ([]int64, error) {
	cacheName := "EC2.DescribeSGPorts"
	item := albcache.Get(cacheName, *sgID)

	if item != nil {
		ports := item.Value().([]int64)
		return ports, nil
	}

	in := &ec2.DescribeSecurityGroupsInput{
		GroupIds: []*string{sgID},
	}

	o, err := e.DescribeSecurityGroups(in)
	if err != nil || len(o.SecurityGroups) != 1 {
		return nil, err
	}

	ports := []int64{}
	for _, perm := range o.SecurityGroups[0].IpPermissions {
		ports = append(ports, *perm.FromPort)
	}

	albcache.Set(cacheName, *sgID, ports, time.Minute*5)
	return ports, nil
}

// DescribeSGInboundCidrs returns the inbound cidrs associated with a SG.
func (e *EC2) DescribeSGInboundCidrs(sgID *string) ([]*string, error) {
	cacheName := "EC2.DescribeSGInboundCidrs"
	item := albcache.Get(cacheName, *sgID)

	if item != nil {
		tags := item.Value().([]*string)
		return tags, nil
	}

	in := &ec2.DescribeSecurityGroupsInput{
		GroupIds: []*string{sgID},
	}

	o, err := e.DescribeSecurityGroups(in)
	if err != nil || len(o.SecurityGroups) != 1 {
		return nil, err
	}

	inboundCidrs := []*string{}
	for _, perm := range o.SecurityGroups[0].IpPermissions {
		for _, ipRange := range perm.IpRanges {
			inboundCidrs = append(inboundCidrs, ipRange.CidrIp)
		}
	}

	albcache.Set(cacheName, *sgID, inboundCidrs, time.Minute*5)
	return inboundCidrs, nil
}

// DescribeSGTags returns tags for an sg when the sg-id is provided.
func (e *EC2) DescribeSGTags(sgID *string) ([]*ec2.TagDescription, error) {
	cacheName := "EC2.DescribeSGTags"
	item := albcache.Get(cacheName, *sgID)

	if item != nil {
		tags := item.Value().([]*ec2.TagDescription)
		return tags, nil
	}

	in := &ec2.DescribeTagsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("resource-id"),
				Values: []*string{sgID},
			},
		},
	}

	o, err := e.DescribeTags(in)
	if err != nil {
		return nil, err
	}

	albcache.Set(cacheName, *sgID, o.Tags, time.Minute*5)
	return o.Tags, nil
}

// DeleteSecurityGroupByID deletes a security group based on its provided ID
func (e *EC2) DeleteSecurityGroupByID(sgID *string) error {
	in := &ec2.DeleteSecurityGroupInput{
		GroupId: sgID,
	}
	if _, err := e.DeleteSecurityGroup(in); err != nil {
		return err
	}

	return nil
}

func (e *EC2) GetSubnets(names []*string) (subnets []*string, err error) {
	vpcID, err := EC2svc.GetVPCID()
	if err != nil {
		return
	}

	cacheName := "EC2.GetSubnets"
	var queryNames []*string

	for _, n := range names {
		item := albcache.Get(cacheName, *n)

		if item != nil {
			subnets = append(subnets, item.Value().(*string))
		} else {
			queryNames = append(queryNames, n)
		}
	}

	if len(queryNames) == 0 {
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

	describeSubnetsOutput, err := EC2svc.DescribeSubnets(in)
	if err != nil {
		return subnets, fmt.Errorf("Unable to fetch subnets %v: %v", in.Filters, err)
	}

	for _, subnet := range describeSubnetsOutput.Subnets {
		value, ok := util.EC2Tags(subnet.Tags).Get("Name")
		if ok {
			albcache.Set(cacheName, value, subnet.SubnetId, GetSubnetsCacheTTL)
			subnets = append(subnets, subnet.SubnetId)
		}
	}
	return
}

func (e *EC2) GetSecurityGroups(names []*string) (sgs []*string, err error) {
	vpcID, err := EC2svc.GetVPCID()
	if err != nil {
		return
	}

	cacheName := "EC2.GetSecurityGroups"
	var queryNames []*string

	for _, n := range names {
		item := albcache.Get(cacheName, *n)

		if item != nil {
			sgs = append(sgs, item.Value().(*string))
		} else {
			queryNames = append(queryNames, n)
		}
	}

	if len(queryNames) == 0 {
		return
	}

	in := &ec2.DescribeSecurityGroupsInput{Filters: []*ec2.Filter{
		{
			Name:   aws.String("tag:Name"),
			Values: queryNames,
		},
		{
			Name:   aws.String("vpc-id"),
			Values: []*string{vpcID},
		},
	}}

	describeSecurityGroupsOutput, err := EC2svc.DescribeSecurityGroups(in)
	if err != nil {
		return sgs, fmt.Errorf("Unable to fetch security groups %v: %v", in.Filters, err)
	}

	for _, sg := range describeSecurityGroupsOutput.SecurityGroups {
		name, _ := util.EC2Tags(sg.Tags).Get("Name")
		albcache.Set(cacheName, name, sg.GroupId, GetSecurityGroupsCacheTTL)
		sgs = append(sgs, sg.GroupId)
	}

	return
}

// DisassociateSGFromInstanceIfNeeded loops through a list of instances to see if a managedSG
// exists. If it does, it attempts to remove the managedSG from the list.
func (e *EC2) DisassociateSGFromInstanceIfNeeded(instances []*string, managedSG *string) error {
	if managedSG == nil {
		return fmt.Errorf("Managed SG passed was empty unable to disassociate from instances")
	}

	if len(instances) < 1 {
		return nil
	}

	in := &ec2.DescribeInstancesInput{
		InstanceIds: instances,
	}

	for {
		insts, err := e.DescribeInstances(in)
		if err != nil {
			return err
		}

		// Compile the list of instances from which we will remove the ALB
		// security group in the next step.
		removeManagedSG := []*ec2.Instance{}
		for _, reservation := range insts.Reservations {
			for _, inst := range reservation.Instances {
				hasGroup := false
				for _, sg := range inst.SecurityGroups {
					if *managedSG == *sg.GroupId {
						hasGroup = true
					}
				}
				if hasGroup {
					removeManagedSG = append(removeManagedSG, inst)
				}
			}
		}

		for _, inst := range removeManagedSG {
			groups := []*string{}
			for _, sg := range inst.SecurityGroups {
				if *sg.GroupId != *managedSG {
					groups = append(groups, sg.GroupId)
				}
			}
			inAttr := &ec2.ModifyInstanceAttributeInput{
				InstanceId: inst.InstanceId,
				Groups:     groups,
			}
			if _, err := e.ModifyInstanceAttribute(inAttr); err != nil {
				return err
			}
		}

		if insts.NextToken == nil {
			break
		}

		in = &ec2.DescribeInstancesInput{
			NextToken: insts.NextToken,
		}
	}

	return nil
}

// AssociateSGToInstanceIfNeeded loops through a list of instances to see if newSG exists
// for them. It not, it is appended to the instances(s).
func (e *EC2) AssociateSGToInstanceIfNeeded(instances []*string, newSG *string) error {
	if len(instances) < 1 {
		return nil
	}

	in := &ec2.DescribeInstancesInput{
		InstanceIds: instances,
	}

	for {
		insts, err := e.DescribeInstances(in)
		if err != nil {
			return err
		}

		// Compile the list of instances with the security group that
		// facilitates instance <-> ALB communication.
		needsManagedSG := []*ec2.Instance{}
		for _, reservation := range insts.Reservations {
			for _, inst := range reservation.Instances {
				hasGroup := false
				for _, sg := range inst.SecurityGroups {
					if *newSG == *sg.GroupId {
						hasGroup = true
					}
				}
				if !hasGroup {
					needsManagedSG = append(needsManagedSG, inst)
				}
			}
		}

		for _, inst := range needsManagedSG {
			groups := []*string{}
			for _, sg := range inst.SecurityGroups {
				groups = append(groups, sg.GroupId)
			}
			groups = append(groups, newSG)
			inAttr := &ec2.ModifyInstanceAttributeInput{
				InstanceId: inst.InstanceId,
				Groups:     groups,
			}
			if _, err := e.ModifyInstanceAttribute(inAttr); err != nil {
				return err
			}
		}

		if insts.NextToken == nil {
			break
		}

		in = &ec2.DescribeInstancesInput{
			NextToken: insts.NextToken,
		}
	}

	return nil
}

// UpdateSGIfNeeded attempts to resolve a security group based on its description.
// If one is found, it'll run an update that is effectivley a no-op when the groups are
// identical. Finally it'll attempt to find the associated instance SG and return that
// as the second string.
func (e *EC2) UpdateSGIfNeeded(vpcID *string, sgName *string, currentPorts []int64, desiredPorts []int64, currentCidrs []*string, desiredCidrs []*string) (*string, *string, error) {
	// attempt to locate sg
	in := &ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{vpcID},
			},
			{
				Name:   aws.String("group-name"),
				Values: []*string{sgName},
			},
		},
	}
	o, err := e.DescribeSecurityGroups(in)
	if err != nil {
		return nil, nil, err
	}

	// when no results were returned, security group doesn't exist and no need to attempt modification.
	if len(o.SecurityGroups) < 1 {
		return nil, nil, nil
	}
	groupId := o.SecurityGroups[0].GroupId

	// if no currentPorts were known to the LB but the sg stil resoled, query the SG to see if any ports can be resvoled
	if len(currentPorts) < 1 {
		currentPorts, err = e.DescribeSGPorts(groupId)
		if err != nil {
			return nil, nil, err
		}
	}

	// for each addPort, run an authorize to ensure it's added
	for _, port := range desiredPorts {
		ipRanges := []*ec2.IpRange{}
		for _, cidr := range desiredCidrs {
			if existsInOtherPortRange(port, currentPorts) && existsInOtherIngressCidrRange(cidr, currentCidrs) {
				continue
			}
			ipRanges = append(ipRanges, &ec2.IpRange{
				CidrIp:      cidr,
				Description: aws.String(fmt.Sprintf("Allow ingress on port %v from %v.", port, aws.StringValue(cidr))),
			})

		}

		if len(ipRanges) == 0 {
			continue
		}

		in := &ec2.AuthorizeSecurityGroupIngressInput{
			GroupId: groupId,
			IpPermissions: []*ec2.IpPermission{
				{
					ToPort:     aws.Int64(port),
					FromPort:   aws.Int64(port),
					IpProtocol: aws.String("tcp"),
					IpRanges:   ipRanges,
				},
			},
		}
		_, err := e.AuthorizeSecurityGroupIngress(in)
		if err != nil {
			return nil, nil, err
		}
	}

	// for each currentPort, run a revoke to ensure it can be removed
	for _, port := range currentPorts {
		ipRanges := []*ec2.IpRange{}
		for _, cidr := range currentCidrs {
			if existsInOtherPortRange(port, desiredPorts) && existsInOtherIngressCidrRange(cidr, desiredCidrs) {
				continue
			}
			ipRanges = append(ipRanges, &ec2.IpRange{
				CidrIp:      cidr,
				Description: aws.String(fmt.Sprintf("Allow ingress on port %v from %v.", port, aws.StringValue(cidr))),
			})
		}

		if len(ipRanges) == 0 {
			continue
		}

		in := &ec2.RevokeSecurityGroupIngressInput{
			GroupId: groupId,
			IpPermissions: []*ec2.IpPermission{
				{
					ToPort:     aws.Int64(port),
					FromPort:   aws.Int64(port),
					IpProtocol: aws.String("tcp"),
					IpRanges:   ipRanges,
				},
			},
		}
		_, err := e.RevokeSecurityGroupIngress(in)
		if err != nil {
			return nil, nil, err
		}
	}

	// attempt to resolve instance sg
	instanceSGName := fmt.Sprintf("%s-%s", instSpecifierTag, *sgName)
	inInstance := &ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{vpcID},
			},
			{
				Name:   aws.String("group-name"),
				Values: []*string{aws.String(instanceSGName)},
			},
		},
	}
	oInstance, err := e.DescribeSecurityGroups(inInstance)
	if err != nil {
		return nil, nil, err
	}

	// managed sg may have existed but instance sg didn't
	if len(oInstance.SecurityGroups) < 1 {
		return o.SecurityGroups[0].GroupId, nil, nil
	}

	return groupId, oInstance.SecurityGroups[0].GroupId, nil
}

func existsInOtherPortRange(a int64, list []int64) bool {
	for _, p := range list {
		if a == p {
			return true
		}
	}
	return false
}

func existsInOtherIngressCidrRange(a *string, list []*string) bool {
	for _, p := range list {
		if *a == *p {
			return true
		}
	}
	return false
}

// CreateSecurityGroupFromPorts generates a new security group in AWS based on a list of ports. If
// successful, it returns the security group ID.
func (e *EC2) CreateSecurityGroupFromPorts(vpcID *string, sgName *string, ports []int64, cidrs []*string) (*string, *string, error) {
	inSG := &ec2.CreateSecurityGroupInput{
		VpcId:       vpcID,
		GroupName:   sgName,
		Description: sgName,
	}
	oSG, err := e.CreateSecurityGroup(inSG)
	if err != nil {
		return nil, nil, err
	}

	inSGRule := &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: oSG.GroupId,
	}

	// for every port specified, allow all tcp traffic.
	for _, port := range ports {
		ipRanges := []*ec2.IpRange{}
		for _, cidr := range cidrs {
			ipRanges = append(ipRanges, &ec2.IpRange{
				CidrIp:      cidr,
				Description: aws.String(fmt.Sprintf("Allow ingress on port %v from %v.", port, aws.StringValue(cidr))),
			})
		}
		newRule := &ec2.IpPermission{
			FromPort:   aws.Int64(port),
			ToPort:     aws.Int64(port),
			IpProtocol: aws.String("tcp"),
			IpRanges:   ipRanges,
		}
		inSGRule.IpPermissions = append(inSGRule.IpPermissions, newRule)
	}

	_, err = e.AuthorizeSecurityGroupIngress(inSGRule)
	if err != nil {
		return nil, nil, err
	}

	// tag the newly create security group with a name and managed by key
	inTags := &ec2.CreateTagsInput{
		Resources: []*string{oSG.GroupId},
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("Name"),
				Value: sgName,
			},
			{
				Key:   aws.String(ManagedByKey),
				Value: aws.String(ManagedByValue),
			},
		},
	}
	if _, err := e.CreateTags(inTags); err != nil {
		return nil, nil, err
	}

	instanceGroupID, err := e.CreateNewInstanceSG(sgName, oSG.GroupId, vpcID)
	if err != nil {
		return nil, nil, err
	}

	return oSG.GroupId, instanceGroupID, nil
}

func (e *EC2) CreateNewInstanceSG(sgName *string, sgID *string, vpcID *string) (*string, error) {
	// create SG associated with above ALB securty group to attach to instances
	instanceSGName := fmt.Sprintf("%s-%s", instSpecifierTag, *sgName)
	inSG := &ec2.CreateSecurityGroupInput{
		VpcId:       vpcID,
		GroupName:   aws.String(instanceSGName),
		Description: aws.String(instanceSGName),
	}
	oInstanceSG, err := e.CreateSecurityGroup(inSG)
	if err != nil {
		return nil, err
	}

	inSGRule := &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: oInstanceSG.GroupId,
		IpPermissions: []*ec2.IpPermission{
			{
				IpProtocol: aws.String("tcp"),
				ToPort:     aws.Int64(65535),
				FromPort:   aws.Int64(0),
				UserIdGroupPairs: []*ec2.UserIdGroupPair{
					{
						VpcId:   vpcID,
						GroupId: sgID,
					},
				},
			},
		},
	}
	_, err = e.AuthorizeSecurityGroupIngress(inSGRule)
	if err != nil {
		return nil, err
	}

	// tag the newly create security group with a name and managed by key
	inTags := &ec2.CreateTagsInput{
		Resources: []*string{oInstanceSG.GroupId},
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("Name"),
				Value: aws.String(instanceSGName),
			},
			{
				Key:   aws.String(ManagedByKey),
				Value: aws.String(ManagedByValue),
			},
		},
	}
	if _, err := e.CreateTags(inTags); err != nil {
		return nil, err
	}

	return oInstanceSG.GroupId, nil
}

// GetVPCID returns the VPC of the instance the controller is currently running on.
// This is achieved by getting the identity document of the EC2 instance and using
// the DescribeInstances call to determine its VPC ID.
func (e *EC2) GetVPCID() (*string, error) {
	var vpc *string

	if v := os.Getenv("AWS_VPC_ID"); v != "" {
		return &v, nil
	}

	// If previously looked up (and not expired) the VpcId will be stored in the cache under the
	// key 'vpc'.
	cacheName := "EC2.GetVPCID"
	item := albcache.Get(cacheName, "")

	// cache hit: return (pointer of) VpcId value
	if item != nil {
		vpc = item.Value().(*string)
		return vpc, nil
	}

	// cache miss: begin lookup of VpcId based on current EC2 instance
	// retrieve identity of current running instance
	identityDoc, err := EC2Metadatasvc.GetInstanceIdentityDocument()
	if err != nil {
		return nil, err
	}

	// capture instance ID for lookup in DescribeInstances
	// don't bother caching this value as it should never be re-retrieved unless
	// the cache for the VpcId (looked up below) expires.
	descInstancesInput := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(identityDoc.InstanceID)},
	}

	// capture description of this instance for later capture of VpcId
	descInstancesOutput, err := e.DescribeInstances(descInstancesInput)
	if err != nil {
		return nil, err
	}

	// Before attempting to return VpcId of instance, ensure at least 1 reservation and instance
	// (in that reservation) was found.
	if err = instanceVPCIsValid(descInstancesOutput); err != nil {
		return nil, err
	}

	vpc = descInstancesOutput.Reservations[0].Instances[0].VpcId
	// cache the retrieved VpcId for next call
	albcache.Set(cacheName, "", vpc, time.Minute*60)
	return vpc, nil
}

func (e *EC2) GetVPC(id *string) (*ec2.Vpc, error) {
	cacheName := "EC2.GetVPCID"
	item := albcache.Get(cacheName, *id)

	// cache hit: return (pointer of) VpcId value
	if item != nil {
		vpc := item.Value().(*ec2.Vpc)
		return vpc, nil
	}

	o, err := e.DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []*string{id},
	})
	if err != nil {
		return nil, err
	}
	if len(o.Vpcs) != 1 {
		return nil, fmt.Errorf("Invalid amount of VPCs %d returned for %s", len(o.Vpcs), *id)
	}

	albcache.Set(cacheName, *id, o.Vpcs[0], time.Minute*60)
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

// Status validates EC2 connectivity
func (e *EC2) Status() func() error {
	return func() error {
		in := &ec2.DescribeTagsInput{}
		in.SetMaxResults(6)

		if _, err := e.DescribeTags(in); err != nil {
			return fmt.Errorf("[ec2.DescribeTags]: %v", err)
		}
		return nil
	}
}

// ClusterSubnets returns the subnets that are tagged for the cluster
func ClusterSubnets(scheme *string) (util.Subnets, error) {
	var useableSubnets []*ec2.Subnet
	var out util.AWSStringSlice
	var key string

	cacheName := "ClusterSubnets"

	if *scheme == elbv2.LoadBalancerSchemeEnumInternal {
		key = tagNameSubnetInternalELB
	} else if *scheme == elbv2.LoadBalancerSchemeEnumInternetFacing {
		key = tagNameSubnetPublicELB
	} else {
		return nil, fmt.Errorf("Invalid scheme [%s]", *scheme)
	}

	resources, err := albrgt.RGTsvc.GetClusterResources()
	if err != nil {
		return nil, fmt.Errorf("Failed to get AWS tags. Error: %s", err.Error())
	}

	var filterValues []*string
	for arn, subnetTags := range resources.Subnets {
		for _, tag := range subnetTags {
			if *tag.Key == key {
				p := strings.Split(arn, "/")
				subnetID := &p[len(p)-1]
				item := albcache.Get(cacheName, *subnetID)
				if item != nil {
					if subnetIsUsable(item.Value().(*ec2.Subnet), useableSubnets) {
						useableSubnets = append(useableSubnets, item.Value().(*ec2.Subnet))
						out = append(out, item.Value().(*ec2.Subnet).SubnetId)
					}
				} else {
					filterValues = append(filterValues, subnetID)
				}
			}
		}
	}

	if len(filterValues) == 0 {
		sort.Sort(out)
		return util.Subnets(out), nil
	}

	input := &ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   aws.String("subnet-id"),
				Values: filterValues,
			},
		},
	}
	o, err := EC2svc.DescribeSubnets(input)
	if err != nil {
		return nil, fmt.Errorf("Unable to fetch subnets %v: %v", log.Prettify(input.Filters), err)
	}

	for _, subnet := range o.Subnets {
		if subnetIsUsable(subnet, useableSubnets) {
			useableSubnets = append(useableSubnets, subnet)
			out = append(out, subnet.SubnetId)
			albcache.Set(cacheName, *subnet.SubnetId, subnet, time.Minute*60)
		}
	}

	if len(out) < 2 {
		return nil, fmt.Errorf("Retrieval of subnets failed to resolve 2 qualified subnets. Subnets must "+
			"contain the %s/<cluster name> tag with a value of shared or owned and the %s tag signifying it should be used for ALBs "+
			"Additionally, there must be at least 2 subnets with unique availability zones as required by "+
			"ALBs. Either tag subnets to meet this requirement or use the subnets annotation on the "+
			"ingress resource to explicitly call out what subnets to use for ALB creation. The subnets "+
			"that did resolve were %v.", tagNameCluster, tagNameSubnetInternalELB,
			log.Prettify(out))
	}

	sort.Sort(out)
	return util.Subnets(out), nil
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
func (e *EC2) IsNodeHealthy(instanceid string) (bool, error) {
	cacheName := "ec2.IsNodeHealthy"
	item := albcache.Get(cacheName, instanceid)

	if item != nil {
		return item.Value().(bool), nil
	}

	in := &ec2.DescribeInstanceStatusInput{
		InstanceIds: []*string{aws.String(instanceid)},
	}
	o, err := e.DescribeInstanceStatus(in)
	if err != nil {
		return false, fmt.Errorf("Unable to DescribeInstanceStatus on %v: %v", instanceid, err.Error())
	}

	for _, instanceStatus := range o.InstanceStatuses {
		if *instanceStatus.InstanceId != instanceid {
			continue
		}
		if *instanceStatus.InstanceState.Code == 16 { // running
			albcache.Set(cacheName, instanceid, true, IsNodeHealthyCacheTTL)
			return true, nil
		}
		albcache.Set(cacheName, instanceid, false, IsNodeHealthyCacheTTL)
		return false, nil
	}

	return false, nil
}
