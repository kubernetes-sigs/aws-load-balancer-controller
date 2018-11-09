package sg

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"k8s.io/apimachinery/pkg/util/sets"
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
	loadBalancer, err := controller.cloud.GetLoadBalancerByArn(ctx, attachment.LbArn)
	if err != nil {
		return err
	}
	if loadBalancer == nil {
		return fmt.Errorf("loadBalancer %s doesn't exists", attachment.LbArn)
	}

	desiredGroups := sets.NewString(attachment.GroupIDs...)
	currentGroups := sets.NewString(aws.StringValueSlice(loadBalancer.SecurityGroups)...)
	if !desiredGroups.Equal(currentGroups) {
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
	loadBalancer, err := controller.cloud.GetLoadBalancerByArn(ctx, attachment.LbArn)
	if err != nil {
		return err
	}
	if loadBalancer == nil {
		return fmt.Errorf("loadBalancer %s doesn't exist", attachment.LbArn)
	}

	undesiredGroups := sets.NewString(attachment.GroupIDs...)
	currentGroups := sets.NewString(aws.StringValueSlice(loadBalancer.SecurityGroups)...)
	groupsToKeep := currentGroups.Difference(undesiredGroups)
	if len(groupsToKeep) != len(currentGroups) {
		if len(groupsToKeep) == 0 {
			defaultSGID, err := controller.getDefaultSecurityGroupID()
			if err != nil {
				return fmt.Errorf("failed to get default securityGroup for current vpc due to %v", err)
			}
			groupsToKeep.Insert(defaultSGID)
		}
		desiredGroups := groupsToKeep.List()
		albctx.GetLogger(ctx).Infof("modify securityGroup on LoadBalancer %s to be %v", attachment.LbArn, desiredGroups)
		_, err := controller.cloud.SetSecurityGroupsWithContext(ctx, &elbv2.SetSecurityGroupsInput{
			LoadBalancerArn: aws.String(attachment.LbArn),
			SecurityGroups:  aws.StringSlice(desiredGroups),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (controller *lbAttachmentController) getDefaultSecurityGroupID() (string, error) {
	defaultSG, err := controller.cloud.GetSecurityGroupByName("default")
	if err != nil {
		return "", err
	}
	return aws.StringValue(defaultSG.GroupId), nil
}
