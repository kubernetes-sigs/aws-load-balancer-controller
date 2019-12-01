package sg

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/errors"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

func NewInstanceAttachmentControllerV2(
	sgController SecurityGroupController,
	targetENIsResolver TargetENIsResolver,
	nameTagGen NameTagGenerator,
	store store.Storer,
	cloud aws.CloudAPI) InstanceAttachmentController {

	return &instanceAttachmentControllerV2{
		sgController:       sgController,
		targetENIsResolver: targetENIsResolver,
		nameTagGen:         nameTagGen,
		store:              store,
		cloud:              cloud,
	}
}

// instanceAttachmentControllerV2 will re-use existing securityGroups on ENIs,
// and modify the rules on them to allow traffic from LB securityGroup.
type instanceAttachmentControllerV2 struct {
	sgController       SecurityGroupController
	targetENIsResolver TargetENIsResolver
	nameTagGen         NameTagGenerator

	store store.Storer
	cloud aws.CloudAPI
}

func (c *instanceAttachmentControllerV2) Reconcile(ctx context.Context, ingKey types.NamespacedName, lbSGID string, tgGroup tg.TargetGroupGroup) error {
	targetInstanceSGs, err := c.findInstanceSGsForTgGroup(ctx, tgGroup)
	if err != nil {
		return err
	}
	targetInstanceSGIDs := sets.StringKeySet(targetInstanceSGs)

	attachedInstanceSGs, err := c.findInstanceSGsAttachedWithLBSG(ctx, lbSGID)
	if err != nil {
		return err
	}
	attachedInstanceSGIDs := sets.StringKeySet(attachedInstanceSGs)

	shouldAttachInstanceSGs := targetInstanceSGIDs.Difference(attachedInstanceSGIDs)
	for instanceSGID := range shouldAttachInstanceSGs {
		if err := c.ensureLBSGAttachedToInstanceSG(ctx, lbSGID, targetInstanceSGs[instanceSGID]); err != nil {
			return err
		}
	}

	shouldDetachENIIDs := attachedInstanceSGIDs.Difference(targetInstanceSGIDs)
	for instanceSGID := range shouldDetachENIIDs {
		if err := c.ensureLBSGDetachedFromInstanceSG(ctx, lbSGID, attachedInstanceSGs[instanceSGID]); err != nil {
			return err
		}
	}
	return nil
}

func (c *instanceAttachmentControllerV2) Delete(ctx context.Context, ingKey types.NamespacedName) error {
	sgName := c.nameTagGen.NameLBSG(ingKey.Namespace, ingKey.Name)
	sgInstance, err := c.cloud.GetSecurityGroupByName(sgName)
	if err != nil {
		return err
	}
	if sgInstance == nil {
		return nil
	}
	lbSGID := aws.StringValue(sgInstance.GroupId)
	attachedInstanceSGs, err := c.findInstanceSGsAttachedWithLBSG(ctx, lbSGID)
	if err != nil {
		return err
	}

	for _, instanceSG := range attachedInstanceSGs {
		if err := c.ensureLBSGDetachedFromInstanceSG(ctx, lbSGID, instanceSG); err != nil {
			return err
		}
	}
	return nil
}

func (c *instanceAttachmentControllerV2) findInstanceSGsForTgGroup(ctx context.Context, tgGroup tg.TargetGroupGroup) (map[string]*ec2.SecurityGroup, error) {
	targetENIs, err := c.targetENIsResolver.Resolve(ctx, tgGroup)
	if err != nil {
		return nil, err
	}

	sgIDs := sets.NewString()
	for _, eni := range targetENIs {
		sgIDs.Insert(eni.SecurityGroups()...)
	}
	if len(sgIDs) == 0 {
		return nil, nil
	}
	sgs, err := c.cloud.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		GroupIds: aws.StringSlice(sgIDs.List()),
	})
	if err != nil {
		return nil, err
	}
	if len(sgIDs) != len(sgs) {
		return nil, errors.Errorf("failed to describe all securityGroups, expect: %v, got: %v", sgIDs, len(sgs))
	}

	sgByID := make(map[string]*ec2.SecurityGroup, len(sgs))
	for _, sg := range sgs {
		sgByID[aws.StringValue(sg.GroupId)] = sg
	}

	clusterTag := "kubernetes.io/cluster/" + c.cloud.GetClusterName()
	instanceSGIDs := sets.NewString()
	for eniID, eni := range targetENIs {
		eniSGIDs := eni.SecurityGroups()
		if len(eniSGIDs) == 1 {
			instanceSGIDs.Insert(eniSGIDs[0])
			continue
		}
		var instanceSGIDsWithClusterTag []string
		for _, eniSGID := range eniSGIDs {
			instanceSG := sgByID[eniSGID]
			for _, tag := range instanceSG.Tags {
				if aws.StringValue(tag.Key) == clusterTag {
					instanceSGIDsWithClusterTag = append(instanceSGIDsWithClusterTag, eniSGID)
					break
				}
			}
		}
		if len(instanceSGIDsWithClusterTag) != 1 {
			return nil, errors.Errorf("expect one securityGroup tagged with %v on eni %v, got %v",
				clusterTag, eniID, len(instanceSGIDsWithClusterTag),
			)
		}
		instanceSGIDs.Insert(instanceSGIDsWithClusterTag[0])
	}

	result := make(map[string]*ec2.SecurityGroup, len(instanceSGIDs))
	for instanceSGID := range instanceSGIDs {
		result[instanceSGID] = sgByID[instanceSGID]
	}
	return result, nil
}

func (c *instanceAttachmentControllerV2) findInstanceSGsAttachedWithLBSG(ctx context.Context, lbSGID string) (map[string]*ec2.SecurityGroup, error) {
	sgs, err := c.cloud.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("ip-permission.group-id"),
				Values: aws.StringSlice([]string{lbSGID}),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	sgByID := make(map[string]*ec2.SecurityGroup, len(sgs))
	for _, sg := range sgs {
		sgByID[aws.StringValue(sg.GroupId)] = sg
	}
	return sgByID, nil
}

func (c *instanceAttachmentControllerV2) ensureLBSGAttachedToInstanceSG(ctx context.Context, lbSGID string, instanceSG *ec2.SecurityGroup) error {
	inboundPermissions := []*ec2.IpPermission{
		{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int64(0),
			ToPort:     aws.Int64(65535),
			UserIdGroupPairs: []*ec2.UserIdGroupPair{
				{
					GroupId: aws.String(lbSGID),
				},
			},
		},
	}

	albctx.GetLogger(ctx).Infof("granting inbound permissions to securityGroup %s: %v", aws.StringValue(instanceSG.GroupId), log.Prettify(inboundPermissions))
	if _, err := c.cloud.AuthorizeSecurityGroupIngressWithContext(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId:       instanceSG.GroupId,
		IpPermissions: inboundPermissions,
	}); err != nil {
		return fmt.Errorf("failed to grant inbound permissions due to %v", err)
	}
	return nil
}

func (c *instanceAttachmentControllerV2) ensureLBSGDetachedFromInstanceSG(ctx context.Context, lbSGID string, instanceSG *ec2.SecurityGroup) error {
	inboundPermissions := []*ec2.IpPermission{
		{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int64(0),
			ToPort:     aws.Int64(65535),
			UserIdGroupPairs: []*ec2.UserIdGroupPair{
				{
					GroupId: aws.String(lbSGID),
				},
			},
		},
	}

	albctx.GetLogger(ctx).Infof("revoking inbound permissions from securityGroup %s: %v", aws.StringValue(instanceSG.GroupId), log.Prettify(inboundPermissions))
	if _, err := c.cloud.RevokeSecurityGroupIngressWithContext(ctx, &ec2.RevokeSecurityGroupIngressInput{
		GroupId:       instanceSG.GroupId,
		IpPermissions: inboundPermissions,
	}); err != nil {
		return fmt.Errorf("failed to revoke inbound permissions due to %v", err)
	}
	return nil
}
