package sg

import (
	"context"
	"fmt"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"

	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
)

// LbAttachment represents the desired SecurityGroups attached to Lb
type LbAttachment struct {
	GroupIDs []string
	LbArn    string
}

// LbAttachmentController controls the LbAttachment
type LbAttachmentController interface {
	// Reconcile ensures `only specified SecurityGroups` exists in LoadBalancer.
	Reconcile(context.Context, *LbAttachment) error

	// Delete ensures specified SecurityGroup don't exists in LoadBalancer, other sg are kept.
	// If there are remaining sg, the default SG for vpc will be kept.
	Delete(context.Context, *LbAttachment) error
}

type lbAttachmentController struct {
	cloud aws.CloudAPI
}

func (controller *lbAttachmentController) Reconcile(ctx context.Context, attachment *LbAttachment) error {
	loadBalancer, err := controller.cloud.GetLoadBalancerByArn(attachment.LbArn)
	if err != nil {
		return err
	}
	if loadBalancer == nil {
		return fmt.Errorf("loadBalancer %s doesn't exists", attachment.LbArn)
	}

	groupsInLb := aws.StringValueSlice(loadBalancer.SecurityGroups)
	groupsToAdd := diffStringSet(attachment.GroupIDs, groupsInLb)
	groupsToDelete := diffStringSet(groupsInLb, attachment.GroupIDs)
	if len(groupsToAdd) != 0 || len(groupsToDelete) != 0 {
		albctx.GetLogger(ctx).Infof("modify securityGroup on LoadBalancer %s to be %v", attachment.LbArn, attachment.GroupIDs)
		_, err := controller.cloud.SetSecurityGroupsWithContext(ctx, &elbv2.SetSecurityGroupsInput{
			LoadBalancerArn: aws.String(attachment.LbArn),
			SecurityGroups:  aws.StringSlice(attachment.GroupIDs),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (controller *lbAttachmentController) Delete(ctx context.Context, attachment *LbAttachment) error {
	loadBalancer, err := controller.cloud.GetLoadBalancerByArn(attachment.LbArn)
	if err != nil {
		return err
	}
	if loadBalancer == nil {
		return fmt.Errorf("loadBalancer %s doesn't exist", attachment.LbArn)
	}

	groupsInLb := aws.StringValueSlice(loadBalancer.SecurityGroups)
	groupsShouldRemain := diffStringSet(groupsInLb, attachment.GroupIDs)
	if len(groupsShouldRemain) != len(groupsInLb) {
		if len(groupsShouldRemain) == 0 {
			defaultSGID, err := controller.getDefaultSecurityGroupID()
			if err != nil {
				return fmt.Errorf("failed to get default securityGroup for current vpc due to %v", err)
			}
			groupsShouldRemain = append(groupsShouldRemain, *defaultSGID)
		}

		albctx.GetLogger(ctx).Infof("modify securityGroup on LoadBalancer %s to be %v", attachment.LbArn, groupsShouldRemain)
		_, err := controller.cloud.SetSecurityGroupsWithContext(ctx, &elbv2.SetSecurityGroupsInput{
			LoadBalancerArn: aws.String(attachment.LbArn),
			SecurityGroups:  aws.StringSlice(groupsShouldRemain),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (controller *lbAttachmentController) getDefaultSecurityGroupID() (*string, error) {
	vpcID, err := controller.cloud.GetVPCID()
	if err != nil {
		return nil, err
	}

	defaultSG, err := controller.cloud.GetSecurityGroupByName(*vpcID, "default")
	if err != nil {
		return nil, err
	}
	return defaultSG.GroupId, nil
}

// diffStringSet calcuates the set_difference as source - target
func diffStringSet(source []string, target []string) (diffs []string) {
	targetSet := make(map[string]bool)
	for _, t := range target {
		targetSet[t] = true
	}
	for _, s := range source {
		if _, ok := targetSet[s]; !ok {
			diffs = append(diffs, s)
		}
	}
	return diffs
}
