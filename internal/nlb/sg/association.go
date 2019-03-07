package sg

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/nlb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	corev1 "k8s.io/api/core/v1"
)

// AssociationController provides functionality to manage Association
type AssociationController interface {
	// Reconcile ensured the securityGroups in AWS matches the state specified by association.
	Reconcile(ctx context.Context, service *corev1.Service, lbInstance *elbv2.LoadBalancer, tgGroup tg.TargetGroupGroup) error

	// Delete ensures the securityGroups created by ingress controller for specified LbID doesn't exists.
	Delete(ctx context.Context, serviceKey types.NamespacedName, lbInstance *elbv2.LoadBalancer) error
}

// NewAssociationController constructs a new association controller
func NewAssociationController(store store.Storer, cloud aws.CloudAPI, tagsController tags.Controller, nameTagGen NameTagGenerator) AssociationController {
	lbAttachmentController := &lbAttachmentController{
		cloud: cloud,
	}
	instanceAttachmentController := &instanceAttachmentController{
		store: store,
		cloud: cloud,
	}
	sgController := &securityGroupController{
		cloud:          cloud,
		tagsController: tagsController,
	}
	return &associationController{
		lbAttachmentController:       lbAttachmentController,
		instanceAttachmentController: instanceAttachmentController,
		sgController:                 sgController,
		nameTagGen:                   nameTagGen,
		store:                        store,
		cloud:                        cloud,
	}
}

type associationController struct {
	lbAttachmentController       LbAttachmentController
	instanceAttachmentController InstanceAttachmentController
	sgController                 SecurityGroupController
	nameTagGen                   NameTagGenerator

	store store.Storer
	cloud aws.CloudAPI
}

type associationConfig struct {
	LbPorts        []int64
	LbInboundCIDRs []string
	LbExternalSGs  []string
	AdditionalTags map[string]string
}

func (c *associationController) Reconcile(ctx context.Context, service *corev1.Service, lbInstance *elbv2.LoadBalancer, tgGroup tg.TargetGroupGroup) error {
	cfg, err := c.buildAssociationConfig(ctx, service)
	if err != nil {
		return fmt.Errorf("failed to build SG association config due to %v", err)
	}
	serviceKey := types.NamespacedName{Namespace: service.Namespace, Name: service.Name}
	if len(cfg.LbExternalSGs) != 0 {
		return c.reconcileWithExternalSGs(ctx, serviceKey, lbInstance, cfg)
	}
	return c.reconcileWithManagedSGs(ctx, serviceKey, lbInstance, cfg, tgGroup)
}

func (c *associationController) Delete(ctx context.Context, serviceKey types.NamespacedName, lbInstance *elbv2.LoadBalancer) error {
	if err := c.deleteInstanceSGAndAttachment(ctx, serviceKey); err != nil {
		return fmt.Errorf("failed to delete managed instance securityGroups due to %v", err)
	}
	if err := c.lbAttachmentController.Delete(ctx, lbInstance); err != nil {
		return fmt.Errorf("failed to reconcile external LoadBalancer securityGroup due to %v", err)
	}
	if err := c.deleteLbSG(ctx, serviceKey); err != nil {
		return fmt.Errorf("failed to delete managed LoadBalancer securityGroups due to %v", err)
	}
	return nil
}

func (c *associationController) reconcileWithManagedSGs(ctx context.Context, serviceKey types.NamespacedName, lbInstance *elbv2.LoadBalancer, cfg associationConfig, tgGroup tg.TargetGroupGroup) error {
	lbSGID, err := c.reconcileLbSG(ctx, serviceKey, cfg)
	if err != nil {
		return err
	}
	if err := c.lbAttachmentController.Reconcile(ctx, lbInstance, []string{lbSGID}); err != nil {
		return err
	}

	instanceSGID, err := c.reconcileInstanceSG(ctx, serviceKey, cfg, lbSGID)
	if err != nil {
		return err
	}
	if err := c.instanceAttachmentController.Reconcile(ctx, instanceSGID, tgGroup); err != nil {
		return err
	}
	return nil
}

func (c *associationController) reconcileWithExternalSGs(ctx context.Context, serviceKey types.NamespacedName, lbInstance *elbv2.LoadBalancer, cfg associationConfig) error {
	if err := c.deleteInstanceSGAndAttachment(ctx, serviceKey); err != nil {
		return fmt.Errorf("failed to delete managed instance securityGroups due to %v", err)
	}
	if err := c.lbAttachmentController.Reconcile(ctx, lbInstance, cfg.LbExternalSGs); err != nil {
		return fmt.Errorf("failed to reconcile external LoadBalancer securityGroup due to %v", err)
	}
	if err := c.deleteLbSG(ctx, serviceKey); err != nil {
		return fmt.Errorf("failed to delete managed LoadBalancer securityGroups due to %v", err)
	}
	return nil
}

func (c *associationController) reconcileLbSG(ctx context.Context, serviceKey types.NamespacedName, cfg associationConfig) (string, error) {
	sgName := c.nameTagGen.NameLBSG(serviceKey.Namespace, serviceKey.Name)
	sgInstance, err := c.ensureSGInstance(ctx, sgName, "managed LoadBalancer securityGroup by ALB Ingress Controller")
	if err != nil {
		return "", fmt.Errorf("failed to reconcile managed LoadBalancer securityGroup due to %v", err)
	}
	sgTags := c.nameTagGen.TagLBSG(serviceKey.Namespace, serviceKey.Name)
	for k, v := range cfg.AdditionalTags {
		sgTags[k] = v
	}
	var inboundPermissions []*ec2.IpPermission
	for _, port := range cfg.LbPorts {
		ipRanges := make([]*ec2.IpRange, 0, len(cfg.LbInboundCIDRs))
		for _, cidr := range cfg.LbInboundCIDRs {
			ipRanges = append(ipRanges, &ec2.IpRange{
				CidrIp:      aws.String(cidr),
				Description: aws.String(fmt.Sprintf("Allow service on port %v from %v", port, cidr)),
			})
		}
		permission := &ec2.IpPermission{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int64(port),
			ToPort:     aws.Int64(port),
			IpRanges:   ipRanges,
		}
		inboundPermissions = append(inboundPermissions, permission)
	}
	if err := c.sgController.Reconcile(ctx, sgInstance, inboundPermissions, sgTags); err != nil {
		return "", fmt.Errorf("failed to reconcile managed LoadBalancer securityGroup due to %v", err)
	}
	return aws.StringValue(sgInstance.GroupId), nil
}

func (c *associationController) deleteLbSG(ctx context.Context, serviceKey types.NamespacedName) error {
	sgName := c.nameTagGen.NameLBSG(serviceKey.Namespace, serviceKey.Name)
	sgInstance, err := c.cloud.GetSecurityGroupByName(sgName)
	if err != nil {
		return err
	}
	if sgInstance == nil {
		return nil
	}
	return c.deleteSGInstance(ctx, sgInstance)
}

func (c *associationController) reconcileInstanceSG(ctx context.Context, serviceKey types.NamespacedName, cfg associationConfig, lbSGID string) (string, error) {
	sgName := c.nameTagGen.NameInstanceSG(serviceKey.Namespace, serviceKey.Name)
	sgInstance, err := c.ensureSGInstance(ctx, sgName, "managed instance securityGroup by ALB Service Controller")
	if err != nil {
		return "", fmt.Errorf("failed to reconcile managed instance securityGroup due to %v", err)
	}
	sgTags := c.nameTagGen.TagInstanceSG(serviceKey.Namespace, serviceKey.Name)
	for k, v := range cfg.AdditionalTags {
		sgTags[k] = v
	}
	inboundPermissions := []*ec2.IpPermission{
		{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int64(0),
			ToPort:     aws.Int64(65535),
			UserIdGroupPairs: []*ec2.UserIdGroupPair{
				{
					GroupId: aws.String(lbSGID),
				},
			},
		},
	}
	if err := c.sgController.Reconcile(ctx, sgInstance, inboundPermissions, sgTags); err != nil {
		return "", fmt.Errorf("failed to reconcile managed instance securityGroup due to %v", err)
	}
	return aws.StringValue(sgInstance.GroupId), nil
}

func (c *associationController) deleteInstanceSGAndAttachment(ctx context.Context, serviceKey types.NamespacedName) error {
	sgName := c.nameTagGen.NameInstanceSG(serviceKey.Namespace, serviceKey.Name)
	sgInstance, err := c.cloud.GetSecurityGroupByName(sgName)
	if err != nil {
		return err
	}
	if sgInstance == nil {
		return nil
	}
	if err := c.instanceAttachmentController.Delete(ctx, aws.StringValue(sgInstance.GroupId)); err != nil {
		return err
	}
	return c.deleteSGInstance(ctx, sgInstance)
}

func (c *associationController) ensureSGInstance(ctx context.Context, groupName string, description string) (*ec2.SecurityGroup, error) {
	sgInstance, err := c.cloud.GetSecurityGroupByName(groupName)
	if err != nil {
		return nil, err
	}
	if sgInstance != nil {
		return sgInstance, nil
	}
	albctx.GetLogger(ctx).Infof("creating securityGroup %v:%v", groupName, description)
	resp, err := c.cloud.CreateSecurityGroupWithContext(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(groupName),
		Description: aws.String(description),
	})
	if err != nil {
		return nil, err
	}
	return &ec2.SecurityGroup{
		GroupId:   resp.GroupId,
		GroupName: aws.String(groupName),
	}, nil
}

func (c *associationController) deleteSGInstance(ctx context.Context, instance *ec2.SecurityGroup) error {
	albctx.GetLogger(ctx).Infof("deleting securityGroup %v:%v", aws.StringValue(instance.GroupName), aws.StringValue(instance.Description))
	return c.cloud.DeleteSecurityGroupByID(aws.StringValue(instance.GroupId))
}

func (c *associationController) buildAssociationConfig(ctx context.Context, service *corev1.Service) (associationConfig, error) {
	serviceAnnos, err := c.store.GetServiceAnnotations(k8s.MetaNamespaceKey(service), nil)
	if err != nil {
		return associationConfig{}, err
	}

	lbPorts := make([]int64, 0, len(serviceAnnos.LoadBalancer.Ports))
	for _, port := range serviceAnnos.LoadBalancer.Ports {
		lbPorts = append(lbPorts, port.Port)
	}
	lbExternalSGs, err := c.resolveSecurityGroupIDs(ctx, serviceAnnos.LoadBalancer.SecurityGroups)
	if err != nil {
		return associationConfig{}, err
	}
	return associationConfig{
		LbPorts:        lbPorts,
		LbInboundCIDRs: serviceAnnos.LoadBalancer.InboundCidrs,
		LbExternalSGs:  lbExternalSGs,
		AdditionalTags: serviceAnnos.Tags.LoadBalancer,
	}, nil
}

func (c *associationController) resolveSecurityGroupIDs(ctx context.Context, sgIDOrNames []string) ([]string, error) {
	var names []string
	var output []string

	for _, sg := range sgIDOrNames {
		if strings.HasPrefix(sg, "sg-") {
			output = append(output, sg)
			continue
		}

		names = append(names, sg)
	}

	if len(names) > 0 {
		groups, err := c.cloud.GetSecurityGroupsByName(ctx, names)
		if err != nil {
			return output, err
		}

		for _, sg := range groups {
			output = append(output, aws.StringValue(sg.GroupId))
		}
	}

	if len(output) != len(sgIDOrNames) {
		return output, fmt.Errorf("not all security groups were resolvable, (%v != %v)", strings.Join(sgIDOrNames, ","), strings.Join(output, ","))
	}

	return output, nil
}
