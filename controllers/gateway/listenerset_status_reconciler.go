package gateway

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/gateway/routeutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ListenerSetStatusReconciler interface {
	Run()
	ListenerSetStatusSubmitter
}

type ListenerSetStatusSubmitter interface {
	Enqueue(status routeutils.ListenerSetStatusData, listenerStatuses []gwv1.ListenerEntryStatus)
}

// NoopListenerSetStatusSubmitter is used when the GatewayListenerSet feature gate is disabled.
type NoopListenerSetStatusSubmitter struct{}

func (n *NoopListenerSetStatusSubmitter) Enqueue(_ routeutils.ListenerSetStatusData, _ []gwv1.ListenerEntryStatus) {
}

var _ ListenerSetStatusSubmitter = &NoopListenerSetStatusSubmitter{}

const (
	maxRetryDelay  = 30 * time.Second
	baseRetryDelay = 1 * time.Second
	maxRetries     = 10
)

// NewListenerSetStatusRateLimiter returns the backoff used between failed status update attempts.
func NewListenerSetStatusRateLimiter() workqueue.TypedRateLimiter[types.NamespacedName] {
	return workqueue.NewTypedItemExponentialFailureRateLimiter[types.NamespacedName](baseRetryDelay, maxRetryDelay)
}

// pendingListenerSetStatus is the latest desired status for a ListenerSet.
// version changes on every Enqueue and is used to detect whether a newer
// status arrived while an update was in flight.
type pendingListenerSetStatus struct {
	version   uint64
	data      routeutils.ListenerSetStatusData
	listeners []gwv1.ListenerEntryStatus
}

// listenerSetStatusReconcilerImpl writes ListenerSet status asynchronously.
// The queue only carries ListenerSet keys; the desired status lives in pending.
// Multiple Enqueues for the same ListenerSet collapse into one sync of the
// latest state, and retries always pick up the newest state rather than
// replaying a stale one.
type listenerSetStatusReconcilerImpl struct {
	queue     workqueue.TypedRateLimitingInterface[types.NamespacedName]
	mu        sync.Mutex
	pending   map[types.NamespacedName]pendingListenerSetStatus
	nextVer   uint64
	k8sClient client.Client
	logger    logr.Logger
}

// NewListenerSetStatusReconciler
// Responsible for updating the status of ListenerSet objects
func NewListenerSetStatusReconciler(queue workqueue.TypedRateLimitingInterface[types.NamespacedName], k8sClient client.Client, logger logr.Logger) ListenerSetStatusReconciler {
	return &listenerSetStatusReconcilerImpl{
		logger:    logger,
		queue:     queue,
		k8sClient: k8sClient,
		pending:   make(map[types.NamespacedName]pendingListenerSetStatus),
	}
}

func (statusUpdater *listenerSetStatusReconcilerImpl) Enqueue(status routeutils.ListenerSetStatusData, listenerStatuses []gwv1.ListenerEntryStatus) {
	nsn := types.NamespacedName{
		Namespace: status.ListenerSetMetadata.ListenerSetNamespace,
		Name:      status.ListenerSetMetadata.ListenerSetName,
	}
	statusUpdater.mu.Lock()
	statusUpdater.nextVer++
	statusUpdater.pending[nsn] = pendingListenerSetStatus{
		version:   statusUpdater.nextVer,
		data:      status,
		listeners: listenerStatuses,
	}
	statusUpdater.mu.Unlock()
	// Add after writing pending so the worker always finds this entry when it dequeues the key.
	statusUpdater.queue.Add(nsn)
}

func (statusUpdater *listenerSetStatusReconcilerImpl) Run() {
	for {
		nsn, shutDown := statusUpdater.queue.Get()
		if shutDown {
			break
		}
		statusUpdater.handleItem(nsn)
		statusUpdater.queue.Done(nsn)
	}
}

func (statusUpdater *listenerSetStatusReconcilerImpl) handleItem(nsn types.NamespacedName) {
	statusUpdater.mu.Lock()
	snapshot, ok := statusUpdater.pending[nsn]
	statusUpdater.mu.Unlock()
	if !ok {
		// Already synced by an earlier pass for this key.
		statusUpdater.queue.Forget(nsn)
		return
	}

	err := client.IgnoreNotFound(statusUpdater.doStatusUpdate(snapshot.data, snapshot.listeners))
	if err != nil {
		if statusUpdater.queue.NumRequeues(nsn) < maxRetries {
			statusUpdater.logger.Error(err, "Failed to update listener set status", "listenerSet", nsn)
			// The retry re-reads pending, so it writes whatever is latest at that time.
			statusUpdater.queue.AddRateLimited(nsn)
			return
		}
		statusUpdater.logger.Error(err, "Max retries exceeded, dropping item", "listenerSet", nsn, "retries", statusUpdater.queue.NumRequeues(nsn))
	}

	// Terminal outcome (success, NotFound, or retries exhausted): reset backoff and clean up.
	statusUpdater.queue.Forget(nsn)
	statusUpdater.mu.Lock()
	if cur, ok := statusUpdater.pending[nsn]; ok && cur.version == snapshot.version {
		delete(statusUpdater.pending, nsn)
	}
	// Otherwise a newer Enqueue arrived during the update; its key is already queued.
	statusUpdater.mu.Unlock()
}

func (statusUpdater *listenerSetStatusReconcilerImpl) doStatusUpdate(status routeutils.ListenerSetStatusData, listenerStatuses []gwv1.ListenerEntryStatus) error {
	currentListenerSet := &gwv1.ListenerSet{}
	listenerSetNsn := types.NamespacedName{Namespace: status.ListenerSetMetadata.ListenerSetNamespace, Name: status.ListenerSetMetadata.ListenerSetName}
	if err := statusUpdater.k8sClient.Get(context.Background(), listenerSetNsn, currentListenerSet); err != nil {
		return err
	}

	oldListenerSet := currentListenerSet.DeepCopyObject().(*gwv1.ListenerSet)
	currentListenerSet.Status.Conditions = statusUpdater.buildListenerSetConditions(status)
	currentListenerSet.Status.Listeners = listenerStatuses
	if !statusUpdater.isStatusIdentical(oldListenerSet.Status, currentListenerSet.Status) {
		if err := statusUpdater.k8sClient.Status().Patch(context.Background(), currentListenerSet, client.MergeFrom(oldListenerSet)); err != nil {
			return err
		}
	}
	return nil
}

func (statusUpdater *listenerSetStatusReconcilerImpl) isStatusIdentical(o, n gwv1.ListenerSetStatus) bool {
	if !areConditionsIdentical(o.Conditions, n.Conditions) {
		return false
	}
	return areListenerEntryStatusesIdentical(o.Listeners, n.Listeners)
}

func areConditionsIdentical(o, n []metav1.Condition) bool {
	if len(o) != len(n) {
		return false
	}
	oCopy := make([]metav1.Condition, len(o))
	nCopy := make([]metav1.Condition, len(n))
	copy(oCopy, o)
	copy(nCopy, n)
	sort.Slice(oCopy, func(i, j int) bool { return oCopy[i].Type < oCopy[j].Type })
	sort.Slice(nCopy, func(i, j int) bool { return nCopy[i].Type < nCopy[j].Type })
	for i := range oCopy {
		if oCopy[i].Type != nCopy[i].Type ||
			oCopy[i].Status != nCopy[i].Status ||
			oCopy[i].Reason != nCopy[i].Reason ||
			oCopy[i].Message != nCopy[i].Message ||
			oCopy[i].ObservedGeneration != nCopy[i].ObservedGeneration {
			return false
		}
	}
	return true
}

func areListenerEntryStatusesIdentical(o, n []gwv1.ListenerEntryStatus) bool {
	if len(o) != len(n) {
		return false
	}
	oCopy := make([]gwv1.ListenerEntryStatus, len(o))
	nCopy := make([]gwv1.ListenerEntryStatus, len(n))
	copy(oCopy, o)
	copy(nCopy, n)
	sort.Slice(oCopy, func(i, j int) bool { return oCopy[i].Name < oCopy[j].Name })
	sort.Slice(nCopy, func(i, j int) bool { return nCopy[i].Name < nCopy[j].Name })
	for i := range oCopy {
		if oCopy[i].Name != nCopy[i].Name {
			return false
		}
		if !compareSupportedKinds(oCopy[i].SupportedKinds, nCopy[i].SupportedKinds) {
			return false
		}
		if oCopy[i].AttachedRoutes != nCopy[i].AttachedRoutes {
			return false
		}
		if !areConditionsIdentical(oCopy[i].Conditions, nCopy[i].Conditions) {
			return false
		}
	}
	return true
}

func (statusUpdater *listenerSetStatusReconcilerImpl) buildListenerSetConditions(status routeutils.ListenerSetStatusData) []metav1.Condition {

	acceptedCondition := metav1.ConditionTrue
	if !status.ListenerSetStatusInfo.Accepted {
		acceptedCondition = metav1.ConditionFalse
	}

	programmedCondition := metav1.ConditionTrue
	if !status.ListenerSetStatusInfo.Programmed {
		programmedCondition = metav1.ConditionFalse
	}

	return []metav1.Condition{
		{
			Type:               string(gwv1.ListenerSetConditionAccepted),
			Status:             acceptedCondition,
			ObservedGeneration: status.ListenerSetMetadata.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             status.ListenerSetStatusInfo.AcceptedReason,
			Message:            status.ListenerSetStatusInfo.AcceptedMessage,
		},
		{
			Type:               string(gwv1.ListenerSetConditionProgrammed),
			Status:             programmedCondition,
			ObservedGeneration: status.ListenerSetMetadata.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             status.ListenerSetStatusInfo.ProgrammedReason,
			Message:            status.ListenerSetStatusInfo.ProgrammedMessage,
		},
	}
}
