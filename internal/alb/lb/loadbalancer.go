package lb

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"

	"github.com/aws/aws-sdk-go/service/ec2"
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
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
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
	sgAssociationController sg.AssociationController,
	tagsController tags.Controller) Controller {
	attrsController := NewAttributesController(cloud)

	return &defaultController{
		cloud:                   cloud,
		store:                   store,
		nameTagGen:              nameTagGen,
		tgGroupController:       tgGroupController,
		lsGroupController:       lsGroupController,
		sgAssociationController: sgAssociationController,
		tagsController:          tagsController,
		attrsController:         attrsController,
	}
}

type loadBalancerConfig struct {
	Name string
	Tags map[string]string

	Type           *string
	Scheme         *string
	IpAddressType  *string
	SecurityGroups []string
	Subnets        []string
}

type defaultController struct {
	cloud aws.CloudAPI
	store store.Storer

	nameTagGen              NameTagGenerator
	tgGroupController       tg.GroupController
	lsGroupController       ls.GroupController
	sgAssociationController sg.AssociationController
	tagsController          tags.Controller
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

	if controller.store.GetConfig().FeatureGate.Enabled(config.WAF) {
		if err := controller.reconcileWAF(ctx, lbArn, ingressAnnos.LoadBalancer.WebACLId); err != nil {
			return nil, err
		}
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

	if err := controller.sgAssociationController.Reconcile(ctx, ingress, instance, tgGroup); err != nil {
		return nil, fmt.Errorf("failed to reconcile securityGroup associations due to %v", err)
	}
	return &LoadBalancer{
		Arn:     lbArn,
		DNSName: aws.StringValue(instance.DNSName),
	}, nil
}

func (controller *defaultController) Delete(ctx context.Context, ingressKey types.NamespacedName) error {
	lbName := controller.nameTagGen.NameLB(ingressKey.Namespace, ingressKey.Name)
	instance, err := controller.cloud.GetLoadBalancerByName(ctx, lbName)
	if err != nil {
		return fmt.Errorf("failed to find existing LoadBalancer due to %v", err)
	}
	if instance != nil {
		if err = controller.sgAssociationController.Delete(ctx, ingressKey, instance); err != nil {
			return fmt.Errorf("failed to clean up securityGroups due to %v", err)
		}
		if err = controller.lsGroupController.Delete(ctx, aws.StringValue(instance.LoadBalancerArn)); err != nil {
			return fmt.Errorf("failed to delete listeners due to %v", err)
		}
		if err = controller.tgGroupController.Delete(ctx, ingressKey); err != nil {
			return fmt.Errorf("failed to GC targetGroups due to %v", err)
		}

		albctx.GetLogger(ctx).Infof("deleting LoadBalancer %v", aws.StringValue(instance.LoadBalancerArn))
		if err = controller.cloud.DeleteLoadBalancerByArn(ctx, aws.StringValue(instance.LoadBalancerArn)); err != nil {
			return err
		}
	}

	return nil
}

func (controller *defaultController) ensureLBInstance(ctx context.Context, lbConfig *loadBalancerConfig) (*elbv2.LoadBalancer, error) {
	instance, err := controller.cloud.GetLoadBalancerByName(ctx, lbConfig.Name)
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
	if err := controller.reconcileLBInstance(ctx, instance, lbConfig); err != nil {
		return nil, err
	}
	return instance, nil
}

func (controller *defaultController) newLBInstance(ctx context.Context, lbConfig *loadBalancerConfig) (*elbv2.LoadBalancer, error) {
	albctx.GetLogger(ctx).Infof("creating LoadBalancer %v", lbConfig.Name)
	resp, err := controller.cloud.CreateLoadBalancerWithContext(ctx, &elbv2.CreateLoadBalancerInput{
		Name:           aws.String(lbConfig.Name),
		Type:           lbConfig.Type,
		Scheme:         lbConfig.Scheme,
		IpAddressType:  lbConfig.IpAddressType,
		SecurityGroups: aws.StringSlice(lbConfig.SecurityGroups),
		Subnets:        aws.StringSlice(lbConfig.Subnets),
		Tags:           tags.ConvertToELBV2(lbConfig.Tags),
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
	if err := controller.cloud.DeleteLoadBalancerByArn(ctx, existingLBArn); err != nil {
		return nil, err
	}
	return controller.newLBInstance(ctx, lbConfig)
}

func (controller *defaultController) reconcileLBInstance(ctx context.Context, instance *elbv2.LoadBalancer, lbConfig *loadBalancerConfig) error {
	lbArn := aws.StringValue(instance.LoadBalancerArn)
	if !util.DeepEqual(instance.IpAddressType, lbConfig.IpAddressType) {
		albctx.GetLogger(ctx).Infof("modifying LoadBalancer %v due to IpAddressType change (%v => %v)", lbArn, aws.StringValue(instance.IpAddressType), aws.StringValue(lbConfig.IpAddressType))
		if _, err := controller.cloud.SetIpAddressTypeWithContext(ctx, &elbv2.SetIpAddressTypeInput{
			LoadBalancerArn: instance.LoadBalancerArn,
			IpAddressType:   lbConfig.IpAddressType,
		}); err != nil {
			albctx.GetEventf(ctx)(corev1.EventTypeNormal, "ERROR", "failed to modify IpAddressType of %v due to %v", lbArn, err)
			return fmt.Errorf("failed to modify IpAddressType of %v due to %v", lbArn, err)
		}
		albctx.GetEventf(ctx)(corev1.EventTypeNormal, "MODIFY", "IpAddressType of %v modified", lbArn)
	}

	desiredSubnets := sets.NewString(lbConfig.Subnets...)
	currentSubnets := sets.NewString(aws.StringValueSlice(util.AvailabilityZones(instance.AvailabilityZones).AsSubnets())...)
	if !currentSubnets.Equal(desiredSubnets) {
		albctx.GetLogger(ctx).Infof("modifying LoadBalancer %v due to Subnets change (%v => %v)", lbArn, currentSubnets.List(), desiredSubnets.List())
		if _, err := controller.cloud.SetSubnetsWithContext(ctx, &elbv2.SetSubnetsInput{
			LoadBalancerArn: instance.LoadBalancerArn,
			Subnets:         aws.StringSlice(lbConfig.Subnets),
		}); err != nil {
			albctx.GetEventf(ctx)(corev1.EventTypeNormal, "ERROR", "failed to modify Subnets of %v due to %v", lbArn, err)
			return fmt.Errorf("failed to modify Subnets of %v due to %v", lbArn, err)
		}
	}

	if err := controller.tagsController.ReconcileELB(ctx, lbArn, lbConfig.Tags); err != nil {
		return fmt.Errorf("failed to reconcile tags of %v due to %v", lbArn, err)
	}
	return nil
}

func (controller *defaultController) reconcileWAF(ctx context.Context, lbArn string, webACLID *string) error {
	webACLSummary, err := controller.cloud.GetWebACLSummary(ctx, aws.String(lbArn))
	if err != nil {
		return fmt.Errorf("error getting web acl for load balancer %v: %v", lbArn, err)
	}

	if webACLID != nil {
		b, err := controller.cloud.WebACLExists(ctx, webACLID)
		if err != nil {
			return fmt.Errorf("error fetching web acl %v: %v", aws.StringValue(webACLID), err)
		}
		if !b {
			return fmt.Errorf("web acl %v does not exist", aws.StringValue(webACLID))
		}
	}

	switch {
	case webACLSummary != nil && webACLID == nil:
		{
			albctx.GetLogger(ctx).Infof("disassociate WAF on %v", lbArn)
			if _, err := controller.cloud.DisassociateWAF(ctx, aws.String(lbArn)); err != nil {
				return fmt.Errorf("failed to disassociate webACL on loadBalancer %v due to %v", lbArn, err)
			}
		}
	case webACLSummary != nil && webACLID != nil && aws.StringValue(webACLSummary.WebACLId) != aws.StringValue(webACLID):
		{
			albctx.GetLogger(ctx).Infof("associate WAF on %v to %v", lbArn, aws.StringValue(webACLID))
			if _, err := controller.cloud.AssociateWAF(ctx, aws.String(lbArn), webACLID); err != nil {
				return fmt.Errorf("failed to associate webACL on loadBalancer %v due to %v", lbArn, err)
			}
		}
	case webACLSummary == nil && webACLID != nil:
		{
			albctx.GetLogger(ctx).Infof("associate WAF on %v to %v", lbArn, aws.StringValue(webACLID))
			if _, err := controller.cloud.AssociateWAF(ctx, aws.String(lbArn), webACLID); err != nil {
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
	subnets, err := controller.resolveSubnets(ctx, aws.StringValue(ingressAnnos.LoadBalancer.Scheme), ingressAnnos.LoadBalancer.Subnets)
	if err != nil {
		return nil, err
	}
	return &loadBalancerConfig{
		Name: controller.nameTagGen.NameLB(ingress.Namespace, ingress.Name),
		Tags: lbTags,

		Type:           aws.String(elbv2.LoadBalancerTypeEnumApplication),
		Scheme:         ingressAnnos.LoadBalancer.Scheme,
		IpAddressType:  ingressAnnos.LoadBalancer.IPAddressType,
		SecurityGroups: ingressAnnos.LoadBalancer.SecurityGroups,
		Subnets:        subnets,
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

func (controller *defaultController) resolveSubnets(ctx context.Context, scheme string, in []string) ([]string, error) {
	if len(in) == 0 {
		subnets, err := controller.clusterSubnets(ctx, scheme)
		return subnets, err

	}

	var names []string
	var subnets []string

	for _, subnet := range in {
		if strings.HasPrefix(subnet, "subnet-") {
			subnets = append(subnets, subnet)
			continue
		}
		names = append(names, subnet)
	}

	if len(names) > 0 {
		o, err := controller.cloud.GetSubnetsByNameOrID(ctx, names)
		if err != nil {
			return subnets, err
		}

		for _, subnet := range o {
			subnets = append(subnets, aws.StringValue(subnet.SubnetId))
		}
	}

	sort.Strings(subnets)
	if len(subnets) != len(in) {
		return subnets, fmt.Errorf("not all subnets were resolvable, (%v != %v)", strings.Join(in, ","), strings.Join(subnets, ","))
	}

	return subnets, nil
}

func (controller *defaultController) clusterSubnets(ctx context.Context, scheme string) ([]string, error) {
	var subnetIds []string
	var useableSubnets []*ec2.Subnet
	var out []string
	var key string

	if scheme == elbv2.LoadBalancerSchemeEnumInternal {
		key = aws.TagNameSubnetInternalELB
	} else if scheme == elbv2.LoadBalancerSchemeEnumInternetFacing {
		key = aws.TagNameSubnetPublicELB
	} else {
		return nil, fmt.Errorf("invalid scheme [%s]", scheme)
	}

	clusterSubnets, err := controller.cloud.GetClusterSubnets()
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS tags. Error: %s", err.Error())
	}

	for arn, subnetTags := range clusterSubnets {
		for _, tag := range subnetTags {
			if aws.StringValue(tag.Key) == key {
				p := strings.Split(arn, "/")
				subnetID := p[len(p)-1]
				subnetIds = append(subnetIds, subnetID)
			}
		}
	}

	o, err := controller.cloud.GetSubnetsByNameOrID(ctx, subnetIds)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch subnets due to %v", err)
	}

	for _, subnet := range o {
		if subnetIsUsable(subnet, useableSubnets) {
			useableSubnets = append(useableSubnets, subnet)
			out = append(out, aws.StringValue(subnet.SubnetId))
		}
	}

	if len(out) < 2 {
		return nil, fmt.Errorf("retrieval of subnets failed to resolve 2 qualified subnets. Subnets must "+
			"contain the %s/<cluster name> tag with a value of shared or owned and the %s tag signifying it should be used for ALBs "+
			"Additionally, there must be at least 2 subnets with unique availability zones as required by "+
			"ALBs. Either tag subnets to meet this requirement or use the subnets annotation on the "+
			"ingress resource to explicitly call out what subnets to use for ALB creation. The subnets "+
			"that did resolve were %v", aws.TagNameCluster, key,
			log.Prettify(out))
	}

	sort.Strings(out)
	return out, nil
}

// subnetIsUsable determines if the subnet shares the same availability zone as a subnet in the
// existing list. If it does, false is returned as you cannot have albs provisioned to 2 subnets in
// the same availability zone.
func subnetIsUsable(new *ec2.Subnet, existing []*ec2.Subnet) bool {
	for _, subnet := range existing {
		if *new.AvailabilityZone == *subnet.AvailabilityZone {
			return false
		}
	}
	return true
}
