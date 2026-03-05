package gateway

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/gatewayutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type gatewayConfigResolver interface {
	getLoadBalancerConfigForGateway(ctx context.Context, k8sClient client.Client, finalizerManager k8s.FinalizerManager, gw *gwv1.Gateway, gwClass *gwv1.GatewayClass) (elbv2gw.LoadBalancerConfiguration, *elbv2gw.TargetGroupConfiguration, error)
}

type gatewayConfigResolverImpl struct {
	configMergeFn       func(gwClassLbConfig elbv2gw.LoadBalancerConfiguration, gwLbConfig elbv2gw.LoadBalancerConfiguration) elbv2gw.LoadBalancerConfiguration
	configResolverFn    func(ctx context.Context, k8sClient client.Client, reference *gwv1.ParametersReference) (*elbv2gw.LoadBalancerConfiguration, error)
	tgConfigConstructor gateway.TargetGroupConfigConstructor
	logger              logr.Logger
}

func newGatewayConfigResolver(logger logr.Logger) gatewayConfigResolver {
	return &gatewayConfigResolverImpl{
		configMergeFn:       gateway.NewLoadBalancerConfigMerger().Merge,
		configResolverFn:    gatewayutils.ResolveLoadBalancerConfig,
		tgConfigConstructor: gateway.NewTargetGroupConfigConstructor(),
		logger:              logger,
	}
}

func (resolver *gatewayConfigResolverImpl) getLoadBalancerConfigForGateway(ctx context.Context, k8sClient client.Client, finalizerManager k8s.FinalizerManager, gw *gwv1.Gateway, gwClass *gwv1.GatewayClass) (elbv2gw.LoadBalancerConfiguration, *elbv2gw.TargetGroupConfiguration, error) {

	// If the Gateway Class isn't accepted, we shouldn't try to reconcile this Gateway.
	derivedStatusIndx, ok := deriveAcceptedConditionIndex(gwClass)

	if !ok || gwClass.Status.Conditions[derivedStatusIndx].Status != metav1.ConditionTrue {
		return elbv2gw.LoadBalancerConfiguration{}, nil, errors.Errorf("Unable to materialize gateway when gateway class [%s] is not accepted", gwClass.Name)
	}

	gatewayClassLBConfig, err := resolver.configResolverFn(ctx, k8sClient, gwClass.Spec.ParametersRef)

	if err != nil {
		return elbv2gw.LoadBalancerConfiguration{}, nil, err
	}

	if gatewayClassLBConfig != nil {
		if !k8s.HasFinalizer(gatewayClassLBConfig, shared_constants.LoadBalancerConfigurationFinalizer) {
			if err := finalizerManager.AddFinalizers(ctx, gatewayClassLBConfig, shared_constants.LoadBalancerConfigurationFinalizer); err != nil {
				return elbv2gw.LoadBalancerConfiguration{}, nil, errors.Errorf("failed to add finalizers on load balancer configuration %s", k8s.NamespacedName(gatewayClassLBConfig))
			}
		}
		storedVersion := getStoredProcessedConfig(gwClass)
		if storedVersion == nil || *storedVersion != gatewayClassLBConfig.ResourceVersion {
			var safeVersion string
			if storedVersion != nil {
				safeVersion = *storedVersion
			}
			return elbv2gw.LoadBalancerConfiguration{}, nil, errors.Errorf("GatewayClass [%s] hasn't processed latest loadbalancer config. Processed version %s, Latest version %s", gwClass.Name, safeVersion, gatewayClassLBConfig.ResourceVersion)
		}
	}

	var gwParametersRef = gatewayutils.GetNamespacedParamRefForGateway(gw)

	gatewayLBConfig, err := resolver.configResolverFn(ctx, k8sClient, gwParametersRef)

	if err != nil {
		return elbv2gw.LoadBalancerConfiguration{}, nil, err
	}

	if gatewayLBConfig != nil {
		if !k8s.HasFinalizer(gatewayLBConfig, shared_constants.LoadBalancerConfigurationFinalizer) {
			if err := finalizerManager.AddFinalizers(ctx, gatewayLBConfig, shared_constants.LoadBalancerConfigurationFinalizer); err != nil {
				return elbv2gw.LoadBalancerConfiguration{}, nil, errors.Errorf("failed to add finalizers on load balancer configuration %s", k8s.NamespacedName(gatewayLBConfig))
			}
		}
	}

	// Resolve default TGCs from both LBCs before merging.
	resolvedDefaultTGC := resolver.resolveAndMergeDefaultTGCs(ctx, k8sClient, gatewayClassLBConfig, gatewayLBConfig)

	var mergedLBConfig elbv2gw.LoadBalancerConfiguration
	if gatewayClassLBConfig == nil && gatewayLBConfig == nil {
		mergedLBConfig = elbv2gw.LoadBalancerConfiguration{}
	} else if gatewayClassLBConfig == nil {
		mergedLBConfig = *gatewayLBConfig
	} else if gatewayLBConfig == nil {
		mergedLBConfig = *gatewayClassLBConfig
	} else {
		mergedLBConfig = resolver.configMergeFn(*gatewayClassLBConfig, *gatewayLBConfig)
	}

	return mergedLBConfig, resolvedDefaultTGC, nil
}

// resolveAndMergeDefaultTGCs resolves the default TGC from both the GatewayClass LBC and Gateway LBC,
// then merges their props based on mergingMode. For the Gateway LBC, the TGC must be in the same namespace
// as the Gateway; otherwise it is treated as nil.
func (resolver *gatewayConfigResolverImpl) resolveAndMergeDefaultTGCs(ctx context.Context, k8sClient client.Client, gwClassLBC *elbv2gw.LoadBalancerConfiguration, gwLBC *elbv2gw.LoadBalancerConfiguration) *elbv2gw.TargetGroupConfiguration {
	var gwClassDefaultTGC *elbv2gw.TargetGroupConfiguration
	var gwDefaultTGC *elbv2gw.TargetGroupConfiguration

	// Resolve GatewayClass-level default TGC (from the GatewayClass LBC's namespace)
	if gwClassLBC != nil && gwClassLBC.Spec.DefaultTargetGroupConfiguration != nil {
		tgc, err := lookUpDefaultTGCByName(ctx, k8sClient, gwClassLBC.Spec.DefaultTargetGroupConfiguration.Name, gwClassLBC.Namespace)
		if err != nil {
			resolver.logger.Error(fmt.Errorf("TargetGroupConfiguration %q not found in namespace %q",
				gwClassLBC.Spec.DefaultTargetGroupConfiguration.Name, gwClassLBC.Namespace),
				"default TargetGroupConfiguration referenced by GatewayClass LoadBalancerConfiguration not found, proceeding without GatewayClass-level defaults")
		} else if tgc != nil {
			gwClassDefaultTGC = tgc
		}
	}

	// Resolve Gateway-level default TGC (looked up in the LBC's namespace, which is the same as the Gateway's namespace)
	if gwLBC != nil && gwLBC.Spec.DefaultTargetGroupConfiguration != nil {
		tgc, err := lookUpDefaultTGCByName(ctx, k8sClient, gwLBC.Spec.DefaultTargetGroupConfiguration.Name, gwLBC.Namespace)
		if err != nil {
			resolver.logger.Error(fmt.Errorf("TargetGroupConfiguration %q not found in namespace %q",
				gwLBC.Spec.DefaultTargetGroupConfiguration.Name, gwLBC.Namespace),
				"default TargetGroupConfiguration referenced by Gateway LoadBalancerConfiguration not found, the TGC must be in the same namespace as the LBC")
		} else if tgc != nil {
			gwDefaultTGC = tgc
		}
	}

	mergeMode := elbv2gw.MergeModePreferGatewayClass
	if gwClassLBC != nil && gwClassLBC.Spec.MergingMode != nil {
		mergeMode = *gwClassLBC.Spec.MergingMode
	}

	return resolver.tgConfigConstructor.MergeDefaultTGCs(gwClassDefaultTGC, gwDefaultTGC, mergeMode)
}

func lookUpDefaultTGCByName(ctx context.Context, k8sClient client.Client, name, namespace string) (*elbv2gw.TargetGroupConfiguration, error) {
	tgc := &elbv2gw.TargetGroupConfiguration{}
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, tgc)
	if err != nil {
		return nil, err
	}
	return tgc, nil
}
