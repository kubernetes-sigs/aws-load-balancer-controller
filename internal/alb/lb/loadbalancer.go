package lb

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/ls"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/sg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

// LoadBalancerController manages loadBalancer for ingress objects
type Controller interface {
	// Reconcile will make sure an LoadBalancer exists for specified ingress.
	Reconcile(ctx context.Context, ingress *extensions.Ingress) (*LoadBalancer, error)

	// Deletes will ensure no LoadBalancer exists for specified ingressKey.
	Delete(ctx context.Context, ingressKey types.NamespacedName) error
}

func NewController(
	cloud aws.CloudAPI,
	store store.Storer,
	nameTagGen NameTagGenerator,
	tgGroupController tg.GroupController,
	lsGroupController ls.GroupController,
	sgAssociationController sg.AssociationController) Controller {
	attrsController := NewAttributesController(cloud)

	return &defaultController{
		cloud:                   cloud,
		store:                   store,
		nameTagGen:              nameTagGen,
		tgGroupController:       tgGroupController,
		lsGroupController:       lsGroupController,
		sgAssociationController: sgAssociationController,
		attrsController:         attrsController,
	}
}

type loadBalancerConfig struct {
	Name string
	Tags map[string]string

	Type          *string
	Scheme        *string
	IpAddressType *string
	Subnets       []*string
}

type defaultController struct {
	cloud aws.CloudAPI
	store store.Storer

	nameTagGen              NameTagGenerator
	tgGroupController       tg.GroupController
	lsGroupController       ls.GroupController
	sgAssociationController sg.AssociationController
	attrsController         AttributesController
}

var _ Controller = (*defaultController)(nil)

func (controller *defaultController) Reconcile(ctx context.Context, ingress *extensions.Ingress) (*LoadBalancer, error) {
	ingressAnnos, err := controller.store.GetIngressAnnotations(k8s.MetaNamespaceKey(ingress))
	if err != nil {
		return nil, err
	}
	lbConfig, err := controller.buildLBConfig(ctx, ingress, ingressAnnos)
	if err != nil {
		return nil, fmt.Errorf("failed to build LoadBalancer configuration due to %v", err)
	}
	if err := controller.validateLBConfig(ctx, ingress, lbConfig); err != nil {
		return nil, err
	}

	instance, err := controller.ensureLBInstance(ctx, lbConfig)
	if err != nil {
		return nil, err
	}
	lbArn := aws.StringValue(instance.LoadBalancerArn)
	if err := controller.attrsController.Reconcile(ctx, lbArn, ingressAnnos.LoadBalancer.Attributes); err != nil {
		return nil, fmt.Errorf("failed to reconcile attributes of %v due to %v", lbArn, err)
	}
	if err := controller.reconcileWAF(ctx, lbArn, ingressAnnos.LoadBalancer.WebACLId); err != nil {
		return nil, err
	}

	tgGroup, err := controller.tgGroupController.Reconcile(ctx, ingress)
	if err != nil {
		return nil, fmt.Errorf("failed to reconcile targetGroups due to %v", err)
	}
	if err := controller.lsGroupController.Reconcile(ctx, lbArn, ingress, tgGroup); err != nil {
		return nil, fmt.Errorf("failed to reconcile listeners due to %v", err)
	}
	if err := controller.tgGroupController.GC(ctx, tgGroup); err != nil {
		return nil, fmt.Errorf("failed to GC targetGroups due to %v", err)
	}

	lbPorts := []int64{}
	for _, port := range ingressAnnos.LoadBalancer.Ports {
		lbPorts = append(lbPorts, port.Port)
	}
	if err := controller.sgAssociationController.Reconcile(ctx, &sg.Association{
		LbID:           lbConfig.Name,
		LbArn:          lbArn,
		LbPorts:        lbPorts,
		LbInboundCIDRs: ingressAnnos.LoadBalancer.InboundCidrs,
		ExternalSGIDs:  aws.StringValueSlice(ingressAnnos.LoadBalancer.SecurityGroups),
		TGGroup:        tgGroup,
	}); err != nil {
		return nil, fmt.Errorf("failed to reconcile securityGroup associations due to %v", err)
	}
	return &LoadBalancer{
		Arn:     lbArn,
		DNSName: aws.StringValue(instance.DNSName),
	}, nil
}

func (controller *defaultController) Delete(ctx context.Context, ingressKey types.NamespacedName) error {
	lbName := controller.nameTagGen.NameLB(ingressKey.Namespace, ingressKey.Name)
	instance, err := controller.cloud.GetLoadBalancerByName(lbName)
	if err != nil {
		return fmt.Errorf("failed to find existing LoadBalancer due to %v", err)
	}
	if instance != nil {
		if err = controller.sgAssociationController.Delete(ctx, &sg.Association{
			LbID:  lbName,
			LbArn: aws.StringValue(instance.LoadBalancerArn),
		}); err != nil {
			return fmt.Errorf("failed to clean up securityGroups due to %v", err)
		}
		if err = controller.lsGroupController.Delete(ctx, aws.StringValue(instance.LoadBalancerArn)); err != nil {
			return fmt.Errorf("failed to delete listeners due to %v", err)
		}
		if err = controller.tgGroupController.Delete(ctx, ingressKey); err != nil {
			return fmt.Errorf("failed to GC targetGroups due to %v", err)
		}

		if err = controller.cloud.DeleteLoadBalancerByArn(aws.StringValue(instance.LoadBalancerArn)); err != nil {
			return err
		}
	}

	return nil
}

func (controller *defaultController) ensureLBInstance(ctx context.Context, lbConfig *loadBalancerConfig) (*elbv2.LoadBalancer, error) {
	instance, err := controller.cloud.GetLoadBalancerByName(lbConfig.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to find existing LoadBalancer due to %v", err)
	}
	if instance == nil {
		instance, err = controller.newLBInstance(ctx, lbConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create LoadBalancer due to %v", err)
		}
		return instance, nil
	}
	if controller.isLBInstanceNeedRecreation(ctx, instance, lbConfig) {
		instance, err = controller.recreateLBInstance(ctx, instance, lbConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to recreate LoadBalancer due to %v", err)
		}
		return instance, nil
	}
	controller.reconcileLBInstance(ctx, instance, lbConfig)
	return instance, nil
}

func (controller *defaultController) newLBInstance(ctx context.Context, lbConfig *loadBalancerConfig) (*elbv2.LoadBalancer, error) {
	albctx.GetLogger(ctx).Infof("creating LoadBalancer %v", lbConfig.Name)
	resp, err := controller.cloud.CreateLoadBalancer(&elbv2.CreateLoadBalancerInput{
		Name:          aws.String(lbConfig.Name),
		Type:          lbConfig.Type,
		Scheme:        lbConfig.Scheme,
		IpAddressType: lbConfig.IpAddressType,
		Subnets:       lbConfig.Subnets,
		Tags:          tags.ConvertToELBV2(lbConfig.Tags),
	})
	if err != nil {
		albctx.GetLogger(ctx).Errorf("failed to create LoadBalancer %v due to %v", lbConfig.Name, err)
		albctx.GetEventf(ctx)(corev1.EventTypeWarning, "ERROR", "failed to create LoadBalancer %v due to %v", lbConfig.Name, err)
		return nil, err
	}

	instance := resp.LoadBalancers[0]
	albctx.GetLogger(ctx).Infof("LoadBalancer %v created, ARN: %v", lbConfig.Name, aws.StringValue(instance.LoadBalancerArn))
	albctx.GetEventf(ctx)(corev1.EventTypeNormal, "CREATE", "LoadBalancer %v created, ARN: %v", lbConfig.Name, aws.StringValue(instance.LoadBalancerArn))
	return instance, nil
}

func (controller *defaultController) recreateLBInstance(ctx context.Context, existingInstance *elbv2.LoadBalancer, lbConfig *loadBalancerConfig) (*elbv2.LoadBalancer, error) {
	existingLBArn := aws.StringValue(existingInstance.LoadBalancerArn)
	albctx.GetLogger(ctx).Infof("deleting LoadBalancer %v for recreation", existingLBArn)
	if err := controller.cloud.DeleteLoadBalancerByArn(existingLBArn); err != nil {
		return nil, err
	}
	return controller.newLBInstance(ctx, lbConfig)
}

func (controller *defaultController) reconcileLBInstance(ctx context.Context, instance *elbv2.LoadBalancer, lbConfig *loadBalancerConfig) error {
	lbArn := aws.StringValue(instance.LoadBalancerArn)
	if !util.DeepEqual(instance.IpAddressType, lbConfig.IpAddressType) {
		albctx.GetLogger(ctx).Infof("modifying LoadBalancer %v due to IpAddressType change (%v => %v)", lbArn, aws.StringValue(instance.IpAddressType), aws.StringValue(lbConfig.IpAddressType))
		if _, err := controller.cloud.SetIpAddressType(&elbv2.SetIpAddressTypeInput{
			LoadBalancerArn: instance.LoadBalancerArn,
			IpAddressType:   lbConfig.IpAddressType,
		}); err != nil {
			albctx.GetEventf(ctx)(corev1.EventTypeNormal, "ERROR", "failed to modify IpAddressType of %v due to %v", lbArn, err)
			return fmt.Errorf("failed to modify IpAddressType of %v due to %v", lbArn, err)
		}
		albctx.GetEventf(ctx)(corev1.EventTypeNormal, "MODIFY", "IpAddressType of %v modified", lbArn)
	}

	desiredSubnets := sets.NewString(aws.StringValueSlice(lbConfig.Subnets)...)
	currentSubnets := sets.NewString(aws.StringValueSlice(util.AvailabilityZones(instance.AvailabilityZones).AsSubnets())...)
	if !currentSubnets.Equal(desiredSubnets) {
		albctx.GetLogger(ctx).Infof("modifying LoadBalancer %v due to Subnets change (%v => %v)", lbArn, currentSubnets.List(), desiredSubnets.List())
		if _, err := controller.cloud.SetSubnets(&elbv2.SetSubnetsInput{
			LoadBalancerArn: instance.LoadBalancerArn,
			Subnets:         lbConfig.Subnets,
		}); err != nil {
			albctx.GetEventf(ctx)(corev1.EventTypeNormal, "ERROR", "failed to modify Subnets of %v due to %v", lbArn, err)
			return fmt.Errorf("failed to modify Subnets of %v due to %v", lbArn, err)
		}
	}
	return nil
}

func (controller *defaultController) reconcileWAF(ctx context.Context, lbArn string, webACLID *string) error {
	webACLSummary, err := controller.cloud.GetWebACLSummary(aws.String(lbArn))
	if err != nil {
		return fmt.Errorf("failed to check webACL on loadBalancer %v due to %v", lbArn, err)
	}
	switch {
	case webACLSummary != nil && webACLID == nil:
		{
			if _, err := controller.cloud.Disassociate(aws.String(lbArn)); err != nil {
				return fmt.Errorf("failed to disassociate webACL on loadBalancer %v due to %v", lbArn, err)
			}
		}
	case webACLSummary != nil && webACLID != nil && aws.StringValue(webACLSummary.WebACLId) != aws.StringValue(webACLID):
		{
			if _, err := controller.cloud.Associate(aws.String(lbArn), webACLID); err != nil {
				return fmt.Errorf("failed to associate webACL on loadBalancer %v due to %v", lbArn, err)
			}
		}
	case webACLSummary == nil && webACLID != nil:
		{
			if _, err := controller.cloud.Associate(aws.String(lbArn), webACLID); err != nil {
				return fmt.Errorf("failed to associate webACL on loadBalancer %v due to %v", lbArn, err)
			}
		}
	}
	return nil
}

func (controller *defaultController) isLBInstanceNeedRecreation(ctx context.Context, instance *elbv2.LoadBalancer, lbConfig *loadBalancerConfig) bool {
	if !util.DeepEqual(instance.Scheme, lbConfig.Scheme) {
		albctx.GetLogger(ctx).Infof("LoadBalancer %s need recreation due to scheme changed(%s => %s)",
			lbConfig.Name, aws.StringValue(instance.Scheme), aws.StringValue(lbConfig.Scheme))
		return true
	}
	return false
}

func (controller *defaultController) buildLBConfig(ctx context.Context, ingress *extensions.Ingress, ingressAnnos *annotations.Ingress) (*loadBalancerConfig, error) {
	lbTags := controller.nameTagGen.TagLB(ingress.Namespace, ingress.Name)
	for k, v := range ingressAnnos.Tags.LoadBalancer {
		lbTags[k] = v
	}
	return &loadBalancerConfig{
		Name: controller.nameTagGen.NameLB(ingress.Namespace, ingress.Name),
		Tags: lbTags,

		Type:          aws.String(elbv2.LoadBalancerTypeEnumApplication),
		Scheme:        ingressAnnos.LoadBalancer.Scheme,
		IpAddressType: ingressAnnos.LoadBalancer.IPAddressType,
		Subnets:       ingressAnnos.LoadBalancer.Subnets,
	}, nil
}

func (controller *defaultController) validateLBConfig(ctx context.Context, ingress *extensions.Ingress, lbConfig *loadBalancerConfig) error {
	controllerCfg := controller.store.GetConfig()
	if controllerCfg.RestrictScheme && aws.StringValue(lbConfig.Scheme) == elbv2.LoadBalancerSchemeEnumInternetFacing {
		whitelisted := false
		for _, name := range controllerCfg.InternetFacingIngresses[ingress.Namespace] {
			if name == ingress.Name {
				whitelisted = true
				break
			}
		}
		if !whitelisted {
			return fmt.Errorf("ingress %v/%v is not in internetFacing whitelist", ingress.Namespace, ingress.Name)
		}
	}

	return nil
}
