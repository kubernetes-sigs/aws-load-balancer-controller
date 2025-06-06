package gateway

import (
	"context"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/gatewayutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type gatewayConfigResolver interface {
	getLoadBalancerConfigForGateway(ctx context.Context, k8sClient client.Client, gw *gwv1.Gateway, gwClass *gwv1.GatewayClass) (elbv2gw.LoadBalancerConfiguration, error)
}

type gatewayConfigResolverImpl struct {
	configMergeFn    func(gwClassLbConfig elbv2gw.LoadBalancerConfiguration, gwLbConfig elbv2gw.LoadBalancerConfiguration) elbv2gw.LoadBalancerConfiguration
	configResolverFn func(ctx context.Context, k8sClient client.Client, reference *gwv1.ParametersReference) (*elbv2gw.LoadBalancerConfiguration, error)
}

func newGatewayConfigResolver() gatewayConfigResolver {
	return &gatewayConfigResolverImpl{
		configMergeFn:    gateway.NewLoadBalancerConfigMerger().Merge,
		configResolverFn: gatewayutils.ResolveLoadBalancerConfig,
	}
}

func (resolver *gatewayConfigResolverImpl) getLoadBalancerConfigForGateway(ctx context.Context, k8sClient client.Client, gw *gwv1.Gateway, gwClass *gwv1.GatewayClass) (elbv2gw.LoadBalancerConfiguration, error) {

	// If the Gateway Class isn't accepted, we shouldn't try to reconcile this Gateway.
	derivedStatusIndx, ok := deriveAcceptedConditionIndex(gwClass)

	if !ok || gwClass.Status.Conditions[derivedStatusIndx].Status != metav1.ConditionTrue {
		return elbv2gw.LoadBalancerConfiguration{}, errors.Errorf("Unable to materialize gateway when gateway class [%s] is not accepted", gwClass.Name)
	}

	gatewayClassLBConfig, err := resolver.configResolverFn(ctx, k8sClient, gwClass.Spec.ParametersRef)

	if err != nil {
		return elbv2gw.LoadBalancerConfiguration{}, err
	}

	if gatewayClassLBConfig != nil {
		storedVersion := getStoredProcessedConfig(gwClass)
		if storedVersion == nil || *storedVersion != gatewayClassLBConfig.ResourceVersion {
			var safeVersion string
			if storedVersion != nil {
				safeVersion = *storedVersion
			}
			return elbv2gw.LoadBalancerConfiguration{}, errors.Errorf("GatewayClass [%s] hasn't processed latest loadbalancer config. Processed version %s, Latest version %s", gwClass.Name, safeVersion, gatewayClassLBConfig.ResourceVersion)
		}
	}

	var gwParametersRef = gatewayutils.GetNamespacedParamRefForGateway(gw)

	gatewayLBConfig, err := resolver.configResolverFn(ctx, k8sClient, gwParametersRef)

	if err != nil {
		return elbv2gw.LoadBalancerConfiguration{}, err
	}

	if gatewayClassLBConfig == nil && gatewayLBConfig == nil {
		return elbv2gw.LoadBalancerConfiguration{}, nil
	}

	if gatewayClassLBConfig == nil {
		return *gatewayLBConfig, nil
	}

	if gatewayLBConfig == nil {
		return *gatewayClassLBConfig, nil
	}

	return resolver.configMergeFn(*gatewayClassLBConfig, *gatewayLBConfig), nil
}
