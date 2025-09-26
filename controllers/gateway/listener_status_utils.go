package gateway

import (
	"fmt"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func buildListenerStatus(controllerName string, gateway gwv1.Gateway, attachedRoutesMap map[gwv1.SectionName]int32, validateListenerResults *routeutils.ListenerValidationResults) []gwv1.ListenerStatus {
	var listenerStatuses []gwv1.ListenerStatus

	// if validateListenerResults is nil, getListenerConditions will build condition with accepted condition
	for _, listener := range gateway.Spec.Listeners {
		supportedKinds, _ := routeutils.GetSupportedKinds(controllerName, listener)
		var condition []metav1.Condition
		if validateListenerResults == nil {
			condition = getListenerConditions(gateway, nil)
		} else {
			listenerValidationResult := validateListenerResults.Results[listener.Name]
			condition = getListenerConditions(gateway, &listenerValidationResult)
		}

		listenerStatus := gwv1.ListenerStatus{
			Name:           listener.Name,
			SupportedKinds: supportedKinds,
			AttachedRoutes: attachedRoutesMap[listener.Name],
			Conditions:     condition,
		}
		listenerStatuses = append(listenerStatuses, listenerStatus)
	}
	return listenerStatuses
}

func getListenerConditions(gw gwv1.Gateway, listenerValidationResult *routeutils.ListenerValidationResult) []metav1.Condition {
	var conditions []metav1.Condition

	// Determine condition type based on reason
	if listenerValidationResult == nil {
		return append(conditions, buildAcceptedCondition(gw, gwv1.ListenerReasonAccepted, gateway_constants.ListenerAcceptedMessage))
	}
	listenerReason := listenerValidationResult.Reason
	listenerErrMessage := listenerValidationResult.Message
	switch listenerReason {
	case gwv1.ListenerReasonHostnameConflict, gwv1.ListenerReasonProtocolConflict:
		conditions = append(conditions, buildConflictedCondition(gw, listenerReason, listenerErrMessage))
	case gwv1.ListenerReasonPortUnavailable, gwv1.ListenerReasonUnsupportedProtocol:
		conditions = append(conditions, buildAcceptedCondition(gw, listenerReason, listenerErrMessage))
	case gwv1.ListenerReasonInvalidRouteKinds, gwv1.ListenerReasonRefNotPermitted:
		conditions = append(conditions, buildResolvedRefsCondition(gw, listenerReason, listenerErrMessage))
	default:
		conditions = append(conditions, buildAcceptedCondition(gw, gwv1.ListenerReasonAccepted, gateway_constants.ListenerAcceptedMessage))
	}

	return conditions
}

func buildAcceptedCondition(gw gwv1.Gateway, reason gwv1.ListenerConditionReason, message string) metav1.Condition {
	status := metav1.ConditionTrue
	if reason != gwv1.ListenerReasonAccepted {
		status = metav1.ConditionFalse
	}

	return metav1.Condition{
		Type:               string(gwv1.ListenerConditionAccepted),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
		ObservedGeneration: gw.GetGeneration(),
	}
}

func buildConflictedCondition(gw gwv1.Gateway, reason gwv1.ListenerConditionReason, message string) metav1.Condition {
	status := metav1.ConditionTrue
	if reason != gwv1.ListenerReasonAccepted {
		status = metav1.ConditionFalse
	}
	return metav1.Condition{
		Type:               string(gwv1.ListenerConditionConflicted),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
		ObservedGeneration: gw.GetGeneration(),
	}
}

func buildResolvedRefsCondition(gw gwv1.Gateway, reason gwv1.ListenerConditionReason, message string) metav1.Condition {
	status := metav1.ConditionTrue
	if reason != gwv1.ListenerReasonAccepted {
		status = metav1.ConditionFalse
	}
	return metav1.Condition{
		Type:               string(gwv1.ListenerConditionResolvedRefs),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
		ObservedGeneration: gw.GetGeneration(),
	}
}

func isListenerStatusIdentical(listenerStatus []gwv1.ListenerStatus, listenerStatusOld []gwv1.ListenerStatus) bool {
	if len(listenerStatus) != len(listenerStatusOld) {
		return false
	}
	// Sort both slices by Name before comparison
	sort.Slice(listenerStatus, func(i, j int) bool {
		return listenerStatus[i].Name < listenerStatus[j].Name
	})
	sort.Slice(listenerStatusOld, func(i, j int) bool {
		return listenerStatusOld[i].Name < listenerStatusOld[j].Name
	})
	for i := range listenerStatus {
		if listenerStatus[i].Name != listenerStatusOld[i].Name {
			return false
		}

		if !compareSupportedKinds(listenerStatus[i].SupportedKinds, listenerStatusOld[i].SupportedKinds) {
			return false
		}

		if listenerStatus[i].AttachedRoutes != listenerStatusOld[i].AttachedRoutes {
			return false
		}
		if len(listenerStatus[i].Conditions) != len(listenerStatusOld[i].Conditions) {
			return false
		}
		for j := range listenerStatus[i].Conditions {
			if listenerStatus[i].Conditions[j].Type != listenerStatusOld[i].Conditions[j].Type {
				return false
			}
			if listenerStatus[i].Conditions[j].Status != listenerStatusOld[i].Conditions[j].Status {
				return false
			}
			if listenerStatus[i].Conditions[j].Reason != listenerStatusOld[i].Conditions[j].Reason {
				return false
			}
			if listenerStatus[i].Conditions[j].Message != listenerStatusOld[i].Conditions[j].Message {
				return false
			}
			if listenerStatus[i].Conditions[j].ObservedGeneration != listenerStatusOld[i].Conditions[j].ObservedGeneration {
				return false
			}
		}
	}
	return true
}

func compareSupportedKinds(kinds1, kinds2 []gwv1.RouteGroupKind) bool {
	if len(kinds1) != len(kinds2) {
		return false
	}

	kindMap := make(map[string]int)
	for _, kind := range kinds1 {
		key := fmt.Sprintf("%s/%s", *kind.Group, kind.Kind)
		kindMap[key]++
	}

	for _, kind := range kinds2 {
		key := fmt.Sprintf("%s/%s", *kind.Group, kind.Kind)
		if kindMap[key] == 0 {
			return false
		}
		kindMap[key]--
	}

	return true
}
