package eventhandlers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// IsGatewayManagedByLBController checks if a Gateway is managed by the ALB/NLB Gateway Controller
// by verifying its associated GatewayClass controller name.
func IsGatewayManagedByLBController(ctx context.Context, k8sClient client.Client, gw *gwv1.Gateway, gwController string) bool {
	if gw == nil {
		return false
	}

	gwClass := &gwv1.GatewayClass{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: string(gw.Spec.GatewayClassName)}, gwClass); err != nil {
		return false
	}
	return string(gwClass.Spec.ControllerName) == gwController
}

// GetGatewayClassesManagedByLBController retrieves all GatewayClasses managed by the ALB/NLB Gateway Controller.
func GetGatewayClassesManagedByLBController(ctx context.Context, k8sClient client.Client, gwControllers sets.Set[string]) []*gwv1.GatewayClass {
	managedGatewayClasses := make([]*gwv1.GatewayClass, 0)
	gwClassList := &gwv1.GatewayClassList{}
	if err := k8sClient.List(ctx, gwClassList); err != nil {
		return managedGatewayClasses
	}
	managedGatewayClasses = make([]*gwv1.GatewayClass, 0, len(gwClassList.Items))

	for i := range gwClassList.Items {
		if gwControllers.Has(string(gwClassList.Items[i].Spec.ControllerName)) {
			managedGatewayClasses = append(managedGatewayClasses, &gwClassList.Items[i])
		}
	}
	return managedGatewayClasses
}

// GetGatewaysManagedByLBController retrieves all Gateways managed by the ALB/NLB Gateway Controller.
func GetGatewaysManagedByLBController(ctx context.Context, k8sClient client.Client, gwController string) []*gwv1.Gateway {
	managedGateways := make([]*gwv1.Gateway, 0)
	gwList := &gwv1.GatewayList{}

	if err := k8sClient.List(ctx, gwList); err != nil {
		return managedGateways
	}

	managedGateways = make([]*gwv1.Gateway, 0, len(gwList.Items))

	for i := range gwList.Items {
		if IsGatewayManagedByLBController(ctx, k8sClient, &gwList.Items[i], gwController) {
			managedGateways = append(managedGateways, &gwList.Items[i])
		}
	}
	return managedGateways
}

// GetImpactedGatewaysFromParentRefs identifies Gateways affected by changes in parent references.
// Returns Gateways that are impacted and managed by the LB controller.
func GetImpactedGatewaysFromParentRefs(ctx context.Context, k8sClient client.Client, parentRefs []gwv1.ParentReference, originalParentRefsFromStatus []gwv1.RouteParentStatus, resourceNamespace string, gwController string) ([]types.NamespacedName, error) {
	for _, originalParentRef := range originalParentRefsFromStatus {
		parentRefs = append(parentRefs, originalParentRef.ParentRef)
	}
	parentRefs = removeDuplicateParentRefs(parentRefs, resourceNamespace)

	if len(parentRefs) == 0 {
		return nil, nil
	}
	impactedGateways := make([]types.NamespacedName, 0, len(parentRefs))
	unknownGateways := make([]types.NamespacedName, 0, len(parentRefs))
	var err error
	for _, parent := range parentRefs {
		gwNamespace := resourceNamespace
		if parent.Namespace != nil {
			gwNamespace = string(*parent.Namespace)
		}

		gwName := types.NamespacedName{
			Namespace: gwNamespace,
			Name:      string(parent.Name),
		}

		gw := &gwv1.Gateway{}
		if err := k8sClient.Get(ctx, gwName, gw); err != nil {
			// Ignore and continue processing other refs
			unknownGateways = append(unknownGateways, gwName)
			continue
		}

		if IsGatewayManagedByLBController(ctx, k8sClient, gw, gwController) {
			impactedGateways = append(impactedGateways, gwName)
		}
	}
	if len(unknownGateways) > 0 {
		err = fmt.Errorf("failed to list gateways, %s", unknownGateways)
	}
	return impactedGateways, err
}

// GetImpactedGatewayClassesFromLbConfig identifies GatewayClasses affected by LoadBalancer configuration changes.
// Returns GatewayClasses that reference the specified LoadBalancer configuration.
func GetImpactedGatewayClassesFromLbConfig(ctx context.Context, k8sClient client.Client, lbconfig *elbv2gw.LoadBalancerConfiguration, gwControllers sets.Set[string]) map[string]*gwv1.GatewayClass {
	if lbconfig == nil {
		return nil
	}
	managedGwClasses := GetGatewayClassesManagedByLBController(ctx, k8sClient, gwControllers)
	impactedGatewayClasses := make(map[string]*gwv1.GatewayClass, len(managedGwClasses))
	for _, gwClass := range managedGwClasses {
		if gwClass.Spec.ParametersRef != nil && string(gwClass.Spec.ParametersRef.Kind) == constants.LoadBalancerConfiguration && string(*gwClass.Spec.ParametersRef.Namespace) == lbconfig.Namespace && gwClass.Spec.ParametersRef.Name == lbconfig.Name {
			impactedGatewayClasses[gwClass.Name] = gwClass
		}
	}
	return impactedGatewayClasses
}

// GetImpactedGatewaysFromLbConfig identifies Gateways affected by LoadBalancer configuration changes.
// Returns Gateways that reference the specified LoadBalancer configuration.
func GetImpactedGatewaysFromLbConfig(ctx context.Context, k8sClient client.Client, lbconfig *elbv2gw.LoadBalancerConfiguration, gwController string) []*gwv1.Gateway {
	if lbconfig == nil {
		return nil
	}
	managedGateways := GetGatewaysManagedByLBController(ctx, k8sClient, gwController)
	impactedGateways := make([]*gwv1.Gateway, 0, len(managedGateways))
	for _, gw := range managedGateways {
		if gw.Spec.Infrastructure != nil && gw.Spec.Infrastructure.ParametersRef != nil && string(gw.Spec.Infrastructure.ParametersRef.Kind) == constants.LoadBalancerConfiguration && gw.Spec.Infrastructure.ParametersRef.Name == lbconfig.Name {
			impactedGateways = append(impactedGateways, gw)
		}
	}
	return impactedGateways
}

// removeDuplicateParentRefs make sure parentRefs in list is unique
func removeDuplicateParentRefs(parentRefs []gwv1.ParentReference, resourceNamespace string) []gwv1.ParentReference {
	result := make([]gwv1.ParentReference, 0, len(parentRefs))
	exist := sets.Set[types.NamespacedName]{}
	for _, parentRef := range parentRefs {
		if parentRef.Namespace != nil {
			resourceNamespace = string(*parentRef.Namespace)
		}
		namespacedName := types.NamespacedName{
			Namespace: resourceNamespace,
			Name:      string(parentRef.Name),
		}
		if !exist.Has(namespacedName) {
			exist.Insert(namespacedName)
			result = append(result, parentRef)
		}
	}
	return result
}

// RemoveTargetGroupConfigurationFinalizer removes target group configuration finalizer when service is deleted
func RemoveTargetGroupConfigurationFinalizer(ctx context.Context, svc *corev1.Service, k8sClient client.Client, logger logr.Logger, recorder record.EventRecorder) {
	tgConfig, err := routeutils.LookUpTargetGroupConfiguration(ctx, k8sClient, k8s.NamespacedName(svc))
	if err != nil {
		logger.Error(err, "failed to look up target group configuration", "service", svc.Name)
		return
	}
	if tgConfig == nil {
		logger.V(1).Info("TargetGroupConfigurationNotFound, ignoring remove finalizer.", "TargetGroupConfiguration", svc.Name)
		return
	}

	tgFinalizer := shared_constants.TargetGroupConfigurationFinalizer
	if k8s.HasFinalizer(tgConfig, tgFinalizer) {
		finalizerManager := k8s.NewDefaultFinalizerManager(k8sClient, logr.Discard())
		if err := finalizerManager.RemoveFinalizers(ctx, tgConfig, tgFinalizer); err != nil {
			recorder.Event(tgConfig, corev1.EventTypeWarning, k8s.TargetGroupBindingEventReasonFailedRemoveFinalizer, fmt.Sprintf("Failed to remove target group configuration finalizer due to %v", err))
		}
		logger.V(1).Info("Successfully removed target group configuration finalizer.", "TargetGroupConfiguration", tgConfig.Name)
	}
}
