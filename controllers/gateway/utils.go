package gateway

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/controllers/gateway/eventhandlers"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	gatewayClassAnnotationLastProcessedConfig          = "elbv2.k8s.aws/last-processed-config"
	gatewayClassAnnotationLastProcessedConfigTimestamp = gatewayClassAnnotationLastProcessedConfig + "-timestamp"

	// The max message that can be stored in a condition
	maxMessageLength = 32700
)

// updateGatewayClassLastProcessedConfig updates the gateway class annotations with the last processed lb config resource version or "" if no lb config is attached to the gatewayclass
func updateGatewayClassLastProcessedConfig(ctx context.Context, k8sClient client.Client, gwClass *gwv1.GatewayClass, lbConf *elbv2gw.LoadBalancerConfiguration) error {

	calculatedVersion := ""

	if lbConf != nil {
		calculatedVersion = lbConf.ResourceVersion
	}

	storedVersion := getStoredProcessedConfig(gwClass)

	if storedVersion != nil && *storedVersion == calculatedVersion {
		return nil
	}

	if gwClass.Annotations == nil {
		gwClass.Annotations = make(map[string]string)
	}

	gwClassOld := gwClass.DeepCopy()
	if gwClass.Annotations == nil {
		gwClass.Annotations = make(map[string]string)
	}
	gwClass.Annotations[gatewayClassAnnotationLastProcessedConfig] = calculatedVersion
	gwClass.Annotations[gatewayClassAnnotationLastProcessedConfigTimestamp] = strconv.FormatInt(time.Now().Unix(), 10)

	return k8sClient.Patch(ctx, gwClass, client.MergeFrom(gwClassOld))
}

// getStoredProcessedConfig retrieves the resource version attached to the lb config referenced by the gateway class or nil if no such mapping exists.
func getStoredProcessedConfig(gwClass *gwv1.GatewayClass) *string {
	var storedVersion *string

	if gwClass.Annotations != nil {
		v, exists := gwClass.Annotations[gatewayClassAnnotationLastProcessedConfig]
		if exists {
			storedVersion = &v
		}
	}
	return storedVersion
}

// updateGatewayClassAcceptedCondition updates the 'accepted' condition on the gateway class to the passed in parameters. if no 'Accepted' condition exists, do nothing.
func updateGatewayClassAcceptedCondition(ctx context.Context, k8sClient client.Client, gwClass *gwv1.GatewayClass, newStatus metav1.ConditionStatus, reason string, message string) error {
	indxToUpdate, ok := deriveAcceptedConditionIndex(gwClass)

	if ok {

		storedStatus := gwClass.Status.Conditions[indxToUpdate].Status
		storedMessage := gwClass.Status.Conditions[indxToUpdate].Message
		storedReason := gwClass.Status.Conditions[indxToUpdate].Reason

		if storedStatus == newStatus && storedMessage == message && storedReason == reason {
			return nil
		}

		gwClassOld := gwClass.DeepCopy()
		gwClass.Status.Conditions[indxToUpdate].LastTransitionTime = metav1.NewTime(time.Now())
		gwClass.Status.Conditions[indxToUpdate].ObservedGeneration = gwClass.Generation
		gwClass.Status.Conditions[indxToUpdate].Status = newStatus
		gwClass.Status.Conditions[indxToUpdate].Message = message
		gwClass.Status.Conditions[indxToUpdate].Reason = reason
		if err := k8sClient.Status().Patch(ctx, gwClass, client.MergeFrom(gwClassOld)); err != nil {
			return errors.Wrapf(err, "failed to update gatewayclass status")
		}
	}
	return nil
}

// prepareGatewayConditionUpdate inserts the necessary data into the condition field of the gateway. The caller should patch the corresponding gateway. Returns false when no change was performed.
func prepareGatewayConditionUpdate(gw *gwv1.Gateway, targetConditionType string, newStatus metav1.ConditionStatus, reason string, message string) bool {

	indxToUpdate := -1
	var derivedCondition metav1.Condition
	for i, condition := range gw.Status.Conditions {
		if condition.Type == targetConditionType {
			indxToUpdate = i
			derivedCondition = condition
			break
		}
	}

	// 32768 is the max message limit
	truncatedMessage := truncateMessage(message)

	if indxToUpdate != -1 {
		if derivedCondition.Status != newStatus || derivedCondition.Message != truncatedMessage || derivedCondition.Reason != reason {
			gw.Status.Conditions[indxToUpdate].LastTransitionTime = metav1.NewTime(time.Now())
			gw.Status.Conditions[indxToUpdate].ObservedGeneration = gw.Generation
			gw.Status.Conditions[indxToUpdate].Status = newStatus
			gw.Status.Conditions[indxToUpdate].Message = truncatedMessage
			gw.Status.Conditions[indxToUpdate].Reason = reason
			return true
		}
	}
	return false
}

func truncateMessage(s string) string {
	if utf8.RuneCountInString(s) <= maxMessageLength {
		return s
	}

	runes := []rune(s)
	return string(runes[:maxMessageLength]) + "..."
}

// deriveAcceptedConditionIndex returns the index of the condition pertaining to the accepted condition.
// -1 if the condition doesn't exist
func deriveAcceptedConditionIndex(gwClass *gwv1.GatewayClass) (int, bool) {
	for i, v := range gwClass.Status.Conditions {
		if v.Type == string(gwv1.GatewayClassReasonAccepted) {
			return i, true
		}
	}
	return -1, false
}

// resolveLoadBalancerConfig returns the lb config referenced in the ParametersReference.
func resolveLoadBalancerConfig(ctx context.Context, k8sClient client.Client, reference *gwv1.ParametersReference) (*elbv2gw.LoadBalancerConfiguration, error) {
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

// generateRouteList generate a deterministic route list.
//
//	Due to the nature of golang maps, we need to sort the keys and for good measure we sort the route descriptors too
func generateRouteList(listenerRoutes map[int32][]routeutils.RouteDescriptor) string {

	allRoutes := make([]string, 0)

	for _, lr := range listenerRoutes {
		for _, r := range lr {
			allRoutes = append(allRoutes, fmt.Sprintf("(%s, %s:%s)", r.GetRouteKind(), r.GetRouteNamespacedName().Namespace, r.GetRouteNamespacedName().Name))
		}
	}

	sort.Strings(allRoutes)

	return strings.Join(allRoutes, ",")
}

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

func RemoveLoadBalancerConfigurationFinalizers(ctx context.Context, gw *gwv1.Gateway, gwClass *gwv1.GatewayClass, k8sClient client.Client, manager k8s.FinalizerManager, controllerName string) error {
	// remove finalizer from lbConfig - gatewayClass
	if gwClass.Spec.ParametersRef != nil && string(gwClass.Spec.ParametersRef.Kind) == constants.LoadBalancerConfiguration {
		lbConfig := &elbv2gw.LoadBalancerConfiguration{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Namespace: string(*gwClass.Spec.ParametersRef.Namespace),
			Name:      gwClass.Spec.ParametersRef.Name,
		}, lbConfig); err != nil {
			return client.IgnoreNotFound(err)
		}
		// remove finalizer if it exists and it not in use
		if k8s.HasFinalizer(lbConfig, shared_constants.LoadBalancerConfigurationFinalizer) && !isLBConfigInUse(ctx, lbConfig, gw, gwClass, k8sClient, controllerName) {
			if err := manager.RemoveFinalizers(ctx, lbConfig, shared_constants.LoadBalancerConfigurationFinalizer); err != nil {
				return err
			}
		}
	}

	// remove finalizer from lbConfig - gateway
	if gw.Spec.Infrastructure != nil && gw.Spec.Infrastructure.ParametersRef != nil && string(gw.Spec.Infrastructure.ParametersRef.Kind) == constants.LoadBalancerConfiguration {
		lbConfig := &elbv2gw.LoadBalancerConfiguration{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Spec.Infrastructure.ParametersRef.Name,
		}, lbConfig); err != nil {
			return client.IgnoreNotFound(err)
		}
		// remove finalizer if it exists and it is not in use
		if k8s.HasFinalizer(lbConfig, shared_constants.LoadBalancerConfigurationFinalizer) && !isLBConfigInUse(ctx, lbConfig, gw, gwClass, k8sClient, controllerName) {
			if err := manager.RemoveFinalizers(ctx, lbConfig, shared_constants.LoadBalancerConfigurationFinalizer); err != nil {
				return err
			}
		}
	}
	return nil
}

func isLBConfigInUse(ctx context.Context, lbConfig *elbv2gw.LoadBalancerConfiguration, gw *gwv1.Gateway, gwClass *gwv1.GatewayClass, k8sClient client.Client, controllerName string) bool {
	// check if lbConfig is referred by any other gateway
	gwsUsingLBConfig := eventhandlers.GetImpactedGatewaysFromLbConfig(ctx, k8sClient, lbConfig, controllerName)
	for _, gwUsingLBConfig := range gwsUsingLBConfig {
		if gwUsingLBConfig.Name != gw.Name || gwUsingLBConfig.Namespace != gw.Namespace {
			return true
		}
	}

	// check if lbConfig is referred by any other gatewayClass
	gwClassesUsingLBConfig := eventhandlers.GetImpactedGatewayClassesFromLbConfig(ctx, k8sClient, lbConfig, sets.New(controllerName))
	return len(gwClassesUsingLBConfig) > 0
}
