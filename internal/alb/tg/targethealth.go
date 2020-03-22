package tg

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/healthcheck"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/backend"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"
	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type targetGroupWatch struct {
	// ingress is the ingress for the target group
	ingress *extensions.Ingress

	// backend is the ingress backend for the target group
	backend *extensions.IngressBackend

	// channels used to communicate the reconiliation interval and list of targets to reconcile into the go routine
	interval           chan int64
	targetsToReconcile chan []*elbv2.TargetDescription
	cancel             context.CancelFunc
}

type targetGroupWatches map[string]*targetGroupWatch

func newTargetGroupWatch(ctx context.Context, ingress *extensions.Ingress, backend *extensions.IngressBackend) (*targetGroupWatch, context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	return &targetGroupWatch{
		ingress:            ingress,
		backend:            backend,
		interval:           make(chan int64),
		targetsToReconcile: make(chan []*elbv2.TargetDescription),
		cancel:             cancel,
	}, ctx
}

// TargetHealthController provides functionality to reconcile pod condition status from target health of targets in a target group
type TargetHealthController interface {
	// Reconcile ensures the target group targets in AWS matches the targets configured in the ingress backend.
	SyncTargetsForReconciliation(ctx context.Context, t *Targets, desiredTargets []*elbv2.TargetDescription) error
	RemovePodConditions(ctx context.Context, t *Targets, targets []*elbv2.TargetDescription) error
	StopReconcilingPodConditionStatus(tgArn string)
}

// NewTargetHealthController constructs a new target health controller
func NewTargetHealthController(cloud aws.CloudAPI, store store.Storer, endpointResolver backend.EndpointResolver, client client.Client) TargetHealthController {
	return &targetHealthController{
		cloud:            cloud,
		store:            store,
		endpointResolver: endpointResolver,
		client:           client,
		tgWatches:        make(targetGroupWatches),
	}
}

type targetHealthController struct {
	cloud            aws.CloudAPI
	store            store.Storer
	endpointResolver backend.EndpointResolver
	client           client.Client
	tgWatches        targetGroupWatches
}

// SyncTargetsForReconciliation starts a go routine for reconciling pod condition statuses for the given targets in the background until they are healthy in the target group
func (c *targetHealthController) SyncTargetsForReconciliation(ctx context.Context, t *Targets, desiredTargets []*elbv2.TargetDescription) error {
	conditionType := podConditionTypeForIngressBackend(t.Ingress, t.Backend)

	targetsToReconcile, err := c.filterTargetsNeedingReconciliation(conditionType, t, desiredTargets)
	if err != nil {
		return err
	}

	// create, update or remove targetGroupWatch for this target group;
	// a targetGroupWatch exists as long as there are targets in the target group whose pod condition statuses need to be reconciled;
	// while the targetGroupWatch exists, a go routine regularly monitors the target health of the targets in the target group and updates the pod condition status for the corresponding pods
	tgWatch, ok := c.tgWatches[t.TgArn]
	if ok {
		if len(targetsToReconcile) == 0 {
			tgWatch.cancel()
			delete(c.tgWatches, t.TgArn)
			return nil
		}
	} else {
		if len(targetsToReconcile) == 0 {
			return nil
		}

		// targetGroupWatch for this target group doesn't exist yet -> create it
		tgWatch, ctx = newTargetGroupWatch(ctx, t.Ingress, t.Backend)
		c.tgWatches[t.TgArn] = tgWatch

		// start watching target health in target group and updating pod condition status
		go c.reconcilePodConditionsLoop(ctx, t.TgArn, conditionType, tgWatch)
	}
	tgWatch.interval <- c.ingressTargetHealthReconciliationInterval(t.Backend.ServiceName, t.Ingress)
	tgWatch.targetsToReconcile <- targetsToReconcile

	return nil
}

// StopReconcilingPodConditionStatus stops a running go routine (if there is any) which was started to reconcile pod condition statuses in the background for a specific target group
func (c *targetHealthController) StopReconcilingPodConditionStatus(tgArn string) {
	tgWatch, ok := c.tgWatches[tgArn]
	if ok {
		tgWatch.cancel()
		delete(c.tgWatches, tgArn)
	}
}

// RemovePodConditions removes the condition (that we added before for the given ingress/backend) from the given pods
func (c *targetHealthController) RemovePodConditions(ctx context.Context, t *Targets, targets []*elbv2.TargetDescription) error {
	pods, err := c.endpointResolver.ReverseResolve(t.Ingress, t.Backend, targets)
	if err != nil {
		return err
	}

	conditionType := podConditionTypeForIngressBackend(t.Ingress, t.Backend)

	for _, pod := range pods {
		if pod != nil {
			if i, cond := podConditionForReadinessGate(pod, conditionType); cond != nil {
				pod.Status.Conditions = append(pod.Status.Conditions[:i], pod.Status.Conditions[i+1:]...)
				err := c.client.Status().Update(ctx, pod)
				if err != nil && !k8serrors.IsNotFound(err) {
					return err
				}
			}
		}
	}

	return nil
}

// Background loop which keeps reconciling pod condition statuses for the given target groups until the given context is cancelled.
func (c *targetHealthController) reconcilePodConditionsLoop(ctx context.Context, tgArn string, conditionType api.PodConditionType, tgWatch *targetGroupWatch) {
	logger := albctx.GetLogger(ctx)
	logger.Infof("Starting reconciliation of pod condition status for target group: %v", tgArn)

	var interval int64
	var targetsToReconcile []*elbv2.TargetDescription

	for {
		var reconcile <-chan time.Time
		if len(targetsToReconcile) > 0 && interval > 0 { // only reconcile if we have at least one target
			reconcile = time.After(time.Duration(interval) * time.Second)
		}

		select {
		case interval = <-tgWatch.interval: // update interval
		case targetsToReconcile = <-tgWatch.targetsToReconcile: // update targets

		case <-reconcile:
			notReadyTargets, err := c.reconcilePodConditions(ctx, tgArn, conditionType, tgWatch.ingress, tgWatch.backend, targetsToReconcile)
			if err == nil {
				targetsToReconcile = notReadyTargets
			} else {
				logger.Errorf("Failed to reconcile pod condition status: %v", err)
				albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", "Error reconciling pod condition status via target group %s: %s", tgArn, err.Error())
			}

		case <-ctx.Done():
			logger.Infof("Stopping reconciliation of pod condition status for target group: %v", tgArn)
			return
		}
	}

}

// For each given pod, checks for the health status of the corresponding target in the target group and adds/updates a pod condition that can be used for pod readiness gates.
func (c *targetHealthController) reconcilePodConditions(ctx context.Context, tgArn string, conditionType api.PodConditionType, ingress *extensions.Ingress, backend *extensions.IngressBackend, targetsToReconcile []*elbv2.TargetDescription) ([]*elbv2.TargetDescription, error) {
	var notReadyTargets []*elbv2.TargetDescription

	in := &elbv2.DescribeTargetHealthInput{
		TargetGroupArn: aws.String(tgArn),
		Targets:        targetsToReconcile,
	}
	resp, err := c.cloud.DescribeTargetHealthWithContext(ctx, in)
	if err != nil {
		return notReadyTargets, err
	}
	targetsHealth := map[string]*elbv2.TargetHealth{}
	for _, desc := range resp.TargetHealthDescriptions {
		targetsHealth[*desc.Target.Id] = desc.TargetHealth
	}

	pods, err := c.endpointResolver.ReverseResolve(ingress, backend, targetsToReconcile)
	if err != nil {
		return notReadyTargets, err
	}

	for i, target := range targetsToReconcile {
		pod := pods[i]
		if pod == nil {
			continue
		}
		targetHealth, ok := targetsHealth[pod.Status.PodIP]
		if ok && podHasReadinessGate(pod, conditionType) {
			if aws.StringValue(targetHealth.State) != elbv2.TargetHealthStateEnumHealthy {
				notReadyTargets = append(notReadyTargets, target)
			}
			if err := c.reconcilePodCondition(ctx, conditionType, pod, targetHealth, true); err != nil {
				return notReadyTargets, err
			}
		}
	}
	return notReadyTargets, nil
}

// Creates or updates the condition status for the given pod with the given target health.
func (c *targetHealthController) reconcilePodCondition(ctx context.Context, conditionType api.PodConditionType, pod *api.Pod, targetHealth *elbv2.TargetHealth, updateTimes bool) error {
	conditionStatus := podConditionStatusFromTargetHealth(targetHealth)

	// check if condition already exists
	now := metav1.Now()
	i, cond := podConditionForReadinessGate(pod, conditionType)
	if cond == nil {
		// new condition
		targetHealthCondition := api.PodCondition{
			Type:    conditionType,
			Status:  conditionStatus,
			Reason:  aws.StringValue(targetHealth.Reason),
			Message: aws.StringValue(targetHealth.Description),
		}
		if updateTimes {
			targetHealthCondition.LastProbeTime = now
			targetHealthCondition.LastTransitionTime = now
		}
		pod.Status.Conditions = append(pod.Status.Conditions, targetHealthCondition)
	} else {
		// update condition
		if updateTimes {
			cond.LastProbeTime = now
			if cond.Status != conditionStatus {
				cond.LastTransitionTime = now
			}
		}
		cond.Status = conditionStatus
		cond.Reason = aws.StringValue(targetHealth.Reason)
		cond.Message = aws.StringValue(targetHealth.Description)
		pod.Status.Conditions[i] = *cond
	}

	// pod will always be updated (at least to update `LastProbeTime`);
	// this will trigger another invocation of `Reconcile`, which will remove this pod from the list of pods to reconcile if its health status is ok
	err := c.client.Status().Update(ctx, pod)
	if err != nil {
		return err
	}

	return nil
}

// From the given targets, only returns the ones that have a readiness gate for the given ingress/service and whose pod conditions actually need to be reconciled.
func (c *targetHealthController) filterTargetsNeedingReconciliation(conditionType api.PodConditionType, t *Targets, desiredTargets []*elbv2.TargetDescription) ([]*elbv2.TargetDescription, error) {
	targetsToReconcile := []*elbv2.TargetDescription{}
	if len(desiredTargets) == 0 {
		return targetsToReconcile, nil
	}

	// find the pods that correspond to the targets
	pods, err := c.endpointResolver.ReverseResolve(t.Ingress, t.Backend, desiredTargets)
	if err != nil {
		return targetsToReconcile, err
	}

	// filter out targets whose pods don't have the `readinessGate` for this target group or whose pod condition status is already `True`
	for i, target := range desiredTargets {
		pod := pods[i]
		if pod != nil && podHasReadinessGate(pod, conditionType) {
			if _, cond := podConditionForReadinessGate(pod, conditionType); cond == nil || cond.Status != api.ConditionTrue {
				targetsToReconcile = append(targetsToReconcile, target)
			}
		}
	}

	return targetsToReconcile, nil
}

func (c *targetHealthController) ingressTargetHealthReconciliationInterval(serviceName string, ingress *extensions.Ingress) int64 {
	ingressAnnos, err := c.store.GetIngressAnnotations(k8s.MetaNamespaceKey(ingress))
	if err == nil {
		serviceKey := types.NamespacedName{Namespace: ingress.Namespace, Name: serviceName}
		serviceAnnos, err := c.store.GetServiceAnnotations(serviceKey.String(), ingressAnnos)
		if err == nil {
			return *serviceAnnos.HealthCheck.IntervalSeconds
		}
	}

	return healthcheck.DefaultIntervalSeconds
}

// PodConditionTypeForIngressBackend returns the PodConditionType that is associated with the given ingress and backend
func podConditionTypeForIngressBackend(ingress *extensions.Ingress, backend *extensions.IngressBackend) api.PodConditionType {
	return api.PodConditionType(fmt.Sprintf(
		"target-health.alb.ingress.k8s.aws/%s_%s_%s",
		ingress.Name,
		backend.ServiceName,
		backend.ServicePort.String(),
	))
}

// PodHasReadinessGate returns true if the given pod has a readinessGate with the given conditionType
func podHasReadinessGate(pod *api.Pod, conditionType api.PodConditionType) bool {
	for _, rg := range pod.Spec.ReadinessGates {
		if rg.ConditionType == conditionType {
			return true
		}
	}
	return false
}

func podConditionForReadinessGate(pod *api.Pod, conditionType api.PodConditionType) (int, *api.PodCondition) {
	for i, condition := range pod.Status.Conditions {
		if condition.Type == conditionType {
			return i, &condition
		}
	}
	return -1, nil
}

func podConditionStatusFromTargetHealth(targetHealth *elbv2.TargetHealth) api.ConditionStatus {
	switch *targetHealth.State {
	case elbv2.TargetHealthStateEnumHealthy:
		return api.ConditionTrue
	case elbv2.TargetHealthStateEnumUnhealthy, elbv2.TargetHealthStateEnumInitial, elbv2.TargetHealthStateEnumDraining:
		return api.ConditionFalse
	default:
		return api.ConditionUnknown
	}
}
