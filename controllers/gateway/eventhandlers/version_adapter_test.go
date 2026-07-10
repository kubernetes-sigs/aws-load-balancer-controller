package eventhandlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type recordingTCPRouteHandler struct {
	created []*gwv1.TCPRoute
	updated []*gwv1.TCPRoute
	updatedOld []*gwv1.TCPRoute
	deleted []*gwv1.TCPRoute
	deletedStateUnknown []bool
	generic []*gwv1.TCPRoute
}

func (r *recordingTCPRouteHandler) Create(_ context.Context, e event.TypedCreateEvent[*gwv1.TCPRoute], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	r.created = append(r.created, e.Object)
}
func (r *recordingTCPRouteHandler) Update(_ context.Context, e event.TypedUpdateEvent[*gwv1.TCPRoute], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	r.updated = append(r.updated, e.ObjectNew)
	r.updatedOld = append(r.updatedOld, e.ObjectOld)
}
func (r *recordingTCPRouteHandler) Delete(_ context.Context, e event.TypedDeleteEvent[*gwv1.TCPRoute], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	r.deleted = append(r.deleted, e.Object)
	r.deletedStateUnknown = append(r.deletedStateUnknown, e.DeleteStateUnknown)
}
func (r *recordingTCPRouteHandler) Generic(_ context.Context, e event.TypedGenericEvent[*gwv1.TCPRoute], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	r.generic = append(r.generic, e.Object)
}

var _ handler.TypedEventHandler[*gwv1.TCPRoute, reconcile.Request] = &recordingTCPRouteHandler{}

func TestVersionAdapterHandler(t *testing.T) {
	inner := &recordingTCPRouteHandler{}
	adapter := NewVersionAdapterHandler[*gwalpha2.TCPRoute, *gwv1.TCPRoute](routeutils.ConvertAlpha2TCPRouteToV1, inner)

	alphaRoute := &gwalpha2.TCPRoute{ObjectMeta: metav1.ObjectMeta{Name: "r1", Namespace: "ns"}}
	alphaRouteOld := &gwalpha2.TCPRoute{ObjectMeta: metav1.ObjectMeta{Name: "r1-old", Namespace: "ns"}}

	adapter.Create(context.Background(), event.TypedCreateEvent[*gwalpha2.TCPRoute]{Object: alphaRoute}, nil)
	adapter.Update(context.Background(), event.TypedUpdateEvent[*gwalpha2.TCPRoute]{ObjectOld: alphaRouteOld, ObjectNew: alphaRoute}, nil)
	adapter.Delete(context.Background(), event.TypedDeleteEvent[*gwalpha2.TCPRoute]{Object: alphaRoute, DeleteStateUnknown: true}, nil)
	adapter.Generic(context.Background(), event.TypedGenericEvent[*gwalpha2.TCPRoute]{Object: alphaRoute}, nil)

	assert.Len(t, inner.created, 1)
	assert.Len(t, inner.updated, 1)
	assert.Len(t, inner.updatedOld, 1)
	assert.Len(t, inner.deleted, 1)
	assert.Len(t, inner.deletedStateUnknown, 1)
	assert.Len(t, inner.generic, 1)
	assert.Equal(t, "r1", inner.created[0].Name)
	assert.Equal(t, "ns", inner.created[0].Namespace)
	assert.Equal(t, "r1-old", inner.updatedOld[0].Name)
	assert.Equal(t, "ns", inner.updatedOld[0].Namespace)
	assert.Equal(t, "r1", inner.deleted[0].Name)
	assert.Equal(t, "ns", inner.deleted[0].Namespace)
	assert.True(t, inner.deletedStateUnknown[0])
	assert.Equal(t, "r1", inner.generic[0].Name)
	assert.Equal(t, "ns", inner.generic[0].Namespace)
}
