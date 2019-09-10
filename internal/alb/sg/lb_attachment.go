package sg

import (
	"context"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"k8s.io/apimachinery/pkg/util/sets"
)

// LbAttachmentController controls the LbAttachment
type LbAttachmentController interface {
	// Reconcile ensures `only specified SecurityGroups` exists in LoadBalancer.
	Reconcile(ctx context.Context, lbInstance *elbv2.LoadBalancer, groupIDs []string) error
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
