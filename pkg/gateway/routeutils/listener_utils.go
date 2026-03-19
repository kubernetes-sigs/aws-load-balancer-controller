package routeutils

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ListenerValidationResult struct {
	ListenerName   gwv1.SectionName
	IsValid        bool
	Reason         gwv1.ListenerConditionReason
	Message        string
	SupportedKinds []gwv1.RouteGroupKind
}

type ListenerValidationResults struct {
	Results   map[gwv1.SectionName]ListenerValidationResult
	HasErrors bool
}

// validateListeners validates all listeners configurations in a Gateway against controller-specific requirements.
// it is different from listener <-> route validation
// It checks for supported route kinds, valid port ranges (1-65535), controller-compatible protocols
// (ALB: HTTP/HTTPS/GRPC, NLB: TCP/UDP/TLS), protocol conflicts on same ports (except TCP+UDP),
// hostname conflicts - same port trying to use same hostname
func validateListeners(listeners []gwv1.Listener, controllerName string) ListenerValidationResults {
	results := ListenerValidationResults{
		Results: make(map[gwv1.SectionName]ListenerValidationResult),
	}

	if len(listeners) == 0 {
		return results
	}

	portHostnameMap := make(map[string]bool)
	portProtocolMap := make(map[gwv1.PortNumber]gwv1.ProtocolType)

	for _, listener := range listeners {
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
