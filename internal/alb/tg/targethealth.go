package tg

import (
	"context"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/backend"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/errors"
	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	SyncTargetsForReconcilation(ctx context.Context, t *Targets, desiredTargets []*elbv2.TargetDescription) error
	RemovePodConditions(ctx context.Context, t *Targets, targets []*elbv2.TargetDescription) error
	StopReconcilingPodConditionStatus(tgArn string)
}

// NewTargetHealthController constructs a new target health controller
func NewTargetHealthController(cloud aws.CloudAPI, endpointResolver backend.EndpointResolver, client client.Client) TargetHealthController {
	return &targetHealthController{
		cloud:            cloud,
		endpointResolver: endpointResolver,
		client:           client,
		tgWatches:        make(targetGroupWatches),
	}
}

type targetHealthController struct {
	cloud            aws.CloudAPI
	endpointResolver backend.EndpointResolver
	client           client.Client
	tgWatches        targetGroupWatches
	tgWatchesMux     sync.Mutex
}

// SyncTargetsForReconcilation starts a go routine for reconciling pod condition statuses for the given targets in the background until they are healthy in the target group
func (c *targetHealthController) SyncTargetsForReconcilation(ctx context.Context, t *Targets, desiredTargets []*elbv2.TargetDescription) error {
	conditionType := t.conditionType()

	targetsToReconcile, err := c.filterTargetsNeedingReconcilation(conditionType, t, desiredTargets)
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
			// not needed anymore
			tgWatch.cancel()
			delete(c.tgWatches, t.TgArn)
		} else {
			// update targets
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

// RemovePodConditions removes the condition (that we added before for the given ingress/service) from the given pods
func (c *targetHealthController) RemovePodConditions(ctx context.Context, t *Targets, targets []*elbv2.TargetDescription) error {
	pods, err := c.endpointResolver.ReverseResolve(t.Ingress, t.Backend, targets)
	if err != nil {
		return err
	}

	conditionType := t.conditionType()

	for _, pod := range pods {
		if pod == nil {
			continue
		}

		var conditions []api.PodCondition
		for _, condition := range pod.Status.Conditions {
			if condition.Type != conditionType {
				conditions = append(conditions, condition)
			}
		}

		if len(conditions) != len(pod.Status.Conditions) {
			pod.Status.Conditions = conditions
			err := c.client.Status().Update(ctx, pod)
			if err != nil && !k8serrors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

// Background loop which keeps reconciling pod condition statuses for the given target groups until the given context is cancelled.
func (c *targetHealthController) reconcilePodConditionsLoop(ctx context.Context, tgArn string, conditionType api.PodConditionType, tgWatch *targetGroupWatch) {
	logger := albctx.GetLogger(ctx)
	logger.Infof("Starting reconcilation of pod condition status for target group: %v", tgArn)
	for {
		tgWatch.mux.Lock()
		interval := int64(10)
		annot, err := parser.GetInt64Annotation("target-health-reconcilation-interval-seconds", tgWatch.ingress)
		if err != nil && annot != nil {
			interval = *annot
		}
		ingress := tgWatch.ingress
		backend := tgWatch.backend
		targetsToReconcile := append([]*elbv2.TargetDescription{}, tgWatch.targetsToReconcile...) // make copy
		tgWatch.mux.Unlock()

		select {
		case <-time.After(time.Duration(interval) * time.Second):
			if err := c.reconcilePodConditions(ctx, tgArn, conditionType, ingress, backend, targetsToReconcile); err != nil {
				logger.Errorf("Failed to reconcile pod condition status: %v", err)
				albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", "Error reconciling pod condition status via target group %s: %s", tgArn, err.Error())
			}
		case <-ctx.Done():
			logger.Infof("Stopping reconcilation of pod condition status for target group: %v", tgArn)
			return
		}
	}
}

// For each given pod, checks for the health status of the corresponding target in the target group and adds/updates a pod condition that can be used for pod readiness gates.
func (c *targetHealthController) reconcilePodConditions(ctx context.Context, tgArn string, conditionType api.PodConditionType, ingress *extensions.Ingress, backend *extensions.IngressBackend, targetsToReconcile []*elbv2.TargetDescription) error {
	// fetch current target health
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
		targetHealth := targetsHealth[pod.Status.PodIP]
		if targetHealth == nil {
			continue
		}

		// check if pod has readiness gate for this ingress / service
		found := false
		for _, rg := range pod.Spec.ReadinessGates {
			if rg.ConditionType == conditionType {
				found = true
				break
			}
		}
		if !found {
			return nil
		}

		if err := c.reconcilePodCondition(ctx, conditionType, pod, targetHealth, true); err != nil {
			return err
		}
	}
	return nil
}

// Creates or updates the condition status for the given pod with the given target health.
func (c *targetHealthController) reconcilePodCondition(ctx context.Context, conditionType api.PodConditionType, pod *api.Pod, targetHealth *elbv2.TargetHealth, updateTimes bool) error {
	// translate target health into pod condition status
	var conditionStatus api.ConditionStatus
	switch *targetHealth.State {
	case elbv2.TargetHealthStateEnumHealthy:
		conditionStatus = api.ConditionTrue
	case elbv2.TargetHealthStateEnumUnhealthy, elbv2.TargetHealthStateEnumInitial, elbv2.TargetHealthStateEnumDraining:
		conditionStatus = api.ConditionFalse
	default:
		conditionStatus = api.ConditionUnknown
	}

	// check if condition already exists
	found := false
	now := metav1.Now()
	for i, cond := range pod.Status.Conditions {
		if cond.Type == conditionType {
			if updateTimes {
				cond.LastProbeTime = now
				if cond.Status != conditionStatus {
					cond.LastTransitionTime = now
				}
			}
			cond.Status = conditionStatus
			cond.Reason = aws.StringValue(targetHealth.Reason)
			pod.Status.Conditions[i] = cond
			found = true
			break
		}
	}
	if !found {
		targetHealthCondition := api.PodCondition{
			Type:   conditionType,
			Status: conditionStatus,
			Reason: aws.StringValue(targetHealth.Reason),
		}
		if updateTimes {
			targetHealthCondition.LastProbeTime = now
			targetHealthCondition.LastTransitionTime = now
		}
		pod.Status.Conditions = append(pod.Status.Conditions, targetHealthCondition)
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
func (c *targetHealthController) filterTargetsNeedingReconcilation(conditionType api.PodConditionType, t *Targets, desiredTargets []*elbv2.TargetDescription) ([]*elbv2.TargetDescription, error) {
	targetsToReconcile := []*elbv2.TargetDescription{}
	if len(desiredTargets) == 0 {
		return targetsToReconcile, nil
	}

	// reconcilation strategy; valid options:
	// * initial: stop reconciling pod condition status after it has been set to `True`; will be reset when pod gets deregistered from ALB
	// * continous: keep reconciling pod condition status while the target group exists
	strategy := "initial"
	annot, err := parser.GetStringAnnotation("target-health-reconcilation-strategy", t.Ingress)
	if err != nil && !errors.IsMissingAnnotations(err) {
		return targetsToReconcile, err
	}
	if annot != nil {
		strategy = *annot
	}

	// find the pods that correspond to the targets
	pods, err := c.endpointResolver.ReverseResolve(t.Ingress, t.Backend, desiredTargets)
	if err != nil {
		return targetsToReconcile, err
	}

	// filter out targets whose pods don't have the `readinessGate` for this target group or whose pod condition status is already `True`
	for i, target := range desiredTargets {
		pod := pods[i]
		if pod == nil {
			continue
		}

		// check if pod has readiness gate for this ingress / service
		found := false
		for _, rg := range pod.Spec.ReadinessGates {
			if rg.ConditionType == conditionType {
				found = true
				break
			}
		}
		if !found {
			continue
		}

		needsReconcilation := true
		if strategy == "initial" {
			for _, condition := range pod.Status.Conditions {
				if condition.Type == conditionType {
					needsReconcilation = condition.Status != "True"
					break
				}
			}
		}

		if needsReconcilation {
			targetsToReconcile = append(targetsToReconcile, target)
		}
	}

	return targetsToReconcile, nil
}
