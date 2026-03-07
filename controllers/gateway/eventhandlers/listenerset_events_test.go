package eventhandlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func Test_enqueueRequestsForListenerSetEvent_enqueueParentGateway(t *testing.T) {
	gwNamespace := gwv1.Namespace("other-ns")

	tests := []struct {
		name             string
		listenerSet      *gwv1.ListenerSet
		gateway          *gwv1.Gateway
		gatewayClass     *gwv1.GatewayClass
		gwController     string
		expectedQueueLen int
		expectedRequest  *reconcile.Request
	}{
		{
			name: "ListenerSet referencing managed Gateway in same namespace",
			listenerSet: &gwv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ls",
					Namespace: "test-ns",
				},
				Spec: gwv1.ListenerSetSpec{
					ParentRef: gwv1.ParentGatewayReference{
						Name: "test-gw",
					},
				},
			},
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gw",
					Namespace: "test-ns",
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "alb-class",
				},
			},
			gatewayClass: &gwv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "alb-class",
				},
				Spec: gwv1.GatewayClassSpec{
					ControllerName: gwv1.GatewayController(constants.ALBGatewayController),
				},
			},
			gwController:     constants.ALBGatewayController,
			expectedQueueLen: 1,
			expectedRequest: &reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-gw", Namespace: "test-ns"},
			},
		},
		{
			name: "ListenerSet referencing managed Gateway in different namespace via explicit parentRef namespace",
			listenerSet: &gwv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cross-ns-ls",
					Namespace: "ls-ns",
				},
				Spec: gwv1.ListenerSetSpec{
					ParentRef: gwv1.ParentGatewayReference{
						Name:      "test-gw",
						Namespace: &gwNamespace,
					},
				},
			},
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gw",
					Namespace: "other-ns",
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "nlb-class",
				},
			},
			gatewayClass: &gwv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nlb-class",
				},
				Spec: gwv1.GatewayClassSpec{
					ControllerName: gwv1.GatewayController(constants.NLBGatewayController),
				},
			},
			gwController:     constants.NLBGatewayController,
			expectedQueueLen: 1,
			expectedRequest: &reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-gw", Namespace: "other-ns"},
			},
		},
		{
			name: "ListenerSet referencing non-existent Gateway",
			listenerSet: &gwv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "orphan-ls",
					Namespace: "test-ns",
				},
				Spec: gwv1.ListenerSetSpec{
					ParentRef: gwv1.ParentGatewayReference{
						Name: "missing-gw",
					},
				},
			},
			gateway:          nil,
			gatewayClass:     nil,
			gwController:     constants.ALBGatewayController,
			expectedQueueLen: 0,
		},
		{
			name: "ListenerSet referencing Gateway managed by different controller",
			listenerSet: &gwv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-ctrl-ls",
					Namespace: "test-ns",
				},
				Spec: gwv1.ListenerSetSpec{
					ParentRef: gwv1.ParentGatewayReference{
						Name: "test-gw",
					},
				},
			},
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gw",
					Namespace: "test-ns",
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "other-class",
				},
			},
			gatewayClass: &gwv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "other-class",
				},
				Spec: gwv1.GatewayClassSpec{
					ControllerName: "some.other.controller",
				},
			},
			gwController:     constants.ALBGatewayController,
			expectedQueueLen: 0,
		},
		{
			name:             "nil ListenerSet does not panic",
			listenerSet:      nil,
			gateway:          nil,
			gatewayClass:     nil,
			gwController:     constants.ALBGatewayController,
			expectedQueueLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sClient := testutils.GenerateTestClient()

			if tt.gatewayClass != nil {
				assert.NoError(t, k8sClient.Create(ctx, tt.gatewayClass))
			}
			if tt.gateway != nil {
				assert.NoError(t, k8sClient.Create(ctx, tt.gateway))
			}

			handler := NewEnqueueRequestsForListenerSetEvent(
				k8sClient, nil, tt.gwController, log.Log,
			)

			queue := workqueue.NewTypedRateLimitingQueue[reconcile.Request](
				workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
			)
			defer queue.ShutDown()

			h := handler.(*enqueueRequestsForListenerSetEvent)
			h.enqueueParentGateway(ctx, tt.listenerSet, queue)

			assert.Equal(t, tt.expectedQueueLen, queue.Len())
			if tt.expectedRequest != nil && queue.Len() > 0 {
				item, _ := queue.Get()
				assert.Equal(t, tt.expectedRequest.NamespacedName, item.NamespacedName)
			}
		})
	}
}

func Test_enqueueRequestsForListenerSetEvent_Create(t *testing.T) {
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

	handler := NewEnqueueRequestsForListenerSetEvent(k8sClient, nil, constants.ALBGatewayController, log.Log)
	queue := workqueue.NewTypedRateLimitingQueue[reconcile.Request](
		workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
	)
	defer queue.ShutDown()

	handler.Create(ctx, event.TypedCreateEvent[*gwv1.ListenerSet]{Object: ls}, queue)

	assert.Equal(t, 1, queue.Len())
	item, _ := queue.Get()
	assert.Equal(t, types.NamespacedName{Name: "my-gw", Namespace: "default"}, item.NamespacedName)
}

func Test_enqueueRequestsForListenerSetEvent_Update(t *testing.T) {
	t.Run("same parentRef enqueues gateway once (deduped by rate limiter)", func(t *testing.T) {
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

		lsOld := &gwv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "my-ls", Namespace: "default"},
			Spec: gwv1.ListenerSetSpec{
				ParentRef: gwv1.ParentGatewayReference{Name: "my-gw"},
			},
		}
		lsNew := &gwv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "my-ls", Namespace: "default"},
			Spec: gwv1.ListenerSetSpec{
				ParentRef: gwv1.ParentGatewayReference{Name: "my-gw"},
			},
		}

		h := NewEnqueueRequestsForListenerSetEvent(k8sClient, nil, constants.ALBGatewayController, log.Log)
		queue := workqueue.NewTypedRateLimitingQueue[reconcile.Request](
			workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
		)
		defer queue.ShutDown()

		h.Update(ctx, event.TypedUpdateEvent[*gwv1.ListenerSet]{ObjectOld: lsOld, ObjectNew: lsNew}, queue)

		assert.Equal(t, 1, queue.Len())
		item, _ := queue.Get()
		assert.Equal(t, types.NamespacedName{Name: "my-gw", Namespace: "default"}, item.NamespacedName)
	})

	t.Run("changed parentRef enqueues both old and new gateways", func(t *testing.T) {
		ctx := context.Background()
		k8sClient := testutils.GenerateTestClient()

		gwClass := &gwv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "alb-class"},
			Spec:       gwv1.GatewayClassSpec{ControllerName: gwv1.GatewayController(constants.ALBGatewayController)},
		}
		gwOld := &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw-old", Namespace: "default"},
			Spec:       gwv1.GatewaySpec{GatewayClassName: "alb-class"},
		}
		gwNew := &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw-new", Namespace: "default"},
			Spec:       gwv1.GatewaySpec{GatewayClassName: "alb-class"},
		}
		assert.NoError(t, k8sClient.Create(ctx, gwClass))
		assert.NoError(t, k8sClient.Create(ctx, gwOld))
		assert.NoError(t, k8sClient.Create(ctx, gwNew))

		lsOld := &gwv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "my-ls", Namespace: "default"},
			Spec: gwv1.ListenerSetSpec{
				ParentRef: gwv1.ParentGatewayReference{Name: "gw-old"},
			},
		}
		lsNew := &gwv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "my-ls", Namespace: "default"},
			Spec: gwv1.ListenerSetSpec{
				ParentRef: gwv1.ParentGatewayReference{Name: "gw-new"},
			},
		}

		h := NewEnqueueRequestsForListenerSetEvent(k8sClient, nil, constants.ALBGatewayController, log.Log)
		queue := workqueue.NewTypedRateLimitingQueue[reconcile.Request](
			workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
		)
		defer queue.ShutDown()

		h.Update(ctx, event.TypedUpdateEvent[*gwv1.ListenerSet]{ObjectOld: lsOld, ObjectNew: lsNew}, queue)

		assert.Equal(t, 2, queue.Len())
		enqueued := map[string]bool{}
		for i := 0; i < 2; i++ {
			item, _ := queue.Get()
			enqueued[item.NamespacedName.Name] = true
		}
		assert.True(t, enqueued["gw-old"], "old gateway should be enqueued")
		assert.True(t, enqueued["gw-new"], "new gateway should be enqueued")
	})
}

func Test_enqueueRequestsForListenerSetEvent_Delete(t *testing.T) {
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

	handler := NewEnqueueRequestsForListenerSetEvent(k8sClient, nil, constants.ALBGatewayController, log.Log)
	queue := workqueue.NewTypedRateLimitingQueue[reconcile.Request](
		workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
	)
	defer queue.ShutDown()

	handler.Delete(ctx, event.TypedDeleteEvent[*gwv1.ListenerSet]{Object: ls}, queue)

	assert.Equal(t, 1, queue.Len())
	item, _ := queue.Get()
	assert.Equal(t, types.NamespacedName{Name: "my-gw", Namespace: "default"}, item.NamespacedName)
}
