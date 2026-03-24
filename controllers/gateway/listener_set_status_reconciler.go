package gateway

/*
type ListenerSetStatusReconciler interface {
	Enqueue(routeutils.ListenerSetStatusData)
	Run()
}

type listenerSetStatusReconcilerImpl struct {
	queue            workqueue.DelayingInterface
	statusCache      map[types.NamespacedName]routeutils.ListenerSetStatusInfo
	statusCacheMutex sync.RWMutex
	k8sClient        client.Client
	logger           logr.Logger
}

// NewListenerSetStatusReconciler
// Responsible for updating the status of ListenerSet objects
func NewListenerSetStatusReconciler(queue workqueue.DelayingInterface, k8sClient client.Client, logger logr.Logger) ListenerSetStatusReconciler {
	return &listenerSetStatusReconcilerImpl{
		logger:    logger,
		queue:     queue,
		k8sClient: k8sClient,
	}
}

func (statusUpdater *listenerSetStatusReconcilerImpl) Enqueue(status routeutils.ListenerSetStatusData) {
	statusUpdater.statusCacheMutex.Lock()
	statusUpdater.statusCache[types.NamespacedName{
		Namespace: status.ListenerSetMetadata.ListenerSetNamespace,
		Name:      status.ListenerSetMetadata.ListenerSetName,
	}] = status.ListenerSetStatusInfo
	statusUpdater.statusCacheMutex.Unlock()
	statusUpdater.queue.Add(status.ListenerSetMetadata)
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

func (statusUpdater *listenerSetStatusReconcilerImpl) handleItem(md routeutils.ListenerSetMetadata) {
	err := statusUpdater.doStatusUpdate(md)
	if err != nil {
		statusUpdater.handleItemError(md, err, "Failed to update listener set status")
		return
	}
}

func (statusUpdater *listenerSetStatusReconcilerImpl) handleItemError(md routeutils.ListenerSetMetadata, err error, msg string) {
	err = client.IgnoreNotFound(err)
	if err != nil {
		statusUpdater.logger.Error(err, msg)
		statusUpdater.Enqueue(md)
	}
}

func (statusUpdater *listenerSetStatusReconcilerImpl) doStatusUpdate(md routeutils.ListenerSetMetadata) error {
	currentListenerSet := &gwv1.ListenerSet{}
	if err := statusUpdater.k8sClient.Get(context.Background(), types.NamespacedName{Namespace: md.ListenerSetNamespace, Name: md.ListenerSetName}, currentListenerSet); err != nil {
		return client.IgnoreNotFound(err)
	}

	if currentListenerSet.ObjectMeta.Generation > md.Generation {
		return nil
	}

	oldListenerSet := currentListenerSet.DeepCopyObject()

}


*/
