package eventhandlers

import (
	"context"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewVersionAdapterHandler wraps a typed event handler for API version V so it
// can be registered on a watch for the same resource served at another API
// version A. Objects are converted at the event boundary; the inner handler
// only ever sees the controller's internal (v1) representation. This supports
// watching TCPRoute/UDPRoute at v1alpha2 on clusters running Gateway API < 1.6.
func NewVersionAdapterHandler[A client.Object, V client.Object](
	convert func(A) V,
	inner handler.TypedEventHandler[V, reconcile.Request],
) handler.TypedEventHandler[A, reconcile.Request] {
	return &versionAdapterHandler[A, V]{convert: convert, inner: inner}
}

type versionAdapterHandler[A client.Object, V client.Object] struct {
	convert func(A) V
	inner   handler.TypedEventHandler[V, reconcile.Request]
}

func (h *versionAdapterHandler[A, V]) Create(ctx context.Context, e event.TypedCreateEvent[A], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.inner.Create(ctx, event.TypedCreateEvent[V]{Object: h.convert(e.Object)}, queue)
}

func (h *versionAdapterHandler[A, V]) Update(ctx context.Context, e event.TypedUpdateEvent[A], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.inner.Update(ctx, event.TypedUpdateEvent[V]{ObjectOld: h.convert(e.ObjectOld), ObjectNew: h.convert(e.ObjectNew)}, queue)
}

func (h *versionAdapterHandler[A, V]) Delete(ctx context.Context, e event.TypedDeleteEvent[A], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.inner.Delete(ctx, event.TypedDeleteEvent[V]{Object: h.convert(e.Object), DeleteStateUnknown: e.DeleteStateUnknown}, queue)
}

func (h *versionAdapterHandler[A, V]) Generic(ctx context.Context, e event.TypedGenericEvent[A], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.inner.Generic(ctx, event.TypedGenericEvent[V]{Object: h.convert(e.Object)}, queue)
}
