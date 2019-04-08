package deploy

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/build"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/logging"
	"strings"
	"time"
)

func NewLBSecurityGroupActuator(cloud cloud.Cloud, tagProvider TagProvider, stack *build.LoadBalancingStack) Actuator {
	return &lbSecurityGroupActuator{
		cloud:       cloud,
		tagProvider: tagProvider,
		stack:       stack,
	}
}

type lbSecurityGroupActuator struct {
	cloud       cloud.Cloud
	tagProvider TagProvider
	stack       *build.LoadBalancingStack

	// the ID of existing Managed LB SecurityGroup found
	existingManagedLBSG *string
}

func (a *lbSecurityGroupActuator) Initialize(ctx context.Context) error {
	sgID, err := a.findManagedLBSecurityGroupIDByTags(ctx)
	if err != nil {
		return err
	}
	a.existingManagedLBSG = sgID

	if a.stack.ManagedLBSecurityGroup != nil {
		if sgID == nil {
			if err := a.reconcileSecurityGroupByCreate(ctx, a.stack.ManagedLBSecurityGroup); err != nil {
				return err
			}
		} else {
			if err := a.reconcileSecurityGroupByUpdate(ctx, a.stack.ManagedLBSecurityGroup, *sgID); err != nil {
				return err
			}
		}
		// TODO(@M0nnF1sh): optimize this code, smells
		if err := a.reconcileInstanceSGIngressRules(ctx, a.stack.ManagedLBSecurityGroup.Status.ID, a.stack.InstanceSecurityGroups); err != nil {
			return err
		}
	}
	return nil
}

// Finalize is responsible for GC extra resources.
func (a *lbSecurityGroupActuator) Finalize(ctx context.Context) error {
	if a.stack.ManagedLBSecurityGroup == nil && a.existingManagedLBSG != nil {
		if err := a.reconcileInstanceSGIngressRules(ctx, *a.existingManagedLBSG, nil); err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		logging.FromContext(ctx).Info("deleting SecurityGroup", "sgID", *a.existingManagedLBSG)
		if err := wait.PollImmediateUntil(2*time.Second, func() (done bool, err error) {
			if _, err := a.cloud.EC2().DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
				GroupId: a.existingManagedLBSG,
			}); err != nil {
				if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == "DependencyViolation" {
					return false, nil
				}
				return false, err
			}
			return true, nil
		}, ctx.Done()); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("deleted SecurityGroup", "sgID", *a.existingManagedLBSG)
	}

	return nil
}

func (a *lbSecurityGroupActuator) findManagedLBSecurityGroupIDByTags(ctx context.Context) (*string, error) {
	tags := a.tagProvider.TagResource(a.stack.ID, build.ResourceIDManagedLBSecurityGroup, nil)
	req := &resourcegroupstaggingapi.GetResourcesInput{
		TagFilters:          cloud.NewRGTTagFilters(tags),
		ResourceTypeFilters: aws.StringSlice([]string{cloud.ResourceTypeEC2SecurityGroup}),
	}
	resources, err := a.cloud.RGT().GetResourcesAsList(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resources) > 1 {
		return nil, errors.Errorf("multiple LB security group found!")
	}
	if len(resources) == 0 {
		return nil, nil
	}
	sgARN, err := arn.Parse(aws.StringValue(resources[0].ResourceARN))
	if err != nil {
		return nil, err
	}
	parts := strings.Split(sgARN.Resource, "/")
	subnetID := parts[len(parts)-1]
	return &subnetID, nil
}

func (a *lbSecurityGroupActuator) reconcileSecurityGroupByCreate(ctx context.Context, sg *api.SecurityGroup) error {
	logging.FromContext(ctx).Info("creating SecurityGroup")
	resp, err := a.cloud.EC2().CreateSecurityGroupWithContext(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(sg.Spec.SecurityGroupName),
		Description: aws.String(sg.Spec.Description),
		VpcId:       aws.String(a.cloud.VpcID()),
	})
	if err != nil {
		return err

	}
	sgID := aws.StringValue(resp.GroupId)
	logging.FromContext(ctx).Info("created SecurityGroup", "sgID", sgID)

	tags := a.tagProvider.TagResource(a.stack.ID, build.ResourceIDManagedLBSecurityGroup, sg.Spec.Tags)
	if err := a.tagProvider.ReconcileEC2Tags(ctx, sgID, tags, nil); err != nil {
		return err
	}

	if err := a.reconcileInboundPermissions(ctx, sg, sgID, nil); err != nil {
		return err
	}

	sg.Status.ID = sgID
	return nil
}

func (a *lbSecurityGroupActuator) reconcileSecurityGroupByUpdate(ctx context.Context, sg *api.SecurityGroup, sgID string) error {
	resp, err := a.cloud.EC2().DescribeSecurityGroupsWithContext(ctx, &ec2.DescribeSecurityGroupsInput{
		GroupIds: aws.StringSlice([]string{sgID}),
	})
	if err != nil {
		return err
	}
	sgInstance := resp.SecurityGroups[0]

	if err := a.reconcileInboundPermissions(ctx, sg, sgID, sgInstance.IpPermissions); err != nil {
		return err
	}

	tags := a.tagProvider.TagResource(a.stack.ID, build.ResourceIDManagedLBSecurityGroup, sg.Spec.Tags)
	if err := a.tagProvider.ReconcileEC2Tags(ctx, sgID, tags, sgInstance.Tags); err != nil {
		return err
	}

	sg.Status.ID = sgID
	return nil
}

func (a *lbSecurityGroupActuator) reconcileInboundPermissions(ctx context.Context, sg *api.SecurityGroup, sgID string, actualPermissions []*ec2.IpPermission) error {
	var expandedActualPermissions []*ec2.IpPermission
	for _, perm := range actualPermissions {
		expandedActualPermissions = append(expandedActualPermissions, expandEC2IPPermission(perm)...)
	}
	desiredPermissions := buildEC2IPPermissions(sg.Spec.Permissions)

	permissionsToRevoke := diffIPPermissions(expandedActualPermissions, desiredPermissions)
	if len(permissionsToRevoke) != 0 {
		logging.FromContext(ctx).Info("revoking inbound permission", "sgID", sgID, "permissions", awsutil.Prettify(permissionsToRevoke))
		if _, err := a.cloud.EC2().RevokeSecurityGroupIngressWithContext(ctx, &ec2.RevokeSecurityGroupIngressInput{
			GroupId:       aws.String(sgID),
			IpPermissions: permissionsToRevoke,
		}); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("revoked inbound permission", "sgID", sgID)
	}
	permissionsToGrant := diffIPPermissions(desiredPermissions, expandedActualPermissions)
	if len(permissionsToGrant) != 0 {
		logging.FromContext(ctx).Info("granting inbound permission", "sgID", sgID, "permissions", awsutil.Prettify(permissionsToGrant))
		if _, err := a.cloud.EC2().AuthorizeSecurityGroupIngressWithContext(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
			GroupId:       aws.String(sgID),
			IpPermissions: permissionsToGrant,
		}); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("granted inbound permission", "sgID", sgID)
	}
	return nil
}

// diffIPPermissions calculates set_difference as source - target
func diffIPPermissions(source []*ec2.IpPermission, target []*ec2.IpPermission) (diffs []*ec2.IpPermission) {
	for _, sPermission := range source {
		containsInTarget := false
		for _, tPermission := range target {
			if ipPermissionEquals(sPermission, tPermission) {
				containsInTarget = true
				break
			}
		}
		if !containsInTarget {
			diffs = append(diffs, sPermission)
		}
	}
	return diffs
}

// ipPermissionEquals test whether two IPPermission instance are equals
func ipPermissionEquals(source *ec2.IpPermission, target *ec2.IpPermission) bool {
	if aws.StringValue(source.IpProtocol) != aws.StringValue(target.IpProtocol) {
		return false
	}
	if aws.Int64Value(source.FromPort) != aws.Int64Value(target.FromPort) {
		return false
	}
	if aws.Int64Value(source.ToPort) != aws.Int64Value(target.ToPort) {
		return false
	}
	if !ipv4RangesEquals(source.IpRanges, target.IpRanges) {
		return false
	}
	if !ipv6RangesEquals(source.Ipv6Ranges, target.Ipv6Ranges) {
		return false
	}
	if !userIDGroupPairsEquals(source.UserIdGroupPairs, target.UserIdGroupPairs) {
		return false
	}

	return true
}

func ipv4RangesEquals(source []*ec2.IpRange, target []*ec2.IpRange) bool {
	sourceRanges := sets.String{}
	for _, ipRange := range source {
		sourceRanges.Insert(aws.StringValue(ipRange.CidrIp))
	}
	targetRanges := sets.String{}
	for _, ipRange := range target {
		targetRanges.Insert(aws.StringValue(ipRange.CidrIp))
	}
	return sourceRanges.Equal(targetRanges)
}

func ipv6RangesEquals(source []*ec2.Ipv6Range, target []*ec2.Ipv6Range) bool {
	sourceRanges := sets.String{}
	for _, ipRange := range source {
		sourceRanges.Insert(aws.StringValue(ipRange.CidrIpv6))
	}
	targetRanges := sets.String{}
	for _, ipRange := range target {
		targetRanges.Insert(aws.StringValue(ipRange.CidrIpv6))
	}
	return sourceRanges.Equal(targetRanges)
}

func userIDGroupPairsEquals(source []*ec2.UserIdGroupPair, target []*ec2.UserIdGroupPair) bool {
	sourceIDs := sets.String{}
	for _, item := range source {
		sourceIDs.Insert(aws.StringValue(item.GroupId))
	}
	targetIDs := sets.String{}
	for _, item := range target {
		targetIDs.Insert(aws.StringValue(item.GroupId))
	}
	return sourceIDs.Equal(targetIDs)
}

func expandEC2IPPermission(permission *ec2.IpPermission) []*ec2.IpPermission {
	var expandedPermissions []*ec2.IpPermission
	master := &ec2.IpPermission{
		FromPort:   permission.FromPort,
		ToPort:     permission.ToPort,
		IpProtocol: permission.IpProtocol,
	}

	for _, ipRange := range permission.IpRanges {
		perm := &ec2.IpPermission{}
		*perm = *master
		perm.IpRanges = []*ec2.IpRange{ipRange}
		expandedPermissions = append(expandedPermissions, perm)
	}

	for _, ipRange := range permission.Ipv6Ranges {
		perm := &ec2.IpPermission{}
		*perm = *master
		perm.Ipv6Ranges = []*ec2.Ipv6Range{ipRange}
		expandedPermissions = append(expandedPermissions, perm)
	}

	for _, ug := range permission.UserIdGroupPairs {
		perm := &ec2.IpPermission{}
		*perm = *master
		perm.UserIdGroupPairs = []*ec2.UserIdGroupPair{ug}
		expandedPermissions = append(expandedPermissions, perm)
	}

	if len(expandedPermissions) == 0 {
		expandedPermissions = append(expandedPermissions, permission)
	}
	return expandedPermissions
}

func buildEC2IPPermissions(permissions []api.IPPermission) []*ec2.IpPermission {
	ec2Permissions := make([]*ec2.IpPermission, 0, len(permissions))
	for _, permission := range permissions {
		ec2Permission := &ec2.IpPermission{
			FromPort:   aws.Int64(permission.FromPort),
			ToPort:     aws.Int64(permission.ToPort),
			IpProtocol: aws.String(permission.IPProtocol.String()),
		}
		if len(permission.CIDRIP) != 0 {
			ec2Permission.IpRanges = []*ec2.IpRange{{
				CidrIp:      aws.String(permission.CIDRIP),
				Description: aws.String(permission.Description),
			}}
		}
		if len(permission.CIDRIPV6) != 0 {
			ec2Permission.Ipv6Ranges = []*ec2.Ipv6Range{{
				CidrIpv6:    aws.String(permission.CIDRIPV6),
				Description: aws.String(permission.Description),
			}}
		}

		ec2Permissions = append(ec2Permissions, ec2Permission)
	}
	return ec2Permissions
}

func (a *lbSecurityGroupActuator) reconcileInstanceSGIngressRules(ctx context.Context, lbSGID string, instanceSecurityGroups []string) error {
	instances, err := a.cloud.EC2().DescribeSecurityGroupsAsList(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("ip-permission.group-id"),
				Values: aws.StringSlice([]string{lbSGID}),
			},
		},
	})
	if err != nil {
		return err
	}

	desiredInstanceSGs := sets.NewString(instanceSecurityGroups...)
	// TODO(@M00nF1sh): check the permission really matches the permission we grant,
	//  and raise an error to alert user, if they manually created rule that referenced this lbSGID.
	currentInstanceSGs := sets.String{}
	for _, instance := range instances {
		currentInstanceSGs.Insert(aws.StringValue(instance.GroupId))
	}

	permissions := []*ec2.IpPermission{
		{
			IpProtocol: aws.String("-1"),
			UserIdGroupPairs: []*ec2.UserIdGroupPair{
				{
					GroupId: aws.String(lbSGID),
				},
			},
		},
	}
	for sgID := range desiredInstanceSGs.Difference(currentInstanceSGs) {
		logging.FromContext(ctx).Info("Authorizing inbound permission to instance SG", "lbSG", lbSGID, "instanceSG", sgID)
		if _, err := a.cloud.EC2().AuthorizeSecurityGroupIngressWithContext(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
			GroupId:       aws.String(sgID),
			IpPermissions: permissions,
		}); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("Authorized inbound permission to instance SG", "lbSG", lbSGID, "instanceSG", sgID)
	}

	for sgID := range currentInstanceSGs.Difference(desiredInstanceSGs) {
		logging.FromContext(ctx).Info("revoking inbound permission to instance SG", "lbSG", lbSGID, "instanceSG", sgID)
		if _, err := a.cloud.EC2().RevokeSecurityGroupIngressWithContext(ctx, &ec2.RevokeSecurityGroupIngressInput{
			GroupId:       aws.String(sgID),
			IpPermissions: permissions,
		}); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("revoked inbound permission to instance SG", "lbSG", lbSGID, "instanceSG", sgID)
	}

	return nil
}
