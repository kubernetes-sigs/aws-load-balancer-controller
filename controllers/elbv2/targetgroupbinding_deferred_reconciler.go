package controllers

import (
	"context"
	"math/rand"
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
	Run()
}

type deferredTargetGroupBindingReconcilerImpl struct {
	delayQueue workqueue.DelayingInterface
	syncPeriod time.Duration
	k8sClient  client.Client
	logger     logr.Logger

	delayedReconcileTime time.Duration
	maxJitter            time.Duration
}

func NewDeferredTargetGroupBindingReconciler(delayQueue workqueue.DelayingInterface, syncPeriod time.Duration, k8sClient client.Client, logger logr.Logger) DeferredTargetGroupBindingReconciler {
	return &deferredTargetGroupBindingReconcilerImpl{
		syncPeriod: syncPeriod,
		logger:     logger,
		delayQueue: delayQueue,
		k8sClient:  k8sClient,

		delayedReconcileTime: defaultDelayedReconcileTime,
		maxJitter:            defaultMaxJitter,
	}
}

func (d *deferredTargetGroupBindingReconcilerImpl) Enqueue(tgb *elbv2api.TargetGroupBinding) {
	nsn := k8s.NamespacedName(tgb)
	if d.isEligibleForDefer(tgb) {
		d.enqueue(nsn)
		d.logger.Info("enqueued new deferred TGB", "tgb", nsn.Name)
	}
}

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

	// Re-check that this tgb hasn't been updated since it was enqueued
	if !d.isEligibleForDefer(tgb) {
		d.logger.Info("TGB not eligible for deferral", "tgb", nsn)
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

func (d *deferredTargetGroupBindingReconcilerImpl) isEligibleForDefer(tgb *elbv2api.TargetGroupBinding) bool {
	then := time.Unix(targetgroupbinding.GetTGBReconcileCheckpointTimestamp(tgb), 0)
	return time.Now().Sub(then) > d.syncPeriod
}

func (d *deferredTargetGroupBindingReconcilerImpl) enqueue(nsn types.NamespacedName) {
	delayedTime := d.jitterReconcileTime()
	d.delayQueue.AddAfter(nsn, delayedTime)
}

func (d *deferredTargetGroupBindingReconcilerImpl) jitterReconcileTime() time.Duration {

	if d.maxJitter == 0 {
		return d.delayedReconcileTime
	}

	return d.delayedReconcileTime + time.Duration(rand.Int63n(int64(d.maxJitter)))
}
