package sg

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

// LbAttachment represents the desired SecurityGroups attached to Lb
type LbAttachment struct {
	GroupIDs []string
	LbArn    string
}

// LbAttachmentController controls the LbAttachment
type LbAttachmentController interface {
	// Reconcile ensures the specified SecurityGroups exists in LoadBalancer.
	// other securityGroups are kept in Loadbalancer.
	Reconcile(*LbAttachment) error

	// Delete ensures the specified SecurityGroups doesn't exist in LoadBalancer.
	// any other securityGroups are kept in Loadbalancer.
	Delete(*LbAttachment) error
}

type lbAttachmentController struct {
	elbv2 albelbv2.ELBV2API
}

func (controller *lbAttachmentController) Reconcile(attachment *LbAttachment) error {
	loadBalancer, err := controller.elbv2.GetLoadBalancerByArn(attachment.LbArn)
	if err != nil {
		return err
	}
	if loadBalancer == nil {
		return fmt.Errorf("loadBalancer %s doesn't exists", attachment.LbArn)
	}

	groupIDs := types.UnionAWSStringSlices(loadBalancer.SecurityGroups, aws.StringSlice(attachment.GroupIDs))
	_, err = controller.elbv2.SetSecurityGroups(&elbv2.SetSecurityGroupsInput{
		LoadBalancerArn: aws.String(attachment.LbArn),
		SecurityGroups:  groupIDs,
	})
	return err
}

func (controller *lbAttachmentController) Delete(attachment *LbAttachment) error {
	loadBalancer, err := controller.elbv2.GetLoadBalancerByArn(attachment.LbArn)
	if err != nil {
		return err
	}
	if loadBalancer == nil {
		return fmt.Errorf("loadBalancer %s doesn't exists", attachment.LbArn)
	}
	groupIDs := types.DiffAWSStringSlices(loadBalancer.SecurityGroups, aws.StringSlice(attachment.GroupIDs))
	_, err = controller.elbv2.SetSecurityGroups(&elbv2.SetSecurityGroupsInput{
		LoadBalancerArn: aws.String(attachment.LbArn),
		SecurityGroups:  groupIDs,
	})
	return err
}
