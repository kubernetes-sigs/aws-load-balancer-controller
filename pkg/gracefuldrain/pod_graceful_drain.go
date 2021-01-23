package gracefuldrain

import (
	"context"
	"encoding/json"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/targetgroupbinding"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"sync"
	"time"
)

// NewPodGracefulDrain constructs new PodGracefulDrain
func NewPodGracefulDrain(config Config, k8sClient client.Client, logger logr.Logger) *PodGracefulDrain {
	ctx, cancel := context.WithCancel(context.Background())

	return &PodGracefulDrain{
		config:    config,
		k8sClient: k8sClient,
		logger:    logger,

		deleterStopper:    make(chan struct{}, 1),
		deleterContext:    ctx,
		deleterCancelFunc: cancel,
	}
}

// The controller is notified when a pod is deleted and it'll deregister the pod from an ELB.
// However, there is inheritant delay until the pod is fully deregistered.
// This is the cause of 5XX error: ELB send traffic to a already terminated pods during the delay.
// PodGracefulDrain will delay a pod deletion while trigger the deregistration by isolating the pod from Endpoints and ReplicaSets.
type PodGracefulDrain struct {
	config    Config
	k8sClient client.Client
	logger    logr.Logger

	allowedReentry      map[types.UID]bool
	allowedReentryMutex sync.RWMutex

	deleterStopper    chan struct{}
	deleterWaitGroup  sync.WaitGroup
	deleterContext    context.Context
	deleterCancelFunc context.CancelFunc
}

const (
	gracefulDrainPrefix = "graceful-drain.elbv2.k8s.aws"
	// withLabelKey labels the pod is waiting for the draining.
	waitLabelKey                = gracefulDrainPrefix + "/wait"
	deleteAtAnnotationKey       = gracefulDrainPrefix + "/deleteAt"
	originalLabelsAnnotationKey = gracefulDrainPrefix + "/originalLabels"

	defaultDeleterGracefulTerminationPeriod = 10 * time.Second
)

// InterceptPodDeletion intercepts the pod deletion on validatingAdmissionWebhook, isolates the pod, and schedules the delayed deletion if needed.
// It won't bother if the pod is/was not bound to any TargetGroupBinding, or any errors have been occurred during this process.
func (d *PodGracefulDrain) InterceptPodDeletion(ctx context.Context, pod *corev1.Pod) error {
	if d.config.PodGracefulDrainDelay == time.Duration(0) {
		return nil
	}

	logger := d.logger.WithValues("pod", types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      pod.Name,
	})

	canDeleteNow, err := d.canDeleteNow(ctx, pod)
	if err != nil {
		logger.Error(err, "unable to determine pod deletion should be delayed")
		return nil
	} else if canDeleteNow {
		return nil
	}

	// At this point, it might be one of the following cases:
	// 1) The target pod is going to be deleted
	//    => we should patch to isolate them, schedule the delayed deletion and deny the admission.
	// 2) apiserver immediately retried the deletion when we patched the pod and denied the admission
	//    since it is indistinguishable from the collision.
	//    => isolatePod should be idempotent. Keep denying the admission until it forgive.
	// 3) GC tries to delete the pod if there is lingering ownerReferences.
	//    => isolatePod should patch properly so the GC doesn't kick in.
	// 4) Users and controllers manually tries to delete the pod before deleteAt.
	//    => User can see the admission report message. Controller should handle admission failures.
	// 5) We disabled wait sentinel label and deleted the pod, but the patch hasn't been propagated fast enough
	//    so ValidatingAdmissionWebhook read the wait label of the old version
	//    => deleteAfter will retry with back-offs, so we keep denying the admission.

	isolated, err := d.isolatePod(ctx, pod)
	if err != nil {
		logger.Error(err, "unable to isolate the pod")
		return nil
	}

	if isolated {
		d.deleteAfter(pod, d.config.PodGracefulDrainDelay)
	}

	return errors.New("pod-graceful-drain took over the pod's deletion. It will eventually be deleted")
}

func (d *PodGracefulDrain) canDeleteNow(ctx context.Context, pod *corev1.Pod) (bool, error) {
	req := webhook.ContextGetAdmissionRequest(ctx)
	if req.DryRun != nil && *req.DryRun == true {
		return true, nil
	}

	waitLabelValue := pod.Labels[waitLabelKey]
	deleteAt, err := getDeleteAtAnnotation(pod)
	if len(waitLabelValue) > 0 {
		now := time.Now()
		if err != nil && now.Before(deleteAt) {
			return false, nil
		}
		// deleteAt is missing or malformed, but it is okay to delete it later.
		return false, nil
	} else if err == nil {
		// The wait label might be deleted by the user, or this controller. Allow its deletions.
		return true, err
	}

	tgbs, err := d.fetchTGBsForDelayedDeletion(ctx, pod)
	if err != nil {
		return true, err
	}

	if len(tgbs) == 0 {
		for _, item := range pod.Spec.ReadinessGates {
			if strings.HasPrefix(string(item.ConditionType), targetgroupbinding.TargetHealthPodConditionTypePrefix) {
				// The pod once had TargetGroupBindings, but it is somehow gone.
				// We don't know whether its TargetType is IP, it's target group, etc.
				// It might be worth to to give some time to ELB.
				return false, nil
			}
		}
		return true, nil
	}
	return false, nil
}

func (d *PodGracefulDrain) fetchTGBsForDelayedDeletion(ctx context.Context, pod *corev1.Pod) ([]elbv2api.TargetGroupBinding, error) {
	tgbList := &elbv2api.TargetGroupBindingList{}
	if err := d.k8sClient.List(ctx, tgbList, client.InNamespace(pod.Namespace)); err != nil {
		d.logger.V(1).Info("unable to list TargetGroupBindings", "namespace", pod.Namespace)
		return nil, err
	}
	var tgbs []elbv2api.TargetGroupBinding
	for _, tgb := range tgbList.Items {
		if tgb.Spec.TargetType == nil || (*tgb.Spec.TargetType) != elbv2api.TargetTypeIP {
			continue
		}

		svcKey := types.NamespacedName{Namespace: tgb.Namespace, Name: tgb.Spec.ServiceRef.Name}
		svc := &corev1.Service{}
		if err := d.k8sClient.Get(ctx, svcKey, svc); err != nil {
			// If the service is not found, ignore
			if apierrors.IsNotFound(err) {
				d.logger.Info("unable to lookup service", "service", svcKey)
				continue
			}
			return nil, err
		}
		var svcSelector labels.Selector
		if len(svc.Spec.Selector) == 0 {
			svcSelector = labels.Nothing()
		} else {
			svcSelector = labels.SelectorFromSet(svc.Spec.Selector)
		}
		if svcSelector.Matches(labels.Set(pod.Labels)) {
			tgbs = append(tgbs, tgb)
		}
	}
	return tgbs, nil
}

func (d *PodGracefulDrain) isolatePod(ctx context.Context, pod *corev1.Pod) (bool, error) {
	patchCond := func(pod *corev1.Pod) bool {
		existingLabel := pod.Labels[waitLabelKey]
		return len(existingLabel) > 0
	}
	patchMutate := func(pod *corev1.Pod) error {
		deleteAt := time.Now().Add(d.config.PodGracefulDrainDelay)

		oldLabels, err := json.Marshal(pod.Labels)
		if err != nil {
			return err
		}

		pod.Labels = map[string]string{
			waitLabelKey: "true",
		}
		pod.Annotations[deleteAtAnnotationKey] = deleteAt.Format(time.RFC3339)
		pod.Annotations[originalLabelsAnnotationKey] = string(oldLabels)

		var newOwnerReferences []metav1.OwnerReference
		// To stop the GC kicking in, we cut the OwnerReferences.
		for _, item := range pod.OwnerReferences {
			newItem := item.DeepCopy()
			newItem.Controller = nil
			newOwnerReferences = append(newOwnerReferences, *newItem)
		}
		pod.OwnerReferences = newOwnerReferences

		return nil
	}

	return d.patchPod(ctx, pod, patchCond, patchMutate)
}

func (d *PodGracefulDrain) deleteAfter(pod *corev1.Pod, dur time.Duration) {
	logger := d.logger.WithValues("pod", types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      pod.Name,
	})

	d.deleterWaitGroup.Add(1)
	ctx, cancel := context.WithCancel(d.deleterContext)
	go func(pod corev1.Pod) {
		defer d.deleterWaitGroup.Done()
		defer cancel()

		select {
		case <-d.deleterStopper:
		case <-time.After(dur):
		}

		patched, err := d.disableWaitLabel(ctx, &pod)
		if err != nil {
			logger.Error(err, "unable to disable the wait label")
			return
		}
		if !patched {
			return // pod have been deleted
		}

		err = wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
			if err := d.k8sClient.Delete(ctx, &pod, client.Preconditions{UID: &pod.UID}); err != nil {
				if apierrors.IsNotFound(err) || apierrors.IsConflict(err) {
					// The pod is already deleted. Okay to ignore
					return true, nil
				}
				// InterceptPodDeletion might deny the deletion as TooEarly until disableWaitLabel patch is propagated.
				// TODO: error is actually admission denial
				return false, nil
			}
			return true, nil
		})

		if err != nil {
			logger.Error(err, "unable to delete the pod")
		} else {
			logger.V(1).Info("successfully deleted the delayed pod")
		}
	}(*pod.DeepCopy())

	logger.V(1).Info("scheduled pod deletion", "deleteAt", time.Now().Add(dur))
}

func (d *PodGracefulDrain) disableWaitLabel(ctx context.Context, pod *corev1.Pod) (bool, error) {
	patchCond := func(pod *corev1.Pod) bool {
		existingLabel := pod.Labels[waitLabelKey]
		return len(existingLabel) == 0
	}
	patchMutate := func(pod *corev1.Pod) error {
		// set empty rather than removing it. It helps to manually find delayed pods.
		pod.Labels[waitLabelKey] = ""
		return nil
	}
	return d.patchPod(ctx, pod, patchCond, patchMutate)
}

// returns true when it successfully patched.
// returns false when the pod is deleted or the condition is already met.
func (d *PodGracefulDrain) patchPod(ctx context.Context, pod *corev1.Pod, condition func(*corev1.Pod) bool, mutate func(*corev1.Pod) error) (bool, error) {
	podUID := pod.UID
	podKey := types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      pod.Name,
	}

	for {
		if condition(pod) {
			return false, nil
		}

		oldPod := pod.DeepCopy()
		oldPod.UID = "" // only put the uid in the new object to ensure it appears in the patch as a precondition

		if err := mutate(pod); err != nil {
			return false, nil
		}

		podMergeOption := client.MergeFromWithOptions(oldPod, client.MergeFromWithOptimisticLock{})
		if err := d.k8sClient.Patch(ctx, pod, podMergeOption); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			if apierrors.IsConflict(err) {
				if err := d.k8sClient.Get(ctx, podKey, pod); err != nil {
					return false, err
				}
				if pod.UID != podUID {
					return false, nil // UID conflict -> pod is gone
				}
				continue
			}
			return false, err
		}

		// see https://github.com/kubernetes-sigs/controller-runtime/issues/1257
		err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
			if condition(pod) {
				return true, nil
			}
			if err := d.k8sClient.Get(ctx, podKey, pod); err != nil {
				return false, err
			}
			if pod.UID != podUID {
				return true, nil // UID conflict -> pod is gone
			}
			return false, nil
		})
		if err != nil {
			return false, err
		}
		return true, err
	}
}

// CleanupPreviousRun finds pods that are not deleted properly in the previous run, and reschedule them.
func (d *PodGracefulDrain) cleanupPreviousRun(ctx context.Context) error {
	podList := &corev1.PodList{}
	// select all pods regardless of its value. The pod was about to be deleted when its value is empty.
	if err := d.k8sClient.List(ctx, podList, client.HasLabels{waitLabelKey}); err != nil {
		return err
	}

	now := time.Now()
	for _, pod := range podList.Items {
		deleteAfter := d.config.PodGracefulDrainDelay
		deleteAt, err := getDeleteAtAnnotation(&pod)
		if err == nil {
			deleteAfter = deleteAt.Sub(now)
		}

		d.deleteAfter(&pod, deleteAfter)
	}
	return nil
}

func getDeleteAtAnnotation(pod *corev1.Pod) (time.Time, error) {
	value, ok := pod.Annotations[deleteAtAnnotationKey]
	if !ok {
		return time.Time{}, errors.New("unable to lookup deleteAt annotation")
	}
	deleteAt, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return deleteAt, nil
}

func (d *PodGracefulDrain) Start(stop <-chan struct{}) error {
	if d.config.PodGracefulDrainDelay == time.Duration(0) {
		return nil
	}

	d.logger.Info("Starting pod-graceful-drain")

	if err := d.cleanupPreviousRun(context.Background()); err != nil {
		d.logger.Error(err, "problem while cleaning up pods")
	}
	<-stop

	stopped := make(chan struct{}, 1)
	go func() {
		d.deleterWaitGroup.Wait()
		stopped <- struct{}{}
	}()

	select {
	case <-stopped:
		// pod drained all deleter goroutines in time.
	case <-time.After(d.config.PodGracefulDrainDelay):
		// I gave them enough time, but they haven't finished their job, so I signal them to hurry up
		close(d.deleterStopper)
		// and give them a little more time to cleanup.
		select {
		case <-stopped:
		case <-time.After(defaultDeleterGracefulTerminationPeriod):
		}
	}
	d.deleterCancelFunc()
	return nil
}
