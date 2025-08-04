package ingress

import (
	"context"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"k8s.io/apimachinery/pkg/util/cache"
	"reflect"
	"strconv"
	"sync"
	"time"

	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"

	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	certs "sigs.k8s.io/aws-load-balancer-controller/pkg/certs"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	errmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/error"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	networkingpkg "sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	controllerName                      = "ingress"
	defaultTargetGroupNameToARNCacheTTL = 20 * time.Minute
)

// ModelBuilder is responsible for build mode stack for a IngressGroup.
type ModelBuilder interface {
	// build mode stack for a IngressGroup.
	Build(ctx context.Context, ingGroup Group, metricsCollector lbcmetrics.MetricCollector) (core.Stack, *elbv2model.LoadBalancer, []types.NamespacedName, bool, *core.FrontendNlbTargetGroupDesiredState, *elbv2model.LoadBalancer, error)
}

// NewDefaultModelBuilder constructs new defaultModelBuilder.
func NewDefaultModelBuilder(k8sClient client.Client, eventRecorder record.EventRecorder,
	ec2Client services.EC2, elbv2Client services.ELBV2, acmClient services.ACM,
	annotationParser annotations.Parser, subnetsResolver networkingpkg.SubnetsResolver,
	authConfigBuilder AuthConfigBuilder, enhancedBackendBuilder EnhancedBackendBuilder,
	trackingProvider tracking.Provider, elbv2TaggingManager elbv2deploy.TaggingManager, featureGates config.FeatureGates,
	vpcID string, clusterName string, defaultTags map[string]string, externalManagedTags []string, defaultSSLPolicy string, defaultTargetType string, defaultLoadBalancerScheme string,
	backendSGProvider networkingpkg.BackendSGProvider, sgResolver networkingpkg.SecurityGroupResolver,
	enableBackendSG bool, defaultEnableManageBackendSGRules bool, disableRestrictedSGRules bool, allowedCAARNs []string, enableIPTargetType bool, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector) *defaultModelBuilder {
	certDiscovery := certs.NewACMCertDiscovery(acmClient, allowedCAARNs, logger)
	ruleOptimizer := NewDefaultRuleOptimizer(logger)
	return &defaultModelBuilder{
		k8sClient:                  k8sClient,
		eventRecorder:              eventRecorder,
		ec2Client:                  ec2Client,
		elbv2Client:                elbv2Client,
		vpcID:                      vpcID,
		clusterName:                clusterName,
		annotationParser:           annotationParser,
		subnetsResolver:            subnetsResolver,
		backendSGProvider:          backendSGProvider,
		sgResolver:                 sgResolver,
		certDiscovery:              certDiscovery,
		authConfigBuilder:          authConfigBuilder,
		enhancedBackendBuilder:     enhancedBackendBuilder,
		ruleOptimizer:              ruleOptimizer,
		trackingProvider:           trackingProvider,
		elbv2TaggingManager:        elbv2TaggingManager,
		featureGates:               featureGates,
		defaultTags:                defaultTags,
		externalManagedTags:        sets.NewString(externalManagedTags...),
		defaultSSLPolicy:           defaultSSLPolicy,
		defaultTargetType:          elbv2model.TargetType(defaultTargetType),
		defaultLoadBalancerScheme:  elbv2model.LoadBalancerScheme(defaultLoadBalancerScheme),
		enableBackendSG:            enableBackendSG,
		enableManageBackendSGRules: defaultEnableManageBackendSGRules,
		disableRestrictedSGRules:   disableRestrictedSGRules,
		enableIPTargetType:         enableIPTargetType,
		targetGroupNameToArnMapper: newTargetGroupNameToArnMapper(elbv2Client, defaultTargetGroupNameToARNCacheTTL),
		logger:                     logger,
		metricsCollector:           metricsCollector,
	}
}

var _ ModelBuilder = &defaultModelBuilder{}

// default implementation for ModelBuilder
type defaultModelBuilder struct {
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	ec2Client     services.EC2
	elbv2Client   services.ELBV2

	vpcID       string
	clusterName string

	annotationParser           annotations.Parser
	subnetsResolver            networkingpkg.SubnetsResolver
	backendSGProvider          networkingpkg.BackendSGProvider
	sgResolver                 networkingpkg.SecurityGroupResolver
	certDiscovery              certs.CertDiscovery
	authConfigBuilder          AuthConfigBuilder
	enhancedBackendBuilder     EnhancedBackendBuilder
	ruleOptimizer              RuleOptimizer
	trackingProvider           tracking.Provider
	elbv2TaggingManager        elbv2deploy.TaggingManager
	featureGates               config.FeatureGates
	defaultTags                map[string]string
	externalManagedTags        sets.String
	defaultSSLPolicy           string
	defaultTargetType          elbv2model.TargetType
	defaultLoadBalancerScheme  elbv2model.LoadBalancerScheme
	enableBackendSG            bool
	enableManageBackendSGRules bool
	disableRestrictedSGRules   bool
	enableIPTargetType         bool
	targetGroupNameToArnMapper *targetGroupNameToArnMapper

	logger           logr.Logger
	metricsCollector lbcmetrics.MetricCollector
}

// build mode stack for a IngressGroup.
func (b *defaultModelBuilder) Build(ctx context.Context, ingGroup Group, metricsCollector lbcmetrics.MetricCollector) (core.Stack, *elbv2model.LoadBalancer, []types.NamespacedName, bool, *core.FrontendNlbTargetGroupDesiredState, *elbv2model.LoadBalancer, error) {
	stack := core.NewDefaultStack(core.StackID(ingGroup.ID))
	frontendNlbTargetGroupDesiredState := core.NewFrontendNlbTargetGroupDesiredState()

	task := &defaultModelBuildTask{
		k8sClient:                  b.k8sClient,
		eventRecorder:              b.eventRecorder,
		ec2Client:                  b.ec2Client,
		elbv2Client:                b.elbv2Client,
		vpcID:                      b.vpcID,
		clusterName:                b.clusterName,
		annotationParser:           b.annotationParser,
		subnetsResolver:            b.subnetsResolver,
		certDiscovery:              b.certDiscovery,
		authConfigBuilder:          b.authConfigBuilder,
		enhancedBackendBuilder:     b.enhancedBackendBuilder,
		ruleOptimizer:              b.ruleOptimizer,
		trackingProvider:           b.trackingProvider,
		elbv2TaggingManager:        b.elbv2TaggingManager,
		featureGates:               b.featureGates,
		backendSGProvider:          b.backendSGProvider,
		sgResolver:                 b.sgResolver,
		logger:                     b.logger,
		enableBackendSG:            b.enableBackendSG,
		enableManageBackendSGRules: b.enableManageBackendSGRules,
		disableRestrictedSGRules:   b.disableRestrictedSGRules,
		enableIPTargetType:         b.enableIPTargetType,
		metricsCollector:           b.metricsCollector,

		ingGroup:                           ingGroup,
		stack:                              stack,
		frontendNlbTargetGroupDesiredState: frontendNlbTargetGroupDesiredState,

		defaultTags:                               b.defaultTags,
		externalManagedTags:                       b.externalManagedTags,
		defaultIPAddressType:                      elbv2model.IPAddressTypeIPV4,
		defaultScheme:                             b.defaultLoadBalancerScheme,
		defaultSSLPolicy:                          b.defaultSSLPolicy,
		defaultTargetType:                         b.defaultTargetType,
		defaultBackendProtocol:                    elbv2model.ProtocolHTTP,
		defaultBackendProtocolVersion:             elbv2model.ProtocolVersionHTTP1,
		defaultHealthCheckPathHTTP:                "/",
		defaultHealthCheckPathGRPC:                "/AWS.ALB/healthcheck",
		defaultHealthCheckIntervalSeconds:         15,
		defaultHealthCheckTimeoutSeconds:          5,
		defaultHealthCheckHealthyThresholdCount:   2,
		defaultHealthCheckUnhealthyThresholdCount: 2,
		defaultHealthCheckMatcherHTTPCode:         "200",
		defaultHealthCheckMatcherGRPCCode:         "12",

		loadBalancer:               nil,
		frontendNlb:                nil,
		tgByResID:                  make(map[string]*elbv2model.TargetGroup),
		backendServices:            make(map[types.NamespacedName]*corev1.Service),
		targetGroupNameToArnMapper: b.targetGroupNameToArnMapper,
	}
	if err := task.run(ctx); err != nil {
		return nil, nil, nil, false, nil, nil, err
	}
	return task.stack, task.loadBalancer, task.secretKeys, task.backendSGAllocated, frontendNlbTargetGroupDesiredState, task.frontendNlb, nil
}

// the default model build task
type defaultModelBuildTask struct {
	k8sClient              client.Client
	eventRecorder          record.EventRecorder
	ec2Client              services.EC2
	elbv2Client            services.ELBV2
	vpcID                  string
	clusterName            string
	annotationParser       annotations.Parser
	subnetsResolver        networkingpkg.SubnetsResolver
	backendSGProvider      networkingpkg.BackendSGProvider
	sgResolver             networkingpkg.SecurityGroupResolver
	certDiscovery          certs.CertDiscovery
	authConfigBuilder      AuthConfigBuilder
	enhancedBackendBuilder EnhancedBackendBuilder
	ruleOptimizer          RuleOptimizer
	trackingProvider       tracking.Provider
	elbv2TaggingManager    elbv2deploy.TaggingManager
	featureGates           config.FeatureGates
	logger                 logr.Logger

	ingGroup                   Group
	sslRedirectConfig          *SSLRedirectConfig
	stack                      core.Stack
	backendSGIDToken           core.StringToken
	backendSGAllocated         bool
	enableBackendSG            bool
	enableManageBackendSGRules bool
	disableRestrictedSGRules   bool
	enableIPTargetType         bool

	defaultTags                               map[string]string
	externalManagedTags                       sets.String
	defaultIPAddressType                      elbv2model.IPAddressType
	defaultScheme                             elbv2model.LoadBalancerScheme
	defaultSSLPolicy                          string
	defaultTargetType                         elbv2model.TargetType
	defaultBackendProtocol                    elbv2model.Protocol
	defaultBackendProtocolVersion             elbv2model.ProtocolVersion
	defaultHealthCheckPathHTTP                string
	defaultHealthCheckPathGRPC                string
	defaultHealthCheckTimeoutSeconds          int32
	defaultHealthCheckIntervalSeconds         int32
	defaultHealthCheckHealthyThresholdCount   int32
	defaultHealthCheckUnhealthyThresholdCount int32
	defaultHealthCheckMatcherHTTPCode         string
	defaultHealthCheckMatcherGRPCCode         string

	loadBalancer                       *elbv2model.LoadBalancer
	tgByResID                          map[string]*elbv2model.TargetGroup
	backendServices                    map[types.NamespacedName]*corev1.Service
	secretKeys                         []types.NamespacedName
	frontendNlb                        *elbv2model.LoadBalancer
	frontendNlbTargetGroupDesiredState *core.FrontendNlbTargetGroupDesiredState
	targetGroupNameToArnMapper         *targetGroupNameToArnMapper

	metricsCollector lbcmetrics.MetricCollector
}

func (t *defaultModelBuildTask) run(ctx context.Context) error {
	for _, inactiveMember := range t.ingGroup.InactiveMembers {
		if !inactiveMember.DeletionTimestamp.IsZero() {
			deletionProtectionEnabled, err := t.getDeletionProtectionViaAnnotation(inactiveMember)
			if err != nil {
				return err
			}
			if deletionProtectionEnabled {
				return errors.Errorf("deletion_protection is enabled, cannot delete the ingress: %v", inactiveMember.Name)
			}
		}
	}
	if len(t.ingGroup.Members) == 0 {
		return nil
	}

	listenerPortConfigByIngress := make(map[types.NamespacedName]map[int32]listenPortConfig)
	ingListByPort := make(map[int32][]ClassifiedIngress)
	listenPortConfigsByPort := make(map[int32][]listenPortConfigWithIngress)
	for _, member := range t.ingGroup.Members {
		ingKey := k8s.NamespacedName(member.Ing)
		listenPortConfigByPortForIngress, err := t.computeIngressListenPortConfigByPort(ctx, &member)
		if err != nil {
			return errors.Wrapf(err, "ingress: %v", ingKey.String())
		}

		listenerPortConfigByIngress[ingKey] = listenPortConfigByPortForIngress

		for port, cfg := range listenPortConfigByPortForIngress {
			ingListByPort[port] = append(ingListByPort[port], member)
			listenPortConfigsByPort[port] = append(listenPortConfigsByPort[port], listenPortConfigWithIngress{
				ingKey:           ingKey,
				listenPortConfig: cfg,
			})
		}
	}

	listenPortConfigByPort := make(map[int32]listenPortConfig)
	for port, cfgs := range listenPortConfigsByPort {
		mergedCfg, err := t.mergeListenPortConfigs(ctx, cfgs)
		if err != nil {
			return errors.Wrapf(err, "failed to merge listenPort config for port: %v", port)
		}
		listenPortConfigByPort[port] = mergedCfg
	}

	lb, err := t.buildLoadBalancer(ctx, listenPortConfigByPort)
	if err != nil {
		return errmetrics.NewErrorWithMetrics(controllerName, "build_load_balancer_error", err, t.metricsCollector)
	}

	t.sslRedirectConfig, err = t.buildSSLRedirectConfig(ctx, listenPortConfigByPort)
	if err != nil {
		return errmetrics.NewErrorWithMetrics(controllerName, "build_ssl_redirct_config_error", err, t.metricsCollector)
	}
	for port, cfg := range listenPortConfigByPort {
		ingList := ingListByPort[port]
		ls, err := t.buildListener(ctx, lb.LoadBalancerARN(), port, cfg, ingList)
		if err != nil {
			return errmetrics.NewErrorWithMetrics(controllerName, "build_listener_error", err, t.metricsCollector)
		}
		if err := t.buildListenerRules(ctx, ls.ListenerARN(), port, cfg.protocol, ingList); err != nil {
			return errmetrics.NewErrorWithMetrics(controllerName, "build_listener_rule_error", err, t.metricsCollector)
		}
	}

	if err := t.buildLoadBalancerAddOns(ctx, lb.LoadBalancerARN()); err != nil {
		return errmetrics.NewErrorWithMetrics(controllerName, "build_load_balancer_addons", err, t.metricsCollector)
	}

	if err := t.buildFrontendNlbModel(ctx, lb, listenerPortConfigByIngress); err != nil {
		return errmetrics.NewErrorWithMetrics(controllerName, "build_frontend_nlb", err, t.metricsCollector)
	}

	return nil
}

func (t *defaultModelBuildTask) mergeListenPortConfigs(_ context.Context, listenPortConfigs []listenPortConfigWithIngress) (listenPortConfig, error) {
	var mergedProtocolProvider *types.NamespacedName
	var mergedProtocol elbv2model.Protocol

	var mergedInboundCIDRsProvider *types.NamespacedName
	mergedInboundCIDRv6s := sets.NewString()
	mergedInboundCIDRv4s := sets.NewString()

	var mergedInboundPrefixListsProvider *types.NamespacedName
	mergedInboundPrefixLists := sets.NewString()

	var mergedSSLPolicyProvider *types.NamespacedName
	var mergedSSLPolicy *string

	var mergedTLSCerts []string

	// Set the default cert as the first cert
	// This process allows the same certificate to be specified for both the default certificate and the SNI certificate.
	var defaultCertMemberIndex int
	for i, cfg := range listenPortConfigs {
		if len(cfg.listenPortConfig.tlsCerts) > 0 {
			mergedTLSCerts = append(mergedTLSCerts, cfg.listenPortConfig.tlsCerts[0])
			defaultCertMemberIndex = i
			break
		}
	}

	mergedTLSCertsSet := sets.NewString()

	var mergedMtlsAttributesProvider *types.NamespacedName
	var mergedMtlsAttributes *elbv2model.MutualAuthenticationAttributes

	for i, cfg := range listenPortConfigs {
		if mergedProtocolProvider == nil {
			mergedProtocolProvider = &cfg.ingKey
			mergedProtocol = cfg.listenPortConfig.protocol
		} else if mergedProtocol != cfg.listenPortConfig.protocol {
			return listenPortConfig{}, errors.Errorf("conflicting protocol, %v: %v | %v: %v",
				*mergedProtocolProvider, mergedProtocol, cfg.ingKey, cfg.listenPortConfig.protocol)
		}

		if len(cfg.listenPortConfig.inboundCIDRv4s) != 0 || len(cfg.listenPortConfig.inboundCIDRv6s) != 0 {
			cfgInboundCIDRv4s := sets.NewString(cfg.listenPortConfig.inboundCIDRv4s...)
			cfgInboundCIDRv6s := sets.NewString(cfg.listenPortConfig.inboundCIDRv6s...)
			if mergedInboundCIDRsProvider == nil {
				mergedInboundCIDRsProvider = &cfg.ingKey
				mergedInboundCIDRv4s = cfgInboundCIDRv4s
				mergedInboundCIDRv6s = cfgInboundCIDRv6s
			} else if !mergedInboundCIDRv4s.Equal(cfgInboundCIDRv4s) || !mergedInboundCIDRv6s.Equal(cfgInboundCIDRv6s) {
				return listenPortConfig{}, errors.Errorf("conflicting inbound-cidrs, %v: %v, %v | %v: %v, %v",
					*mergedInboundCIDRsProvider, mergedInboundCIDRv4s.List(), mergedInboundCIDRv6s.List(), cfg.ingKey, cfgInboundCIDRv4s.List(), cfgInboundCIDRv6s.List())
			}
		}

		if len(cfg.listenPortConfig.prefixLists) != 0 {
			cfgInboundPrefixLists := sets.NewString(cfg.listenPortConfig.prefixLists...)
			if mergedInboundPrefixListsProvider == nil {
				mergedInboundPrefixListsProvider = &cfg.ingKey
				mergedInboundPrefixLists = cfgInboundPrefixLists
			} else if !mergedInboundPrefixLists.Equal(cfgInboundPrefixLists) {
				return listenPortConfig{}, errors.Errorf("conflicting inbound-prefix-lists, %v: %v | %v: %v",
					*mergedInboundPrefixListsProvider, mergedInboundPrefixLists.List(), cfg.ingKey, cfgInboundPrefixLists.List())
			}
		}

		if cfg.listenPortConfig.sslPolicy != nil {
			if mergedSSLPolicyProvider == nil {
				mergedSSLPolicyProvider = &cfg.ingKey
				mergedSSLPolicy = cfg.listenPortConfig.sslPolicy
			} else if awssdk.ToString(mergedSSLPolicy) != awssdk.ToString(cfg.listenPortConfig.sslPolicy) {
				return listenPortConfig{}, errors.Errorf("conflicting sslPolicy, %v: %v | %v: %v",
					*mergedSSLPolicyProvider, awssdk.ToString(mergedSSLPolicy), cfg.ingKey, awssdk.ToString(cfg.listenPortConfig.sslPolicy))
			}
		}

		for j, cert := range cfg.listenPortConfig.tlsCerts {
			// The first certificate is ignored as it is the default certificate, which has already been added to the mergedTLSCerts.
			if i == defaultCertMemberIndex && j == 0 {
				continue
			}
			if mergedTLSCertsSet.Has(cert) {
				continue
			}
			mergedTLSCertsSet.Insert(cert)
			mergedTLSCerts = append(mergedTLSCerts, cert)
		}

		if cfg.listenPortConfig.mutualAuthentication != nil {
			if mergedMtlsAttributesProvider == nil {
				mergedMtlsAttributesProvider = &cfg.ingKey
				mergedMtlsAttributes = cfg.listenPortConfig.mutualAuthentication
			} else if !reflect.DeepEqual(mergedMtlsAttributes, cfg.listenPortConfig.mutualAuthentication) {
				return listenPortConfig{}, errors.Errorf("conflicting mTLS Attributes, %v: %v | %v: %v",
					*mergedMtlsAttributesProvider, mergedMtlsAttributes, cfg.ingKey, cfg.listenPortConfig.mutualAuthentication)
			}
		}

	}

	if len(mergedInboundCIDRv4s) == 0 && len(mergedInboundCIDRv6s) == 0 && len(mergedInboundPrefixLists) == 0 {
		mergedInboundCIDRv4s.Insert("0.0.0.0/0")
		mergedInboundCIDRv6s.Insert("::/0")
	}
	if mergedProtocol == elbv2model.ProtocolHTTPS && mergedSSLPolicy == nil {
		mergedSSLPolicy = awssdk.String(t.defaultSSLPolicy)
	}

	return listenPortConfig{
		protocol:             mergedProtocol,
		inboundCIDRv4s:       mergedInboundCIDRv4s.List(),
		inboundCIDRv6s:       mergedInboundCIDRv6s.List(),
		prefixLists:          mergedInboundPrefixLists.List(),
		sslPolicy:            mergedSSLPolicy,
		tlsCerts:             mergedTLSCerts,
		mutualAuthentication: mergedMtlsAttributes,
	}, nil
}

// buildSSLRedirectConfig computes the SSLRedirect config for the IngressGroup. Returns nil if there is no SSLRedirect configured.
func (t *defaultModelBuildTask) buildSSLRedirectConfig(ctx context.Context, listenPortConfigByPort map[int32]listenPortConfig) (*SSLRedirectConfig, error) {
	explicitSSLRedirectPorts := sets.Int32{}
	for _, member := range t.ingGroup.Members {
		var rawSSLRedirectPort int32
		exists, err := t.annotationParser.ParseInt32Annotation(annotations.IngressSuffixSSLRedirect, &rawSSLRedirectPort, member.Ing.Annotations)
		if err != nil {
			return nil, errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(member.Ing))
		}
		if exists {
			explicitSSLRedirectPorts.Insert(rawSSLRedirectPort)
		}
	}

	if len(explicitSSLRedirectPorts) == 0 {
		return nil, nil
	}
	if len(explicitSSLRedirectPorts) > 1 {
		return nil, errors.Errorf("conflicting sslRedirect port: %v", explicitSSLRedirectPorts.List())
	}
	rawSSLRedirectPort, _ := explicitSSLRedirectPorts.PopAny()
	if listenPortConfig, ok := listenPortConfigByPort[rawSSLRedirectPort]; !ok {
		return nil, errors.Errorf("listener does not exist for SSLRedirect port: %v", rawSSLRedirectPort)
	} else if listenPortConfig.protocol != elbv2model.ProtocolHTTPS {
		return nil, errors.Errorf("listener protocol non-SSL for SSLRedirect port: %v", rawSSLRedirectPort)
	}

	return &SSLRedirectConfig{
		SSLPort:    rawSSLRedirectPort,
		StatusCode: string(elbv2types.RedirectActionStatusCodeEnumHttp301),
	}, nil
}

func (t *defaultModelBuildTask) getDeletionProtectionViaAnnotation(ing *networking.Ingress) (bool, error) {
	var lbAttributes map[string]string
	_, err := t.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixLoadBalancerAttributes, &lbAttributes, ing.Annotations)
	if err != nil {
		return false, err
	}
	if _, deletionProtectionSpecified := lbAttributes[shared_constants.LBAttributeDeletionProtection]; deletionProtectionSpecified {
		deletionProtectionEnabled, err := strconv.ParseBool(lbAttributes[shared_constants.LBAttributeDeletionProtection])
		if err != nil {
			return false, err
		}
		return deletionProtectionEnabled, nil
	}
	return false, nil
}

func (t *defaultModelBuildTask) buildManageSecurityGroupRulesFlag(_ context.Context) (bool, error) {
	explicitManageSGRulesFlag := make(map[bool]struct{})
	// default value from cli flag
	manageSGRules := t.enableManageBackendSGRules

	// check annotation, annotation has a higher priority than cli flag
	for _, member := range t.ingGroup.Members {
		rawManageSGRule := false
		exists, err := t.annotationParser.ParseBoolAnnotation(annotations.IngressSuffixManageSecurityGroupRules, &rawManageSGRule, member.Ing.Annotations)
		if err != nil {
			return false, err
		}
		if exists {
			explicitManageSGRulesFlag[rawManageSGRule] = struct{}{}
			if rawManageSGRule != manageSGRules {
				manageSGRules = rawManageSGRule
				t.logger.V(1).Info("Override enable manage backend security group rules flag with annotation", "value: ", rawManageSGRule, "for ingress", k8s.NamespacedName(member.Ing).String(), "in ingress yaml file")
			}
		}
	}
	// if annotation is not specified, take value from cli flag
	if len(explicitManageSGRulesFlag) == 0 {
		return t.enableManageBackendSGRules, nil
	}
	if len(explicitManageSGRulesFlag) > 1 {
		return false, errors.New("conflicting manage backend security group rules settings")
	}
	return manageSGRules, nil
}

// the listen port config for specific Ingress's listener port.
type listenPortConfigWithIngress struct {
	ingKey           types.NamespacedName
	listenPortConfig listenPortConfig
}

type targetGroupNameToArnMapper struct {
	elbv2Client services.ELBV2
	cache       *cache.Expiring
	cacheTTL    time.Duration
	cacheMutex  sync.RWMutex
}

func newTargetGroupNameToArnMapper(elbv2Client services.ELBV2, ttl time.Duration) *targetGroupNameToArnMapper {
	return &targetGroupNameToArnMapper{
		elbv2Client: elbv2Client,
		cache:       cache.NewExpiring(),
		cacheTTL:    ttl,
	}
}

// getArnByName returns the ARN of an AWS target group identified by its name
func (t *targetGroupNameToArnMapper) getArnByName(ctx context.Context, targetGroupName string) (string, error) {
	t.cacheMutex.Lock()
	defer t.cacheMutex.Unlock()

	if rawCacheItem, exists := t.cache.Get(targetGroupName); exists {
		return rawCacheItem.(string), nil
	}
	req := &elbv2sdk.DescribeTargetGroupsInput{
		Names: []string{targetGroupName},
	}

	targetGroups, err := t.elbv2Client.DescribeTargetGroupsAsList(ctx, req)
	if err != nil {
		return "", err
	}
	if len(targetGroups) != 1 {
		return "", errors.Errorf("expecting a single targetGroup with query [%s] but got %v", targetGroupName, len(targetGroups))
	}
	arn := *targetGroups[0].TargetGroupArn
	t.cache.Set(targetGroupName, arn, t.cacheTTL)
	return arn, nil
}
