package routeutils

import (
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// This file contains utils used for gateway api route reconciler

// RouteData
// RouteStatusInfo: contains status condition info
// RouteMetadata: contains route metadata: name, namespace, kind and generation
// ParentRefGateway: contains gateway information, each routeStatusInfo should have a correlated parentRefGateway
type RouteData struct {
	RouteStatusInfo  RouteStatusInfo
	RouteMetadata    RouteMetadata
	ParentRefGateway ParentRefGateway
}

type RouteStatusInfo struct {
	Accepted     bool
	ResolvedRefs bool
	Reason       string
	Message      string
}

type RouteMetadata struct {
	RouteName       string
	RouteNamespace  string
	RouteKind       string
	RouteGeneration int64
}

type ParentRefGateway struct {
	Name      string
	Namespace string
}

type RouteReconciler interface {
	Enqueue(routeData RouteData)
	Run()
}

// constants

const (
	RouteStatusInfoAcceptedMessage                   = "Route is accepted by Gateway"
	RouteStatusInfoRejectedMessageNoMatchingHostname = "Listener does not allow route attachment, no matching hostname"
	RouteStatusInfoRejectedMessageNamespaceNotMatch  = "Listener does not allow route attachment, namespace does not match between listener and route"
	RouteStatusInfoRejectedMessageKindNotMatch       = "Listener does not allow route attachment, kind does not match between listener and route"
	RouteStatusInfoRejectedParentRefNotExist         = "ParentRefDoesNotExist"
)

func GenerateRouteData(accepted bool, resolvedRefs bool, reason string, message string, routeNamespaceName types.NamespacedName, routeKind RouteKind, routeGeneration int64, gw gwv1.Gateway) RouteData {
	return RouteData{
		RouteStatusInfo: RouteStatusInfo{
			Accepted:     accepted,
			ResolvedRefs: resolvedRefs,
			Reason:       reason,
			Message:      message,
		},
		RouteMetadata: RouteMetadata{
			RouteName:       routeNamespaceName.Name,
			RouteNamespace:  routeNamespaceName.Namespace,
			RouteKind:       string(routeKind),
			RouteGeneration: routeGeneration,
		},
		ParentRefGateway: ParentRefGateway{
			Name:      gw.Name,
			Namespace: gw.Namespace,
		},
	}
}
