package model

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/addon"
	config2 "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// Builder builds the model stack for a Gateway resource.
type Builder interface {
	// Build model stack for a gateway
	Build(ctx context.Context, gw *gwv1.Gateway, lbConf elbv2gw.LoadBalancerConfiguration, routes map[int32][]routeutils.RouteDescriptor, currentAddonConfig []addon.Addon, secretsManager k8s.SecretsManager) (core.Stack, *elbv2model.LoadBalancer, []addon.AddonMetadata, bool, []types.NamespacedName, error)
}

// NewModelBuilder construct a new baseModelBuilder
func NewModelBuilder(subnetsResolver networking.SubnetsResolver,
	vpcInfoProvider networking.VPCInfoProvider, vpcID string, loadBalancerType elbv2model.LoadBalancerType, trackingProvider tracking.Provider,
	elbv2TaggingManager elbv2deploy.TaggingManager, lbcConfig config.ControllerConfig, ec2Client services.EC2, elbv2Client services.ELBV2, acmClient services.ACM, k8sClient client.Client, featureGates config.FeatureGates, clusterName string, defaultTags map[string]string,
	externalManagedTags sets.Set[string], defaultSSLPolicy string, defaultTargetType string, defaultLoadBalancerScheme string,
	backendSGProvider networking.BackendSGProvider, sgResolver networking.SecurityGroupResolver, enableBackendSG bool,
	disableRestrictedSGRules bool, allowedCAARNs []string, supportedAddons []addon.Addon, logger logr.Logger) Builder {

	gwTagHelper := newTagHelper(sets.New(lbcConfig.ExternalManagedTags...), lbcConfig.DefaultTags)
	subnetBuilder := newSubnetModelBuilder(loadBalancerType, trackingProvider, subnetsResolver, elbv2TaggingManager)
	sgBuilder := newSecurityGroupBuilder(gwTagHelper, clusterName, enableBackendSG, sgResolver, backendSGProvider, logger)
	lbBuilder := newLoadBalancerBuilder(loadBalancerType, gwTagHelper, clusterName)
	tgConfigConstructor := config2.NewTargetGroupConfigConstructor()

	return &baseModelBuilder{
		clusterName:              clusterName,
		vpcID:                    vpcID,
		subnetsResolver:          subnetsResolver,
		backendSGProvider:        backendSGProvider,
		tgPropertiesConstructor:  tgConfigConstructor,
		sgResolver:               sgResolver,
		vpcInfoProvider:          vpcInfoProvider,
		elbv2TaggingManager:      elbv2TaggingManager,
		featureGates:             featureGates,
		ec2Client:                ec2Client,
		elbv2Client:              elbv2Client,
		acmClient:                acmClient,
		k8sClient:                k8sClient,
		subnetBuilder:            subnetBuilder,
		securityGroupBuilder:     sgBuilder,
		loadBalancerType:         loadBalancerType,
		lbBuilder:                lbBuilder,
		gwTagHelper:              gwTagHelper,
		logger:                   logger,
		defaultTargetType:        defaultTargetType,
		externalManagedTags:      externalManagedTags,
		defaultSSLPolicy:         defaultSSLPolicy,
		defaultTags:              defaultTags,
		disableRestrictedSGRules: disableRestrictedSGRules,
		addOnBuilder:             newAddOnBuilder(logger, supportedAddons),

		defaultLoadBalancerScheme: elbv2model.LoadBalancerScheme(defaultLoadBalancerScheme),
		defaultIPType:             elbv2model.IPAddressTypeIPV4,
		allowedCAARNs:             allowedCAARNs,
	}
}

var _ Builder = &baseModelBuilder{}

type baseModelBuilder struct {
	clusterName                string
	vpcID                      string
	loadBalancerType           elbv2model.LoadBalancerType
	annotationParser           annotations.Parser
	subnetsResolver            networking.SubnetsResolver
	vpcInfoProvider            networking.VPCInfoProvider
	backendSGProvider          networking.BackendSGProvider
	sgResolver                 networking.SecurityGroupResolver
	elbv2TaggingManager        elbv2deploy.TaggingManager
	featureGates               config.FeatureGates
	enableIPTargetType         bool
	enableManageBackendSGRules bool
	defaultTags                map[string]string
	externalManagedTags        sets.Set[string]
	defaultSSLPolicy           string
	defaultTargetType          string
	disableRestrictedSGRules   bool
	ec2Client                  services.EC2
	elbv2Client                services.ELBV2
	acmClient                  services.ACM
	k8sClient                  client.Client
	allowedCAARNs              []string
	metricsCollector           lbcmetrics.MetricCollector
	lbBuilder                  loadBalancerBuilder
	gwTagHelper                tagHelper
	listenerBuilder            listenerBuilder
	logger                     logr.Logger

	subnetBuilder           subnetModelBuilder
	securityGroupBuilder    securityGroupBuilder
	tgPropertiesConstructor config2.TargetGroupConfigConstructor

	addOnBuilder addOnBuilder

	defaultLoadBalancerScheme elbv2model.LoadBalancerScheme
	defaultIPType             elbv2model.IPAddressType
}

func (baseBuilder *baseModelBuilder) Build(ctx context.Context, gw *gwv1.Gateway, lbConf elbv2gw.LoadBalancerConfiguration, routes map[int32][]routeutils.RouteDescriptor, currentAddonConfig []addon.Addon, secretsManager k8s.SecretsManager) (core.Stack, *elbv2model.LoadBalancer, []addon.AddonMetadata, bool, []types.NamespacedName, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(gw)))
	tgBuilder := newTargetGroupBuilder(baseBuilder.clusterName, baseBuilder.vpcID, baseBuilder.gwTagHelper, baseBuilder.loadBalancerType, baseBuilder.tgPropertiesConstructor, baseBuilder.disableRestrictedSGRules, baseBuilder.defaultTargetType)
	listenerBuilder := newListenerBuilder(baseBuilder.loadBalancerType, tgBuilder, baseBuilder.gwTagHelper, baseBuilder.clusterName, baseBuilder.defaultSSLPolicy, baseBuilder.elbv2Client, baseBuilder.acmClient, baseBuilder.k8sClient, baseBuilder.allowedCAARNs, secretsManager, baseBuilder.logger)
	var isPreDelete bool
	if gw.DeletionTimestamp != nil && !gw.DeletionTimestamp.IsZero() {
		if baseBuilder.isDeleteProtected(lbConf) {
			return nil, nil, nil, false, nil, errors.Errorf("Unable to delete gateway %+v because deletion protection is enabled.", k8s.NamespacedName(gw))
		}

		if len(currentAddonConfig) == 0 {
			return stack, nil, nil, false, nil, nil
		}

		isPreDelete = true
	}

	/* Basic LB stuff (Scheme, IP Address Type) */
	scheme, err := baseBuilder.buildLoadBalancerScheme(lbConf)

	if err != nil {
		return nil, nil, nil, false, nil, err
	}

	ipAddressType, err := baseBuilder.buildLoadBalancerIPAddressType(lbConf)

	if err != nil {
		return nil, nil, nil, false, nil, err
	}

	/* Subnets */

	subnets, err := baseBuilder.subnetBuilder.buildLoadBalancerSubnets(ctx, lbConf.Spec.LoadBalancerSubnets, lbConf.Spec.LoadBalancerSubnetsSelector, scheme, ipAddressType, stack)

	if err != nil {
		return nil, nil, nil, false, nil, err
	}

	/* Security Groups */

	securityGroups, err := baseBuilder.securityGroupBuilder.buildSecurityGroups(ctx, stack, lbConf, gw, routes, ipAddressType)

	if err != nil {
		return nil, nil, nil, false, nil, err
	}

	/* Combine everything to form a LoadBalancer */
	spec, err := baseBuilder.lbBuilder.buildLoadBalancerSpec(scheme, ipAddressType, gw, lbConf, subnets, securityGroups.securityGroupTokens)

	if err != nil {
		return nil, nil, nil, false, nil, err
	}

	lb := elbv2model.NewLoadBalancer(stack, resourceIDLoadBalancer, spec)

	secrets, err := listenerBuilder.buildListeners(ctx, stack, lb, securityGroups, gw, routes, lbConf)
	if err != nil {
		return nil, nil, nil, false, nil, err
	}

	addOnCfg := lbConf
	if isPreDelete {
		addOnCfg = elbv2gw.LoadBalancerConfiguration{}
	}

	newAddonConfig, err := baseBuilder.addOnBuilder.buildAddons(stack, lb.LoadBalancerARN(), addOnCfg, currentAddonConfig)
	if err != nil {
		return nil, nil, nil, false, nil, err
	}

	return stack, lb, newAddonConfig, securityGroups.backendSecurityGroupAllocated, secrets, nil
}

func (baseBuilder *baseModelBuilder) isDeleteProtected(lbConf elbv2gw.LoadBalancerConfiguration) bool {
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

func (baseBuilder *baseModelBuilder) buildLoadBalancerScheme(lbConf elbv2gw.LoadBalancerConfiguration) (elbv2model.LoadBalancerScheme, error) {
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
func (baseBuilder *baseModelBuilder) buildLoadBalancerIPAddressType(lbConf elbv2gw.LoadBalancerConfiguration) (elbv2model.IPAddressType, error) {

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
