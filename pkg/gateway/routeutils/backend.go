package routeutils

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	serviceKind             = "Service"
	gatewayKind             = "Gateway"
	referenceGrantNotExists = "No explicit ReferenceGrant exists to allow the reference."
	maxWeight               = 999
	gatewayAPIGroup         = "gateway.networking.k8s.io"
	coreAPIGroup            = ""
)

var (
	tgConfigConstructor = gateway.NewTargetGroupConfigConstructor()
)

// TargetGroupConfigurator defines methods used to construct an ELB target group from a Kubernetes based backend.
type TargetGroupConfigurator interface {
	// GetTargetType returns the Target Type to associate with this target group.
	GetTargetType(defaultTargetType elbv2model.TargetType) elbv2model.TargetType
	// GetTargetGroupProps returns the target group properties associated with this backend
	GetTargetGroupProps() *elbv2gw.TargetGroupProps
	// GetBackendNamespacedName returns the namespaced name associated with the underlying backend.
	GetBackendNamespacedName() types.NamespacedName
	// GetIdentifierPort returns the port used when constructing the resource ID for the resource stack.
	GetIdentifierPort() intstr.IntOrString
	// GetExternalTrafficPolicy returns the external traffic policy for this backend service, if not applicable returns "ServiceExternalTrafficPolicyCluster".
	GetExternalTrafficPolicy() corev1.ServiceExternalTrafficPolicyType
	// GetIPAddressType returns the Target Group IP address type
	GetIPAddressType() elbv2model.TargetGroupIPAddressType
	// GetTargetGroupPort returns the port to attach to the Target Group
	GetTargetGroupPort(targetType elbv2model.TargetType) int32
	// GetHealthCheckPort returns the port to send health check traffic
	GetHealthCheckPort(targetType elbv2model.TargetType, isServiceExternalTrafficPolicyTypeLocal bool) (intstr.IntOrString, error)
	// GetProtocolVersion returns the protocol version to use for this target group
	GetProtocolVersion() *elbv2model.ProtocolVersion
}

// Backend an abstraction on the Gateway Backend, meant to hide the underlying backend type from consumers (unless they really want to see it :))
type Backend struct {
	ServiceBackend     *ServiceBackendConfig
	LiteralTargetGroup *LiteralTargetGroupConfig
	GatewayBackend     *GatewayBackendConfig
	Weight             int
}

type attachedRuleAccumulator[RuleType any] interface {
	accumulateRules(ctx context.Context, k8sClient client.Client, route preLoadRouteDescriptor, rules []RuleType, backendRefIterator func(RuleType) []gwv1.BackendRef, listenerRuleConfigRefs func(RuleType) []gwv1.LocalObjectReference, ruleConverter func(*RuleType, []Backend, *elbv2gw.ListenerRuleConfiguration) RouteRule, gatewayDefaultTGConfig *elbv2gw.TargetGroupConfiguration) ([]RouteRule, []routeLoadError)
}

type attachedRuleAccumulatorImpl[RuleType any] struct {
	backendLoader            func(ctx context.Context, k8sClient client.Client, backendRef gwv1.BackendRef, routeIdentifier types.NamespacedName, routeKind RouteKind, gatewayDefaultTGConfig *elbv2gw.TargetGroupConfiguration) (*Backend, error, error)
	listenerRuleConfigLoader func(ctx context.Context, k8sClient client.Client, routeIdentifier types.NamespacedName, routeKind RouteKind, listenerRuleConfigRefs []gwv1.LocalObjectReference) (*elbv2gw.ListenerRuleConfiguration, error, error)
}

func newAttachedRuleAccumulator[RuleType any](backendLoader func(ctx context.Context, k8sClient client.Client, backendRef gwv1.BackendRef, routeIdentifier types.NamespacedName, routeKind RouteKind, gatewayDefaultTGConfig *elbv2gw.TargetGroupConfiguration) (*Backend, error, error),
	listenerRuleConfigLoader func(ctx context.Context, k8sClient client.Client, routeIdentifier types.NamespacedName, routeKind RouteKind, listenerRuleConfigRefs []gwv1.LocalObjectReference) (*elbv2gw.ListenerRuleConfiguration, error, error)) attachedRuleAccumulator[RuleType] {
	return &attachedRuleAccumulatorImpl[RuleType]{
		backendLoader:            backendLoader,
		listenerRuleConfigLoader: listenerRuleConfigLoader,
	}
}

func (ara *attachedRuleAccumulatorImpl[RuleType]) accumulateRules(ctx context.Context, k8sClient client.Client, route preLoadRouteDescriptor, rules []RuleType, backendRefIterator func(RuleType) []gwv1.BackendRef, listenerRuleConfigRefs func(RuleType) []gwv1.LocalObjectReference, ruleConverter func(*RuleType, []Backend, *elbv2gw.ListenerRuleConfiguration) RouteRule, gatewayDefaultTGConfig *elbv2gw.TargetGroupConfiguration) ([]RouteRule, []routeLoadError) {
	convertedRules := make([]RouteRule, 0)
	allErrors := make([]routeLoadError, 0)
	for _, rule := range rules {
		convertedBackends := make([]Backend, 0)
		listenerRuleConfig, lrcWarningErr, lrcfatalErr := ara.listenerRuleConfigLoader(ctx, k8sClient, route.GetRouteNamespacedName(), route.GetRouteKind(), listenerRuleConfigRefs(rule))
		if lrcWarningErr != nil {
			allErrors = append(allErrors, routeLoadError{
				Err: lrcWarningErr,
			})
		}
		// usually happens due to K8s Api outage
		if lrcfatalErr != nil {
			allErrors = append(allErrors, routeLoadError{
				Err:   lrcfatalErr,
				Fatal: true,
			})
			return nil, allErrors
		}
		// If ListenerRuleConfig is loaded properly without any warning errors, then only load backends, else it should be treated as no valid backend to send with fixed 503 response
		if lrcWarningErr == nil {
			for _, backend := range backendRefIterator(rule) {
				convertedBackend, warningErr, fatalErr := ara.backendLoader(ctx, k8sClient, backend, route.GetRouteNamespacedName(), route.GetRouteKind(), gatewayDefaultTGConfig)
				if warningErr != nil {
					allErrors = append(allErrors, routeLoadError{
						Err: warningErr,
					})
				}

				if fatalErr != nil {
					allErrors = append(allErrors, routeLoadError{
						Err:   fatalErr,
						Fatal: true,
					})
					return nil, allErrors
				}

				if convertedBackend != nil {
					convertedBackends = append(convertedBackends, *convertedBackend)
				}
			}
		}
		convertedRules = append(convertedRules, ruleConverter(&rule, convertedBackends, listenerRuleConfig))
	}
	return convertedRules, allErrors
}

// returns (loaded backend, warning error, fatal error)
// warning error -> continue with reconcile cycle.
// fatal error -> stop reconcile cycle (probably k8s api outage)
// commonBackendLoader this function will load the services and target group configurations associated with this gateway backend.
func commonBackendLoader(ctx context.Context, k8sClient client.Client, backendRef gwv1.BackendRef, routeIdentifier types.NamespacedName, routeKind RouteKind, gatewayDefaultTGConfig *elbv2gw.TargetGroupConfiguration) (*Backend, error, error) {

	var serviceBackend *ServiceBackendConfig
	var literalTargetGroup *LiteralTargetGroupConfig
	var gatewayBackend *GatewayBackendConfig
	var warn error
	var fatal error
	// We only support references of type service.
	if backendRef.Kind == nil || *backendRef.Kind == serviceKind {
		serviceBackend, warn, fatal = serviceLoader(ctx, k8sClient, routeIdentifier, routeKind, backendRef, gatewayDefaultTGConfig)
	} else if string(*backendRef.Kind) == targetGroupNameBackend {
		literalTargetGroup, warn, fatal = literalTargetGroupLoader(backendRef)
	} else if string(*backendRef.Kind) == gatewayKind {
		gatewayBackend, warn, fatal = gatewayLoader(ctx, k8sClient, routeIdentifier, routeKind, backendRef)
	}

	if warn != nil || fatal != nil {
		return nil, warn, fatal
	}

	if serviceBackend == nil && literalTargetGroup == nil && gatewayBackend == nil {
		initialErrorMessage := "Unknown backend reference kind"
		wrappedGatewayErrorMessage := generateInvalidMessageWithRouteDetails(initialErrorMessage, routeKind, routeIdentifier)
		return nil, wrapError(errors.Errorf("%s", initialErrorMessage), gwv1.GatewayReasonListenersNotValid, gwv1.RouteReasonInvalidKind, &wrappedGatewayErrorMessage, nil), nil
	}

	// Weight specifies the proportion of requests forwarded to the referenced
	// backend. This is computed as weight/(sum of all weights in this
	// BackendRefs list). For non-zero values, there may be some epsilon from
	// the exact proportion defined here depending on the precision an
	// implementation supports. Weight is not a percentage and the sum of
	// weights does not need to equal 100.
	//
	// If only one backend is specified, and it has a weight greater than 0, 100%
	// of the traffic is forwarded to that backend. If weight is set to 0, no
	// traffic should be forwarded for this entry. If unspecified, weight
	// defaults to 1.
	weight := 1
	if backendRef.Weight != nil {
		weight = int(*backendRef.Weight)
	}

	if weight > maxWeight {
		return nil, nil, errors.Errorf("Weight [%d] must be less than or equal to %d", weight, maxWeight)
	}
	return &Backend{
		ServiceBackend:     serviceBackend,
		GatewayBackend:     gatewayBackend,
		LiteralTargetGroup: literalTargetGroup,
		Weight:             weight,
	}, nil, nil
}

func literalTargetGroupLoader(backendRef gwv1.BackendRef) (*LiteralTargetGroupConfig, error, error) {
	return &LiteralTargetGroupConfig{
		Name: string(backendRef.Name),
	}, nil, nil
}

// LookUpTargetGroupConfiguration given a service, lookup the target group configuration associated with the service.
// recall that target group configuration always lives within the same namespace as the service.
func LookUpTargetGroupConfiguration(ctx context.Context, k8sClient client.Client, objectKind string, objectMetadata types.NamespacedName) (*elbv2gw.TargetGroupConfiguration, error) {
	tgConfigList := &elbv2gw.TargetGroupConfigurationList{}

	// TODO - Add index
	if err := k8sClient.List(ctx, tgConfigList, client.InNamespace(objectMetadata.Namespace)); err != nil {
		return nil, err
	}

	for _, tgConfig := range tgConfigList.Items {

		var isEligible bool
		// Special case, nil kind == Service.
		if tgConfig.Spec.TargetReference.Kind == nil && objectKind == serviceKind {
			isEligible = true
		} else if tgConfig.Spec.TargetReference.Kind != nil && objectKind == *tgConfig.Spec.TargetReference.Kind {
			isEligible = true
		}

		if !isEligible {
			continue
		}

		// TODO - Add an index for this
		if tgConfig.Spec.TargetReference.Name == objectMetadata.Name {
			return &tgConfig, nil
		}
	}
	return nil, nil
}

func listenerRuleConfigLoader(ctx context.Context, k8sClient client.Client, routeIdentifier types.NamespacedName, routeKind RouteKind, listenerRuleConfigsRefs []gwv1.LocalObjectReference) (*elbv2gw.ListenerRuleConfiguration, error, error) {
	if len(listenerRuleConfigsRefs) == 0 {
		return nil, nil, nil
	}
	// This is warning error so that the reconcile cycle does not stop.
	if len(listenerRuleConfigsRefs) > 1 {
		initialErrorMessage := "Only one listener rule config can be referenced per route rule, found multiple"
		wrappedGatewayErrorMessage := generateInvalidMessageWithRouteDetails(initialErrorMessage, routeKind, routeIdentifier)
		return nil, wrapError(errors.Errorf("%s", initialErrorMessage), gwv1.GatewayReasonListenersNotValid, gwv1.RouteReasonIncompatibleFilters, &wrappedGatewayErrorMessage, nil), nil
	}
	listenerRuleCfgId := types.NamespacedName{
		Namespace: routeIdentifier.Namespace,
		Name:      string(listenerRuleConfigsRefs[0].Name),
	}
	listenerRuleCfg := &elbv2gw.ListenerRuleConfiguration{}
	err := k8sClient.Get(ctx, listenerRuleCfgId, listenerRuleCfg)
	if err != nil {
		convertToNotFoundError := client.IgnoreNotFound(err)

		if convertToNotFoundError == nil {
			// ListenerRuleConfig not found, post an updated status.
			initialErrorMessage := fmt.Sprintf("ListenerRuleConfiguration [%v] not found)", listenerRuleCfgId.String())
			wrappedGatewayErrorMessage := generateInvalidMessageWithRouteDetails(initialErrorMessage, routeKind, routeIdentifier)
			return nil, wrapError(errors.Errorf("%s", initialErrorMessage), gwv1.GatewayReasonListenersNotValid, gwv1.RouteReasonIncompatibleFilters, &wrappedGatewayErrorMessage, nil), nil
		}

		return nil, nil, errors.Wrapf(err, "Unable to load listener rule config [%v] for route [%v]", listenerRuleCfgId.String(), routeIdentifier.String())
	}
	// Check if LRC is accepted
	if listenerRuleCfg.Status.Accepted == nil || !*listenerRuleCfg.Status.Accepted {
		message := "status unknown"
		if listenerRuleCfg.Status.Message != nil {
			message = *listenerRuleCfg.Status.Message
		}
		initialErrorMessage := fmt.Sprintf("ListenerRuleConfiguration [%v] is not accepted. Reason:  %s)", listenerRuleCfgId.String(), message)
		wrappedGatewayErrorMessage := generateInvalidMessageWithRouteDetails(initialErrorMessage, routeKind, routeIdentifier)
		return nil, wrapError(errors.Errorf("%s", initialErrorMessage), gwv1.GatewayReasonListenersNotValid, gwv1.RouteReasonIncompatibleFilters, &wrappedGatewayErrorMessage, nil), nil
	}
	return listenerRuleCfg, nil, nil
}

// getListenerRuleConfigForRuleGeneric is a generic helper that extracts ListenerRuleConfiguration
// references from ExtensionRef filters in route rules
func getListenerRuleConfigForRuleGeneric[FilterType any](
	filters []FilterType,
	isExtensionRefType func(filter FilterType) bool,
	getExtensionRef func(filter FilterType) *gwv1.LocalObjectReference,
) []gwv1.LocalObjectReference {
	listenerRuleConfigsRefs := make([]gwv1.LocalObjectReference, 0)
	for _, filter := range filters {
		if !isExtensionRefType(filter) {
			continue
		}
		extRef := getExtensionRef(filter)
		if extRef != nil &&
			string(extRef.Group) == constants.ControllerCRDGroupVersion &&
			string(extRef.Kind) == constants.ListenerRuleConfiguration {
			listenerRuleConfigsRefs = append(listenerRuleConfigsRefs, gwv1.LocalObjectReference{
				Group: constants.ControllerCRDGroupVersion,
				Kind:  constants.ListenerRuleConfiguration,
				Name:  extRef.Name,
			})
		}
	}
	return listenerRuleConfigsRefs
}
