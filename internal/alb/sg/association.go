package sg

import (
	"context"
	"fmt"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

// Association represents the desired state of securityGroups & attachments for an Ingress resource.
type Association struct {
	// We identify Association by LbID
	LbID string

	LbArn          string
	LbPorts        []int64
	LbInboundCIDRs types.Cidrs

	// ExternalSGIDs are custom securityGroups intended to be attached to LoadBalancer.
	// If customers specified these securityGroups via annotation on ingress, the ingress controller will then stop creating securityGroups for loadbalancer or ec2-instances.
	ExternalSGIDs []string

	Targets tg.TargetGroups
}

// AssociationController provides functionality to manage Association
type AssociationController interface {
	// Reconcile ensured the securityGroups in AWS matches the state specified by assocation.
	Reconcile(context.Context, *Association) error

	// Delete ensures the securityGroups created by ingress controller for specified LbID doesn't exists.
	Delete(context.Context, *Association) error
}

// NewAssociationController constructs a new association controller
func NewAssociationController(store store.Storer, ec2 albec2.EC2API, elbv2 albelbv2.ELBV2API) AssociationController {
	lbAttachmentController := &lbAttachmentController{
		elbv2: elbv2,
		ec2:   ec2,
	}
	instanceAttachmentController := &instanceAttachmentController{
		store: store,
		ec2:   ec2,
	}
	sgController := &securityGroupController{
		ec2: ec2,
	}
	namer := &namer{}
	return &associationController{
		lbAttachmentController:       lbAttachmentController,
		instanceAttachmentController: instanceAttachmentController,
		sgController:                 sgController,
		namer:                        namer,
		ec2:                          ec2,
	}
}

type associationController struct {
	lbAttachmentController       LbAttachmentController
	instanceAttachmentController InstanceAttachementController
	sgController                 SecurityGroupController
	namer                        Namer
	ec2                          albec2.EC2API
}

func (controller *associationController) Reconcile(ctx context.Context, association *Association) error {
	if len(association.ExternalSGIDs) != 0 {
		return controller.reconcileWithExternalSGs(ctx, association)
	}
	return controller.reconcileWithManagedSGs(ctx, association)
}

func (controller *associationController) Delete(ctx context.Context, association *Association) error {
	return controller.deletedManagedSGs(ctx, association)
}

func (controller *associationController) reconcileWithExternalSGs(ctx context.Context, association *Association) error {
	lbSGAttachment := &LbAttachment{
		GroupIDs: association.ExternalSGIDs,
		LbArn:    association.LbArn,
	}
	err := controller.lbAttachmentController.Reconcile(ctx, lbSGAttachment)
	if err != nil {
		return fmt.Errorf("failed to reconcile external LoadBalancer securityGroup due to %v", err)
	}

	err = controller.deletedManagedSGs(ctx, association)
	if err != nil {
		return fmt.Errorf("failed to delete managed securityGroups due to %v", err)
	}
	return nil
}

func (controller *associationController) reconcileWithManagedSGs(ctx context.Context, association *Association) error {
	lbSG, err := controller.reconcileManagedLbSG(ctx, association)
	if err != nil {
		return err
	}
	err = controller.reconcileManagedInstanceSG(ctx, association, lbSG)
	if err != nil {
		return err
	}
	return nil
}

func (controller *associationController) reconcileManagedLbSG(ctx context.Context, association *Association) (*SecurityGroup, error) {
	lbSGName := controller.namer.NameLbSG(association.LbID)
	lbSG := &SecurityGroup{
		GroupName: &lbSGName,
	}
	for _, port := range association.LbPorts {
		ipRanges := []*ec2.IpRange{}
		for _, cidr := range association.LbInboundCIDRs {
			ipRanges = append(ipRanges, &ec2.IpRange{
				CidrIp:      cidr,
				Description: aws.String(fmt.Sprintf("Allow ingress on port %v from %v", port, aws.StringValue(cidr))),
			})
		}
		permission := &ec2.IpPermission{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int64(port),
			ToPort:     aws.Int64(port),
			IpRanges:   ipRanges,
		}
		lbSG.InboundPermissions = append(lbSG.InboundPermissions, permission)
	}

	err := controller.sgController.Reconcile(ctx, lbSG)
	if err != nil {
		return lbSG, fmt.Errorf("failed to reconcile managed LoadBalancer securityGroup due to %v", err)
	}
	lbSGAttachment := &LbAttachment{
		GroupIDs: []string{*lbSG.GroupID},
		LbArn:    association.LbArn,
	}
	err = controller.lbAttachmentController.Reconcile(ctx, lbSGAttachment)
	if err != nil {
		return lbSG, fmt.Errorf("failed to reconcile managed LoadBalancer securityGroup attachment due to %v", err)
	}
	return lbSG, nil
}

func (controller *associationController) reconcileManagedInstanceSG(ctx context.Context, association *Association, lbSG *SecurityGroup) error {
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
						GroupId: lbSG.GroupID,
					},
				},
			},
		},
	}
	err := controller.sgController.Reconcile(ctx, instanceSG)
	if err != nil {
		return fmt.Errorf("failed to reconcile managed Instance securityGroup due to %v", err)
	}
	instanceSGAttachment := &InstanceAttachment{
		GroupID: *instanceSG.GroupID,
		Targets: association.Targets,
	}
	err = controller.instanceAttachmentController.Reconcile(ctx, instanceSGAttachment)
	if err != nil {
		return fmt.Errorf("failed to reconcile managed Instance securityGroup attachment due to %v", err)
	}
	return nil
}

func (controller *associationController) deletedManagedSGs(ctx context.Context, association *Association) error {
	err := controller.deleteManagedInstanceSG(ctx, association)
	if err != nil {
		return fmt.Errorf("failed to delete managed Instance securityGroup due to %v", err)
	}
	err = controller.deleteManagedLbSG(ctx, association)
	if err != nil {
		return fmt.Errorf("failed to delete managed LoadBalancer securityGroup due to %v", err)
	}
	return nil
}

func (controller *associationController) deleteManagedLbSG(ctx context.Context, association *Association) error {
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
	err = controller.lbAttachmentController.Delete(ctx, lbSGAttachment)
	if err != nil {
		return err
	}
	lbSG := &SecurityGroup{
		GroupID: lbSGID,
	}
	err = controller.sgController.Delete(ctx, lbSG)
	if err != nil {
		return err
	}
	return nil
}

func (controller *associationController) deleteManagedInstanceSG(ctx context.Context, association *Association) error {
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
	err = controller.instanceAttachmentController.Delete(ctx, instanceSGAttachment)
	if err != nil {
		return err
	}
	instanceSG := &SecurityGroup{
		GroupID: instanceSGID,
	}
	err = controller.sgController.Delete(ctx, instanceSG)
	if err != nil {
		return err
	}
	return nil
}

func (controller *associationController) findSGIDByName(sgName string) (*string, error) {
	sg, err := controller.ec2.GetSecurityGroupByName(aws.StringValue(controller.ec2.GetVPC().VpcId), sgName)
	if err != nil {
		return nil, err
	}
	if sg != nil {
		return sg.GroupId, nil
	}
	return nil, nil
}
