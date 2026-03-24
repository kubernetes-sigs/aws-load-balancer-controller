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

func buildListenerStatus(generation int64, attachedRoutesMap map[gwv1.SectionName]int32, validatedListeners routeutils.ValidatedGatewayListeners, isProgrammed bool) []gwv1.ListenerStatus {
	var listenerStatuses []gwv1.ListenerStatus
	validateListenerResults := validatedListeners.GatewayListenerValidation

	for listenerName, listenerValidationResult := range validateListenerResults.Results {
		conditions := getListenerConditions(generation, listenerValidationResult, isProgrammed)

		listenerStatus := gwv1.ListenerStatus{
			Name:           listenerName,
			SupportedKinds: listenerValidationResult.SupportedKinds,
			AttachedRoutes: attachedRoutesMap[listenerName],
			Conditions:     conditions,
		}
		listenerStatuses = append(listenerStatuses, listenerStatus)
	}
	return listenerStatuses
}

func getListenerConditions(generation int64, listenerValidationResult routeutils.ListenerValidationResult, isProgrammed bool) []metav1.Condition {
	var conditions []metav1.Condition

	// Default
	listenerReason := listenerValidationResult.Reason
	listenerErrMessage := listenerValidationResult.Message

	// Build Conflict Conditions
	switch listenerReason {
	case gwv1.ListenerReasonHostnameConflict, gwv1.ListenerReasonProtocolConflict:
		conditions = append(conditions, buildConflictedCondition(generation, listenerReason, listenerErrMessage))
	default:
		conditions = append(conditions, buildConflictedCondition(generation, gwv1.ListenerReasonNoConflicts, gateway_constants.ListenerNoConflictMessage))
	}

	// Build Accepted Conditions
	switch listenerReason {
	case gwv1.ListenerReasonPortUnavailable, gwv1.ListenerReasonUnsupportedProtocol:
		conditions = append(conditions, buildAcceptedCondition(generation, listenerReason, listenerErrMessage))
	default:
		conditions = append(conditions, buildAcceptedCondition(generation, gwv1.ListenerReasonAccepted, gateway_constants.ListenerAcceptedMessage))
	}

	// Build ResolvedRefs Conditions
	switch listenerReason {
	case gwv1.ListenerReasonInvalidRouteKinds, gwv1.ListenerReasonRefNotPermitted:
		conditions = append(conditions, buildResolvedRefsCondition(generation, listenerReason, listenerErrMessage))
	default:
		conditions = append(conditions, buildResolvedRefsCondition(generation, gwv1.ListenerReasonResolvedRefs, gateway_constants.ListenerResolvedRefMessage))
	}

	// Build Programmed Conditions
	isAccepted := listenerReason == gwv1.ListenerReasonAccepted
	conditions = append(conditions, buildProgrammedCondition(generation, isProgrammed, isAccepted))

	return conditions
}

func buildProgrammedCondition(generation int64, isProgrammed bool, isAccepted bool) metav1.Condition {
	if !isAccepted {
		return metav1.Condition{
			Type:               string(gwv1.ListenerConditionProgrammed),
			Status:             metav1.ConditionFalse,
			Reason:             string(gwv1.ListenerReasonInvalid),
			Message:            gateway_constants.ListenerNotAcceptedMessage,
			LastTransitionTime: metav1.NewTime(time.Now()),
			ObservedGeneration: generation,
		}
	}

	if isProgrammed {
		return metav1.Condition{
			Type:               string(gwv1.ListenerConditionProgrammed),
			Status:             metav1.ConditionTrue,
			Reason:             string(gwv1.ListenerReasonProgrammed),
			Message:            gateway_constants.ListenerProgrammedMessage,
			LastTransitionTime: metav1.NewTime(time.Now()),
			ObservedGeneration: generation,
		}
	}

	return metav1.Condition{
		Type:               string(gwv1.ListenerConditionProgrammed),
		Status:             metav1.ConditionFalse,
		Reason:             string(gwv1.ListenerReasonPending),
		Message:            gateway_constants.ListenerPendingProgrammedMessage,
		LastTransitionTime: metav1.NewTime(time.Now()),
		ObservedGeneration: generation,
	}
}

func buildAcceptedCondition(generation int64, reason gwv1.ListenerConditionReason, message string) metav1.Condition {
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
		ObservedGeneration: generation,
	}
}

func buildConflictedCondition(generation int64, reason gwv1.ListenerConditionReason, message string) metav1.Condition {
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
		ObservedGeneration: generation,
	}
}

func buildResolvedRefsCondition(generation int64, reason gwv1.ListenerConditionReason, message string) metav1.Condition {
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
		ObservedGeneration: generation,
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
