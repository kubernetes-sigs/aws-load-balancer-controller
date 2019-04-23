package sg

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"k8s.io/apimachinery/pkg/util/sets"
)

// LbAttachmentController controls the LbAttachment
type LbAttachmentController interface {
	// Reconcile ensures `only specified SecurityGroups` exists in LoadBalancer.
	Reconcile(ctx context.Context, lbInstance *elbv2.LoadBalancer, groupIDs []string) error

	// Delete will restore the securityGroup on LoadBalancer to be default securityGroup of VPC
	Delete(ctx context.Context, lbInstance *elbv2.LoadBalancer) error
}

type lbAttachmentController struct {
	cloud aws.CloudAPI
}

func (controller *lbAttachmentController) Reconcile(ctx context.Context, lbInstance *elbv2.LoadBalancer, groupIDs []string) error {
	desiredGroups := sets.NewString(groupIDs...)
	currentGroups := sets.NewString(aws.StringValueSlice(lbInstance.SecurityGroups)...)
	if !desiredGroups.Equal(currentGroups) {
		albctx.GetLogger(ctx).Infof("modify securityGroup on LoadBalancer %s to be %v", aws.StringValue(lbInstance.LoadBalancerArn), desiredGroups.List())
		if _, err := controller.cloud.SetSecurityGroupsWithContext(ctx, &elbv2.SetSecurityGroupsInput{
			LoadBalancerArn: lbInstance.LoadBalancerArn,
			SecurityGroups:  aws.StringSlice(desiredGroups.List()),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (controller *lbAttachmentController) Delete(ctx context.Context, lbInstance *elbv2.LoadBalancer) error {
	defaultSGID, err := controller.getDefaultSecurityGroupID()
	if err != nil {
		return fmt.Errorf("failed to get default securityGroup for current vpc due to %v", err)
	}
	desiredGroups := sets.NewString(defaultSGID)
	currentGroups := sets.NewString(aws.StringValueSlice(lbInstance.SecurityGroups)...)
	if !desiredGroups.Equal(currentGroups) {
		albctx.GetLogger(ctx).Infof("modify securityGroup on LoadBalancer %s to be %v", aws.StringValue(lbInstance.LoadBalancerArn), desiredGroups.List())
		if _, err := controller.cloud.SetSecurityGroupsWithContext(ctx, &elbv2.SetSecurityGroupsInput{
			LoadBalancerArn: lbInstance.LoadBalancerArn,
			SecurityGroups:  aws.StringSlice(desiredGroups.List()),
		}); err != nil {
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
	if defaultSG == nil {
		return "", errors.New("default security group not found")
	}
	return aws.StringValue(defaultSG.GroupId), nil
}
