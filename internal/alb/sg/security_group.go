package sg

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

// SecurityGroupController manages configuration on securityGroup.
type SecurityGroupController interface {
	// EnsureSGInstance ensures security group with name exists.
	EnsureSGInstanceByName(ctx context.Context, name string, description string) (*ec2.SecurityGroup, error)

	// Reconcile ensures the securityGroup configuration matches specification.
	Reconcile(ctx context.Context, instance *ec2.SecurityGroup, inboundPermissions []*ec2.IpPermission, tags map[string]string) error
}

type securityGroupController struct {
	cloud          aws.CloudAPI
	tagsController tags.Controller
}

func (c *securityGroupController) EnsureSGInstanceByName(ctx context.Context, name string, description string) (*ec2.SecurityGroup, error) {
	sgInstance, err := c.cloud.GetSecurityGroupByName(name)
	if err != nil {
		return nil, err
	}
	if sgInstance != nil {
		return sgInstance, nil
	}
	albctx.GetLogger(ctx).Infof("creating securityGroup %v:%v", name, description)
	resp, err := c.cloud.CreateSecurityGroupWithContext(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(name),
		Description: aws.String(description),
	})
	if err != nil {
		return nil, err
	}
	return &ec2.SecurityGroup{
		GroupId:   resp.GroupId,
		GroupName: aws.String(name),
	}, nil
}

func (c *securityGroupController) Reconcile(ctx context.Context, sgInstance *ec2.SecurityGroup, inboundPermissions []*ec2.IpPermission, tags map[string]string) error {
	if err := c.reconcileTags(ctx, sgInstance, tags); err != nil {
		return err
	}
	if err := c.reconcileInboundPermissions(ctx, sgInstance, inboundPermissions); err != nil {
		return err
	}
	return nil
}

// reconcileInboundPermissions ensures inboundPermissions on securityGroup matches desired.
func (c *securityGroupController) reconcileInboundPermissions(ctx context.Context, sgInstance *ec2.SecurityGroup, inboundPermissions []*ec2.IpPermission) error {
	permissionsToRevoke := diffIPPermissions(sgInstance.IpPermissions, inboundPermissions)
	if len(permissionsToRevoke) != 0 {
		albctx.GetLogger(ctx).Infof("revoking inbound permissions from securityGroup %s: %v", aws.StringValue(sgInstance.GroupId), log.Prettify(permissionsToRevoke))
		if _, err := c.cloud.RevokeSecurityGroupIngressWithContext(ctx, &ec2.RevokeSecurityGroupIngressInput{
			GroupId:       sgInstance.GroupId,
			IpPermissions: permissionsToRevoke,
		}); err != nil {
			return fmt.Errorf("failed to revoke inbound permissions due to %v", err)
		}
	}

	permissionsToGrant := diffIPPermissions(inboundPermissions, sgInstance.IpPermissions)
	if len(permissionsToGrant) != 0 {
		albctx.GetLogger(ctx).Infof("granting inbound permissions to securityGroup %s: %v", aws.StringValue(sgInstance.GroupId), log.Prettify(permissionsToGrant))
		if _, err := c.cloud.AuthorizeSecurityGroupIngressWithContext(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
			GroupId:       sgInstance.GroupId,
			IpPermissions: permissionsToGrant,
		}); err != nil {
			return fmt.Errorf("failed to grant inbound permissions due to %v", err)
		}
	}

	return nil
}

// reconcileTags ensures tags on securityGroup matches desired.
func (c *securityGroupController) reconcileTags(ctx context.Context, sgInstance *ec2.SecurityGroup, tags map[string]string) error {
	curTags := make(map[string]string, len(sgInstance.Tags))
	for _, tag := range sgInstance.Tags {
		curTags[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
	}
	if err := c.tagsController.ReconcileEC2WithCurTags(ctx, aws.StringValue(sgInstance.GroupId), tags, curTags); err != nil {
		return fmt.Errorf("failed to reconcile tags due to %v", err)
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
	if len(diffIPRanges(source.IpRanges, target.IpRanges)) != 0 {
		return false
	}
	if len(diffIPRanges(target.IpRanges, source.IpRanges)) != 0 {
		return false
	}
	if len(diffIPv6Ranges(source.Ipv6Ranges, target.Ipv6Ranges)) != 0 {
		return false
	}
	if len(diffIPv6Ranges(target.Ipv6Ranges, source.Ipv6Ranges)) != 0 {
		return false
	}
	if len(diffUserIDGroupPairs(source.UserIdGroupPairs, target.UserIdGroupPairs)) != 0 {
		return false
	}
	if len(diffUserIDGroupPairs(target.UserIdGroupPairs, source.UserIdGroupPairs)) != 0 {
		return false
	}

	return true
}

// diffIPv6Ranges calculates set_difference as source - target
func diffIPv6Ranges(source []*ec2.Ipv6Range, target []*ec2.Ipv6Range) (diffs []*ec2.Ipv6Range) {
	for _, sRange := range source {
		containsInTarget := false
		for _, tRange := range target {
			if ipRangeEquals(sRange.CidrIpv6, tRange.CidrIpv6) {
				containsInTarget = true
				break
			}
		}
		if !containsInTarget {
			diffs = append(diffs, sRange)
		}
	}
	return diffs
}

// diffIPRanges calculates set_difference as source - target
func diffIPRanges(source []*ec2.IpRange, target []*ec2.IpRange) (diffs []*ec2.IpRange) {
	for _, sRange := range source {
		containsInTarget := false
		for _, tRange := range target {
			if ipRangeEquals(sRange.CidrIp, tRange.CidrIp) {
				containsInTarget = true
				break
			}
		}
		if !containsInTarget {
			diffs = append(diffs, sRange)
		}
	}
	return diffs
}

// ipRangeEquals test whether two IPRange instance are equals
func ipRangeEquals(source *string, target *string) bool {
	return aws.StringValue(source) == aws.StringValue(target)
}

// diffUserIDGroupPairs calculates set_difference as source - target
func diffUserIDGroupPairs(source []*ec2.UserIdGroupPair, target []*ec2.UserIdGroupPair) (diffs []*ec2.UserIdGroupPair) {
	for _, sPair := range source {
		containsInTarget := false
		for _, tPair := range target {
			if userIDGroupPairEquals(sPair, tPair) {
				containsInTarget = true
				break
			}
		}
		if !containsInTarget {
			diffs = append(diffs, sPair)
		}
	}
	return diffs
}

// userIDGroupPairEquals test whether two UserIdGroupPair equals
// currently we only check for groupId
func userIDGroupPairEquals(source *ec2.UserIdGroupPair, target *ec2.UserIdGroupPair) bool {
	return aws.StringValue(source.GroupId) == aws.StringValue(target.GroupId)
}
