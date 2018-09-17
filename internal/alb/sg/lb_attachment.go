package sg

import (
	"fmt"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"

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

	// Delete ensures specified SecurityGroup don't exists in LoadBalancer, other sg are kept.
	// If there are remaining sg, the default SG for vpc will be kept.
	Delete(*LbAttachment) error
}

type lbAttachmentController struct {
	elbv2  albelbv2.ELBV2API
	ec2    *albec2.EC2
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
		controller.logger.Infof("modify securityGroup on LoadBalancer %s to be %v", attachment.LbArn, attachment.GroupIDs)
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

func (controller *lbAttachmentController) Delete(attachment *LbAttachment) error {
	loadBalancer, err := controller.elbv2.GetLoadBalancerByArn(attachment.LbArn)
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
				return fmt.Errorf("failed to get default securityGroup for current vpc due to %s", err.Error())
			}
			groupsShouldRemain = append(groupsShouldRemain, *defaultSGID)
		}

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

func (controller *lbAttachmentController) getDefaultSecurityGroupID() (*string, error) {
	vpcID, err := controller.ec2.GetVPCID()
	if err != nil {
		return nil, err
	}

	defaultSG, err := controller.ec2.GetSecurityGroupByName(*vpcID, "default")
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
