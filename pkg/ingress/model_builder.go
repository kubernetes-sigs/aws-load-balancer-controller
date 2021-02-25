package ingress

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	ec2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/ec2"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	networkingpkg "sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	eventWarningConflictSettings = "ConflictSettings"
)

// ModelBuilder is responsible for build mode stack for a IngressGroup.
type ModelBuilder interface {
	// build mode stack for a IngressGroup.
	Build(ctx context.Context, ingGroup Group) (core.Stack, *elbv2model.LoadBalancer, error)
}

// NewDefaultModelBuilder constructs new defaultModelBuilder.
func NewDefaultModelBuilder(k8sClient client.Client, eventRecorder record.EventRecorder,
	ec2Client services.EC2, acmClient services.ACM,
	annotationParser annotations.Parser, subnetsResolver networkingpkg.SubnetsResolver,
	authConfigBuilder AuthConfigBuilder, enhancedBackendBuilder EnhancedBackendBuilder,
	vpcID string, clusterName string, defaultTags map[string]string, logger logr.Logger) *defaultModelBuilder {
	certDiscovery := NewACMCertDiscovery(acmClient, logger)
	ruleOptimizer := NewDefaultRuleOptimizer(logger)
	return &defaultModelBuilder{
		k8sClient:              k8sClient,
		eventRecorder:          eventRecorder,
		ec2Client:              ec2Client,
		vpcID:                  vpcID,
		clusterName:            clusterName,
		annotationParser:       annotationParser,
		subnetsResolver:        subnetsResolver,
		certDiscovery:          certDiscovery,
		authConfigBuilder:      authConfigBuilder,
		enhancedBackendBuilder: enhancedBackendBuilder,
		ruleOptimizer:          ruleOptimizer,
		defaultTags:            defaultTags,
		logger:                 logger,
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

	annotationParser       annotations.Parser
	subnetsResolver        networkingpkg.SubnetsResolver
	certDiscovery          CertDiscovery
	authConfigBuilder      AuthConfigBuilder
	enhancedBackendBuilder EnhancedBackendBuilder
	ruleOptimizer          RuleOptimizer
	defaultTags            map[string]string

	logger logr.Logger
}

// build mode stack for a IngressGroup.
func (b *defaultModelBuilder) Build(ctx context.Context, ingGroup Group) (core.Stack, *elbv2model.LoadBalancer, error) {
	stack := core.NewDefaultStack(core.StackID(ingGroup.ID))
	task := &defaultModelBuildTask{
		k8sClient:              b.k8sClient,
		eventRecorder:          b.eventRecorder,
		ec2Client:              b.ec2Client,
		vpcID:                  b.vpcID,
		clusterName:            b.clusterName,
		annotationParser:       b.annotationParser,
		subnetsResolver:        b.subnetsResolver,
		certDiscovery:          b.certDiscovery,
		authConfigBuilder:      b.authConfigBuilder,
		enhancedBackendBuilder: b.enhancedBackendBuilder,
		ruleOptimizer:          b.ruleOptimizer,
		logger:                 b.logger,

		ingGroup: ingGroup,
		stack:    stack,

		defaultTags:                               b.defaultTags,
		defaultIPAddressType:                      elbv2model.IPAddressTypeIPV4,
		defaultScheme:                             elbv2model.LoadBalancerSchemeInternal,
		defaultSSLPolicy:                          "ELBSecurityPolicy-2016-08",
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

		loadBalancer: nil,
		tgByResID:    make(map[string]*elbv2model.TargetGroup),
	}
	if err := task.run(ctx); err != nil {
		return nil, nil, err
	}
	return task.stack, task.loadBalancer, nil
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
	certDiscovery          CertDiscovery
	authConfigBuilder      AuthConfigBuilder
	enhancedBackendBuilder EnhancedBackendBuilder
	ruleOptimizer          RuleOptimizer
	logger                 logr.Logger

	ingGroup          Group
	sslRedirectConfig *SSLRedirectConfig
	stack             core.Stack

	defaultTags                               map[string]string
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

	loadBalancer *elbv2model.LoadBalancer
	managedSG    *ec2model.SecurityGroup
	tgByResID    map[string]*elbv2model.TargetGroup
}

func (t *defaultModelBuildTask) run(ctx context.Context) error {
	if len(t.ingGroup.Members) == 0 {
		return nil
	}

	ingListByPort := make(map[int64][]*networking.Ingress)
	listenPortConfigsByPort := make(map[int64]map[types.NamespacedName]listenPortConfig)
	for _, ing := range t.ingGroup.Members {
		listenPortConfigByPortForIngress, err := t.computeIngressListenPortConfigByPort(ctx, ing)
		if err != nil {
			return errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing))
		}
		ingKey := k8s.NamespacedName(ing)
		for port, cfg := range listenPortConfigByPortForIngress {
			ingListByPort[port] = append(ingListByPort[port], ing)
			if _, exists := listenPortConfigsByPort[port]; !exists {
				listenPortConfigsByPort[port] = make(map[types.NamespacedName]listenPortConfig)
			}
			listenPortConfigsByPort[port][ingKey] = cfg
		}
	}
	listenPortConfigByPort := make(map[int64]listenPortConfig)
	for port, cfgs := range listenPortConfigsByPort {
		mergedCfg, err := t.mergeListenPortConfigs(ctx, cfgs)
		if err != nil {
			return errors.Wrapf(err, "failed to merge listPort config for port: %v", port)
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

func (t *defaultModelBuildTask) mergeListenPortConfigs(_ context.Context, listenPortConfigByIngress map[types.NamespacedName]listenPortConfig) (listenPortConfig, error) {
	var mergedProtocolProvider *types.NamespacedName
	var mergedProtocol elbv2model.Protocol

	var mergedInboundCIDRsProvider *types.NamespacedName
	mergedInboundCIDRv6s := sets.NewString()
	mergedInboundCIDRv4s := sets.NewString()

	var mergedSSLPolicyProvider *types.NamespacedName
	var mergedSSLPolicy *string

	mergedTLSCerts := sets.NewString()

	for ingKey, cfg := range listenPortConfigByIngress {
		if mergedProtocolProvider == nil {
			mergedProtocolProvider = &ingKey
			mergedProtocol = cfg.protocol
		} else if mergedProtocol != cfg.protocol {
			return listenPortConfig{}, errors.Errorf("conflicting protocol, %v: %v | %v: %v",
				*mergedProtocolProvider, mergedProtocol, ingKey, cfg.protocol)
		}

		if len(cfg.inboundCIDRv4s) != 0 || len(cfg.inboundCIDRv6s) != 0 {
			cfgInboundCIDRv4s := sets.NewString(cfg.inboundCIDRv4s...)
			cfgInboundCIDRv6s := sets.NewString(cfg.inboundCIDRv6s...)

			if mergedInboundCIDRsProvider == nil {
				mergedInboundCIDRsProvider = &ingKey
				mergedInboundCIDRv4s = cfgInboundCIDRv4s
				mergedInboundCIDRv6s = cfgInboundCIDRv6s
			} else if !mergedInboundCIDRv4s.Equal(cfgInboundCIDRv4s) ||
				!mergedInboundCIDRv6s.Equal(cfgInboundCIDRv6s) {
				return listenPortConfig{}, errors.Errorf("conflicting inbound-cidrs, %v: %v, %v | %v: %v, %v",
					*mergedInboundCIDRsProvider, mergedInboundCIDRv4s.List(), mergedInboundCIDRv6s.List(), ingKey, cfgInboundCIDRv4s.List(), cfgInboundCIDRv6s.List())
			}
		}

		if cfg.sslPolicy != nil {
			if mergedSSLPolicyProvider == nil {
				mergedSSLPolicyProvider = &ingKey
				mergedSSLPolicy = cfg.sslPolicy
			} else if awssdk.StringValue(mergedSSLPolicy) != awssdk.StringValue(cfg.sslPolicy) {
				return listenPortConfig{}, errors.Errorf("conflicting sslPolicy, %v: %v | %v: %v",
					*mergedSSLPolicyProvider, awssdk.StringValue(mergedSSLPolicy), ingKey, awssdk.StringValue(cfg.sslPolicy))
			}
		}
		mergedTLSCerts.Insert(cfg.tlsCerts...)
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
		tlsCerts:       mergedTLSCerts.List(),
	}, nil
}

// buildSSLRedirectConfig computes the SSLRedirect config for the IngressGroup. Returns nil if there is no SSLRedirect configured.
func (t *defaultModelBuildTask) buildSSLRedirectConfig(ctx context.Context, listenPortConfigByPort map[int64]listenPortConfig) (*SSLRedirectConfig, error) {
	explicitSSLRedirectPorts := sets.Int64{}
	for _, ing := range t.ingGroup.Members {
		var rawSSLRedirectPort int64
		exists, err := t.annotationParser.ParseInt64Annotation(annotations.IngressSuffixSSLRedirect, &rawSSLRedirectPort, ing.Annotations)
		if err != nil {
			return nil, errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing))
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
