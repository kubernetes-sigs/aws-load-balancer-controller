package sg

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

// LbAttachment represents the desired SecurityGroups attached to Lb
type LbAttachment struct {
	GroupIDs []string
	LbArn    string
}

// LbAttachmentController controls the LbAttachment
type LbAttachmentController interface {
	// Reconcile ensures `only specified SecurityGroups` exists in LoadBalancer.
	Reconcile(*LbAttachment) error
}

type lbAttachmentController struct {
	elbv2  albelbv2.ELBV2API
	logger *log.Logger
}

func (controller *lbAttachmentController) Reconcile(attachment *LbAttachment) error {
	loadBalancer, err := controller.elbv2.GetLoadBalancerByArn(attachment.LbArn)
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
		controller.logger.Infof("modify securityGroup of loadBalancer %s to %#v", attachment.LbArn, attachment.GroupIDs)
		_, err := controller.elbv2.SetSecurityGroups(&elbv2.SetSecurityGroupsInput{
			LoadBalancerArn: aws.String(attachment.LbArn),
			SecurityGroups:  aws.StringSlice(attachment.GroupIDs),
		})
		if err != nil {
			return err
		}
	}
	return nil
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
