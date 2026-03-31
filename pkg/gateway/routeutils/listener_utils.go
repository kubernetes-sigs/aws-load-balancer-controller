package routeutils

import (
	"fmt"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ListenerSetStatusData struct {
	ListenerSetStatusInfo ListenerSetStatusInfo
	ListenerSetMetadata   ListenerSetMetadata
	RetryCount            uint
}

type ListenerSetStatusInfo struct {
	Accepted          bool
	AcceptedReason    string
	AcceptedMessage   string
	Programmed        bool
	ProgrammedReason  string
	ProgrammedMessage string
}

type ListenerSetListenerInfo struct {
	Version  time.Time
	Statuses []gwv1.ListenerEntryStatus
}

type ListenerSetMetadata struct {
	ListenerSetName      string
	ListenerSetNamespace string
	Generation           int64
}

type listenerSetListenerSource struct {
	parentRef gwv1.ListenerSet
	listener  gwv1.Listener
}

type allListeners struct {
	GatewayListeners     []gwv1.Listener
	ListenerSetListeners listenerSetLoadResult
}

type routeParentRefTuple struct {
	route     preLoadRouteDescriptor
	parentRef gwv1.ParentReference
}

type ValidatedGatewayListeners struct {
	GatewayListenerValidation     ListenerValidationResults
	ListenerSetListenerValidation map[types.NamespacedName]ListenerValidationResults
}

func (v ValidatedGatewayListeners) HasErrors() bool {
	if v.GatewayListenerValidation.HasErrors {
		return true
	}
	for _, lsValidation := range v.ListenerSetListenerValidation {
		if lsValidation.HasErrors {
			return true
		}
	}
	return false
}

type ListenerValidationResult struct {
	ListenerName        gwv1.SectionName
	IsValid             bool
	Reason              gwv1.ListenerConditionReason
	Message             string
	SupportedKinds      []gwv1.RouteGroupKind
	AttachedRoutesCount int32
}

type ListenerValidationResults struct {
	Results    map[gwv1.SectionName]ListenerValidationResult
	Generation int64
	HasErrors  bool
}

// validateListeners validates all listeners configurations in a Gateway against controller-specific requirements.
// it is different from listener <-> route validation
// It checks for supported route kinds, valid port ranges (1-65535), controller-compatible protocols
// (ALB: HTTP/HTTPS/GRPC, NLB: TCP/UDP/TLS), protocol conflicts on same ports (except TCP+UDP),
// hostname conflicts - same port trying to use same hostname
func validateListeners(configuredListeners allListeners, gatewayGeneration int64, controllerName string) ValidatedGatewayListeners {
	portHostnameMap := make(map[string]bool)
	portProtocolMap := make(map[gwv1.PortNumber]gwv1.ProtocolType)

	// Track portHostnameMap and portProtocolMap throughout the validation cycle. This allows us to give priority
	// to listeners. For example, we need to allow listeners defined in the Gateway directly to be given configuration
	// priority over listeners defined in a listener set. By validating listeners from the Gateway first, we allow those
	// listeners to pass validation, any listener set listeners that will conflict will fail validation but not
	// block the listener attachment process.

	gatewayValidationResults := validateListenerList(configuredListeners.GatewayListeners, portHostnameMap, portProtocolMap, controllerName, gatewayGeneration)

	listenerSetPriorityOrder := arrangeListenerSetsForValidation(configuredListeners.ListenerSetListeners)

	// The sorting is important, as we are building the combined listener representation in
	// portProtocolMap and portHostnameMap.
	// We give precedence to listeners seen previously,
	// meaning that we will accept the initial listener, and then fail the subsequent listener that conflicts.

	listenerSetValidationResults := map[types.NamespacedName]ListenerValidationResults{}

	for _, ls := range listenerSetPriorityOrder {
		listenerSetListeners := configuredListeners.ListenerSetListeners.listenersPerListenerSet[k8s.NamespacedName(ls)]
		listenerSetValidationResults[k8s.NamespacedName(ls)] = validateListenerList(extractListenerFromListenerSource(listenerSetListeners), portHostnameMap, portProtocolMap, controllerName, ls.Generation)
	}

	return ValidatedGatewayListeners{
		GatewayListenerValidation:     gatewayValidationResults,
		ListenerSetListenerValidation: listenerSetValidationResults,
	}
}

func validateListenerList(listenerList []gwv1.Listener, portHostnameMap map[string]bool, portProtocolMap map[gwv1.PortNumber]gwv1.ProtocolType, controllerName string, generation int64) ListenerValidationResults {
	results := ListenerValidationResults{
		Results:    make(map[gwv1.SectionName]ListenerValidationResult),
		Generation: generation,
	}

	for _, listener := range listenerList {
		// check supported kinds
		supportedKinds, isKindSupported := getSupportedKinds(controllerName, listener)
		result := ListenerValidationResult{
			ListenerName:   listener.Name,
			IsValid:        true,
			Reason:         gwv1.ListenerReasonAccepted,
			Message:        gateway_constants.ListenerAcceptedMessage,
			SupportedKinds: supportedKinds,
		}

		if !isKindSupported {
			result.IsValid = false
			result.Reason = gwv1.ListenerReasonInvalidRouteKinds
			result.Message = fmt.Sprintf("Invalid route kind for listener %s", listener.Name)
			results.HasErrors = true
		} else if listener.Port < 1 || listener.Port > 65535 {
			result.IsValid = false
			result.Reason = gwv1.ListenerReasonPortUnavailable
			result.Message = fmt.Sprintf("Port %d is not available (listener name %s)", listener.Port, listener.Name)
			results.HasErrors = true
		} else if controllerName == gateway_constants.ALBGatewayController &&
			(listener.Protocol == gwv1.TCPProtocolType || listener.Protocol == gwv1.UDPProtocolType || listener.Protocol == gwv1.TLSProtocolType) {
			result.IsValid = false
			result.Reason = gwv1.ListenerReasonUnsupportedProtocol
			result.Message = fmt.Sprintf("Unsupported protocol %s for listener %s", listener.Protocol, listener.Name)
			results.HasErrors = true
		} else if controllerName == gateway_constants.NLBGatewayController &&
			(listener.Protocol == gwv1.HTTPProtocolType || listener.Protocol == gwv1.HTTPSProtocolType) {
			result.IsValid = false
			result.Reason = gwv1.ListenerReasonUnsupportedProtocol
			result.Message = fmt.Sprintf("Unsupported protocol %s for listener %s", listener.Protocol, listener.Name)
			results.HasErrors = true
		} else {
			// Check protocol conflicts - same port with different protocols (except TCP+UDP)
			if existingProtocol, exists := portProtocolMap[listener.Port]; exists {
				if existingProtocol != listener.Protocol {
					if !((existingProtocol == gwv1.TCPProtocolType && listener.Protocol == gwv1.UDPProtocolType) ||
						(existingProtocol == gwv1.UDPProtocolType && listener.Protocol == gwv1.TCPProtocolType)) {
						result.IsValid = false
						result.Reason = gwv1.ListenerReasonProtocolConflict
						result.Message = fmt.Sprintf("Protocol conflict for port %d", listener.Port)
						results.HasErrors = true
					}
				}
			} else {
				portProtocolMap[listener.Port] = listener.Protocol
			}

			// Check hostname conflicts - only when hostname is specified
			if listener.Hostname != nil {
				hostname := *listener.Hostname
				key := fmt.Sprintf("%d-%s", listener.Port, hostname)

				if portHostnameMap[key] {
					result.IsValid = false
					result.Reason = gwv1.ListenerReasonHostnameConflict
					result.Message = fmt.Sprintf("Hostname conflict for port %d with hostname %s", listener.Port, hostname)
					results.HasErrors = true
				} else {
					portHostnameMap[key] = true
				}
			}
		}
		results.Results[listener.Name] = result
	}
	return results
}

func getSupportedKinds(controllerName string, listener gwv1.Listener) ([]gwv1.RouteGroupKind, bool) {
	supportedKinds := []gwv1.RouteGroupKind{}
	groupName := gateway_constants.GatewayResourceGroupName
	isKindSupported := true
	// we are allowing empty AllowedRoutes.Kinds
	if listener.AllowedRoutes == nil || listener.AllowedRoutes.Kinds == nil || len(listener.AllowedRoutes.Kinds) == 0 {
		allowedRoutes := sets.New[RouteKind](DefaultProtocolToRouteKindMap[listener.Protocol]...)
		for _, routeKind := range allowedRoutes.UnsortedList() {
			supportedKinds = append(supportedKinds, gwv1.RouteGroupKind{
				Group: (*gwv1.Group)(&groupName),
				Kind:  gwv1.Kind(routeKind),
			})
		}
	}
	for _, routeGroup := range listener.AllowedRoutes.Kinds {
		if controllerName == gateway_constants.ALBGatewayController {
			if string(routeGroup.Kind) == string(HTTPRouteKind) || string(routeGroup.Kind) == string(GRPCRouteKind) {
				supportedKinds = append(supportedKinds, gwv1.RouteGroupKind{
					Group: (*gwv1.Group)(&groupName),
					Kind:  routeGroup.Kind,
				})
			} else {
				isKindSupported = false
			}
		}
		if controllerName == gateway_constants.NLBGatewayController {
			if string(routeGroup.Kind) == string(TCPRouteKind) || string(routeGroup.Kind) == string(TLSRouteKind) || string(routeGroup.Kind) == string(UDPRouteKind) {
				supportedKinds = append(supportedKinds, gwv1.RouteGroupKind{
					Group: (*gwv1.Group)(&groupName),
					Kind:  routeGroup.Kind,
				})
			} else {
				isKindSupported = false
			}
		}
	}

	return supportedKinds, isKindSupported
}

func arrangeListenerSetsForValidation(lsResult listenerSetLoadResult) []*gwv1.ListenerSet {
	/*
		Listeners in a Gateway and their attached ListenerSets are concatenated as a list when programming the underlying infrastructure

		Listeners should be merged using the following precedence:

		    "parent" Gateway
		    ListenerSet ordered by creation time (oldest first)
		    ListenerSet ordered alphabetically by “{namespace}/{name}”.
		https://gateway-api.sigs.k8s.io/geps/gep-1713/#listener-precedence
	*/

	orderedListenerSets := make([]*gwv1.ListenerSet, 0)
	for _, listenerSet := range lsResult.acceptedListenerSets {
		orderedListenerSets = append(orderedListenerSets, &listenerSet)
	}

	// First sort by namespaced name (conveniently the namespaced name generates the string representation in the form
	// “{namespace}/{name}”.
	sort.Slice(orderedListenerSets, func(i, j int) bool {
		insn := k8s.NamespacedName(orderedListenerSets[i])
		jnsn := k8s.NamespacedName(orderedListenerSets[j])
		return insn.String() < jnsn.String()
	})

	// Next, we we sort by the creation time using a stable sort. This means that the list is ordered
	// where oldest listenersets come first and the stable sort breaks the tie for any resources with the same created
	// time.
	sort.SliceStable(orderedListenerSets, func(i, j int) bool {
		return orderedListenerSets[i].CreationTimestamp.Unix() < orderedListenerSets[j].CreationTimestamp.Unix()
	})

	return orderedListenerSets
}

func extractListenerFromListenerSource(listenerSources []listenerSetListenerSource) []gwv1.Listener {
	result := make([]gwv1.Listener, 0)
	for _, src := range listenerSources {
		result = append(result, src.listener)
	}
	return result
}

func CalculateAttachedListenerSets(listenerSetValidations map[types.NamespacedName]ListenerValidationResults) int32 {
	result := int32(0)

	for _, perListenerSetValidation := range listenerSetValidations {
		for _, validation := range perListenerSetValidation.Results {
			if validation.IsValid {
				result = result + 1
				break
			}
		}
	}

	return result
}
