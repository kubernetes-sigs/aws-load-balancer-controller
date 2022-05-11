package ingress

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	networkingpkg "sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
)

const (
	lbAttrsDeletionProtectionEnabled = "deletion_protection.enabled"
)

// ModelBuilder is responsible for build mode stack for a IngressGroup.
type ModelBuilder interface {
	// build mode stack for a IngressGroup.
	Build(ctx context.Context, ingGroup Group) (core.Stack, *elbv2model.LoadBalancer, []types.NamespacedName, error)
}

// NewDefaultModelBuilder constructs new defaultModelBuilder.
func NewDefaultModelBuilder(k8sClient client.Client, eventRecorder record.EventRecorder,
	ec2Client services.EC2, acmClient services.ACM,
	annotationParser annotations.Parser, subnetsResolver networkingpkg.SubnetsResolver,
	authConfigBuilder AuthConfigBuilder, enhancedBackendBuilder EnhancedBackendBuilder,
	trackingProvider tracking.Provider, elbv2TaggingManager elbv2deploy.TaggingManager, featureGates config.FeatureGates,
	vpcID string, clusterName string, defaultTags map[string]string, externalManagedTags []string, defaultSSLPolicy string,
	backendSGProvider networkingpkg.BackendSGProvider, enableBackendSG bool, disableRestrictedSGRules bool, enableIPTargetType bool, logger logr.Logger) *defaultModelBuilder {
	certDiscovery := NewACMCertDiscovery(acmClient, logger)
	ruleOptimizer := NewDefaultRuleOptimizer(logger)
	return &defaultModelBuilder{
		k8sClient:                k8sClient,
		eventRecorder:            eventRecorder,
		ec2Client:                ec2Client,
		vpcID:                    vpcID,
		clusterName:              clusterName,
		annotationParser:         annotationParser,
		subnetsResolver:          subnetsResolver,
		backendSGProvider:        backendSGProvider,
		certDiscovery:            certDiscovery,
		authConfigBuilder:        authConfigBuilder,
		enhancedBackendBuilder:   enhancedBackendBuilder,
		ruleOptimizer:            ruleOptimizer,
		trackingProvider:         trackingProvider,
		elbv2TaggingManager:      elbv2TaggingManager,
		featureGates:             featureGates,
		defaultTags:              defaultTags,
		externalManagedTags:      sets.NewString(externalManagedTags...),
		defaultSSLPolicy:         defaultSSLPolicy,
		enableBackendSG:          enableBackendSG,
		disableRestrictedSGRules: disableRestrictedSGRules,
		enableIPTargetType:       enableIPTargetType,
		logger:                   logger,
	}
}

var _ ModelBuilder = &defaultModelBuilder{}

// default implementation for ModelBuilder
type defaultModelBuilder struct {
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	ec2Client     services.EC2

	vpcID       string
	clusterName string

	annotationParser         annotations.Parser
	subnetsResolver          networkingpkg.SubnetsResolver
	backendSGProvider        networkingpkg.BackendSGProvider
	certDiscovery            CertDiscovery
	authConfigBuilder        AuthConfigBuilder
	enhancedBackendBuilder   EnhancedBackendBuilder
	ruleOptimizer            RuleOptimizer
	trackingProvider         tracking.Provider
	elbv2TaggingManager      elbv2deploy.TaggingManager
	featureGates             config.FeatureGates
	defaultTags              map[string]string
	externalManagedTags      sets.String
	defaultSSLPolicy         string
	enableBackendSG          bool
	disableRestrictedSGRules bool
	enableIPTargetType       bool

	logger logr.Logger
}

// build mode stack for a IngressGroup.
func (b *defaultModelBuilder) Build(ctx context.Context, ingGroup Group) (core.Stack, *elbv2model.LoadBalancer, []types.NamespacedName, error) {
	stack := core.NewDefaultStack(core.StackID(ingGroup.ID))
	task := &defaultModelBuildTask{
		k8sClient:                b.k8sClient,
		eventRecorder:            b.eventRecorder,
		ec2Client:                b.ec2Client,
		vpcID:                    b.vpcID,
		clusterName:              b.clusterName,
		annotationParser:         b.annotationParser,
		subnetsResolver:          b.subnetsResolver,
		certDiscovery:            b.certDiscovery,
		authConfigBuilder:        b.authConfigBuilder,
		enhancedBackendBuilder:   b.enhancedBackendBuilder,
		ruleOptimizer:            b.ruleOptimizer,
		trackingProvider:         b.trackingProvider,
		elbv2TaggingManager:      b.elbv2TaggingManager,
		featureGates:             b.featureGates,
		backendSGProvider:        b.backendSGProvider,
		logger:                   b.logger,
		enableBackendSG:          b.enableBackendSG,
		disableRestrictedSGRules: b.disableRestrictedSGRules,
		enableIPTargetType:       b.enableIPTargetType,

		ingGroup: ingGroup,
		stack:    stack,

		defaultTags:                               b.defaultTags,
		externalManagedTags:                       b.externalManagedTags,
		defaultIPAddressType:                      elbv2model.IPAddressTypeIPV4,
		defaultScheme:                             elbv2model.LoadBalancerSchemeInternal,
		defaultSSLPolicy:                          b.defaultSSLPolicy,
		defaultTargetType:                         elbv2model.TargetTypeInstance,
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

		loadBalancer:    nil,
		tgByResID:       make(map[string]*elbv2model.TargetGroup),
		backendServices: make(map[types.NamespacedName]*corev1.Service),
	}
	if err := task.run(ctx); err != nil {
		return nil, nil, nil, err
	}
	return task.stack, task.loadBalancer, task.secretKeys, nil
}

// the default model build task
type defaultModelBuildTask struct {
	k8sClient              client.Client
	eventRecorder          record.EventRecorder
	ec2Client              services.EC2
	vpcID                  string
	clusterName            string
	annotationParser       annotations.Parser
	subnetsResolver        networkingpkg.SubnetsResolver
	backendSGProvider      networkingpkg.BackendSGProvider
	certDiscovery          CertDiscovery
	authConfigBuilder      AuthConfigBuilder
	enhancedBackendBuilder EnhancedBackendBuilder
	ruleOptimizer          RuleOptimizer
	trackingProvider       tracking.Provider
	elbv2TaggingManager    elbv2deploy.TaggingManager
	featureGates           config.FeatureGates
	logger                 logr.Logger

	ingGroup                 Group
	sslRedirectConfig        *SSLRedirectConfig
	stack                    core.Stack
	backendSGIDToken         core.StringToken
	enableBackendSG          bool
	disableRestrictedSGRules bool
	enableIPTargetType       bool

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
	defaultHealthCheckTimeoutSeconds          int64
	defaultHealthCheckIntervalSeconds         int64
	defaultHealthCheckHealthyThresholdCount   int64
	defaultHealthCheckUnhealthyThresholdCount int64
	defaultHealthCheckMatcherHTTPCode         string
	defaultHealthCheckMatcherGRPCCode         string

	loadBalancer    *elbv2model.LoadBalancer
	tgByResID       map[string]*elbv2model.TargetGroup
	backendServices map[types.NamespacedName]*corev1.Service
	secretKeys      []types.NamespacedName
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

	ingListByPort := make(map[int64][]ClassifiedIngress)
	listenPortConfigsByPort := make(map[int64][]listenPortConfigWithIngress)
	for _, member := range t.ingGroup.Members {
		ingKey := k8s.NamespacedName(member.Ing)
		listenPortConfigByPortForIngress, err := t.computeIngressListenPortConfigByPort(ctx, member.Ing)
		if err != nil {
			return errors.Wrapf(err, "ingress: %v", ingKey.String())
		}
		for port, cfg := range listenPortConfigByPortForIngress {
			ingListByPort[port] = append(ingListByPort[port], member)
			listenPortConfigsByPort[port] = append(listenPortConfigsByPort[port], listenPortConfigWithIngress{
				ingKey:           ingKey,
				listenPortConfig: cfg,
			})
		}
	}

	listenPortConfigByPort := make(map[int64]listenPortConfig)
	for port, cfgs := range listenPortConfigsByPort {
		mergedCfg, err := t.mergeListenPortConfigs(ctx, cfgs)
		if err != nil {
			return errors.Wrapf(err, "failed to merge listenPort config for port: %v", port)
		}
		listenPortConfigByPort[port] = mergedCfg
	}

	lb, err := t.buildLoadBalancer(ctx, listenPortConfigByPort)
	if err != nil {
		return err
	}

	t.sslRedirectConfig, err = t.buildSSLRedirectConfig(ctx, listenPortConfigByPort)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	for port, cfg := range listenPortConfigByPort {
		ingList := ingListByPort[port]
		ls, err := t.buildListener(ctx, lb.LoadBalancerARN(), port, cfg, ingList)
		if err != nil {
			return err
		}
		if err := t.buildListenerRules(ctx, ls.ListenerARN(), port, cfg.protocol, ingList); err != nil {
			return err
		}
	}

	if err := t.buildLoadBalancerAddOns(ctx, lb.LoadBalancerARN()); err != nil {
		return err
	}
	return nil
}

func (t *defaultModelBuildTask) mergeListenPortConfigs(_ context.Context, listenPortConfigs []listenPortConfigWithIngress) (listenPortConfig, error) {
	var mergedProtocolProvider *types.NamespacedName
	var mergedProtocol elbv2model.Protocol

	var mergedInboundCIDRsProvider *types.NamespacedName
	mergedInboundCIDRv6s := sets.NewString()
	mergedInboundCIDRv4s := sets.NewString()

	var mergedSSLPolicyProvider *types.NamespacedName
	var mergedSSLPolicy *string

	var mergedTLSCerts []string
	mergedTLSCertsSet := sets.NewString()

	for _, cfg := range listenPortConfigs {
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

		if cfg.listenPortConfig.sslPolicy != nil {
			if mergedSSLPolicyProvider == nil {
				mergedSSLPolicyProvider = &cfg.ingKey
				mergedSSLPolicy = cfg.listenPortConfig.sslPolicy
			} else if awssdk.StringValue(mergedSSLPolicy) != awssdk.StringValue(cfg.listenPortConfig.sslPolicy) {
				return listenPortConfig{}, errors.Errorf("conflicting sslPolicy, %v: %v | %v: %v",
					*mergedSSLPolicyProvider, awssdk.StringValue(mergedSSLPolicy), cfg.ingKey, awssdk.StringValue(cfg.listenPortConfig.sslPolicy))
			}
		}

		for _, cert := range cfg.listenPortConfig.tlsCerts {
			if mergedTLSCertsSet.Has(cert) {
				continue
			}
			mergedTLSCertsSet.Insert(cert)
			mergedTLSCerts = append(mergedTLSCerts, cert)
		}
	}

	if len(mergedInboundCIDRv4s) == 0 && len(mergedInboundCIDRv6s) == 0 {
		mergedInboundCIDRv4s.Insert("0.0.0.0/0")
		mergedInboundCIDRv6s.Insert("::/0")
	}
	if mergedProtocol == elbv2model.ProtocolHTTPS && mergedSSLPolicy == nil {
		mergedSSLPolicy = awssdk.String(t.defaultSSLPolicy)
	}

	return listenPortConfig{
		protocol:       mergedProtocol,
		inboundCIDRv4s: mergedInboundCIDRv4s.List(),
		inboundCIDRv6s: mergedInboundCIDRv6s.List(),
		sslPolicy:      mergedSSLPolicy,
		tlsCerts:       mergedTLSCerts,
	}, nil
}

// buildSSLRedirectConfig computes the SSLRedirect config for the IngressGroup. Returns nil if there is no SSLRedirect configured.
func (t *defaultModelBuildTask) buildSSLRedirectConfig(ctx context.Context, listenPortConfigByPort map[int64]listenPortConfig) (*SSLRedirectConfig, error) {
	explicitSSLRedirectPorts := sets.Int64{}
	for _, member := range t.ingGroup.Members {
		var rawSSLRedirectPort int64
		exists, err := t.annotationParser.ParseInt64Annotation(annotations.IngressSuffixSSLRedirect, &rawSSLRedirectPort, member.Ing.Annotations)
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
		StatusCode: elbv2sdk.RedirectActionStatusCodeEnumHttp301,
	}, nil
}

func (t *defaultModelBuildTask) getDeletionProtectionViaAnnotation(ing *networking.Ingress) (bool, error) {
	var lbAttributes map[string]string
	_, err := t.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixLoadBalancerAttributes, &lbAttributes, ing.Annotations)
	if err != nil {
		return false, err
	}
	if _, deletionProtectionSpecified := lbAttributes[lbAttrsDeletionProtectionEnabled]; deletionProtectionSpecified {
		deletionProtectionEnabled, err := strconv.ParseBool(lbAttributes[lbAttrsDeletionProtectionEnabled])
		if err != nil {
			return false, err
		}
		return deletionProtectionEnabled, nil
	}
	return false, nil
}

func (t *defaultModelBuildTask) buildManageSecurityGroupRulesFlag(_ context.Context) (bool, error) {
	explicitManageSGRulesFlag := make(map[bool]struct{})
	manageSGRules := false
	for _, member := range t.ingGroup.Members {
		rawManageSGRule := false
		exists, err := t.annotationParser.ParseBoolAnnotation(annotations.IngressSuffixManageSecurityGroupRules, &rawManageSGRule, member.Ing.Annotations)
		if err != nil {
			return false, err
		}
		if exists {
			explicitManageSGRulesFlag[rawManageSGRule] = struct{}{}
			manageSGRules = rawManageSGRule
		}
	}
	if len(explicitManageSGRulesFlag) == 0 {
		return false, nil
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
