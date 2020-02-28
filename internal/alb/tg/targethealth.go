package tg

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/healthcheck"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/targetgroup"
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

	// podsToReconcile is the list of pods whose conditions have not been reconciled yet
	targetsToReconcile []*elbv2.TargetDescription

	cancel context.CancelFunc
	mux    sync.Mutex
}
type targetGroupWatches map[string]*targetGroupWatch

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
	tgWatchesMux     sync.Mutex
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
	c.tgWatchesMux.Lock()
	defer c.tgWatchesMux.Unlock()

	tgWatch, ok := c.tgWatches[t.TgArn]
	if ok {
		// targetGroupWatch for this target group already exists
		if len(targetsToReconcile) == 0 {
			tgWatch.cancel()
			delete(c.tgWatches, t.TgArn)
		} else {
			tgWatch.mux.Lock()
			tgWatch.ingress = t.Ingress
			tgWatch.targetsToReconcile = targetsToReconcile
			tgWatch.mux.Unlock()
		}
	} else {
		if len(targetsToReconcile) == 0 {
			return nil
		}

		// targetGroupWatch for this target group doesn't exist yet -> create it
		ctx, cancel := context.WithCancel(ctx)
		tgWatch = &targetGroupWatch{
			ingress:            t.Ingress,
			backend:            t.Backend,
			targetsToReconcile: targetsToReconcile,
			cancel:             cancel,
		}
		c.tgWatches[t.TgArn] = tgWatch

		// start watching target health in target group and updating pod condition status
		go c.reconcilePodConditionsLoop(ctx, t.TgArn, conditionType, tgWatch)
	}

	return nil
}

// StopReconcilingPodConditionStatus stops a running go routine (if there is any) which was started to reconcile pod condition statuses in the background for a specific target group
func (c *targetHealthController) StopReconcilingPodConditionStatus(tgArn string) {
	c.tgWatchesMux.Lock()
	defer c.tgWatchesMux.Unlock()

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

	initial := true
	for {
		tgWatch.mux.Lock()
		interval := c.ingressTargetHealthReconciliationInterval(initial, tgWatch.backend.ServiceName, tgWatch.ingress)
		ingress := tgWatch.ingress
		backend := tgWatch.backend
		targetsToReconcile := append([]*elbv2.TargetDescription{}, tgWatch.targetsToReconcile...) // make copy
		tgWatch.mux.Unlock()

		initial = false

		select {
		case <-time.After(time.Duration(interval) * time.Second):
			if err := c.reconcilePodConditions(ctx, tgArn, conditionType, ingress, backend, targetsToReconcile); err != nil {
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
func (c *targetHealthController) reconcilePodConditions(ctx context.Context, tgArn string, conditionType api.PodConditionType, ingress *extensions.Ingress, backend *extensions.IngressBackend, targetsToReconcile []*elbv2.TargetDescription) error {
	in := &elbv2.DescribeTargetHealthInput{TargetGroupArn: aws.String(tgArn)}
	resp, err := c.cloud.DescribeTargetHealthWithContext(ctx, in)
	if err != nil {
		return err
	}
	targetsHealth := map[string]*elbv2.TargetHealth{}
	for _, desc := range resp.TargetHealthDescriptions {
		targetsHealth[*desc.Target.Id] = desc.TargetHealth
	}

	pods, err := c.endpointResolver.ReverseResolve(ingress, backend, targetsToReconcile)
	if err != nil {
		return err
	}

	for _, pod := range pods {
		targetHealth, ok := targetsHealth[pod.Status.PodIP]
		if ok && podHasReadinessGate(pod, conditionType) {
			if err := c.reconcilePodCondition(ctx, conditionType, pod, targetHealth, true); err != nil {
				return err
			}
		}
	}
	return nil
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
			Type:   conditionType,
			Status: conditionStatus,
			Reason: podConditionReason(targetHealth),
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
		cond.Reason = podConditionReason(targetHealth)
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

func (c *targetHealthController) ingressTargetHealthReconciliationInterval(initial bool, serviceName string, ingress *extensions.Ingress) int64 {
	ingressAnnos, err := c.store.GetIngressAnnotations(k8s.MetaNamespaceKey(ingress))
	if err == nil {
		serviceKey := types.NamespacedName{Namespace: ingress.Namespace, Name: serviceName}
		serviceAnnos, err := c.store.GetServiceAnnotations(serviceKey.String(), ingressAnnos)
		if err == nil {
			if initial {
				return *serviceAnnos.HealthCheck.IntervalSeconds * *serviceAnnos.TargetGroup.HealthyThresholdCount
			}
			return *serviceAnnos.HealthCheck.IntervalSeconds
		}
	}

	if initial {
		return healthcheck.DefaultIntervalSeconds * targetgroup.DefaultHealthyThresholdCount
	}
	return healthcheck.DefaultIntervalSeconds
}

// PodConditionTypeForIngressBackend returns the PodConditionType that is associated with the given ingress and backend
func podConditionTypeForIngressBackend(ingress *extensions.Ingress, backend *extensions.IngressBackend) api.PodConditionType {
	return api.PodConditionType(fmt.Sprintf(
		"target-health.%s/%s_%s_%s",
		parser.AnnotationsPrefix,
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

func podConditionReason(targetHealth *elbv2.TargetHealth) string {
	if targetHealth.Reason != nil {
		if targetHealth.Description != nil {
			return fmt.Sprintf("%s: %s", aws.StringValue(targetHealth.Reason), aws.StringValue(targetHealth.Description))
		}
		return aws.StringValue(targetHealth.Reason)
	}
	return ""
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
