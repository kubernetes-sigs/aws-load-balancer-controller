package sg

import (
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

// SecurityGroup represents an SecurityGroup resource in AWS
// either GroupID or GroupName must be specified to identify securityGroup
type SecurityGroup struct {
	GroupID            *string
	GroupName          *string
	InboundPermissions []*ec2.IpPermission
}

// SecurityGroupController manages SecurityGroups
type SecurityGroupController interface {
	Reconcile(*SecurityGroup) error
	Delete(*SecurityGroup) error
}

type securityGroupController struct {
	ec2    albec2.EC2
	logger *log.Logger
}

func (controller *securityGroupController) Reconcile(group *SecurityGroup) error {
	sgID, err := controller.resolveExistingSGID(group)
}

func (controller *securityGroupController) Delete(group *SecurityGroup) error {
	sgID, err := controller.resolveExistingSGID(group)
	if err != nil {
		return err
	}
	if sgID != nil {
		return controller.ec2.DeleteSecurityGroupByID(sgID)
	}
	return nil
}

func (controller *securityGroupController) resolveExistingSGID(group *SecurityGroup) (*string, error) {
	sgID := group.GroupID
	if sgID == nil && group.GroupName != nil {
		sgID, err := controller.findSGIDByName(*group.GroupName)
		if err != nil {
			return sgID, err
		}
	}
	return sgID, nil
}

func (controller *securityGroupController) findSGIDByName(sgName string) (*string, error) {
	vpcID, err := controller.ec2.GetVPCID()
	if err != nil {
		return nil, err
	}
	sg, err := controller.ec2.GetSecurityGroupByName(*vpcID, sgName)
	if err != nil {
		return nil, err
	}
	if sg != nil {
		return sg.GroupId, nil
	}
	return nil, nil
}
