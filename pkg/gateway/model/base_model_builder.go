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
	elbv2TaggingManager elbv2deploy.TaggingManager, ec2Client services.EC2, featureGates config.FeatureGates, clusterName string, defaultTags map[string]string,
	externalManagedTags sets.Set[string], defaultSSLPolicy string, defaultTargetType string, defaultLoadBalancerScheme string,
	backendSGProvider networking.BackendSGProvider, sgResolver networking.SecurityGroupResolver, enableBackendSG bool,
	disableRestrictedSGRules bool, logger logr.Logger) Builder {

	subnetBuilder := newSubnetModelBuilder(loadBalancerType, trackingProvider, subnetsResolver, elbv2TaggingManager)

	return &baseModelBuilder{
		lbBuilder: newLoadBalancerBuilder(subnetBuilder, defaultLoadBalancerScheme),
	}
}

var _ Builder = &baseModelBuilder{}

type baseModelBuilder struct {
	lbBuilder loadBalancerBuilder
	logger    logr.Logger
}

func (baseBuilder *baseModelBuilder) Build(ctx context.Context, gw *gwv1.Gateway, lbConf *elbv2gw.LoadBalancerConfiguration, routes map[int][]routeutils.RouteDescriptor) (core.Stack, *elbv2model.LoadBalancer, bool, error) {
	if gw.DeletionTimestamp != nil && !gw.DeletionTimestamp.IsZero() {
		if baseBuilder.isDeleteProtected(lbConf) {
			return nil, nil, false, errors.Errorf("Unable to delete gateway %+v because deletion protection is enabled.", k8s.NamespacedName(gw))
		}
	}

	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(gw)))

	// TODO - Fix
	_, err := baseBuilder.lbBuilder.buildLoadBalancerSpec(ctx, gw, stack, lbConf, routes)

	if err != nil {
		return nil, nil, false, err
	}

	return stack, nil, false, nil
}

func (baseBuilder *baseModelBuilder) isDeleteProtected(lbConf *elbv2gw.LoadBalancerConfiguration) bool {
	if lbConf == nil {
		return false
	}

	for _, attr := range lbConf.Spec.LoadBalancerAttributes {
		if attr.Key == deletionProtectionAttributeKey {
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
