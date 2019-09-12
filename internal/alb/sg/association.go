package sg

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
)

/**
The SecurityGroup management in ALB Ingress controller depends on whether external SecurityGroups
are provided via annotation `alb.ingress.kubernetes.io/security-groups`.

* external SecurityGroups specified:
	1. the external specified SecurityGroups will be applied to LoadBalancer.
	2. no changes will be done to worker node SecurityGroups, customer need to grant inbound permission
		from these external SecurityGroups to worker node SecurityGroups.

* external SecurityGroups unspecified:
	1. controller will automatically create an SecurityGroup, which will be applied to LoadBalancer.
	2. controller will modify the securityGroup on worker nodes to allow inbound traffic from the LB SecurityGroup.
		* under **instance** targeting mode:
			1. controller will modify the SecurityGroup on primary ENI of all worker nodes to allow traffic from LB SecurityGroup.
		* under **ip** targeting mode with amazon-vpc-cni-k8s:
			1. controller will modify the SecurityGroup on ENIs that
	How SecurityGroups on ENI are identified follows process described below:
		1. if there are only single SecurityGroup on ENI, that SecurityGroup will be chosen.
		2. if there are multiple SecurityGroup on ENI, the single SecurityGroup with tag `kubernetes.io/cluster/<cluster-name>` will be chosen.
		3. otherwise, error will be raised.

	NOTE: older versions will try to create an standalone SecurityGroup which allows from traffic from LB SecurityGroup and attach to worker nodes ENI.
	This behavior is changed to above due un-scalability caused by AWS limits of allow securityGroup per ENI.
*/

// AssociationController provides functionality to manage Association
type AssociationController interface {
	// Setup will provides SecurityGroups that should be used by LoadBalancer.
	Setup(ctx context.Context, ingKey types.NamespacedName) (LbAttachmentInfo, error)

	// Reconcile will configure LB to use specified SecurityGroup in attachmentInfo.
	// Also, if managed LoadBalancer SG is used, the SecurityGroups on worker nodes will be adjusted to grant inbound traffic permission to tgGroup.
	Reconcile(ctx context.Context, ingKey types.NamespacedName, attachmentInfo LbAttachmentInfo,
		lbInstance *elbv2.LoadBalancer, tgGroup tg.TargetGroupGroup) error

	// Delete ensures the SecurityGroup created for LB are deleted.
	// Also, if managed LB SecurityGroup is used, the SecurityGroups on worker nodes will be adjusted to remove inbound traffic permission from it.
	Delete(ctx context.Context, ingKey types.NamespacedName) error
}

// NewAssociationController constructs a new association controller
func NewAssociationController(store store.Storer, cloud aws.CloudAPI, tagsController tags.Controller, nameTagGen NameTagGenerator) AssociationController {
	lbAttachmentController := &lbAttachmentController{
		cloud: cloud,
	}
	sgController := &securityGroupController{
		cloud:          cloud,
		tagsController: tagsController,
	}
	targetENIsResolver := NewTargetENIsResolver(store, cloud)
	instanceAttachmentController := NewInstanceAttachmentController(
		sgController, targetENIsResolver, nameTagGen, store, cloud)

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
	LbPorts          []int64
	LbInboundCIDRs   []string
	LbInboundV6CIDRs []string
	LbExternalSGs    []string
	AdditionalTags   map[string]string
}

func (c *associationController) Setup(ctx context.Context, ingKey types.NamespacedName) (LbAttachmentInfo, error) {
	cfg, err := c.buildAssociationConfig(ctx, ingKey)
	if err != nil {
		return LbAttachmentInfo{}, errors.Wrap(err, "failed to build SG association config")
	}
	if len(cfg.LbExternalSGs) != 0 {
		return LbAttachmentInfo{
			ManagedSGID:   "",
			ExternalSGIDs: cfg.LbExternalSGs,
		}, nil
	}

	lbManagedSG, err := c.ensureLBManagedSG(ctx, ingKey, cfg)
	if err != nil {
		return LbAttachmentInfo{}, errors.Wrap(err, "failed to reconcile LB managed SecurityGroup")
	}
	return LbAttachmentInfo{
		ManagedSGID:   lbManagedSG,
		ExternalSGIDs: nil,
	}, nil
}

func (c *associationController) Reconcile(ctx context.Context, ingKey types.NamespacedName, attachmentInfo LbAttachmentInfo,
	lbInstance *elbv2.LoadBalancer, tgGroup tg.TargetGroupGroup) error {

	if len(attachmentInfo.ExternalSGIDs) != 0 {
		return c.reconcileWithExternalSGs(ctx, ingKey, lbInstance, attachmentInfo.ExternalSGIDs)
	}
	return c.reconcileWithManagedSGs(ctx, ingKey, lbInstance, attachmentInfo.ManagedSGID, tgGroup)
}

func (c *associationController) Delete(ctx context.Context, ingKey types.NamespacedName) error {
	if err := c.instanceAttachmentController.Delete(ctx, ingKey); err != nil {
		return errors.Wrap(err, "failed to delete instance securityGroup attachment")
	}
	if err := c.deleteLBManagedSG(ctx, ingKey); err != nil {
		return fmt.Errorf("failed to delete managed LoadBalancer securityGroups due to %v", err)
	}
	return nil
}

func (c *associationController) reconcileWithExternalSGs(ctx context.Context, ingKey types.NamespacedName, lbInstance *elbv2.LoadBalancer, lbExternalSGIDs []string) error {
	if err := c.lbAttachmentController.Reconcile(ctx, lbInstance, lbExternalSGIDs); err != nil {
		return errors.Wrap(err, "failed to reconcile external LoadBalancer securityGroup attachment")
	}
	if err := c.instanceAttachmentController.Delete(ctx, ingKey); err != nil {
		return errors.Wrap(err, "failed to delete instance securityGroup attachment")
	}
	if err := c.deleteLBManagedSG(ctx, ingKey); err != nil {
		return fmt.Errorf("failed to delete managed LoadBalancer securityGroups due to %v", err)
	}
	return nil
}

func (c *associationController) reconcileWithManagedSGs(ctx context.Context, ingKey types.NamespacedName, lbInstance *elbv2.LoadBalancer, lbManagedSGID string, tgGroup tg.TargetGroupGroup) error {
	if err := c.lbAttachmentController.Reconcile(ctx, lbInstance, []string{lbManagedSGID}); err != nil {
		return errors.Wrap(err, "failed to reconcile managed LoadBalancer securityGroup attachment")
	}
	if err := c.instanceAttachmentController.Reconcile(ctx, ingKey, lbManagedSGID, tgGroup); err != nil {
		return errors.Wrap(err, "failed to reconcile instance securityGroup attachment")
	}
	return nil
}

// ensureLBManagedSG will ensure LBManagedSG exists, and rules are correctly setup.
func (c *associationController) ensureLBManagedSG(ctx context.Context, ingKey types.NamespacedName, cfg associationConfig) (string, error) {
	sgName := c.nameTagGen.NameLBSG(ingKey.Namespace, ingKey.Name)
	sgInstance, err := c.sgController.EnsureSGInstanceByName(ctx, sgName, "managed LoadBalancer securityGroup by ALB Ingress Controller")
	if err != nil {
		return "", errors.Wrap(err, "failed to reconcile managed LoadBalancer securityGroup")
	}
	sgTags := c.nameTagGen.TagLBSG(ingKey.Namespace, ingKey.Name)
	for k, v := range cfg.AdditionalTags {
		sgTags[k] = v
	}

	var inboundPermissions []*ec2.IpPermission
	for _, port := range cfg.LbPorts {
		ipRanges := make([]*ec2.IpRange, 0, len(cfg.LbInboundCIDRs))
		for _, cidr := range cfg.LbInboundCIDRs {
			ipRanges = append(ipRanges, &ec2.IpRange{
				CidrIp:      aws.String(cidr),
				Description: aws.String(fmt.Sprintf("Allow ingress on port %v from %v", port, cidr)),
			})
		}
		if len(ipRanges) > 0 {
			inboundPermissions = append(inboundPermissions, &ec2.IpPermission{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int64(port),
				ToPort:     aws.Int64(port),
				IpRanges:   ipRanges,
			})
		}

		ipv6Ranges := make([]*ec2.Ipv6Range, 0, len(cfg.LbInboundV6CIDRs))
		for _, cidr := range cfg.LbInboundV6CIDRs {
			ipv6Ranges = append(ipv6Ranges, &ec2.Ipv6Range{
				CidrIpv6:    aws.String(cidr),
				Description: aws.String(fmt.Sprintf("Allow ingress on port %v from %v", port, cidr)),
			})
		}
		if len(ipv6Ranges) > 0 {
			inboundPermissions = append(inboundPermissions, &ec2.IpPermission{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int64(port),
				ToPort:     aws.Int64(port),
				Ipv6Ranges: ipv6Ranges,
			})
		}
	}
	if err := c.sgController.Reconcile(ctx, sgInstance, inboundPermissions, sgTags); err != nil {
		return "", fmt.Errorf("failed to reconcile managed LoadBalancer securityGroup due to %v", err)
	}
	return aws.StringValue(sgInstance.GroupId), nil
}

// deleteLBManagedSG will ensure LBManagedSG are deleted.
func (c *associationController) deleteLBManagedSG(ctx context.Context, ingKey types.NamespacedName) error {
	sgName := c.nameTagGen.NameLBSG(ingKey.Namespace, ingKey.Name)
	sgInstance, err := c.cloud.GetSecurityGroupByName(sgName)
	if err != nil {
		return err
	}
	if sgInstance == nil {
		return nil
	}

	albctx.GetLogger(ctx).Infof("deleting securityGroup %v:%v", aws.StringValue(sgInstance.GroupName), aws.StringValue(sgInstance.Description))
	return c.cloud.DeleteSecurityGroupByID(ctx, aws.StringValue(sgInstance.GroupId))
}

func (c *associationController) buildAssociationConfig(ctx context.Context, ingKey types.NamespacedName) (associationConfig, error) {
	ingressAnnos, err := c.store.GetIngressAnnotations(ingKey.String())
	if err != nil {
		return associationConfig{}, err
	}

	lbPorts := make([]int64, 0, len(ingressAnnos.LoadBalancer.Ports))
	for _, port := range ingressAnnos.LoadBalancer.Ports {
		lbPorts = append(lbPorts, port.Port)
	}
	lbExternalSGs, err := c.resolveSecurityGroupIDs(ctx, ingressAnnos.LoadBalancer.SecurityGroups)
	if err != nil {
		return associationConfig{}, err
	}
	return associationConfig{
		LbPorts:          lbPorts,
		LbInboundCIDRs:   ingressAnnos.LoadBalancer.InboundCidrs,
		LbInboundV6CIDRs: ingressAnnos.LoadBalancer.InboundV6CIDRs,
		LbExternalSGs:    lbExternalSGs,
		AdditionalTags:   ingressAnnos.Tags.LoadBalancer,
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
