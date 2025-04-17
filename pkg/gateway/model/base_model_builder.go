package model

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"strconv"
)

// Builder builds the model stack for a Gateway resource.
type Builder interface {
	// Build model stack for a gateway
	Build(ctx context.Context, gw *gwv1.Gateway, lbConf *elbv2gw.LoadBalancerConfiguration, routes map[int][]routeutils.RouteDescriptor) (core.Stack, *elbv2model.LoadBalancer, bool, error)
}

// NewModelBuilder construct a new baseModelBuilder
func NewModelBuilder(subnetsResolver networking.SubnetsResolver,
	vpcInfoProvider networking.VPCInfoProvider, vpcID string, loadBalancerType elbv2model.LoadBalancerType, trackingProvider tracking.Provider,
	elbv2TaggingManager elbv2deploy.TaggingManager, lbcConfig config.ControllerConfig, ec2Client services.EC2, featureGates config.FeatureGates, clusterName string, defaultTags map[string]string,
	externalManagedTags sets.Set[string], defaultSSLPolicy string, defaultTargetType string, defaultLoadBalancerScheme string,
	backendSGProvider networking.BackendSGProvider, sgResolver networking.SecurityGroupResolver, enableBackendSG bool,
	disableRestrictedSGRules bool, logger logr.Logger) Builder {

	gwTagHelper := newTagHelper(sets.New(lbcConfig.ExternalManagedTags...), lbcConfig.DefaultTags)
	subnetBuilder := newSubnetModelBuilder(loadBalancerType, trackingProvider, subnetsResolver, elbv2TaggingManager)
	sgBuilder := newSecurityGroupBuilder(gwTagHelper, clusterName, enableBackendSG, sgResolver, backendSGProvider, logger)
	lbBuilder := newLoadBalancerBuilder(loadBalancerType, gwTagHelper, clusterName)
	tgBuilder := newTargetGroupBuilder(clusterName, vpcID, gwTagHelper, loadBalancerType, disableRestrictedSGRules, defaultTargetType)

	return &baseModelBuilder{
		subnetBuilder:        subnetBuilder,
		securityGroupBuilder: sgBuilder,
		lbBuilder:            lbBuilder,
		tgBuilder:            tgBuilder,
		logger:               logger,

		defaultLoadBalancerScheme: elbv2model.LoadBalancerScheme(defaultLoadBalancerScheme),
		defaultIPType:             elbv2model.IPAddressTypeIPV4,
	}
}

var _ Builder = &baseModelBuilder{}

type baseModelBuilder struct {
	lbBuilder loadBalancerBuilder
	logger    logr.Logger

	subnetBuilder        subnetModelBuilder
	securityGroupBuilder securityGroupBuilder
	tgBuilder            targetGroupBuilder

	defaultLoadBalancerScheme elbv2model.LoadBalancerScheme
	defaultIPType             elbv2model.IPAddressType
}

func (baseBuilder *baseModelBuilder) Build(ctx context.Context, gw *gwv1.Gateway, lbConf *elbv2gw.LoadBalancerConfiguration, routes map[int][]routeutils.RouteDescriptor) (core.Stack, *elbv2model.LoadBalancer, bool, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(gw)))
	if gw.DeletionTimestamp != nil && !gw.DeletionTimestamp.IsZero() {
		if baseBuilder.isDeleteProtected(lbConf) {
			return nil, nil, false, errors.Errorf("Unable to delete gateway %+v because deletion protection is enabled.", k8s.NamespacedName(gw))
		}
		return stack, nil, false, nil
	}

	/* Basic LB stuff (Scheme, IP Address Type) */
	scheme, err := baseBuilder.buildLoadBalancerScheme(lbConf)

	if err != nil {
		return nil, nil, false, err
	}

	ipAddressType, err := baseBuilder.buildLoadBalancerIPAddressType(lbConf)

	if err != nil {
		return nil, nil, false, err
	}

	/* Subnets */

	subnets, err := baseBuilder.subnetBuilder.buildLoadBalancerSubnets(ctx, lbConf.Spec.LoadBalancerSubnets, lbConf.Spec.LoadBalancerSubnetsSelector, scheme, ipAddressType, stack)

	if err != nil {
		return nil, nil, false, err
	}

	/* Security Groups */

	securityGroups, err := baseBuilder.securityGroupBuilder.buildSecurityGroups(ctx, stack, lbConf, gw, routes, ipAddressType)

	if err != nil {
		return nil, nil, false, err
	}

	/* Combine everything to form a LoadBalancer */
	spec, err := baseBuilder.lbBuilder.buildLoadBalancerSpec(scheme, ipAddressType, gw, lbConf, subnets, securityGroups.securityGroupTokens)

	if err != nil {
		return nil, nil, false, err
	}

	lb := elbv2model.NewLoadBalancer(stack, resourceIDLoadBalancer, spec)

	baseBuilder.logger.Info("Got this route details", "routes", routes)
	/* Target Groups */
	// TODO - Figure out how to map this back to a listener?
	tgByResID := make(map[string]buildTargetGroupOutput)
	for _, descriptors := range routes {
		for _, descriptor := range descriptors {
			for _, rule := range descriptor.GetAttachedRules() {
				for _, backend := range rule.GetBackends() {
					// TODO -- Figure out what to do with the return value (it's also inserted into the tgByResID map)
					_, tgErr := baseBuilder.tgBuilder.buildTargetGroup(&tgByResID, gw, lbConf, lb.Spec.IPAddressType, descriptor, backend, securityGroups.backendSecurityGroupToken)
					if tgErr != nil {
						return nil, nil, false, err
					}
				}
			}
		}
	}

	for tgResID, tgOut := range tgByResID {
		tg := elbv2model.NewTargetGroup(stack, tgResID, tgOut.targetGroupSpec)
		tgOut.bindingSpec.Template.Spec.TargetGroupARN = tg.TargetGroupARN()
		elbv2model.NewTargetGroupBindingResource(stack, tg.ID(), tgOut.bindingSpec)
	}

	return stack, lb, securityGroups.backendSecurityGroupAllocated, nil
}

func (baseBuilder *baseModelBuilder) isDeleteProtected(lbConf *elbv2gw.LoadBalancerConfiguration) bool {
	if lbConf == nil {
		return false
	}

	for _, attr := range lbConf.Spec.LoadBalancerAttributes {
		if attr.Key == shared_constants.LBAttributeDeletionProtection {
			deletionProtectionEnabled, err := strconv.ParseBool(attr.Value)

			if err != nil {
				baseBuilder.logger.Error(err, "Unable to parse deletion protection value, assuming false.")
				return false
			}

			return deletionProtectionEnabled
		}
	}

	return false
}

func (baseBuilder *baseModelBuilder) buildLoadBalancerScheme(lbConf *elbv2gw.LoadBalancerConfiguration) (elbv2model.LoadBalancerScheme, error) {
	scheme := lbConf.Spec.Scheme

	if scheme == nil {
		return baseBuilder.defaultLoadBalancerScheme, nil
	}
	switch *scheme {
	case elbv2gw.LoadBalancerScheme(elbv2model.LoadBalancerSchemeInternetFacing):
		return elbv2model.LoadBalancerSchemeInternetFacing, nil
	case elbv2gw.LoadBalancerScheme(elbv2model.LoadBalancerSchemeInternal):
		return elbv2model.LoadBalancerSchemeInternal, nil
	default:
		return "", errors.Errorf("unknown scheme: %v", *scheme)
	}
}

// buildLoadBalancerIPAddressType builds the LoadBalancer IPAddressType.
func (baseBuilder *baseModelBuilder) buildLoadBalancerIPAddressType(lbConf *elbv2gw.LoadBalancerConfiguration) (elbv2model.IPAddressType, error) {

	if lbConf.Spec.IpAddressType == nil {
		return baseBuilder.defaultIPType, nil
	}

	switch *lbConf.Spec.IpAddressType {
	case elbv2gw.LoadBalancerIpAddressType(elbv2model.IPAddressTypeIPV4):
		return elbv2model.IPAddressTypeIPV4, nil
	case elbv2gw.LoadBalancerIpAddressType(elbv2model.IPAddressTypeDualStack):
		return elbv2model.IPAddressTypeDualStack, nil
	case elbv2gw.LoadBalancerIpAddressType(elbv2model.IPAddressTypeDualStackWithoutPublicIPV4):
		return elbv2model.IPAddressTypeDualStackWithoutPublicIPV4, nil
	default:
		return "", errors.Errorf("unknown IPAddressType: %v", *lbConf.Spec.IpAddressType)
	}
}
