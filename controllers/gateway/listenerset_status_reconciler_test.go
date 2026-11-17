package gateway

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/gateway/routeutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func groupPtr(g string) *gwv1.Group {
	group := gwv1.Group(g)
	return &group
}

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	clientgoscheme.AddToScheme(s)
	gwv1.AddToScheme(s)
	return s
}

func newTestK8sClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	builder := testclient.NewClientBuilder().WithScheme(scheme)
	if len(objs) > 0 {
		builder = builder.WithStatusSubresource(objs...).WithObjects(objs...)
	}
	return builder.Build()
}

// newTestK8sClientWithInterceptor builds a fake client whose calls can be intercepted,
// e.g. to inject errors or simulate a concurrent Enqueue during a Patch.
func newTestK8sClientWithInterceptor(scheme *runtime.Scheme, funcs interceptor.Funcs, objs ...client.Object) client.Client {
	builder := testclient.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(funcs)
	if len(objs) > 0 {
		builder = builder.WithStatusSubresource(objs...).WithObjects(objs...)
	}
	return builder.Build()
}

func newTestReconciler(k8sClient client.Client) (*listenerSetStatusReconcilerImpl, workqueue.TypedRateLimitingInterface[types.NamespacedName]) {
	// Millisecond backoff keeps retry tests fast; production uses NewListenerSetStatusRateLimiter.
	queue := workqueue.NewTypedRateLimitingQueue[types.NamespacedName](
		workqueue.NewTypedItemExponentialFailureRateLimiter[types.NamespacedName](time.Millisecond, 10*time.Millisecond))
	logger := logr.New(&log.NullLogSink{})
	r := NewListenerSetStatusReconciler(queue, k8sClient, logger)
	return r.(*listenerSetStatusReconcilerImpl), queue
}

// pendingFor returns the pending entry for nsn, if any.
func pendingFor(r *listenerSetStatusReconcilerImpl, nsn types.NamespacedName) (pendingListenerSetStatus, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pending[nsn]
	return p, ok
}

// processNext dequeues one key and handles it, as Run() would.
func processNext(t *testing.T, r *listenerSetStatusReconcilerImpl, queue workqueue.TypedRateLimitingInterface[types.NamespacedName]) {
	t.Helper()
	nsn, shutDown := queue.Get()
	require.False(t, shutDown)
	r.handleItem(nsn)
	queue.Done(nsn)
}

func TestNewListenerSetStatusReconciler(t *testing.T) {
	scheme := newTestScheme()
	k8sClient := newTestK8sClient(scheme)
	reconciler, queue := newTestReconciler(k8sClient)
	defer queue.ShutDown()

	assert.NotNil(t, reconciler.queue)
	assert.NotNil(t, reconciler.k8sClient)
	assert.NotNil(t, reconciler.pending)
	assert.NotNil(t, reconciler.logger)
}

func TestListenerSetEnqueue(t *testing.T) {
	scheme := newTestScheme()
	k8sClient := newTestK8sClient(scheme)
	reconciler, queue := newTestReconciler(k8sClient)
	defer queue.ShutDown()

	status := routeutils.ListenerSetStatusData{
		ListenerSetMetadata: routeutils.ListenerSetMetadata{
			ListenerSetName:      "test-ls",
			ListenerSetNamespace: "test-ns",
			Generation:           1,
		},
		ListenerSetStatusInfo: routeutils.ListenerSetStatusInfo{
			Accepted:         true,
			AcceptedReason:   "Accepted",
			AcceptedMessage:  "accepted",
			Programmed:       true,
			ProgrammedReason: "Programmed",
		},
	}
	listenerStatuses := []gwv1.ListenerEntryStatus{
		{Name: "listener-1", AttachedRoutes: 2},
	}

	reconciler.Enqueue(status, listenerStatuses)

	nsn := types.NamespacedName{Namespace: "test-ns", Name: "test-ls"}
	cached, exists := pendingFor(reconciler, nsn)

	assert.True(t, exists)
	assert.Equal(t, status, cached.data)
	assert.Len(t, cached.listeners, 1)
	assert.Equal(t, gwv1.SectionName("listener-1"), cached.listeners[0].Name)
	assert.Equal(t, int32(2), cached.listeners[0].AttachedRoutes)
	assert.NotZero(t, cached.version)
	assert.Equal(t, 1, queue.Len())
}

func TestListenerSetEnqueue_OverwritesPreviousEntry(t *testing.T) {
	scheme := newTestScheme()
	k8sClient := newTestK8sClient(scheme)
	reconciler, queue := newTestReconciler(k8sClient)
	defer queue.ShutDown()

	statusV1 := routeutils.ListenerSetStatusData{
		ListenerSetMetadata: routeutils.ListenerSetMetadata{
			ListenerSetName:      "test-ls",
			ListenerSetNamespace: "test-ns",
			Generation:           1,
		},
	}
	statusV2 := statusV1
	statusV2.ListenerSetMetadata.Generation = 2

	reconciler.Enqueue(statusV1, []gwv1.ListenerEntryStatus{{Name: "v1"}})
	nsn := types.NamespacedName{Namespace: "test-ns", Name: "test-ls"}
	first, _ := pendingFor(reconciler, nsn)
	reconciler.Enqueue(statusV2, []gwv1.ListenerEntryStatus{{Name: "v2"}})

	cached, _ := pendingFor(reconciler, nsn)
	assert.Equal(t, int64(2), cached.data.ListenerSetMetadata.Generation)
	assert.Len(t, cached.listeners, 1)
	assert.Equal(t, gwv1.SectionName("v2"), cached.listeners[0].Name)
	assert.NotEqual(t, first.version, cached.version)
	// Both enqueues share one key, so the queue holds a single item.
	assert.Equal(t, 1, queue.Len())
}

func TestHandleItem_SuccessfulStatusUpdate(t *testing.T) {
	scheme := newTestScheme()
	ls := &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-ls",
			Namespace:  "test-ns",
			Generation: 3,
		},
	}
	k8sClient := newTestK8sClient(scheme, ls)
	reconciler, queue := newTestReconciler(k8sClient)
	defer queue.ShutDown()

	status := routeutils.ListenerSetStatusData{
		ListenerSetMetadata: routeutils.ListenerSetMetadata{
			ListenerSetName:      "test-ls",
			ListenerSetNamespace: "test-ns",
			Generation:           3,
		},
		ListenerSetStatusInfo: routeutils.ListenerSetStatusInfo{
			Accepted:          true,
			AcceptedReason:    "Accepted",
			AcceptedMessage:   "all good",
			Programmed:        true,
			ProgrammedReason:  "Programmed",
			ProgrammedMessage: "programmed",
		},
	}
	listenerStatuses := []gwv1.ListenerEntryStatus{
		{Name: "listener-1", AttachedRoutes: 5},
	}

	reconciler.Enqueue(status, listenerStatuses)

	processNext(t, reconciler, queue)

	// Verify the status was written to the API server
	updated := &gwv1.ListenerSet{}
	err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "test-ns", Name: "test-ls"}, updated)
	require.NoError(t, err)

	assert.Len(t, updated.Status.Conditions, 2)
	assert.Len(t, updated.Status.Listeners, 1)
	assert.Equal(t, gwv1.SectionName("listener-1"), updated.Status.Listeners[0].Name)
	assert.Equal(t, int32(5), updated.Status.Listeners[0].AttachedRoutes)

	// Verify conditions
	condMap := make(map[string]metav1.Condition)
	for _, c := range updated.Status.Conditions {
		condMap[c.Type] = c
	}
	assert.Equal(t, metav1.ConditionTrue, condMap[string(gwv1.ListenerSetConditionAccepted)].Status)
	assert.Equal(t, "Accepted", condMap[string(gwv1.ListenerSetConditionAccepted)].Reason)
	assert.Equal(t, metav1.ConditionTrue, condMap[string(gwv1.ListenerSetConditionProgrammed)].Status)
	assert.Equal(t, "Programmed", condMap[string(gwv1.ListenerSetConditionProgrammed)].Reason)
	assert.Equal(t, int64(3), condMap[string(gwv1.ListenerSetConditionAccepted)].ObservedGeneration)

	// Cache should be cleaned up after successful update
	nsn := types.NamespacedName{Namespace: "test-ns", Name: "test-ls"}
	_, exists := pendingFor(reconciler, nsn)
	assert.False(t, exists)
	assert.Equal(t, 0, queue.Len())
}

func TestHandleItem_NotFoundListenerSet(t *testing.T) {
	scheme := newTestScheme()
	// No ListenerSet object created — simulates a deleted resource
	k8sClient := newTestK8sClient(scheme)
	reconciler, queue := newTestReconciler(k8sClient)
	defer queue.ShutDown()

	status := routeutils.ListenerSetStatusData{
		ListenerSetMetadata: routeutils.ListenerSetMetadata{
			ListenerSetName:      "missing-ls",
			ListenerSetNamespace: "test-ns",
		},
	}
	reconciler.Enqueue(status, []gwv1.ListenerEntryStatus{{Name: "l1"}})

	processNext(t, reconciler, queue)

	// Should not requeue — NotFound is swallowed
	assert.Equal(t, 0, queue.Len())
	nsn := types.NamespacedName{Namespace: "test-ns", Name: "missing-ls"}
	assert.Equal(t, 0, queue.NumRequeues(nsn))
	// The pending entry for the deleted ListenerSet must not leak
	_, exists := pendingFor(reconciler, nsn)
	assert.False(t, exists, "pending entry should be removed for a deleted ListenerSet")
}

func TestHandleItem_NoPendingEntryIsNoop(t *testing.T) {
	scheme := newTestScheme()
	getCalls := 0
	k8sClient := newTestK8sClientWithInterceptor(scheme, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			getCalls++
			return c.Get(ctx, key, obj, opts...)
		},
	})
	reconciler, queue := newTestReconciler(k8sClient)
	defer queue.ShutDown()

	nsn := types.NamespacedName{Namespace: "test-ns", Name: "test-ls"}
	queue.Add(nsn)
	processNext(t, reconciler, queue)

	assert.Equal(t, 0, getCalls, "no API calls when there is nothing pending")
	assert.Equal(t, 0, queue.Len())
}

func TestHandleItem_CachePreservedWhenVersionChanges(t *testing.T) {
	// Simulate an Enqueue arriving while the status Patch is in flight: the
	// interceptor calls Enqueue with newer data during the first Patch. The
	// worker must keep the newer pending entry and sync it on the next pass.
	scheme := newTestScheme()
	ls := &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-ls",
			Namespace:  "test-ns",
			Generation: 1,
		},
	}
	nsn := types.NamespacedName{Namespace: "test-ns", Name: "test-ls"}

	status := routeutils.ListenerSetStatusData{
		ListenerSetMetadata: routeutils.ListenerSetMetadata{
			ListenerSetName:      "test-ls",
			ListenerSetNamespace: "test-ns",
			Generation:           1,
		},
		ListenerSetStatusInfo: routeutils.ListenerSetStatusInfo{
			Accepted:         true,
			AcceptedReason:   "Accepted",
			Programmed:       true,
			ProgrammedReason: "Programmed",
		},
	}

	var reconciler *listenerSetStatusReconcilerImpl
	patchCalls := 0
	k8sClient := newTestK8sClientWithInterceptor(scheme, interceptor.Funcs{
		SubResourcePatch: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
			patchCalls++
			if patchCalls == 1 {
				reconciler.Enqueue(status, []gwv1.ListenerEntryStatus{{Name: "v2"}})
			}
			return c.SubResource(subResourceName).Patch(ctx, obj, patch, opts...)
		},
	}, ls)
	reconciler, queue := newTestReconciler(k8sClient)
	defer queue.ShutDown()

	reconciler.Enqueue(status, []gwv1.ListenerEntryStatus{{Name: "v1"}})
	processNext(t, reconciler, queue)

	cached, exists := pendingFor(reconciler, nsn)
	require.True(t, exists, "newer pending entry must survive the first pass")
	assert.Equal(t, gwv1.SectionName("v2"), cached.listeners[0].Name)
	assert.Equal(t, 1, queue.Len(), "key should be re-queued for the newer entry")

	processNext(t, reconciler, queue)

	_, exists = pendingFor(reconciler, nsn)
	assert.False(t, exists, "pending entry should be cleaned after syncing the latest version")
	updated := &gwv1.ListenerSet{}
	require.NoError(t, k8sClient.Get(context.Background(), nsn, updated))
	require.Len(t, updated.Status.Listeners, 1)
	assert.Equal(t, gwv1.SectionName("v2"), updated.Status.Listeners[0].Name)
}

func TestHandleItem_RequeuesWithBackoffOnError(t *testing.T) {
	scheme := newTestScheme()
	k8sClient := newTestK8sClientWithInterceptor(scheme, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			return assert.AnError
		},
	})
	reconciler, queue := newTestReconciler(k8sClient)
	defer queue.ShutDown()

	nsn := types.NamespacedName{Namespace: "test-ns", Name: "test-ls"}
	reconciler.Enqueue(routeutils.ListenerSetStatusData{
		ListenerSetMetadata: routeutils.ListenerSetMetadata{
			ListenerSetName:      "test-ls",
			ListenerSetNamespace: "test-ns",
		},
	}, []gwv1.ListenerEntryStatus{{Name: "l1"}})

	processNext(t, reconciler, queue)

	assert.Equal(t, 1, queue.NumRequeues(nsn))
	_, exists := pendingFor(reconciler, nsn)
	assert.True(t, exists, "pending entry must be kept for the retry")
	assert.Eventually(t, func() bool {
		return queue.Len() > 0
	}, 5*time.Second, 5*time.Millisecond, "item should be requeued after backoff")
}

func TestHandleItem_DropsAfterMaxRetries(t *testing.T) {
	scheme := newTestScheme()
	getCalls := 0
	k8sClient := newTestK8sClientWithInterceptor(scheme, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			getCalls++
			return assert.AnError
		},
	})
	reconciler, queue := newTestReconciler(k8sClient)
	defer queue.ShutDown()

	nsn := types.NamespacedName{Namespace: "test-ns", Name: "test-ls"}
	reconciler.Enqueue(routeutils.ListenerSetStatusData{
		ListenerSetMetadata: routeutils.ListenerSetMetadata{
			ListenerSetName:      "test-ls",
			ListenerSetNamespace: "test-ns",
		},
	}, []gwv1.ListenerEntryStatus{{Name: "l1"}})

	// Initial attempt plus maxRetries retries.
	for i := 0; i <= maxRetries; i++ {
		processNext(t, reconciler, queue)
	}

	assert.Equal(t, maxRetries+1, getCalls)
	assert.Equal(t, 0, queue.NumRequeues(nsn), "backoff should be reset after dropping")
	_, exists := pendingFor(reconciler, nsn)
	assert.False(t, exists, "pending entry should not leak after dropping")
	// Nothing further should be scheduled.
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 0, queue.Len())
}

func TestHandleItem_RetryUsesLatestStatus(t *testing.T) {
	// The first update fails. Before the retry, a newer status is enqueued.
	// The retry must write the newer status, never the stale one.
	scheme := newTestScheme()
	ls := &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-ls",
			Namespace:  "test-ns",
			Generation: 2,
		},
	}
	nsn := types.NamespacedName{Namespace: "test-ns", Name: "test-ls"}

	failNext := true
	k8sClient := newTestK8sClientWithInterceptor(scheme, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if failNext {
				failNext = false
				return assert.AnError
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}, ls)
	reconciler, queue := newTestReconciler(k8sClient)
	defer queue.ShutDown()

	stale := routeutils.ListenerSetStatusData{
		ListenerSetMetadata: routeutils.ListenerSetMetadata{
			ListenerSetName:      "test-ls",
			ListenerSetNamespace: "test-ns",
			Generation:           1,
		},
		ListenerSetStatusInfo: routeutils.ListenerSetStatusInfo{
			Programmed:       false,
			ProgrammedReason: "Pending",
		},
	}
	latest := stale
	latest.ListenerSetMetadata.Generation = 2
	latest.ListenerSetStatusInfo.Programmed = true
	latest.ListenerSetStatusInfo.ProgrammedReason = "Programmed"

	reconciler.Enqueue(stale, []gwv1.ListenerEntryStatus{{Name: "stale"}})
	processNext(t, reconciler, queue) // fails, schedules retry

	reconciler.Enqueue(latest, []gwv1.ListenerEntryStatus{{Name: "latest"}})
	processNext(t, reconciler, queue) // succeeds with latest

	updated := &gwv1.ListenerSet{}
	require.NoError(t, k8sClient.Get(context.Background(), nsn, updated))
	require.Len(t, updated.Status.Listeners, 1)
	assert.Equal(t, gwv1.SectionName("latest"), updated.Status.Listeners[0].Name)
	condMap := make(map[string]metav1.Condition)
	for _, c := range updated.Status.Conditions {
		condMap[c.Type] = c
	}
	assert.Equal(t, metav1.ConditionTrue, condMap[string(gwv1.ListenerSetConditionProgrammed)].Status)
	assert.Equal(t, int64(2), condMap[string(gwv1.ListenerSetConditionProgrammed)].ObservedGeneration)

	_, exists := pendingFor(reconciler, nsn)
	assert.False(t, exists)
	assert.Equal(t, 0, queue.NumRequeues(nsn))

	// A delayed retry from the failed attempt may still fire; it must be a no-op.
	time.Sleep(50 * time.Millisecond)
	for queue.Len() > 0 {
		processNext(t, reconciler, queue)
	}
	require.NoError(t, k8sClient.Get(context.Background(), nsn, updated))
	require.Len(t, updated.Status.Listeners, 1)
	assert.Equal(t, gwv1.SectionName("latest"), updated.Status.Listeners[0].Name)
}

func TestDoStatusUpdate_SkipsPatchWhenIdentical(t *testing.T) {
	scheme := newTestScheme()
	fixedTime := metav1.Now()

	ls := &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-ls",
			Namespace:  "test-ns",
			Generation: 1,
		},
		Status: gwv1.ListenerSetStatus{
			Conditions: []metav1.Condition{
				{
					Type:               string(gwv1.ListenerSetConditionAccepted),
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 1,
					LastTransitionTime: fixedTime,
					Reason:             "Accepted",
					Message:            "all good",
				},
				{
					Type:               string(gwv1.ListenerSetConditionProgrammed),
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 1,
					LastTransitionTime: fixedTime,
					Reason:             "Programmed",
					Message:            "done",
				},
			},
			Listeners: []gwv1.ListenerEntryStatus{
				{Name: "listener-1", AttachedRoutes: 3},
			},
		},
	}
	k8sClient := newTestK8sClient(scheme, ls)
	reconciler, queue := newTestReconciler(k8sClient)
	defer queue.ShutDown()

	// Build a status that matches what's already on the object
	status := routeutils.ListenerSetStatusData{
		ListenerSetMetadata: routeutils.ListenerSetMetadata{
			ListenerSetName:      "test-ls",
			ListenerSetNamespace: "test-ns",
			Generation:           1,
		},
		ListenerSetStatusInfo: routeutils.ListenerSetStatusInfo{
			Accepted:          true,
			AcceptedReason:    "Accepted",
			AcceptedMessage:   "all good",
			Programmed:        true,
			ProgrammedReason:  "Programmed",
			ProgrammedMessage: "done",
		},
	}
	listeners := []gwv1.ListenerEntryStatus{{Name: "listener-1", AttachedRoutes: 3}}

	// This should not error — the patch is skipped because status is identical
	err := reconciler.doStatusUpdate(status, listeners)
	assert.NoError(t, err)
}

func TestDoStatusUpdate_PatchesWhenDifferent(t *testing.T) {
	scheme := newTestScheme()
	ls := &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-ls",
			Namespace:  "test-ns",
			Generation: 2,
		},
		Status: gwv1.ListenerSetStatus{
			Conditions: []metav1.Condition{
				{
					Type:               string(gwv1.ListenerSetConditionAccepted),
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 1,
					Reason:             "Accepted",
				},
			},
		},
	}
	k8sClient := newTestK8sClient(scheme, ls)
	reconciler, queue := newTestReconciler(k8sClient)
	defer queue.ShutDown()

	status := routeutils.ListenerSetStatusData{
		ListenerSetMetadata: routeutils.ListenerSetMetadata{
			ListenerSetName:      "test-ls",
			ListenerSetNamespace: "test-ns",
			Generation:           2,
		},
		ListenerSetStatusInfo: routeutils.ListenerSetStatusInfo{
			Accepted:          false,
			AcceptedReason:    "Invalid",
			AcceptedMessage:   "something wrong",
			Programmed:        false,
			ProgrammedReason:  "Invalid",
			ProgrammedMessage: "not programmed",
		},
	}
	listeners := []gwv1.ListenerEntryStatus{{Name: "listener-1", AttachedRoutes: 0}}

	err := reconciler.doStatusUpdate(status, listeners)
	require.NoError(t, err)

	updated := &gwv1.ListenerSet{}
	err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "test-ns", Name: "test-ls"}, updated)
	require.NoError(t, err)

	condMap := make(map[string]metav1.Condition)
	for _, c := range updated.Status.Conditions {
		condMap[c.Type] = c
	}
	assert.Equal(t, metav1.ConditionFalse, condMap[string(gwv1.ListenerSetConditionAccepted)].Status)
	assert.Equal(t, "Invalid", condMap[string(gwv1.ListenerSetConditionAccepted)].Reason)
	assert.Equal(t, metav1.ConditionFalse, condMap[string(gwv1.ListenerSetConditionProgrammed)].Status)
	assert.Equal(t, int64(2), condMap[string(gwv1.ListenerSetConditionAccepted)].ObservedGeneration)
}

func TestBuildListenerSetConditions(t *testing.T) {
	tests := []struct {
		name               string
		status             routeutils.ListenerSetStatusData
		expectedAccepted   metav1.ConditionStatus
		expectedProgrammed metav1.ConditionStatus
	}{
		{
			name: "both accepted and programmed",
			status: routeutils.ListenerSetStatusData{
				ListenerSetMetadata: routeutils.ListenerSetMetadata{Generation: 5},
				ListenerSetStatusInfo: routeutils.ListenerSetStatusInfo{
					Accepted:          true,
					AcceptedReason:    "Accepted",
					AcceptedMessage:   "ok",
					Programmed:        true,
					ProgrammedReason:  "Programmed",
					ProgrammedMessage: "ok",
				},
			},
			expectedAccepted:   metav1.ConditionTrue,
			expectedProgrammed: metav1.ConditionTrue,
		},
		{
			name: "not accepted, not programmed",
			status: routeutils.ListenerSetStatusData{
				ListenerSetMetadata: routeutils.ListenerSetMetadata{Generation: 2},
				ListenerSetStatusInfo: routeutils.ListenerSetStatusInfo{
					Accepted:         false,
					AcceptedReason:   "Invalid",
					AcceptedMessage:  "bad config",
					Programmed:       false,
					ProgrammedReason: "Invalid",
				},
			},
			expectedAccepted:   metav1.ConditionFalse,
			expectedProgrammed: metav1.ConditionFalse,
		},
		{
			name: "accepted but not programmed",
			status: routeutils.ListenerSetStatusData{
				ListenerSetMetadata: routeutils.ListenerSetMetadata{Generation: 3},
				ListenerSetStatusInfo: routeutils.ListenerSetStatusInfo{
					Accepted:         true,
					AcceptedReason:   "Accepted",
					Programmed:       false,
					ProgrammedReason: "Pending",
				},
			},
			expectedAccepted:   metav1.ConditionTrue,
			expectedProgrammed: metav1.ConditionFalse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			k8sClient := newTestK8sClient(scheme)
			reconciler, queue := newTestReconciler(k8sClient)
			defer queue.ShutDown()

			conditions := reconciler.buildListenerSetConditions(tt.status)

			assert.Len(t, conditions, 2)

			condMap := make(map[string]metav1.Condition)
			for _, c := range conditions {
				condMap[c.Type] = c
			}

			accepted := condMap[string(gwv1.ListenerSetConditionAccepted)]
			assert.Equal(t, tt.expectedAccepted, accepted.Status)
			assert.Equal(t, tt.status.ListenerSetStatusInfo.AcceptedReason, accepted.Reason)
			assert.Equal(t, tt.status.ListenerSetStatusInfo.AcceptedMessage, accepted.Message)
			assert.Equal(t, tt.status.ListenerSetMetadata.Generation, accepted.ObservedGeneration)
			assert.False(t, accepted.LastTransitionTime.IsZero())

			programmed := condMap[string(gwv1.ListenerSetConditionProgrammed)]
			assert.Equal(t, tt.expectedProgrammed, programmed.Status)
			assert.Equal(t, tt.status.ListenerSetStatusInfo.ProgrammedReason, programmed.Reason)
			assert.Equal(t, tt.status.ListenerSetStatusInfo.ProgrammedMessage, programmed.Message)
			assert.Equal(t, tt.status.ListenerSetMetadata.Generation, programmed.ObservedGeneration)
		})
	}
}

func TestAreConditionsIdentical(t *testing.T) {
	fixedTime := metav1.Now()
	tests := []struct {
		name     string
		old      []metav1.Condition
		new      []metav1.Condition
		expected bool
	}{
		{
			name:     "both nil",
			old:      nil,
			new:      nil,
			expected: true,
		},
		{
			name:     "different lengths",
			old:      []metav1.Condition{{Type: "A"}},
			new:      []metav1.Condition{{Type: "A"}, {Type: "B"}},
			expected: false,
		},
		{
			name: "same conditions different order",
			old: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionTrue, Reason: "r", Message: "m", ObservedGeneration: 1},
				{Type: "A", Status: metav1.ConditionTrue, Reason: "r", Message: "m", ObservedGeneration: 1},
			},
			new: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "r", Message: "m", ObservedGeneration: 1},
				{Type: "B", Status: metav1.ConditionTrue, Reason: "r", Message: "m", ObservedGeneration: 1},
			},
			expected: true,
		},
		{
			name: "different LastTransitionTime is ignored",
			old: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "r", LastTransitionTime: fixedTime, ObservedGeneration: 1},
			},
			new: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "r", LastTransitionTime: metav1.NewTime(fixedTime.Add(time.Hour)), ObservedGeneration: 1},
			},
			expected: true,
		},
		{
			name: "different status",
			old: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "r", ObservedGeneration: 1},
			},
			new: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionFalse, Reason: "r", ObservedGeneration: 1},
			},
			expected: false,
		},
		{
			name: "different reason",
			old: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "r1", ObservedGeneration: 1},
			},
			new: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "r2", ObservedGeneration: 1},
			},
			expected: false,
		},
		{
			name: "different observed generation",
			old: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "r", ObservedGeneration: 1},
			},
			new: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "r", ObservedGeneration: 2},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, areConditionsIdentical(tt.old, tt.new))
		})
	}
}

func TestAreListenerEntryStatusesIdentical(t *testing.T) {
	tests := []struct {
		name     string
		old      []gwv1.ListenerEntryStatus
		new      []gwv1.ListenerEntryStatus
		expected bool
	}{
		{
			name:     "both nil",
			old:      nil,
			new:      nil,
			expected: true,
		},
		{
			name:     "different lengths",
			old:      []gwv1.ListenerEntryStatus{{Name: "a"}},
			new:      []gwv1.ListenerEntryStatus{{Name: "a"}, {Name: "b"}},
			expected: false,
		},
		{
			name: "same entries different order",
			old: []gwv1.ListenerEntryStatus{
				{Name: "b", AttachedRoutes: 1},
				{Name: "a", AttachedRoutes: 2},
			},
			new: []gwv1.ListenerEntryStatus{
				{Name: "a", AttachedRoutes: 2},
				{Name: "b", AttachedRoutes: 1},
			},
			expected: true,
		},
		{
			name:     "different attached routes",
			old:      []gwv1.ListenerEntryStatus{{Name: "a", AttachedRoutes: 1}},
			new:      []gwv1.ListenerEntryStatus{{Name: "a", AttachedRoutes: 5}},
			expected: false,
		},
		{
			name:     "different names",
			old:      []gwv1.ListenerEntryStatus{{Name: "a"}},
			new:      []gwv1.ListenerEntryStatus{{Name: "b"}},
			expected: false,
		},
		{
			name: "different supported kinds",
			old: []gwv1.ListenerEntryStatus{{
				Name:           "a",
				SupportedKinds: []gwv1.RouteGroupKind{{Group: groupPtr("gateway.networking.k8s.io"), Kind: "HTTPRoute"}},
			}},
			new: []gwv1.ListenerEntryStatus{{
				Name:           "a",
				SupportedKinds: []gwv1.RouteGroupKind{{Group: groupPtr("gateway.networking.k8s.io"), Kind: "GRPCRoute"}},
			}},
			expected: false,
		},
		{
			name: "different conditions",
			old: []gwv1.ListenerEntryStatus{{
				Name:       "a",
				Conditions: []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionTrue, Reason: "r", ObservedGeneration: 1}},
			}},
			new: []gwv1.ListenerEntryStatus{{
				Name:       "a",
				Conditions: []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionFalse, Reason: "r", ObservedGeneration: 1}},
			}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, areListenerEntryStatusesIdentical(tt.old, tt.new))
		})
	}
}

func TestIsStatusIdentical(t *testing.T) {
	scheme := newTestScheme()
	k8sClient := newTestK8sClient(scheme)
	reconciler, queue := newTestReconciler(k8sClient)
	defer queue.ShutDown()

	fixedTime := metav1.Now()
	base := gwv1.ListenerSetStatus{
		Conditions: []metav1.Condition{
			{Type: "Accepted", Status: metav1.ConditionTrue, Reason: "Accepted", Message: "ok", ObservedGeneration: 1, LastTransitionTime: fixedTime},
		},
		Listeners: []gwv1.ListenerEntryStatus{
			{Name: "l1", AttachedRoutes: 2},
		},
	}

	// Identical
	assert.True(t, reconciler.isStatusIdentical(base, base))

	// Different condition
	diff := base.DeepCopy()
	diff.Conditions[0].Status = metav1.ConditionFalse
	assert.False(t, reconciler.isStatusIdentical(base, *diff))

	// Different listeners
	diff2 := base.DeepCopy()
	diff2.Listeners[0].AttachedRoutes = 99
	assert.False(t, reconciler.isStatusIdentical(base, *diff2))
}

func TestRun_StopsOnShutdown(t *testing.T) {
	scheme := newTestScheme()
	k8sClient := newTestK8sClient(scheme)
	reconciler, queue := newTestReconciler(k8sClient)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		reconciler.Run()
	}()

	// Shut down the queue — Run() should exit
	queue.ShutDown()
	wg.Wait() // will hang if Run() doesn't exit
}

// Regression test: two Enqueue calls for the same ListenerSet with different status
// data used to produce two queue items sharing one cache entry. Processing the first
// deleted the entry written by the second, so the second patched Listeners to nil.
func TestHandleItem_DistinctStatusesForSameListenerSet_PreserveListeners(t *testing.T) {
	scheme := newTestScheme()
	ls := &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-ls",
			Namespace:  "test-ns",
			Generation: 1,
		},
	}
	k8sClient := newTestK8sClient(scheme, ls)
	reconciler, queue := newTestReconciler(k8sClient)
	defer queue.ShutDown()

	metadata := routeutils.ListenerSetMetadata{
		ListenerSetName:      "test-ls",
		ListenerSetNamespace: "test-ns",
		Generation:           1,
	}
	// Reconcile #1: gateway not yet programmed.
	statusA := routeutils.ListenerSetStatusData{
		ListenerSetMetadata: metadata,
		ListenerSetStatusInfo: routeutils.ListenerSetStatusInfo{
			Accepted:          true,
			AcceptedReason:    "Accepted",
			AcceptedMessage:   "Accepted",
			Programmed:        false,
			ProgrammedReason:  "Pending",
			ProgrammedMessage: "Parent gateway not yet programmed",
		},
	}
	// Reconcile #2: gateway programmed.
	statusB := statusA
	statusB.ListenerSetStatusInfo.Programmed = true
	statusB.ListenerSetStatusInfo.ProgrammedReason = "Programmed"
	statusB.ListenerSetStatusInfo.ProgrammedMessage = "Programmed"

	reconciler.Enqueue(statusA, []gwv1.ListenerEntryStatus{{Name: "listener-1", AttachedRoutes: 1}})
	reconciler.Enqueue(statusB, []gwv1.ListenerEntryStatus{{Name: "listener-1", AttachedRoutes: 2}})
	require.Equal(t, 1, queue.Len(), "enqueues for the same ListenerSet should collapse into one key")

	// Drain the queue, as Run() would.
	for queue.Len() > 0 {
		processNext(t, reconciler, queue)
	}

	updated := &gwv1.ListenerSet{}
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "test-ns", Name: "test-ls"}, updated))

	// Final conditions should reflect the latest enqueue (B).
	condMap := make(map[string]metav1.Condition)
	for _, c := range updated.Status.Conditions {
		condMap[c.Type] = c
	}
	assert.Equal(t, metav1.ConditionTrue, condMap[string(gwv1.ListenerSetConditionProgrammed)].Status)

	// Final listener statuses should be the latest ones, not wiped.
	require.Len(t, updated.Status.Listeners, 1, "listener statuses were wiped by the second queue item")
	assert.Equal(t, gwv1.SectionName("listener-1"), updated.Status.Listeners[0].Name)
	assert.Equal(t, int32(2), updated.Status.Listeners[0].AttachedRoutes)
}
