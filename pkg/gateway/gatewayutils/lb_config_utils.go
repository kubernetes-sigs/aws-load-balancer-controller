package gatewayutils

import (
	"context"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// AddLoadBalancerConfigurationFinalizers add finalizer to load balancer configuration when it is in use by gateway or gatewayClass
func AddLoadBalancerConfigurationFinalizers(ctx context.Context, gw *gwv1.Gateway, gwClass *gwv1.GatewayClass, k8sClient client.Client, manager k8s.FinalizerManager, controllerName string) error {
	// add finalizer to lbConfig referred by gatewayClass
	if gwClass.Spec.ParametersRef != nil && string(gwClass.Spec.ParametersRef.Kind) == constants.LoadBalancerConfiguration {
		lbConfig := &elbv2gw.LoadBalancerConfiguration{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Namespace: string(*gwClass.Spec.ParametersRef.Namespace),
			Name:      gwClass.Spec.ParametersRef.Name,
		}, lbConfig); err != nil {
			return client.IgnoreNotFound(err)
		}
		if err := manager.AddFinalizers(ctx, lbConfig, shared_constants.LoadBalancerConfigurationFinalizer); err != nil {
			return err
		}
	}

	// add finalizer to lbConfig referred by gateway
	if gw.Spec.Infrastructure != nil && gw.Spec.Infrastructure.ParametersRef != nil && string(gw.Spec.Infrastructure.ParametersRef.Kind) == constants.LoadBalancerConfiguration {
		lbConfig := &elbv2gw.LoadBalancerConfiguration{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Spec.Infrastructure.ParametersRef.Name,
		}, lbConfig); err != nil {
			return client.IgnoreNotFound(err)
		}
		if err := manager.AddFinalizers(ctx, lbConfig, shared_constants.LoadBalancerConfigurationFinalizer); err != nil {
			return err
		}
	}
	return nil
}

func RemoveLoadBalancerConfigurationFinalizers(ctx context.Context, gw *gwv1.Gateway, gwClass *gwv1.GatewayClass, k8sClient client.Client, manager k8s.FinalizerManager, controllerNames sets.Set[string]) error {
	// remove finalizer from lbConfig - gatewayClass
	if gwClass != nil {
		gatewayClassLBConfig, err := ResolveLoadBalancerConfig(ctx, k8sClient, gwClass.Spec.ParametersRef)
		if err != nil {
			return client.IgnoreNotFound(err)
		}
		// remove finalizer if it exists and it not in use
		if gatewayClassLBConfig != nil &&
			k8s.HasFinalizer(gatewayClassLBConfig, shared_constants.LoadBalancerConfigurationFinalizer) &&
			!IsLBConfigInUse(ctx, gatewayClassLBConfig, gw, gwClass, k8sClient, controllerNames) {
			if err := manager.RemoveFinalizers(ctx, gatewayClassLBConfig, shared_constants.LoadBalancerConfigurationFinalizer); err != nil {
				return err
			}
		}
	}
	// remove finalizer from lbConfig - gateway
	if gw != nil {
		var gwParametersRef = GetNamespacedParamRefForGateway(gw)
		gatewayLBConfig, err := ResolveLoadBalancerConfig(ctx, k8sClient, gwParametersRef)
		if err != nil {
			return client.IgnoreNotFound(err)
		}
		// remove finalizer if it exists and it is not in use
		if gatewayLBConfig != nil &&
			k8s.HasFinalizer(gatewayLBConfig, shared_constants.LoadBalancerConfigurationFinalizer) &&
			!IsLBConfigInUse(ctx, gatewayLBConfig, gw, gwClass, k8sClient, controllerNames) {
			if err := manager.RemoveFinalizers(ctx, gatewayLBConfig, shared_constants.LoadBalancerConfigurationFinalizer); err != nil {
				return err
			}
		}

	}
	return nil
}

// ResolveLoadBalancerConfig returns the lb config referenced in the ParametersReference.
func ResolveLoadBalancerConfig(ctx context.Context, k8sClient client.Client, reference *gwv1.ParametersReference) (*elbv2gw.LoadBalancerConfiguration, error) {
	var lbConf *elbv2gw.LoadBalancerConfiguration

	var err error
	if reference != nil {
		lbConf = &elbv2gw.LoadBalancerConfiguration{}
		if reference.Namespace != nil {
			err = k8sClient.Get(ctx, types.NamespacedName{
				Namespace: string(*reference.Namespace),
				Name:      reference.Name,
			}, lbConf)
		} else {
			err = errors.New("Namespace must be specified in ParametersRef")
		}
	}

	return lbConf, err
}

func IsLBConfigInUse(ctx context.Context, lbConfig *elbv2gw.LoadBalancerConfiguration, gw *gwv1.Gateway, gwClass *gwv1.GatewayClass, k8sClient client.Client, controllerNames sets.Set[string]) bool {
	return IsLBConfigInUseByGatewayClass(ctx, lbConfig, gwClass, k8sClient, controllerNames) ||
		IsLBConfigInUseByGateway(ctx, lbConfig, gw, k8sClient, controllerNames)
}
func IsLBConfigInUseByGatewayClass(ctx context.Context, lbConfig *elbv2gw.LoadBalancerConfiguration, gwClass *gwv1.GatewayClass, k8sClient client.Client, controllerNames sets.Set[string]) bool {
	// fetch all the gateway classes referenced by lb config
	gwClassesUsingLBConfig := GetImpactedGatewayClassesFromLbConfig(ctx, k8sClient, lbConfig, controllerNames)

	// if a specific GatewayClass is supplied as a function parameter, it must be ensured
	// that this particular GatewayClass is included within the collection of classes
	// slated for evaluation, thereby guaranteeing its assessment for active Gateway management.
	if gwClass != nil {
		found := false
		for _, gc := range gwClassesUsingLBConfig {
			if gc.Name == gwClass.Name {
				found = true
				break
			}
		}
		if !found {
			gwClassesUsingLBConfig[gwClass.Name] = gwClass
		}
	}
	// iterate through each GatewayClass identified as referencing the LoadBalancerConfiguration
	// the lbconfig is deemed to be in active use if any of these GatewayClasses
	// are found to be managing one or more active Gateway resources.
	for _, controllerName := range controllerNames.UnsortedList() {
		for _, gwClassUsingLBConfig := range gwClassesUsingLBConfig {
			if len(GetGatewaysManagedByGatewayClass(ctx, k8sClient, gwClassUsingLBConfig, controllerName)) > 0 {
				return true
			}
		}
	}

	return false
}

func IsLBConfigInUseByGateway(ctx context.Context, lbConfig *elbv2gw.LoadBalancerConfiguration, gw *gwv1.Gateway, k8sClient client.Client, controllerNames sets.Set[string]) bool {
	var gwsUsingLBConfig []*gwv1.Gateway
	for _, controllerName := range controllerNames.UnsortedList() {
		gws := GetImpactedGatewaysFromLbConfig(ctx, k8sClient, lbConfig, controllerName)
		gwsUsingLBConfig = append(gwsUsingLBConfig, gws...)
	}
	if gw == nil {
		return len(gwsUsingLBConfig) > 0
	}
	// check if lbConfig is referred by any other gateway
	for _, gwUsingLBConfig := range gwsUsingLBConfig {
		if gwUsingLBConfig.Name != gw.Name || gwUsingLBConfig.Namespace != gw.Namespace {
			return true
		}
	}
	return false
}
