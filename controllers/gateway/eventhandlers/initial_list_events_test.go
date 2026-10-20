package eventhandlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func Test_CreateEvent_IsInInitialList(t *testing.T) {
	ctx := context.Background()
	k8sClient := testutils.GenerateTestClient()

	gwClass := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "alb-class"},
		Spec:       gwv1.GatewayClassSpec{ControllerName: gwv1.GatewayController(constants.ALBGatewayController)},
	}
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "default"},
		Spec:       gwv1.GatewaySpec{GatewayClassName: "alb-class"},
	}
	assert.NoError(t, k8sClient.Create(ctx, gwClass))
	assert.NoError(t, k8sClient.Create(ctx, gw))

	ls := &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{Name: "my-ls", Namespace: "default"},
		Spec: gwv1.ListenerSetSpec{
			ParentRef: gwv1.ParentGatewayReference{Name: "my-gw"},
		},
	}
	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "default"},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{Name: "my-gw"}},
			},
		},
	}

	gwHandler := NewEnqueueRequestsForGatewayEventHandler(k8sClient, nil, constants.ALBGatewayController, log.Log)
	gwClassHandler := NewEnqueueRequestsForGatewayClassEvent(k8sClient, nil, constants.ALBGatewayController, log.Log)
	lsHandler := NewEnqueueRequestsForListenerSetEvent(k8sClient, nil, constants.ALBGatewayController, log.Log)
	routeHandler := NewEnqueueRequestsForHTTPRouteEvent(k8sClient, nil, log.Log)

	tests := []struct {
		name            string
		create          func(isInInitialList bool, queue workqueue.TypedRateLimitingInterface[reconcile.Request])
		wantInitialList bool
	}{
		{
			name: "gateway",
			create: func(initial bool, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				gwHandler.Create(ctx, event.TypedCreateEvent[*gwv1.Gateway]{Object: gw, IsInInitialList: initial}, q)
			},
			wantInitialList: true,
		},
		{
			name: "gatewayclass",
			create: func(initial bool, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				gwClassHandler.Create(ctx, event.TypedCreateEvent[*gwv1.GatewayClass]{Object: gwClass, IsInInitialList: initial}, q)
			},
		},
		{
			name: "listenerset",
			create: func(initial bool, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				lsHandler.Create(ctx, event.TypedCreateEvent[*gwv1.ListenerSet]{Object: ls, IsInInitialList: initial}, q)
			},
		},
		{
			name: "httproute",
			create: func(initial bool, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				routeHandler.Create(ctx, event.TypedCreateEvent[*gwv1.HTTPRoute]{Object: route, IsInInitialList: initial}, q)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, initial := range []bool{true, false} {
				queue := workqueue.NewTypedRateLimitingQueue[reconcile.Request](
					workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
				)
				tt.create(initial, queue)

				if initial && !tt.wantInitialList {
					assert.Equal(t, 0, queue.Len(), "initial list event should be skipped")
				} else {
					assert.Equal(t, 1, queue.Len())
					item, _ := queue.Get()
					assert.Equal(t, types.NamespacedName{Name: "my-gw", Namespace: "default"}, item.NamespacedName)
				}
				queue.ShutDown()
			}
		})
	}
}
