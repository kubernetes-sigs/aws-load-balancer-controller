package sg

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

// Association represents the desired state of securityGroups & attachments for an Ingress resource.
type Association struct {
	LbID  string
	LbArn string

	LbPorts        []int64
	LbInboundCIDRs types.Cidrs
	ExternalSGIDs  []string

	Targets tg.TargetGroups
}

// AssociationController provides functionality to manage Association
type AssociationController interface {
	Reconcile(*Association) error
	Delete(*Association) error
}

type associationController struct {
	lbAttachmentController       LbAttachmentController
	instanceAttachmentController InstanceAttachementController
	sgController                 SecurityGroupController
	namer                        Namer
	ec2                          albec2.EC2
	logger                       *log.Logger
}

func (controller *associationController) Reconcile(association *Association) error {
	if len(association.ExternalSGIDs) != 0 {
		return controller.reconcileWithExternalSGs(association)
	}
	return controller.reconcileWithManagedSGs(association)
}

func (controller *associationController) Delete(association *Association) error {
	if len(association.ExternalSGIDs) != 0 {
		return controller.deleteWithExternalSGs(association)
	}
	return controller.deleteWithManagedSGs(association)
}

func (controller *associationController) reconcileWithExternalSGs(association *Association) error {
	lbSGAttachment := &LbAttachment{
		GroupIDs: association.ExternalSGIDs,
		LbArn:    association.LbArn,
	}
	err := controller.lbAttachmentController.Reconcile(lbSGAttachment)
	if err != nil {
		return fmt.Errorf("Failed to reconcile external LoadBalancer securityGroup, Error:%s", err.Error())
	}
	return nil
}

func (controller *associationController) reconcileWithManagedSGs(association *Association) error {
	lbSG, err := controller.reconcileManagedLbSG(association)
	if err != nil {
		return err
	}
	err = controller.reconcileManagedInstanceSG(association, lbSG)
	if err != nil {
		return err
	}
	return nil
}

func (controller *associationController) deleteWithExternalSGs(association *Association) error {
	lbSGAttachment := &LbAttachment{
		GroupIDs: association.ExternalSGIDs,
		LbArn:    association.LbArn,
	}
	err := controller.lbAttachmentController.Delete(lbSGAttachment)
	if err != nil {
		return fmt.Errorf("Failed to delete external LoadBalancer securityGroup attachment, Error:%s", err.Error())
	}
	return nil
}

func (controller *associationController) deleteWithManagedSGs(association *Association) error {
	err := controller.deleteManagedLbSG(association)
	if err != nil {
		return fmt.Errorf("Failed to delete managed LoadBalancer securityGroup, Error:%s", err.Error())
	}
	err = controller.deleteManagedInstanceSG(association)
	if err != nil {
		return fmt.Errorf("Failed to delete managed Instance securityGroup, Error:%s", err.Error())
	}
	return nil
}

func (controller *associationController) reconcileManagedLbSG(association *Association) (*SecurityGroup, error) {
	lbSGName := controller.namer.NameLbSG(association.LbID)
	lbSG := &SecurityGroup{
		GroupName: &lbSGName,
	}
	for _, port := range association.LbPorts {
		ipRanges := []*ec2.IpRange{}
		for _, cidr := range association.LbInboundCIDRs {
			ipRanges = append(ipRanges, &ec2.IpRange{
				CidrIp:      cidr,
				Description: aws.String(fmt.Sprintf("Allow ingress on port %v from %v.", port, aws.StringValue(cidr))),
			})
		}
		rule := &ec2.IpPermission{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int64(port),
			ToPort:     aws.Int64(port),
			IpRanges:   ipRanges,
		}
		lbSG.InboundPermissions = append(lbSG.InboundPermissions, rule)
	}

	err := controller.sgController.Reconcile(lbSG)
	if err != nil {
		return lbSG, fmt.Errorf("Failed to reconcile managed LoadBalancer securityGroup, Error:%s", err.Error())
	}
	lbSGAttachment := &LbAttachment{
		GroupIDs: []string{*lbSG.GroupID},
		LbArn:    association.LbArn,
	}
	err = controller.lbAttachmentController.Reconcile(lbSGAttachment)
	if err != nil {
		return lbSG, fmt.Errorf("Failed to reconcile managed LoadBalancer securityGroup attachment, Error:%s", err.Error())
	}
	return lbSG, nil
}

func (controller *associationController) reconcileManagedInstanceSG(association *Association, lbSG *SecurityGroup) error {
	vpcID, err := controller.ec2.GetVPCID()
	if err != nil {
		return err
	}
	instanceSGName := controller.namer.NameInstanceSG(association.LbID)
	instanceSG := &SecurityGroup{
		GroupName: &instanceSGName,
		InboundPermissions: []*ec2.IpPermission{
			{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int64(0),
				ToPort:     aws.Int64(65535),
				UserIdGroupPairs: []*ec2.UserIdGroupPair{
					{
						VpcId:   vpcID,
						GroupId: lbSG.GroupID,
					},
				},
			},
		},
	}
	err = controller.sgController.Reconcile(instanceSG)
	if err != nil {
		return fmt.Errorf("Failed to reconcile managed Instance securityGroup, Error:%s", err.Error())
	}
	instanceSGAttachment := &InstanceAttachment{
		GroupID: *instanceSG.GroupID,
		Targets: association.Targets,
	}
	err = controller.instanceAttachmentController.Reconcile(instanceSGAttachment)
	if err != nil {
		return fmt.Errorf("Failed to reconcile managed Instance securityGroup attachment, Error:%s", err.Error())
	}
	return nil
}

func (controller *associationController) deleteManagedLbSG(association *Association) error {
	lbSGName := controller.namer.NameLbSG(association.LbID)
	lbSGID, err := controller.findSGIDByName(lbSGName)
	if err != nil {
		return err
	}
	if lbSGID == nil {
		return nil
	}
	lbSGAttachment := &LbAttachment{
		GroupIDs: []string{*lbSGID},
		LbArn:    association.LbArn,
	}
	err = controller.lbAttachmentController.Delete(lbSGAttachment)
	if err != nil {
		return err
	}
	lbSG := &SecurityGroup{
		GroupID: lbSGID,
	}
	err = controller.sgController.Delete(lbSG)
	if err != nil {
		return err
	}
	return nil
}

func (controller *associationController) deleteManagedInstanceSG(association *Association) error {
	instanceSGName := controller.namer.NameInstanceSG(association.LbID)
	instanceSGID, err := controller.findSGIDByName(instanceSGName)
	if err != nil {
		return err
	}
	if instanceSGID == nil {
		return nil
	}
	instanceSGAttachment := &InstanceAttachment{
		GroupID: *instanceSGID,
	}
	err = controller.instanceAttachmentController.Delete(instanceSGAttachment)
	if err != nil {
		return err
	}
	instanceSG := &SecurityGroup{
		GroupID: instanceSGID,
	}
	err = controller.sgController.Delete(instanceSG)
	if err != nil {
		return err
	}
	return nil
}

func (controller *associationController) findSGIDByName(sgName string) (*string, error) {
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
