package handlers

import (
	"context"
	"reflect"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/class"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = (*EnqueueRequestsForEndpointsEvent)(nil)

type EnqueueRequestsForEndpointsEvent struct {
	IngressClass string
	Cache        cache.Cache
}

// Create is called in response to an create event - e.g. Pod Creation.
func (h *EnqueueRequestsForEndpointsEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueImpactedIngresses(e.Object.(*corev1.Endpoints), queue)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *EnqueueRequestsForEndpointsEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	epOld := e.ObjectOld.(*corev1.Endpoints)
	epNew := e.ObjectNew.(*corev1.Endpoints)
	if !reflect.DeepEqual(epOld.Subsets, epNew.Subsets) {
		h.enqueueImpactedIngresses(epNew, queue)
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *EnqueueRequestsForEndpointsEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	h.enqueueImpactedIngresses(e.Object.(*corev1.Endpoints), queue)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (h *EnqueueRequestsForEndpointsEvent) Generic(event.GenericEvent, workqueue.RateLimitingInterface) {
}

func (h *EnqueueRequestsForEndpointsEvent) enqueueImpactedIngresses(endpoints *corev1.Endpoints, queue workqueue.RateLimitingInterface) {
	ingressList := &extensions.IngressList{}
	if err := h.Cache.List(context.Background(), client.InNamespace(endpoints.Namespace), ingressList); err != nil {
		glog.Errorf("failed to fetch impacted ingresses by endpoints due to %v", err)
		return
	}

	for _, ingress := range ingressList.Items {
		if !class.IsValidIngress(h.IngressClass, &ingress) || !IsEndpointsBackendForIngress(endpoints, &ingress) {
			continue
		}
		queue.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: ingress.Namespace,
				Name:      ingress.Name,
			},
		})
	}
}

// IsEndpointsBackendForIngress checks if the given endpoints object is associated with the given ingress
func IsEndpointsBackendForIngress(endpoints *corev1.Endpoints, ingress *extensions.Ingress) bool {
	if ingress.Spec.Backend != nil && EndpointsBelongsToBackend(endpoints, ingress, ingress.Spec.Backend) {
		return true
	}
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				if EndpointsBelongsToBackend(endpoints, ingress, &path.Backend) {
					return true
				}
			}
		}
	}
	return false
}

// EndpointsBelongsToBackend checks if the given endpoints object is associated with the given backend of the given ingress
func EndpointsBelongsToBackend(endpoints *corev1.Endpoints, ingress *extensions.Ingress, backend *extensions.IngressBackend) bool {
	if backend.ServiceName == "use-annotation" {
		config, err := action.NewParser(nil).Parse(&ingress.ObjectMeta)
		if err != nil {
			return false
		}
		for _, action := range config.(*action.Config).Actions {
			if aws.StringValue(action.Type) == elbv2.ActionTypeEnumForward {
				for _, targetGroupTuple := range action.ForwardConfig.TargetGroups {
					if aws.StringValue(targetGroupTuple.ServiceName) == backend.ServiceName {
						return true
					}
				}
			}
		}
		return false
	}
	return backend.ServiceName == endpoints.Name
}
