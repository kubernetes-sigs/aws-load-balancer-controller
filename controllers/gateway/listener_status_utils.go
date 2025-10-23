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

func buildListenerStatus(controllerName string, gateway gwv1.Gateway, attachedRoutesMap map[gwv1.SectionName]int32, validateListenerResults *routeutils.ListenerValidationResults, isProgrammed bool) []gwv1.ListenerStatus {
	var listenerStatuses []gwv1.ListenerStatus

	// if validateListenerResults is nil, getListenerConditions will build condition with accepted condition
	for _, listener := range gateway.Spec.Listeners {
		supportedKinds, _ := routeutils.GetSupportedKinds(controllerName, listener)
		var condition []metav1.Condition
		if validateListenerResults == nil {
			condition = getListenerConditions(gateway, nil, isProgrammed)
		} else {
			listenerValidationResult := validateListenerResults.Results[listener.Name]
			condition = getListenerConditions(gateway, &listenerValidationResult, isProgrammed)
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

func getListenerConditions(gw gwv1.Gateway, listenerValidationResult *routeutils.ListenerValidationResult, isProgrammed bool) []metav1.Condition {
	var conditions []metav1.Condition

	// Default
	listenerReason := gwv1.ListenerReasonAccepted
	listenerErrMessage := gateway_constants.ListenerAcceptedMessage

	if listenerValidationResult != nil {
		listenerReason = listenerValidationResult.Reason
		listenerErrMessage = listenerValidationResult.Message
	}

	// Build Conflict Conditions
	switch listenerReason {
	case gwv1.ListenerReasonHostnameConflict, gwv1.ListenerReasonProtocolConflict:
		conditions = append(conditions, buildConflictedCondition(gw, listenerReason, listenerErrMessage))
	default:
		conditions = append(conditions, buildConflictedCondition(gw, gwv1.ListenerReasonNoConflicts, gateway_constants.ListenerNoConflictMessage))
	}

	// Build Accepted Conditions
	switch listenerReason {
	case gwv1.ListenerReasonPortUnavailable, gwv1.ListenerReasonUnsupportedProtocol:
		conditions = append(conditions, buildAcceptedCondition(gw, listenerReason, listenerErrMessage))
	default:
		conditions = append(conditions, buildAcceptedCondition(gw, gwv1.ListenerReasonAccepted, gateway_constants.ListenerAcceptedMessage))
	}

	// Build ResolvedRefs Conditions
	switch listenerReason {
	case gwv1.ListenerReasonInvalidRouteKinds, gwv1.ListenerReasonRefNotPermitted:
		conditions = append(conditions, buildResolvedRefsCondition(gw, listenerReason, listenerErrMessage))
	default:
		conditions = append(conditions, buildResolvedRefsCondition(gw, gwv1.ListenerReasonResolvedRefs, gateway_constants.ListenerResolvedRefMessage))
	}

	// Build Programmed Conditions
	isAccepted := listenerReason == gwv1.ListenerReasonAccepted
	conditions = append(conditions, buildProgrammedCondition(gw, isProgrammed, isAccepted))

	return conditions
}

func buildProgrammedCondition(gw gwv1.Gateway, isProgrammed bool, isAccepted bool) metav1.Condition {
	if !isAccepted {
		return metav1.Condition{
			Type:               string(gwv1.ListenerConditionProgrammed),
			Status:             metav1.ConditionFalse,
			Reason:             string(gwv1.ListenerReasonInvalid),
			Message:            gateway_constants.ListenerNotAcceptedMessage,
			LastTransitionTime: metav1.NewTime(time.Now()),
			ObservedGeneration: gw.GetGeneration(),
		}
	}

	if isProgrammed {
		return metav1.Condition{
			Type:               string(gwv1.ListenerConditionProgrammed),
			Status:             metav1.ConditionTrue,
			Reason:             string(gwv1.ListenerReasonProgrammed),
			Message:            gateway_constants.ListenerProgrammedMessage,
			LastTransitionTime: metav1.NewTime(time.Now()),
			ObservedGeneration: gw.GetGeneration(),
		}
	}

	return metav1.Condition{
		Type:               string(gwv1.ListenerConditionProgrammed),
		Status:             metav1.ConditionFalse,
		Reason:             string(gwv1.ListenerReasonPending),
		Message:            gateway_constants.ListenerPendingProgrammedMessage,
		LastTransitionTime: metav1.NewTime(time.Now()),
		ObservedGeneration: gw.GetGeneration(),
	}
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
	if reason != gwv1.ListenerReasonNoConflicts {
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
	if reason != gwv1.ListenerReasonResolvedRefs {
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
		// Sort conditions by Type before comparison
		sort.Slice(listenerStatus[i].Conditions, func(j, k int) bool {
			return listenerStatus[i].Conditions[j].Type < listenerStatus[i].Conditions[k].Type
		})
		sort.Slice(listenerStatusOld[i].Conditions, func(j, k int) bool {
			return listenerStatusOld[i].Conditions[j].Type < listenerStatusOld[i].Conditions[k].Type
		})
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
