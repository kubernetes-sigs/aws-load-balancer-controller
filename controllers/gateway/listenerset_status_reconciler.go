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
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
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

type listenerSetStatusReconcilerImpl struct {
	queue                         workqueue.TypedDelayingInterface[routeutils.ListenerSetStatusData]
	listenerSetListenerCache      map[types.NamespacedName]routeutils.ListenerSetListenerInfo
	listenerSetListenerCacheMutex sync.RWMutex
	k8sClient                     client.Client
	logger                        logr.Logger
}

// NewListenerSetStatusReconciler
// Responsible for updating the status of ListenerSet objects
func NewListenerSetStatusReconciler(queue workqueue.TypedDelayingInterface[routeutils.ListenerSetStatusData], k8sClient client.Client, logger logr.Logger) ListenerSetStatusReconciler {
	return &listenerSetStatusReconcilerImpl{
		logger:                   logger,
		queue:                    queue,
		k8sClient:                k8sClient,
		listenerSetListenerCache: make(map[types.NamespacedName]routeutils.ListenerSetListenerInfo),
	}
}

func (statusUpdater *listenerSetStatusReconcilerImpl) Enqueue(status routeutils.ListenerSetStatusData, listenerStatuses []gwv1.ListenerEntryStatus) {
	statusUpdater.listenerSetListenerCacheMutex.Lock()
	statusUpdater.listenerSetListenerCache[types.NamespacedName{
		Namespace: status.ListenerSetMetadata.ListenerSetNamespace,
		Name:      status.ListenerSetMetadata.ListenerSetName,
	}] = routeutils.ListenerSetListenerInfo{
		Version:  time.Now(),
		Statuses: listenerStatuses,
	}
	statusUpdater.listenerSetListenerCacheMutex.Unlock()
	statusUpdater.queue.Add(status)
}

func (statusUpdater *listenerSetStatusReconcilerImpl) Run() {
	for {
		item, shutDown := statusUpdater.queue.Get()
		if shutDown {
			break
		}
		statusUpdater.handleItem(item)
		statusUpdater.queue.Done(item)
	}
}

func (statusUpdater *listenerSetStatusReconcilerImpl) handleItem(status routeutils.ListenerSetStatusData) {
	nsn := types.NamespacedName{
		Namespace: status.ListenerSetMetadata.ListenerSetNamespace,
		Name:      status.ListenerSetMetadata.ListenerSetName,
	}
	statusUpdater.listenerSetListenerCacheMutex.RLock()
	listenerData := statusUpdater.listenerSetListenerCache[nsn]
	statusUpdater.listenerSetListenerCacheMutex.RUnlock()

	err := statusUpdater.doStatusUpdate(status, listenerData)

	if err != nil {
		statusUpdater.handleItemError(status, err, "Failed to update listener set status")
		return
	}

	statusUpdater.listenerSetListenerCacheMutex.Lock()

	if statusUpdater.listenerSetListenerCache[nsn].Version.Equal(listenerData.Version) {
		delete(statusUpdater.listenerSetListenerCache, nsn)
	}

	statusUpdater.listenerSetListenerCacheMutex.Unlock()
}

const (
	maxRetryDelay  = 30 * time.Second
	baseRetryDelay = 1 * time.Second
	maxRetries     = 10
)

func (statusUpdater *listenerSetStatusReconcilerImpl) handleItemError(status routeutils.ListenerSetStatusData, err error, msg string) {
	err = client.IgnoreNotFound(err)
	if err != nil {
		if status.RetryCount >= maxRetries {
			statusUpdater.logger.Error(err, "Max retries exceeded, dropping item", "retries", status.RetryCount)
			return
		}
		statusUpdater.logger.Error(err, msg)
		delay := baseRetryDelay * time.Duration(1<<min(status.RetryCount, 5))
		if delay > maxRetryDelay {
			delay = maxRetryDelay
		}
		status.RetryCount++
		statusUpdater.queue.AddAfter(status, delay)
	}
}

func (statusUpdater *listenerSetStatusReconcilerImpl) doStatusUpdate(status routeutils.ListenerSetStatusData, info routeutils.ListenerSetListenerInfo) error {
	currentListenerSet := &gwv1.ListenerSet{}
	listenerSetNsn := types.NamespacedName{Namespace: status.ListenerSetMetadata.ListenerSetNamespace, Name: status.ListenerSetMetadata.ListenerSetName}
	if err := statusUpdater.k8sClient.Get(context.Background(), listenerSetNsn, currentListenerSet); err != nil {
		return err
	}

	oldListenerSet := currentListenerSet.DeepCopyObject().(*gwv1.ListenerSet)
	currentListenerSet.Status.Conditions = statusUpdater.buildListenerSetConditions(status)
	currentListenerSet.Status.Listeners = info.Statuses
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
