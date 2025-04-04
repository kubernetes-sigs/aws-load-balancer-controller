package model

import (
	"context"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// Builder builds the model stack for a Gateway resource.
type Builder interface {
	// Build model stack for a gateway
	Build(ctx context.Context, gw *gwv1.Gateway, gwClass *gwv1.GatewayClass, routes map[int][]routeutils.RouteDescriptor) (core.Stack, *elbv2model.LoadBalancer, bool, error)
}

// NewDefaultModelBuilder construct a new defaultModelBuilder
func NewDefaultModelBuilder(subnetsResolver networking.SubnetsResolver,
	vpcInfoProvider networking.VPCInfoProvider, vpcID string, trackingProvider tracking.Provider,
	elbv2TaggingManager elbv2deploy.TaggingManager, ec2Client services.EC2, featureGates config.FeatureGates, clusterName string, defaultTags map[string]string,
	externalManagedTags sets.Set[string], defaultSSLPolicy string, defaultTargetType string, defaultLoadBalancerScheme string,
	backendSGProvider networking.BackendSGProvider, sgResolver networking.SecurityGroupResolver, enableBackendSG bool,
	disableRestrictedSGRules bool, logger logr.Logger) Builder {
	return &defaultModelBuilder{
		subnetsResolver:          subnetsResolver,
		vpcInfoProvider:          vpcInfoProvider,
		backendSGProvider:        backendSGProvider,
		sgResolver:               sgResolver,
		trackingProvider:         trackingProvider,
		elbv2TaggingManager:      elbv2TaggingManager,
		featureGates:             featureGates,
		ec2Client:                ec2Client,
		enableBackendSG:          enableBackendSG,
		disableRestrictedSGRules: disableRestrictedSGRules,

		clusterName:               clusterName,
		vpcID:                     vpcID,
		defaultTags:               defaultTags,
		externalManagedTags:       externalManagedTags,
		defaultSSLPolicy:          defaultSSLPolicy,
		defaultTargetType:         elbv2model.TargetType(defaultTargetType),
		defaultLoadBalancerScheme: elbv2model.LoadBalancerScheme(defaultLoadBalancerScheme),
		logger:                    logger,
	}
}

var _ Builder = &defaultModelBuilder{}

type defaultModelBuilder struct {
	subnetsResolver          networking.SubnetsResolver
	vpcInfoProvider          networking.VPCInfoProvider
	backendSGProvider        networking.BackendSGProvider
	sgResolver               networking.SecurityGroupResolver
	trackingProvider         tracking.Provider
	elbv2TaggingManager      elbv2deploy.TaggingManager
	featureGates             config.FeatureGates
	ec2Client                services.EC2
	enableBackendSG          bool
	disableRestrictedSGRules bool

	clusterName               string
	vpcID                     string
	defaultTags               map[string]string
	externalManagedTags       sets.Set[string]
	defaultSSLPolicy          string
	defaultTargetType         elbv2model.TargetType
	defaultLoadBalancerScheme elbv2model.LoadBalancerScheme
	logger                    logr.Logger
}

func (d *defaultModelBuilder) Build(ctx context.Context, gw *gwv1.Gateway, gwClass *gwv1.GatewayClass, routes map[int][]routeutils.RouteDescriptor) (core.Stack, *elbv2model.LoadBalancer, bool, error) {
	//TODO implement me
	panic("implement me")
}
