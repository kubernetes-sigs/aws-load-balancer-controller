package service

import (
	"context"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go/service/ec2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
)

const (
	LoadBalancerTypeNLBIP          = "nlb-ip"
	LoadBalancerTypeExternal       = "external"
	LoadBalancerTargetTypeIP       = "ip"
	LoadBalancerTargetTypeInstance = "instance"
	lbAttrsDeletionProtection      = "deletion_protection.enabled"
)

// ModelBuilder builds the model stack for the service resource.
type ModelBuilder interface {
	// Build model stack for service
	Build(ctx context.Context, service *corev1.Service) (core.Stack, *elbv2model.LoadBalancer, error)
}

// NewDefaultModelBuilder construct a new defaultModelBuilder
func NewDefaultModelBuilder(annotationParser annotations.Parser, subnetsResolver networking.SubnetsResolver,
	vpcInfoProvider networking.VPCInfoProvider, vpcID string, trackingProvider tracking.Provider,
	elbv2TaggingManager elbv2deploy.TaggingManager, featureGates config.FeatureGates, clusterName string, defaultTags map[string]string,
	externalManagedTags []string, defaultSSLPolicy string, enableIPTargetType bool, serviceUtils ServiceUtils) *defaultModelBuilder {
	return &defaultModelBuilder{
		annotationParser:    annotationParser,
		subnetsResolver:     subnetsResolver,
		vpcInfoProvider:     vpcInfoProvider,
		trackingProvider:    trackingProvider,
		elbv2TaggingManager: elbv2TaggingManager,
		featureGates:        featureGates,
		serviceUtils:        serviceUtils,
		clusterName:         clusterName,
		vpcID:               vpcID,
		defaultTags:         defaultTags,
		externalManagedTags: sets.NewString(externalManagedTags...),
		defaultSSLPolicy:    defaultSSLPolicy,
		enableIPTargetType:  enableIPTargetType,
	}
}

var _ ModelBuilder = &defaultModelBuilder{}

type defaultModelBuilder struct {
	annotationParser    annotations.Parser
	subnetsResolver     networking.SubnetsResolver
	vpcInfoProvider     networking.VPCInfoProvider
	trackingProvider    tracking.Provider
	elbv2TaggingManager elbv2deploy.TaggingManager
	featureGates        config.FeatureGates
	serviceUtils        ServiceUtils

	clusterName         string
	vpcID               string
	defaultTags         map[string]string
	externalManagedTags sets.String
	defaultSSLPolicy    string
	enableIPTargetType  bool
}

func (b *defaultModelBuilder) Build(ctx context.Context, service *corev1.Service) (core.Stack, *elbv2model.LoadBalancer, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(service)))
	task := &defaultModelBuildTask{
		clusterName:         b.clusterName,
		vpcID:               b.vpcID,
		annotationParser:    b.annotationParser,
		subnetsResolver:     b.subnetsResolver,
		vpcInfoProvider:     b.vpcInfoProvider,
		trackingProvider:    b.trackingProvider,
		elbv2TaggingManager: b.elbv2TaggingManager,
		featureGates:        b.featureGates,
		serviceUtils:        b.serviceUtils,
		enableIPTargetType:  b.enableIPTargetType,

		service:   service,
		stack:     stack,
		tgByResID: make(map[string]*elbv2model.TargetGroup),

		defaultTags:                          b.defaultTags,
		externalManagedTags:                  b.externalManagedTags,
		defaultSSLPolicy:                     b.defaultSSLPolicy,
		defaultAccessLogS3Enabled:            false,
		defaultAccessLogsS3Bucket:            "",
		defaultAccessLogsS3Prefix:            "",
		defaultIPAddressType:                 elbv2model.IPAddressTypeIPV4,
		defaultLoadBalancingCrossZoneEnabled: false,
		defaultProxyProtocolV2Enabled:        false,
		defaultTargetType:                    elbv2model.TargetTypeInstance,
		defaultHealthCheckProtocol:           elbv2model.ProtocolTCP,
		defaultHealthCheckPort:               healthCheckPortTrafficPort,
		defaultHealthCheckPath:               "/",
		defaultHealthCheckInterval:           10,
		defaultHealthCheckTimeout:            10,
		defaultHealthCheckHealthyThreshold:   3,
		defaultHealthCheckUnhealthyThreshold: 3,
		defaultIPv4SourceRanges:              []string{"0.0.0.0/0"},
		defaultIPv6SourceRanges:              []string{"::/0"},

		defaultHealthCheckPortForInstanceModeLocal:               strconv.Itoa(int(service.Spec.HealthCheckNodePort)),
		defaultHealthCheckProtocolForInstanceModeLocal:           elbv2model.ProtocolHTTP,
		defaultHealthCheckPathForInstanceModeLocal:               "/healthz",
		defaultHealthCheckIntervalForInstanceModeLocal:           10,
		defaultHealthCheckTimeoutForInstanceModeLocal:            6,
		defaultHealthCheckHealthyThresholdForInstanceModeLocal:   2,
		defaultHealthCheckUnhealthyThresholdForInstanceModeLocal: 2,
	}

	if err := task.run(ctx); err != nil {
		return nil, nil, err
	}
	return task.stack, task.loadBalancer, nil
}

type defaultModelBuildTask struct {
	clusterName         string
	vpcID               string
	annotationParser    annotations.Parser
	subnetsResolver     networking.SubnetsResolver
	vpcInfoProvider     networking.VPCInfoProvider
	trackingProvider    tracking.Provider
	elbv2TaggingManager elbv2deploy.TaggingManager
	featureGates        config.FeatureGates
	serviceUtils        ServiceUtils
	enableIPTargetType  bool

	service *corev1.Service

	stack        core.Stack
	loadBalancer *elbv2model.LoadBalancer
	tgByResID    map[string]*elbv2model.TargetGroup
	ec2Subnets   []*ec2.Subnet

	fetchExistingLoadBalancerOnce sync.Once
	existingLoadBalancer          *elbv2deploy.LoadBalancerWithTags

	defaultTags                          map[string]string
	externalManagedTags                  sets.String
	defaultSSLPolicy                     string
	defaultAccessLogS3Enabled            bool
	defaultAccessLogsS3Bucket            string
	defaultAccessLogsS3Prefix            string
	defaultIPAddressType                 elbv2model.IPAddressType
	defaultLoadBalancingCrossZoneEnabled bool
	defaultProxyProtocolV2Enabled        bool
	defaultTargetType                    elbv2model.TargetType
	defaultHealthCheckProtocol           elbv2model.Protocol
	defaultHealthCheckPort               string
	defaultHealthCheckPath               string
	defaultHealthCheckInterval           int64
	defaultHealthCheckTimeout            int64
	defaultHealthCheckHealthyThreshold   int64
	defaultHealthCheckUnhealthyThreshold int64
	defaultDeletionProtectionEnabled     bool
	defaultIPv4SourceRanges              []string
	defaultIPv6SourceRanges              []string

	// Default health check settings for NLB instance mode with spec.ExternalTrafficPolicy set to Local
	defaultHealthCheckProtocolForInstanceModeLocal           elbv2model.Protocol
	defaultHealthCheckPortForInstanceModeLocal               string
	defaultHealthCheckPathForInstanceModeLocal               string
	defaultHealthCheckIntervalForInstanceModeLocal           int64
	defaultHealthCheckTimeoutForInstanceModeLocal            int64
	defaultHealthCheckHealthyThresholdForInstanceModeLocal   int64
	defaultHealthCheckUnhealthyThresholdForInstanceModeLocal int64
}

func (t *defaultModelBuildTask) run(ctx context.Context) error {
	if !t.serviceUtils.IsServiceSupported(t.service) {
		if t.serviceUtils.IsServicePendingFinalization(t.service) {
			deletionProtectionEnabled, err := t.getDeletionProtectionViaAnnotation(*t.service)
			if err != nil {
				return err
			}
			if deletionProtectionEnabled {
				return errors.Errorf("deletion_protection is enabled, cannot delete the service: %v", t.service.Name)
			}
		}
		return nil
	}
	err := t.buildModel(ctx)
	return err
}

func (t *defaultModelBuildTask) buildModel(ctx context.Context) error {
	scheme, err := t.buildLoadBalancerScheme(ctx)
	if err != nil {
		return err
	}
	t.ec2Subnets, err = t.buildLoadBalancerSubnets(ctx, scheme)
	if err != nil {
		return err
	}
	err = t.buildLoadBalancer(ctx, scheme)
	if err != nil {
		return err
	}
	err = t.buildListeners(ctx, scheme)
	if err != nil {
		return err
	}
	return nil
}

func (t *defaultModelBuildTask) getDeletionProtectionViaAnnotation(svc corev1.Service) (bool, error) {
	var lbAttributes map[string]string
	_, err := t.annotationParser.ParseStringMapAnnotation(annotations.SvcLBSuffixLoadBalancerAttributes, &lbAttributes, svc.Annotations)
	if err != nil {
		return false, err
	}
	if _, deletionProtectionSpecified := lbAttributes[lbAttrsDeletionProtection]; deletionProtectionSpecified {
		deletionProtectionEnabled, err := strconv.ParseBool(lbAttributes[lbAttrsDeletionProtection])
		if err != nil {
			return false, err
		}
		return deletionProtectionEnabled, nil
	}
	return false, nil
}
