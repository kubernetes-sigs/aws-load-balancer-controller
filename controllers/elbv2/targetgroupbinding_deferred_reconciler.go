package controllers

import (
	"context"
	"k8s.io/apimachinery/pkg/util/cache"
	"math/rand"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/targetgroupbinding"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// The time to delay the reconcile. Generally, this number should be large enough so we can perform all reconciles
	// that have changes before processing reconciles that have no detected changes.
	defaultDelayedReconcileTime = 30 * time.Minute
	// The max amount of jitter to add to delayedReconcileTime. This is used to ensure that all deferred TGBs are not
	// reconciled together.
	defaultMaxJitter = 15 * time.Minute

	// The hash to set that is guaranteed to trigger a new reconcile loop (the hash calculation always has an '/')
	resetHash = ""
)

type DeferredTargetGroupBindingReconciler interface {
	Enqueue(tgb *elbv2api.TargetGroupBinding)
	MarkProcessed(tgb *elbv2api.TargetGroupBinding)
	Run()
}

type deferredTargetGroupBindingReconcilerImpl struct {
	delayQueue workqueue.DelayingInterface
	k8sClient  client.Client
	logger     logr.Logger

	processedTGBCache      *cache.Expiring
	processedTGBCacheTTL   time.Duration
	processedTGBCacheMutex sync.RWMutex

	delayedReconcileTime time.Duration
	maxJitter            time.Duration
}

func NewDeferredTargetGroupBindingReconciler(delayQueue workqueue.DelayingInterface, syncPeriod time.Duration, k8sClient client.Client, logger logr.Logger) DeferredTargetGroupBindingReconciler {
	return &deferredTargetGroupBindingReconcilerImpl{
		logger:               logger,
		delayQueue:           delayQueue,
		k8sClient:            k8sClient,
		processedTGBCache:    cache.NewExpiring(),
		processedTGBCacheTTL: syncPeriod,

		delayedReconcileTime: defaultDelayedReconcileTime,
		maxJitter:            defaultMaxJitter,
	}
}

// Enqueue enqueues a TGB iff it's not been processed recently.
func (d *deferredTargetGroupBindingReconcilerImpl) Enqueue(tgb *elbv2api.TargetGroupBinding) {
	nsn := k8s.NamespacedName(tgb)
	if !d.tgbInCache(tgb) {
		d.enqueue(nsn)
		d.logger.Info("enqueued new deferred TGB", "tgb", nsn.Name)
	}
}

// MarkProcessed updates the local cache to signify that the TGB has been processed recently and is not eligible to be deferred.
func (d *deferredTargetGroupBindingReconcilerImpl) MarkProcessed(tgb *elbv2api.TargetGroupBinding) {
	if d.tgbInCache(tgb) {
		return
	}
	d.updateCache(k8s.NamespacedName(tgb))
}

// Run starts a loop to process deferred items off the delaying queue.
func (d *deferredTargetGroupBindingReconcilerImpl) Run() {
	var item interface{}
	shutDown := false
	for !shutDown {
		item, shutDown = d.delayQueue.Get()
		if item != nil {
			deferredNamespacedName := item.(types.NamespacedName)
			d.logger.Info("Processing deferred TGB", "item", deferredNamespacedName)
			d.handleDeferredItem(deferredNamespacedName)
			d.delayQueue.Done(deferredNamespacedName)
		}
	}

	d.logger.Info("Shutting down deferred TGB queue")
}

func (d *deferredTargetGroupBindingReconcilerImpl) handleDeferredItem(nsn types.NamespacedName) {
	tgb := &elbv2api.TargetGroupBinding{}

	err := d.k8sClient.Get(context.Background(), nsn, tgb)

	if err != nil {
		d.handleDeferredItemError(nsn, err, "Failed to get TGB in deferred queue")
		return
	}

	tgbOld := tgb.DeepCopy()
	targetgroupbinding.SaveTGBReconcileCheckpoint(tgb, resetHash)

	if err := d.k8sClient.Patch(context.Background(), tgb, client.MergeFrom(tgbOld)); err != nil {
		d.handleDeferredItemError(nsn, err, "Failed to reset TGB checkpoint")
		return
	}
	d.logger.Info("TGB checkpoint reset", "tgb", nsn)
}

func (d *deferredTargetGroupBindingReconcilerImpl) handleDeferredItemError(nsn types.NamespacedName, err error, msg string) {
	err = client.IgnoreNotFound(err)
	if err != nil {
		d.logger.Error(err, msg, "tgb", nsn)
		d.enqueue(nsn)
	}
}

func (d *deferredTargetGroupBindingReconcilerImpl) tgbInCache(tgb *elbv2api.TargetGroupBinding) bool {
	d.processedTGBCacheMutex.RLock()
	defer d.processedTGBCacheMutex.RUnlock()

	_, exists := d.processedTGBCache.Get(k8s.NamespacedName(tgb))
	return exists
}

func (d *deferredTargetGroupBindingReconcilerImpl) enqueue(nsn types.NamespacedName) {
	d.updateCache(nsn)
	delayedTime := d.jitterReconcileTime()
	d.delayQueue.AddAfter(nsn, delayedTime)
}

func (d *deferredTargetGroupBindingReconcilerImpl) jitterReconcileTime() time.Duration {

	if d.maxJitter == 0 {
		return d.delayedReconcileTime
	}

	return d.delayedReconcileTime + time.Duration(rand.Int63n(int64(d.maxJitter)))
}

func (d *deferredTargetGroupBindingReconcilerImpl) updateCache(nsn types.NamespacedName) {
	d.processedTGBCacheMutex.Lock()
	defer d.processedTGBCacheMutex.Unlock()
	d.processedTGBCache.Set(nsn, true, d.processedTGBCacheTTL)
}
