package gateway

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/gateway/routeutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// This route reconciler is currently used for route status update

type routeReconcilerImpl struct {
	queue     workqueue.DelayingInterface
	k8sClient client.Client
	logger    logr.Logger
}

// NewRouteReconciler
// This is responsible for handling status update for Route resources
func NewRouteReconciler(queue workqueue.DelayingInterface, k8sClient client.Client, logger logr.Logger) routeutils.RouteReconciler {
	return &routeReconcilerImpl{
		logger:    logger,
		queue:     queue,
		k8sClient: k8sClient,
	}
}

func (d *routeReconcilerImpl) Enqueue(routeData routeutils.RouteData) {
	d.enqueue(routeData)
	d.logger.V(1).Info("enqueued new Route for status update: ", "route", routeData.RouteMetadata.RouteName)
}

func (d *routeReconcilerImpl) Run() {
	var item interface{}
	shutDown := false
	for !shutDown {
		item, shutDown = d.queue.Get()
		if item != nil {
			routeData := item.(routeutils.RouteData)
			d.handleItem(routeData)
			d.queue.Done(routeData)
		}
	}

	d.logger.V(1).Info("Shutting down route queue")
}

func (d *routeReconcilerImpl) enqueue(routeData routeutils.RouteData) {
	d.queue.Add(routeData)
}

func (d *routeReconcilerImpl) handleItem(routeData routeutils.RouteData) {
	err := d.handleRouteStatusUpdate(routeData)
	if err != nil {
		d.handleItemError(routeData, err, "Failed to update route status")
		return
	}
}

func (d *routeReconcilerImpl) handleItemError(routeData routeutils.RouteData, err error, msg string) {
	err = client.IgnoreNotFound(err)
	if err != nil {
		d.logger.Error(err, msg)
		d.enqueue(routeData)
	}
}

func (d *routeReconcilerImpl) handleRouteStatusUpdate(routeData routeutils.RouteData) error {
	d.logger.V(1).Info("Handling route status update", "route", routeData.RouteMetadata.RouteName)
	routeName := routeData.RouteMetadata.RouteName
	routeNamespace := routeData.RouteMetadata.RouteNamespace
	routeKind := routeData.RouteMetadata.RouteKind
	// double check if route exist and get route
	var route client.Object
	switch routeKind {
	case "HTTPRoute":
		route = &gwv1.HTTPRoute{}
	case "GRPCRoute":
		route = &gwv1.GRPCRoute{}
	case "UDPRoute":
		route = &gwv1.UDPRoute{}
	case "TCPRoute":
		route = &gwv1.TCPRoute{}
	case "TLSRoute":
		route = &gwv1.TLSRoute{}
	}

	if err := d.k8sClient.Get(context.Background(), types.NamespacedName{Namespace: routeNamespace, Name: routeName}, route); err != nil {
		return client.IgnoreNotFound(err)
	}
	routeOld := route.DeepCopyObject().(client.Object)

	// update route with current status
	if err := d.updateRouteStatus(route, routeData); err != nil {
		return err
	}

	// compare it with original status, patch if different
	if !d.isRouteStatusIdentical(routeOld, route) {
		if err := d.k8sClient.Status().Patch(context.Background(), route, client.MergeFromWithOptions(routeOld, client.MergeFromWithOptimisticLock{})); err != nil {
			d.logger.Error(err, "Failed to patch route status", "route", route)
			return err
		}
	}
	return nil
}

// update route status
func (d *routeReconcilerImpl) updateRouteStatus(route client.Object, routeData routeutils.RouteData) error {
	// initial route status if not exist already
	var ParentRefs []gwv1.ParentReference
	controllerName := constants.NLBGatewayController
	var originalRouteStatus []gwv1.RouteParentStatus

	switch r := route.(type) {
	case *gwv1.HTTPRoute:
		controllerName = constants.ALBGatewayController
		if r.Status.Parents == nil {
			r.Status.Parents = []gwv1.RouteParentStatus{}
		}
		originalRouteStatus = r.Status.Parents
		ParentRefs = r.Spec.ParentRefs
	case *gwv1.GRPCRoute:
		controllerName = constants.ALBGatewayController
		if r.Status.Parents == nil {
			r.Status.Parents = []gwv1.RouteParentStatus{}
		}
		originalRouteStatus = r.Status.Parents
		ParentRefs = r.Spec.ParentRefs
	case *gwv1.UDPRoute:
		if r.Status.Parents == nil {
			r.Status.Parents = []gwv1.RouteParentStatus{}
		}
		originalRouteStatus = r.Status.Parents
		ParentRefs = r.Spec.ParentRefs
	case *gwv1.TCPRoute:
		if r.Status.Parents == nil {
			r.Status.Parents = []gwv1.RouteParentStatus{}
		}
		originalRouteStatus = r.Status.Parents
		ParentRefs = r.Spec.ParentRefs
	case *gwv1.TLSRoute:
		if r.Status.Parents == nil {
			r.Status.Parents = []gwv1.RouteParentStatus{}
		}
		originalRouteStatus = r.Status.Parents
		ParentRefs = r.Spec.ParentRefs
	}
	routeNamespace := route.GetNamespace()

	// Entries written by other controllers are not ours to rebuild or drop: keep them as they are and
	// rebuild only our own entries from spec.parentRefs. Start from a non-nil slice so that dropping every
	// entry patches status.parents to [] instead of removing the required field.
	newRouteStatus := []gwv1.RouteParentStatus{}
	var ownRouteStatus []gwv1.RouteParentStatus
	for _, status := range originalRouteStatus {
		if string(status.ControllerName) == controllerName {
			ownRouteStatus = append(ownRouteStatus, status)
		} else {
			newRouteStatus = append(newRouteStatus, status)
		}
	}

	// set conditions
	originalRouteStatusMap := createOriginalRouteStatusMap(ownRouteStatus, routeNamespace)
	for _, parentRef := range ParentRefs {
		parentRefKey := routeutils.ParentRefKey(parentRef, routeNamespace)
		// Generate key for routeData's parentRef
		routeDataParentRefKey := getParentRefKeyFromRouteData(routeData.ParentRef, routeData.RouteMetadata.RouteNamespace)
		if parentRefKey != routeDataParentRefKey {
			// This parent ref is not the one we are looking for.
			status, ok := originalRouteStatusMap[parentRefKey]
			if ok {
				newRouteStatus = append(newRouteStatus, status)
			}
			continue
		}

		if routeData.RouteStatusInfo.Reason == routeutils.RouteStatusInfoParentRefRemovedReason {
			// A cleanup trigger for a parent the route references again (detached, then reattached before
			// the cleanup ran). Leave its entry for the regular status update instead of marking it removed.
			if status, ok := originalRouteStatusMap[parentRefKey]; ok {
				newRouteStatus = append(newRouteStatus, status)
			}
			continue
		}

		newRouteParentStatus := gwv1.RouteParentStatus{
			ParentRef:      parentRef,
			ControllerName: gwv1.GatewayController(controllerName),
			Conditions:     []metav1.Condition{},
		}
		// if status related to parentRef exists, keep the condition first
		if status, exists := originalRouteStatusMap[routeutils.ParentRefKey(parentRef, routeNamespace)]; exists {
			newRouteParentStatus.Conditions = status.Conditions
		}

		// do not allow backward generation update, Accepted and ResolvedRef always have same generation based on our implementation
		if (len(newRouteParentStatus.Conditions) != 0 && newRouteParentStatus.Conditions[0].ObservedGeneration <= routeData.RouteMetadata.RouteGeneration) || len(newRouteParentStatus.Conditions) == 0 {
			// for a given parentRef, if it has a statusInfo, this means condition is updated, override route condition based on route status info
			d.setConditionsWithRouteStatusInfo(route, &newRouteParentStatus, routeData.RouteStatusInfo)
			// handle parentRefNotExist: resolve ref resource, if parentRef does not have namespace, getting it from Route
			if err := d.resolveParentResource(parentRef, route.GetNamespace()); err != nil {
				// set conditions if resolvedRef = false
				d.setConditionsBasedOnResolveRefGateway(route, &newRouteParentStatus, err)
			}
		}
		newRouteStatus = append(newRouteStatus, newRouteParentStatus)
	}

	switch r := route.(type) {
	case *gwv1.HTTPRoute:
		r.Status.Parents = newRouteStatus
	case *gwv1.GRPCRoute:
		r.Status.Parents = newRouteStatus
	case *gwv1.TLSRoute:
		r.Status.Parents = newRouteStatus
	case *gwv1.UDPRoute:
		r.Status.Parents = newRouteStatus
	case *gwv1.TCPRoute:
		r.Status.Parents = newRouteStatus
	}
	return nil
}

func (d *routeReconcilerImpl) resolveParentResource(parentRef gwv1.ParentReference, namespace string) error {

	targetParentRefKind := "Gateway"
	if parentRef.Kind != nil {
		targetParentRefKind = string(*parentRef.Kind)
	}

	if parentRef.Namespace != nil {
		namespace = string(*parentRef.Namespace)
	}

	var obj client.Object

	switch targetParentRefKind {
	case "Gateway":
		obj = &gwv1.Gateway{}
		break
	case "ListenerSet":
		obj = &gwv1.ListenerSet{}
		break
	default:
		return errors.New(fmt.Sprintf("Unknown parent reference kind %s", targetParentRefKind))

	}

	// check if gateway in ParentRef exists
	if err := d.k8sClient.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: string(parentRef.Name)}, obj); err != nil {
		return err
	}

	return nil
}

// setCondition based on RouteStatusInfo
func (d *routeReconcilerImpl) setConditionsWithRouteStatusInfo(route client.Object, parentStatus *gwv1.RouteParentStatus, info routeutils.RouteStatusInfo) {
	timeNow := metav1.NewTime(time.Now())
	var conditions []metav1.Condition
	if !info.ResolvedRefs {
		conditions = append(conditions, metav1.Condition{
			Type:               string(gwv1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionFalse,
			Reason:             info.Reason,
			Message:            info.Message,
			LastTransitionTime: timeNow,
			ObservedGeneration: route.GetGeneration(),
		})
	} else {
		conditions = append(conditions, metav1.Condition{
			Type:               string(gwv1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionTrue,
			Reason:             string(gwv1.RouteReasonResolvedRefs),
			Message:            "",
			LastTransitionTime: timeNow,
			ObservedGeneration: route.GetGeneration(),
		})
	}

	if !info.Accepted {
		conditions = append(conditions, metav1.Condition{
			Type:               string(gwv1.RouteConditionAccepted),
			Status:             metav1.ConditionFalse,
			Reason:             info.Reason,
			Message:            info.Message,
			LastTransitionTime: timeNow,
			ObservedGeneration: route.GetGeneration(),
		})
	} else {
		conditions = append(conditions, metav1.Condition{
			Type:               string(gwv1.RouteConditionAccepted),
			Status:             metav1.ConditionTrue,
			Reason:             string(gwv1.RouteReasonAccepted),
			Message:            "",
			LastTransitionTime: timeNow,
			ObservedGeneration: route.GetGeneration(),
		})
	}

	parentStatus.Conditions = conditions
}

func (d *routeReconcilerImpl) setConditionsBasedOnResolveRefGateway(route client.Object, parentStatus *gwv1.RouteParentStatus, resolveErr error) {
	timeNow := metav1.NewTime(time.Now())
	parentStatus.Conditions = []metav1.Condition{
		{
			Type:               string(gwv1.RouteConditionAccepted),
			Status:             metav1.ConditionFalse,
			Reason:             routeutils.RouteStatusInfoRejectedParentRefNotExist,
			Message:            resolveErr.Error(),
			LastTransitionTime: timeNow,
			ObservedGeneration: route.GetGeneration(),
		},
		{
			Type:               string(gwv1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionTrue,
			Reason:             string(gwv1.RouteConditionResolvedRefs),
			Message:            "",
			LastTransitionTime: timeNow,
			ObservedGeneration: route.GetGeneration(),
		},
	}
	return
}

// check if parent status has changed
func (d *routeReconcilerImpl) isRouteStatusIdentical(routeOld client.Object, route client.Object) bool {
	routeOldStatus := getRouteStatus(routeOld)

	routeNewStatus := getRouteStatus(route)

	// compare if both have same parent number
	if len(routeOldStatus) != len(routeNewStatus) {
		return false
	}

	// compare each parent status, check if they are identical semantically
	oldStatusMap := make(map[string]gwv1.RouteParentStatus)
	newStatusMap := make(map[string]gwv1.RouteParentStatus)

	// build maps, key is the controller name plus a string combined from parentRef fields: two controllers
	// can each hold an entry for the same parentRef
	for _, status := range routeOldStatus {
		key := string(status.ControllerName) + "|" + routeutils.ParentRefKey(status.ParentRef, routeOld.GetNamespace())
		oldStatusMap[key] = status
	}

	for _, status := range routeNewStatus {
		key := string(status.ControllerName) + "|" + routeutils.ParentRefKey(status.ParentRef, route.GetNamespace())
		newStatusMap[key] = status
	}

	// Compare each parent status
	for key, oldStatus := range oldStatusMap {
		newStatus, exists := newStatusMap[key]
		if !exists {
			return false
		}

		// Compare ControllerName
		if oldStatus.ControllerName != newStatus.ControllerName {
			return false
		}

		// Compare Conditions
		if !areConditionsEqual(oldStatus.Conditions, newStatus.Conditions) {
			return false
		}
	}

	return true
}

func getRouteStatus(route client.Object) []gwv1.RouteParentStatus {
	var routeStatus []gwv1.RouteParentStatus

	switch r := route.(type) {
	case *gwv1.HTTPRoute:
		routeStatus = r.Status.Parents
	case *gwv1.GRPCRoute:
		routeStatus = r.Status.Parents
	case *gwv1.TLSRoute:
		routeStatus = r.Status.Parents
	case *gwv1.UDPRoute:
		routeStatus = r.Status.Parents
	case *gwv1.TCPRoute:
		routeStatus = r.Status.Parents
	}

	return routeStatus
}

// Helper function to generate key from RouteData's ParentReference, use same format as routeutils.ParentRefKey
func getParentRefKeyFromRouteData(parentRef gwv1.ParentReference, routeNamespace string) string {
	return routeutils.ParentRefKey(parentRef, routeNamespace)
}

// Helper function to compare conditions
func areConditionsEqual(oldConditions, newConditions []metav1.Condition) bool {
	if len(oldConditions) != len(newConditions) {
		return false
	}

	oldConditionMap := make(map[string]metav1.Condition)
	for _, condition := range oldConditions {
		oldConditionMap[condition.Type] = condition
	}

	for _, newCondition := range newConditions {
		oldCondition, exists := oldConditionMap[newCondition.Type]
		if !exists {
			return false
		}

		// Compare condition fields, ignore LastTransitionTime
		if oldCondition.Type != newCondition.Type ||
			oldCondition.Status != newCondition.Status ||
			oldCondition.Reason != newCondition.Reason ||
			oldCondition.Message != newCondition.Message ||
			oldCondition.ObservedGeneration != newCondition.ObservedGeneration {
			return false
		}
	}

	return true
}

func createOriginalRouteStatusMap(originalRouteStatus []gwv1.RouteParentStatus, routeNamespace string) map[string]gwv1.RouteParentStatus {
	originalStatusMap := make(map[string]gwv1.RouteParentStatus)
	for i := range originalRouteStatus {
		key := routeutils.ParentRefKey(originalRouteStatus[i].ParentRef, routeNamespace)
		originalStatusMap[key] = originalRouteStatus[i]
	}
	return originalStatusMap
}
