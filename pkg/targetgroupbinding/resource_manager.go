package targetgroupbinding

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/cache"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/smithy-go"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/backend"
	ctrlerrors "sigs.k8s.io/aws-load-balancer-controller/pkg/error"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultRequeueDuration = 15 * time.Second
	invalidVPCTTL          = 60 * time.Minute
)
const (
	controllerName = "targetGroupBinding"
)

// ResourceManager manages the TargetGroupBinding resource.
type ResourceManager interface {
	Reconcile(ctx context.Context, tgb *elbv2api.TargetGroupBinding) (bool, error)
	Cleanup(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error
}

// NewDefaultResourceManager constructs new defaultResourceManager.
func NewDefaultResourceManager(k8sClient client.Client, elbv2Client services.ELBV2,
	podInfoRepo k8s.PodInfoRepo, networkingManager networking.NetworkingManager,
	vpcInfoProvider networking.VPCInfoProvider, multiClusterManager MultiClusterManager, metricsCollector lbcmetrics.MetricCollector,
	vpcID string, failOpenEnabled bool, endpointSliceEnabled bool,
	eventRecorder record.EventRecorder, logger logr.Logger, maxTargetsPerInstance int) *defaultResourceManager {

	targetsManager := NewCachedTargetsManager(elbv2Client, logger)
	endpointResolver := backend.NewDefaultEndpointResolver(k8sClient, podInfoRepo, failOpenEnabled, endpointSliceEnabled, logger)
	return &defaultResourceManager{
		k8sClient:             k8sClient,
		targetsManager:        targetsManager,
		endpointResolver:      endpointResolver,
		networkingManager:     networkingManager,
		eventRecorder:         eventRecorder,
		logger:                logger,
		vpcID:                 vpcID,
		vpcInfoProvider:       vpcInfoProvider,
		podInfoRepo:           podInfoRepo,
		maxTargetsPerInstance: maxTargetsPerInstance,
		multiClusterManager:   multiClusterManager,
		metricsCollector:      metricsCollector,

		invalidVpcCache:    cache.NewExpiring(),
		invalidVpcCacheTTL: defaultTargetsCacheTTL,

		requeueDuration: defaultRequeueDuration,
	}
}

var _ ResourceManager = &defaultResourceManager{}

// default implementation for ResourceManager.
type defaultResourceManager struct {
	k8sClient             client.Client
	targetsManager        TargetsManager
	endpointResolver      backend.EndpointResolver
	networkingManager     networking.NetworkingManager
	eventRecorder         record.EventRecorder
	logger                logr.Logger
	vpcInfoProvider       networking.VPCInfoProvider
	podInfoRepo           k8s.PodInfoRepo
	maxTargetsPerInstance int
	multiClusterManager   MultiClusterManager
	metricsCollector      lbcmetrics.MetricCollector
	vpcID                 string

	invalidVpcCache      *cache.Expiring
	invalidVpcCacheTTL   time.Duration
	invalidVpcCacheMutex sync.RWMutex

	requeueDuration time.Duration
}

func (m *defaultResourceManager) Reconcile(ctx context.Context, tgb *elbv2api.TargetGroupBinding) (bool, error) {
	if tgb.Spec.TargetType == nil {
		return false, errors.Errorf("targetType is not specified: %v", k8s.NamespacedName(tgb).String())
	}

	var newCheckPoint string
	var oldCheckPoint string
	var isDeferred bool
	var err error

	if *tgb.Spec.TargetType == elbv2api.TargetTypeIP {
		newCheckPoint, oldCheckPoint, isDeferred, err = m.reconcileWithIPTargetType(ctx, tgb)
	} else {
		newCheckPoint, oldCheckPoint, isDeferred, err = m.reconcileWithInstanceTargetType(ctx, tgb)
	}

	if err != nil {
		return false, err
	}

	if isDeferred {
		return true, nil
	}

	return false, m.updateTGBCheckPoint(ctx, tgb, newCheckPoint, oldCheckPoint)
}

func (m *defaultResourceManager) Cleanup(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	if err := m.cleanupTargets(ctx, tgb); err != nil {
		return err
	}

	if err := m.multiClusterManager.CleanUp(ctx, tgb); err != nil {
		return err
	}

	if err := m.networkingManager.Cleanup(ctx, tgb); err != nil {
		return err
	}
	if err := m.updatePodAsHealthyForDeletedTGB(ctx, tgb); err != nil {
		return err
	}

	return nil
}

func (m *defaultResourceManager) reconcileWithIPTargetType(ctx context.Context, tgb *elbv2api.TargetGroupBinding) (string, string, bool, error) {
	tgbScopedLogger := m.logger.WithValues("tgb", k8s.NamespacedName(tgb))
	svcKey := buildServiceReferenceKey(tgb, tgb.Spec.ServiceRef)

	targetHealthCondType := BuildTargetHealthPodConditionType(tgb)
	resolveOpts := []backend.EndpointResolveOption{
		backend.WithPodReadinessGate(targetHealthCondType),
	}

	var endpoints []backend.PodEndpoint
	var containsPotentialReadyEndpoints bool
	var err error

	oldCheckPoint := GetTGBReconcileCheckpoint(tgb)

	endpoints, containsPotentialReadyEndpoints, err = m.endpointResolver.ResolvePodEndpoints(ctx, svcKey, tgb.Spec.ServiceRef.Port, resolveOpts...)

	if err != nil {
		if errors.Is(err, backend.ErrNotFound) {
			m.eventRecorder.Event(tgb, corev1.EventTypeWarning, k8s.TargetGroupBindingEventReasonBackendNotFound, err.Error())
			return "", oldCheckPoint, false, m.Cleanup(ctx, tgb)
		}
		return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "resolve_pod_endpoints_error", err, m.metricsCollector)
	}

	newCheckPoint, err := calculateTGBReconcileCheckpoint(endpoints, tgb)

	if err != nil {
		return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "calculate_tgb_reconcile_checkpoint_error", err, m.metricsCollector)
	}

	if !containsPotentialReadyEndpoints && oldCheckPoint == newCheckPoint {
		tgbScopedLogger.Info("Skipping targetgroupbinding reconcile", "calculated hash", newCheckPoint)
		return newCheckPoint, oldCheckPoint, true, nil
	}

	targets, err := m.targetsManager.ListTargets(ctx, tgb)
	if err != nil {
		return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "list_targets_error", err, m.metricsCollector)
	}
	totalTargets := len(targets)

	notDrainingTargets, _ := partitionTargetsByDrainingStatus(targets)
	matchedEndpointAndTargets, unmatchedEndpoints, unmatchedTargets := matchPodEndpointWithTargets(endpoints, notDrainingTargets)

	needNetworkingRequeue := false
	if err := m.networkingManager.ReconcileForPodEndpoints(ctx, tgb, endpoints); err != nil {
		tgbScopedLogger.Error(err, "Requesting network requeue due to error from ReconcileForPodEndpoints")
		m.eventRecorder.Event(tgb, corev1.EventTypeWarning, k8s.TargetGroupBindingEventReasonFailedNetworkReconcile, err.Error())
		needNetworkingRequeue = true
	}

	preflightNeedFurtherProbe := false
	for _, endpointAndTarget := range matchedEndpointAndTargets {
		_, localPreflight := m.calculateReadinessGateTransition(endpointAndTarget.endpoint.Pod, targetHealthCondType, endpointAndTarget.target.TargetHealth)
		if localPreflight {
			preflightNeedFurtherProbe = true
			break
		}
	}

	// Any change that we perform should reset the checkpoint.
	// TODO - How to make this cleaner?
	if len(unmatchedEndpoints) > 0 || len(unmatchedTargets) > 0 || needNetworkingRequeue || containsPotentialReadyEndpoints || preflightNeedFurtherProbe {
		// Set to an empty checkpoint, to ensure that no matter what we try to reconcile atleast one more time.
		// Consider this ordering of events (without using this method of overriding the checkpoint)
		// 1. Register some pod IP, don't update TGB checkpoint.
		// 2. Before next invocation of reconcile happens, the pod is removed.
		// 3. The next reconcile loop has no knowledge that it needs to deregister the pod ip, therefore it skips deregistering the removed pod ip.
		err = m.updateTGBCheckPoint(ctx, tgb, "", oldCheckPoint)
		if err != nil {
			tgbScopedLogger.Error(err, "Unable to update checkpoint before mutating change")
			return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "update_tgb_checkpoint_error", err, m.metricsCollector)
		}
	}

	updateTrackedTargets := false
	if len(unmatchedTargets) > 0 {
		updateTrackedTargets, err = m.deregisterTargets(ctx, tgb, unmatchedTargets)
		if err != nil {
			return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "deregister_targets_error", err, m.metricsCollector)
		}
	}

	if len(unmatchedEndpoints) > 0 {
		// In order to support multicluster tgb, we have to write the endpoint map _before_ calling register.
		// By only writing the map when registerPodEndpoints() completes, we could leak targets when
		// registerPodEndpoints() fails however the registration does happen. The specific example is:
		// The ELB API succeeds in registering the targets, however the response isn't returned to us
		// (perhaps the network dropped the response). If this happens and the pod is terminated before
		// the next reconcile then we would leak the target as it would not exist in our endpoint map.

		// We don't want to duplicate write calls, so if we are doing target registration and deregistration
		// in the same reconcile loop, then we can de-dupe these tracking calls. As the tracked targets are used
		// for deregistration, it's safe to update the map here as we have completed all deregister calls already.
		updateTrackedTargets = false

		if err := m.multiClusterManager.UpdateTrackedIPTargets(ctx, true, endpoints, tgb); err != nil {
			return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "update_tracked_ip_targets_error", err, m.metricsCollector)
		}

		eligibleTargetsCount := m.getMaxNewTargets(len(unmatchedEndpoints), totalTargets, tgbScopedLogger)
		unmatchedEndpoints = unmatchedEndpoints[:eligibleTargetsCount]

		if err := m.registerPodEndpoints(ctx, tgb, unmatchedEndpoints); err != nil {
			return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "register_pod_endpoint_error", err, m.metricsCollector)
		}
	}

	if err := m.multiClusterManager.UpdateTrackedIPTargets(ctx, updateTrackedTargets, endpoints, tgb); err != nil {
		return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "update_tracked_ip_targets_error", err, m.metricsCollector)
	}

	anyPodNeedFurtherProbe, err := m.updateTargetHealthPodCondition(ctx, targetHealthCondType, matchedEndpointAndTargets, unmatchedEndpoints, tgb)
	if err != nil {
		return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "update_target_health_pod_condition_error", err, m.metricsCollector)
	}

	if anyPodNeedFurtherProbe {
		tgbScopedLogger.Info("Requeue for target monitor target health")
		return "", "", false, ctrlerrors.NewRequeueNeededAfter("monitor targetHealth", m.requeueDuration)
	}

	if containsPotentialReadyEndpoints {
		tgbScopedLogger.Info("Requeue for potentially ready endpoints")
		return "", "", false, ctrlerrors.NewRequeueNeededAfter("monitor potential ready endpoints", m.requeueDuration)
	}

	if needNetworkingRequeue {
		tgbScopedLogger.Info("Requeue for networking requeue")
		return "", "", false, ctrlerrors.NewRequeueNeededAfter("networking reconciliation", m.requeueDuration)
	}

	tgbScopedLogger.Info("Successful reconcile", "checkpoint", newCheckPoint)
	return newCheckPoint, oldCheckPoint, false, nil
}

func (m *defaultResourceManager) reconcileWithInstanceTargetType(ctx context.Context, tgb *elbv2api.TargetGroupBinding) (string, string, bool, error) {
	tgbScopedLogger := m.logger.WithValues("tgb", k8s.NamespacedName(tgb))
	svcKey := buildServiceReferenceKey(tgb, tgb.Spec.ServiceRef)
	nodeSelector, err := backend.GetTrafficProxyNodeSelector(tgb)
	if err != nil {
		return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "get_traffic_proxy_node_selector_error", err, m.metricsCollector)
	}

	oldCheckPoint := GetTGBReconcileCheckpoint(tgb)
	resolveOpts := []backend.EndpointResolveOption{backend.WithNodeSelector(nodeSelector)}
	endpoints, err := m.endpointResolver.ResolveNodePortEndpoints(ctx, svcKey, tgb.Spec.ServiceRef.Port, resolveOpts...)
	if err != nil {
		if errors.Is(err, backend.ErrNotFound) {
			m.eventRecorder.Event(tgb, corev1.EventTypeWarning, k8s.TargetGroupBindingEventReasonBackendNotFound, err.Error())
			return "", oldCheckPoint, false, m.Cleanup(ctx, tgb)
		}
		return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "resolve_nodeport_endpoints_error", err, m.metricsCollector)
	}

	newCheckPoint, err := calculateTGBReconcileCheckpoint(endpoints, tgb)

	if err != nil {
		return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "calculate_tgb_reconcile_checkpoint_error", err, m.metricsCollector)
	}

	if newCheckPoint == oldCheckPoint {
		tgbScopedLogger.Info("Skipping targetgroupbinding reconcile", "calculated hash", newCheckPoint)
		return newCheckPoint, oldCheckPoint, true, nil
	}

	targets, err := m.targetsManager.ListTargets(ctx, tgb)
	if err != nil {
		return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "list_targets_error", err, m.metricsCollector)
	}
	totalTargets := len(targets)

	notDrainingTargets, _ := partitionTargetsByDrainingStatus(targets)

	_, unmatchedEndpoints, unmatchedTargets := matchNodePortEndpointWithTargets(endpoints, notDrainingTargets)

	if err := m.networkingManager.ReconcileForNodePortEndpoints(ctx, tgb, endpoints); err != nil {
		tgbScopedLogger.Error(err, "Requesting network requeue due to error from ReconcileForNodePortEndpoints")
		return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "reconcile_nodeport_endpoints_error", err, m.metricsCollector)
	}

	if len(unmatchedEndpoints) > 0 || len(unmatchedTargets) > 0 {
		// Same thought process, see the IP target registration code as to why we clear out the check point.
		err = m.updateTGBCheckPoint(ctx, tgb, "", oldCheckPoint)
		if err != nil {
			tgbScopedLogger.Error(err, "Unable to update checkpoint before mutating change")
			return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "update_tgb_checkpoint_error", err, m.metricsCollector)
		}
	}

	updateTrackedTargets := false

	if len(unmatchedTargets) > 0 {
		updateTrackedTargets, err = m.deregisterTargets(ctx, tgb, unmatchedTargets)
		if err != nil {
			return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "deregister_targets_error", err, m.metricsCollector)
		}
	}

	if len(unmatchedEndpoints) > 0 {
		updateTrackedTargets = false
		if err := m.multiClusterManager.UpdateTrackedInstanceTargets(ctx, true, endpoints, tgb); err != nil {
			return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "update_tracked_instance_targets_error", err, m.metricsCollector)
		}

		eligibleTargetsCount := m.getMaxNewTargets(len(unmatchedEndpoints), totalTargets, tgbScopedLogger)
		unmatchedEndpoints = unmatchedEndpoints[:eligibleTargetsCount]

		if err := m.registerNodePortEndpoints(ctx, tgb, unmatchedEndpoints); err != nil {
			return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "update_node_port_endpoints_error", err, m.metricsCollector)
		}
	}

	if err := m.multiClusterManager.UpdateTrackedInstanceTargets(ctx, updateTrackedTargets, endpoints, tgb); err != nil {
		return "", "", false, ctrlerrors.NewErrorWithMetrics(controllerName, "update_tracked_instance_targets_error", err, m.metricsCollector)
	}

	tgbScopedLogger.Info("Successful reconcile", "checkpoint", newCheckPoint)
	return newCheckPoint, oldCheckPoint, false, nil
}

func (m *defaultResourceManager) cleanupTargets(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	targets, err := m.targetsManager.ListTargets(ctx, tgb)
	if err != nil {
		if isELBV2TargetGroupNotFoundError(err) {
			return nil
		} else if isELBV2TargetGroupARNInvalidError(err) {
			return nil
		}
		return err
	}

	_, err = m.deregisterTargets(ctx, tgb, targets)

	if err != nil {
		if isELBV2TargetGroupNotFoundError(err) {
			return nil
		} else if isELBV2TargetGroupARNInvalidError(err) {
			return nil
		}
		return err
	}
	return nil
}

// updateTargetHealthPodCondition will updates pod's targetHealth condition for matchedEndpointAndTargets and unmatchedEndpoints.
// returns whether further probe is needed or not
func (m *defaultResourceManager) updateTargetHealthPodCondition(ctx context.Context, targetHealthCondType corev1.PodConditionType,
	matchedEndpointAndTargets []podEndpointAndTargetPair, unmatchedEndpoints []backend.PodEndpoint, tgb *elbv2api.TargetGroupBinding) (bool, error) {
	anyPodNeedFurtherProbe := false

	for _, endpointAndTarget := range matchedEndpointAndTargets {
		pod := endpointAndTarget.endpoint.Pod
		targetHealth := endpointAndTarget.target.TargetHealth
		needFurtherProbe, err := m.updateTargetHealthPodConditionForPod(ctx, pod, targetHealth, targetHealthCondType, tgb)
		if err != nil {
			return false, err
		}
		if needFurtherProbe {
			anyPodNeedFurtherProbe = true
		}
	}

	for _, endpoint := range unmatchedEndpoints {
		pod := endpoint.Pod
		targetHealth := &elbv2types.TargetHealth{
			State:       elbv2types.TargetHealthStateEnumInitial,
			Reason:      elbv2types.TargetHealthReasonEnumRegistrationInProgress,
			Description: awssdk.String("Target registration is in progress"),
		}
		needFurtherProbe, err := m.updateTargetHealthPodConditionForPod(ctx, pod, targetHealth, targetHealthCondType, tgb)
		if err != nil {
			return false, err
		}
		if needFurtherProbe {
			anyPodNeedFurtherProbe = true
		}
	}
	return anyPodNeedFurtherProbe, nil
}

// updateTargetHealthPodConditionForPod updates pod's targetHealth condition for a single pod and its matched target.
// returns whether further probe is needed or not.
func (m *defaultResourceManager) updateTargetHealthPodConditionForPod(ctx context.Context, pod k8s.PodInfo,
	targetHealth *elbv2types.TargetHealth, targetHealthCondType corev1.PodConditionType, tgb *elbv2api.TargetGroupBinding) (bool, error) {
	if !pod.HasAnyOfReadinessGates([]corev1.PodConditionType{targetHealthCondType}) {
		return false, nil
	}

	var reason, message string
	if targetHealth != nil {
		reason = string(targetHealth.Reason)
		message = awssdk.ToString(targetHealth.Description)
	}

	targetHealthCondStatus, needFurtherProbe := m.calculateReadinessGateTransition(pod, targetHealthCondType, targetHealth)

	existingTargetHealthCond, hasExistingTargetHealthCond := pod.GetPodCondition(targetHealthCondType)
	// we skip patch pod if it matches current computed status/reason/message.
	if hasExistingTargetHealthCond &&
		existingTargetHealthCond.Status == targetHealthCondStatus &&
		existingTargetHealthCond.Reason == reason &&
		existingTargetHealthCond.Message == message {
		return needFurtherProbe, nil
	}

	newTargetHealthCond := corev1.PodCondition{
		Type:    targetHealthCondType,
		Status:  targetHealthCondStatus,
		Reason:  reason,
		Message: message,
	}
	if !hasExistingTargetHealthCond || existingTargetHealthCond.Status != targetHealthCondStatus {
		newTargetHealthCond.LastTransitionTime = metav1.Now()
	} else {
		newTargetHealthCond.LastTransitionTime = existingTargetHealthCond.LastTransitionTime
	}

	podPatchSource := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pod.Key.Namespace,
			Name:      pod.Key.Name,
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{},
		},
	}
	if hasExistingTargetHealthCond {
		podPatchSource.Status.Conditions = []corev1.PodCondition{existingTargetHealthCond}
	}

	podPatchTarget := podPatchSource.DeepCopy()
	podPatchTarget.UID = pod.UID // only put the uid in the new object to ensure it appears in the patch as a precondition
	podPatchTarget.Status.Conditions = []corev1.PodCondition{newTargetHealthCond}

	if err := m.k8sClient.Status().Patch(ctx, podPatchTarget, client.StrategicMergeFrom(podPatchSource)); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Only update duration on unhealthy -> healthy flips.
	if targetHealthCondStatus == corev1.ConditionTrue && hasExistingTargetHealthCond && !existingTargetHealthCond.LastTransitionTime.IsZero() && existingTargetHealthCond.Status != corev1.ConditionTrue {
		delta := newTargetHealthCond.LastTransitionTime.Sub(existingTargetHealthCond.LastTransitionTime.Time)
		m.metricsCollector.ObservePodReadinessGateReady(tgb.Namespace, tgb.Name, delta)
	}

	return needFurtherProbe, nil
}

func (m *defaultResourceManager) calculateReadinessGateTransition(pod k8s.PodInfo, targetHealthCondType corev1.PodConditionType, targetHealth *elbv2types.TargetHealth) (corev1.ConditionStatus, bool) {
	if !pod.HasAnyOfReadinessGates([]corev1.PodConditionType{targetHealthCondType}) {
		return corev1.ConditionTrue, false
	}
	targetHealthCondStatus := corev1.ConditionUnknown
	if targetHealth != nil {
		if string(targetHealth.State) == string(elbv2types.TargetHealthStateEnumHealthy) {
			targetHealthCondStatus = corev1.ConditionTrue
		} else {
			targetHealthCondStatus = corev1.ConditionFalse
		}
	}
	return targetHealthCondStatus, targetHealthCondStatus != corev1.ConditionTrue
}

// updatePodAsHealthyForDeletedTGB updates pod's targetHealth condition as healthy when deleting a TGB
// if the pod has readiness Gate.
func (m *defaultResourceManager) updatePodAsHealthyForDeletedTGB(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	targetHealthCondType := BuildTargetHealthPodConditionType(tgb)

	allPodKeys := m.podInfoRepo.ListKeys(ctx)
	for _, podKey := range allPodKeys {
		// check the pod is in the same namespace with the tgb
		if podKey.Namespace != tgb.Namespace {
			continue
		}
		pod, exists, err := m.podInfoRepo.Get(ctx, podKey)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if pod.HasAnyOfReadinessGates([]corev1.PodConditionType{targetHealthCondType}) {
			targetHealth := &elbv2types.TargetHealth{
				State:       elbv2types.TargetHealthStateEnumHealthy,
				Description: awssdk.String("Target Group Binding is deleted"),
			}
			_, err := m.updateTargetHealthPodConditionForPod(ctx, pod, targetHealth, targetHealthCondType, tgb)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *defaultResourceManager) deregisterTargets(ctx context.Context, tgb *elbv2api.TargetGroupBinding, targets []TargetInfo) (bool, error) {
	filteredTargets, updateTrackedTargets, err := m.multiClusterManager.FilterTargetsForDeregistration(ctx, tgb, targets)
	if err != nil {
		return false, err
	}

	if len(filteredTargets) == 0 {
		return updateTrackedTargets, nil
	}

	sdkTargets := make([]elbv2types.TargetDescription, 0, len(targets))
	for _, target := range filteredTargets {
		sdkTargets = append(sdkTargets, target.Target)
	}
	return true, m.targetsManager.DeregisterTargets(ctx, tgb, sdkTargets)
}

func (m *defaultResourceManager) registerPodEndpoints(ctx context.Context, tgb *elbv2api.TargetGroupBinding, endpoints []backend.PodEndpoint) error {
	vpcID := m.vpcID
	if tgb.Spec.VpcID != "" && tgb.Spec.VpcID != m.vpcID {
		vpcID = tgb.Spec.VpcID
		m.logger.Info(fmt.Sprintf(
			"registering endpoints using the targetGroup's vpcID %s which is different from the cluster's vpcID %s", tgb.Spec.VpcID, m.vpcID))
	}

	overrideAzFn, err := m.generateOverrideAzFn(ctx, vpcID, tgb.Spec.IamRoleArnToAssume)

	if err != nil {
		return err
	}

	sdkTargets, err := m.prepareRegistrationCall(endpoints, overrideAzFn)
	if err != nil {
		return err
	}
	return m.targetsManager.RegisterTargets(ctx, tgb, sdkTargets)
}

func (m *defaultResourceManager) prepareRegistrationCall(endpoints []backend.PodEndpoint, doAzOverride func(addr netip.Addr) bool) ([]elbv2types.TargetDescription, error) {
	sdkTargets := make([]elbv2types.TargetDescription, 0, len(endpoints))
	for _, endpoint := range endpoints {
		target := elbv2types.TargetDescription{
			Id:   awssdk.String(endpoint.IP),
			Port: awssdk.Int32(endpoint.Port),
		}
		podIP, err := netip.ParseAddr(endpoint.IP)
		if err != nil {
			return sdkTargets, err
		}
		if doAzOverride(podIP) {
			target.AvailabilityZone = awssdk.String("all")
		}
		sdkTargets = append(sdkTargets, target)
	}
	return sdkTargets, nil
}

func (m *defaultResourceManager) registerNodePortEndpoints(ctx context.Context, tgb *elbv2api.TargetGroupBinding, endpoints []backend.NodePortEndpoint) error {
	sdkTargets := make([]elbv2types.TargetDescription, 0, len(endpoints))
	for _, endpoint := range endpoints {
		sdkTargets = append(sdkTargets, elbv2types.TargetDescription{
			Id:   awssdk.String(endpoint.InstanceID),
			Port: awssdk.Int32(endpoint.Port),
		})
	}
	return m.targetsManager.RegisterTargets(ctx, tgb, sdkTargets)
}

func (m *defaultResourceManager) updateTGBCheckPoint(ctx context.Context, tgb *elbv2api.TargetGroupBinding, newCheckPoint, previousCheckPoint string) error {
	if newCheckPoint == previousCheckPoint {
		return nil
	}

	tgbOld := tgb.DeepCopy()
	SaveTGBReconcileCheckpoint(tgb, newCheckPoint)

	if err := m.k8sClient.Patch(ctx, tgb, client.MergeFrom(tgbOld)); err != nil {
		return errors.Wrapf(err, "failed to update targetGroupBinding checkpoint: %v", k8s.NamespacedName(tgb))
	}
	return nil
}

func (m *defaultResourceManager) generateOverrideAzFn(ctx context.Context, vpcID string, assumeRole string) (func(addr netip.Addr) bool, error) {
	// Cross-Account is configured by assuming a role.
	usingCrossAccount := assumeRole != ""

	// We need to cache the vpc response for the various assume roles.
	// There are two cases to consider when using assuming a role:
	// 1. Using a peered VPC connection to provide connectivity among accounts.
	// 2. Using RAM shared subnet(s) to provide connectivity among accounts.
	// We need to handle the case where the user is potentially using the same VPC in the peered context
	// as well as the RAM shared context.
	// Using peered VPC connection, we will always need to override the AZ.
	// Using a RAM shared subnet / VPC means that we follow the standard logic of checking the pod ip against the VPC CIDRs.

	invalidVPCCacheKey := fmt.Sprintf("%s-%s", assumeRole, vpcID)

	if usingCrossAccount {
		// Prevent spamming EC2 with requests.
		// We can use the cached result for this VPC ID given for the current assume role ARN
		m.invalidVpcCacheMutex.RLock()
		_, invalidVPC := m.invalidVpcCache.Get(invalidVPCCacheKey)
		m.invalidVpcCacheMutex.RUnlock()

		// In this case, we already received that this VPC was invalid, we can shortcut the EC2 call and just override the AZ.
		if invalidVPC {
			return func(addr netip.Addr) bool {
				return true
			}, nil
		}
	}

	vpcInfo, err := m.vpcInfoProvider.FetchVPCInfo(ctx, vpcID)
	if err != nil {
		// A VPC Not Found Error along with cross-account usage means that the VPC either, is not shared with the assume
		// role account OR this falls into case (1) from above where the VPC is just peered but not shared with RAM.
		// As we can't differentiate if RAM sharing wasn't set up correctly OR the VPC is set up via peering, we will
		// just default to assume that the VPC is peered but not shared.
		if isVPCNotFoundError(err) && usingCrossAccount {
			m.invalidVpcCacheMutex.Lock()
			m.invalidVpcCache.Set(invalidVPCCacheKey, true, m.invalidVpcCacheTTL)
			m.invalidVpcCacheMutex.Unlock()
			return func(addr netip.Addr) bool {
				return true
			}, nil
		}
		return nil, err
	}
	var vpcRawCIDRs []string
	vpcRawCIDRs = append(vpcRawCIDRs, vpcInfo.AssociatedIPv4CIDRs()...)
	vpcRawCIDRs = append(vpcRawCIDRs, vpcInfo.AssociatedIPv6CIDRs()...)
	vpcCIDRs, err := networking.ParseCIDRs(vpcRawCIDRs)
	if err != nil {
		return nil, err
	}
	// By getting here, we have a valid VPC for whatever credential was used. We return "true" in the function below
	// when the pod ip falls outside the VPCs configured CIDRs, other we return "false" to ensure that the "all" is NOT injected.
	return func(addr netip.Addr) bool {
		return !networking.IsIPWithinCIDRs(addr, vpcCIDRs)
	}, nil
}

type podEndpointAndTargetPair struct {
	endpoint backend.PodEndpoint
	target   TargetInfo
}

func partitionTargetsByDrainingStatus(targets []TargetInfo) ([]TargetInfo, []TargetInfo) {
	var notDrainingTargets []TargetInfo
	var drainingTargets []TargetInfo
	for _, target := range targets {
		if target.IsDraining() {
			drainingTargets = append(drainingTargets, target)
		} else {
			notDrainingTargets = append(notDrainingTargets, target)
		}
	}
	return notDrainingTargets, drainingTargets
}

func containsTargetsInInitialState(matchedEndpointAndTargets []podEndpointAndTargetPair) bool {
	for _, endpointAndTarget := range matchedEndpointAndTargets {
		if endpointAndTarget.target.IsInitial() {
			return true
		}
	}
	return false
}

func matchPodEndpointWithTargets(endpoints []backend.PodEndpoint, targets []TargetInfo) ([]podEndpointAndTargetPair, []backend.PodEndpoint, []TargetInfo) {
	var matchedEndpointAndTargets []podEndpointAndTargetPair
	var unmatchedEndpoints []backend.PodEndpoint
	var unmatchedTargets []TargetInfo

	endpointsByUID := make(map[string]backend.PodEndpoint, len(endpoints))
	for _, endpoint := range endpoints {
		endpointUID := fmt.Sprintf("%v:%v", endpoint.IP, endpoint.Port)
		endpointsByUID[endpointUID] = endpoint
	}
	targetsByUID := make(map[string]TargetInfo, len(targets))
	for _, target := range targets {
		targetUID := fmt.Sprintf("%v:%v", awssdk.ToString(target.Target.Id), awssdk.ToInt32(target.Target.Port))
		targetsByUID[targetUID] = target
	}
	endpointUIDs := sets.StringKeySet(endpointsByUID)
	targetUIDs := sets.StringKeySet(targetsByUID)
	for _, uid := range endpointUIDs.Intersection(targetUIDs).List() {
		endpoint := endpointsByUID[uid]
		target := targetsByUID[uid]
		matchedEndpointAndTargets = append(matchedEndpointAndTargets, podEndpointAndTargetPair{
			endpoint: endpoint,
			target:   target,
		})
	}
	for _, uid := range endpointUIDs.Difference(targetUIDs).List() {
		unmatchedEndpoints = append(unmatchedEndpoints, endpointsByUID[uid])
	}
	for _, uid := range targetUIDs.Difference(endpointUIDs).List() {
		unmatchedTargets = append(unmatchedTargets, targetsByUID[uid])
	}
	return matchedEndpointAndTargets, unmatchedEndpoints, unmatchedTargets
}

type nodePortEndpointAndTargetPair struct {
	endpoint backend.NodePortEndpoint
	target   TargetInfo
}

func matchNodePortEndpointWithTargets(endpoints []backend.NodePortEndpoint, targets []TargetInfo) ([]nodePortEndpointAndTargetPair, []backend.NodePortEndpoint, []TargetInfo) {
	var matchedEndpointAndTargets []nodePortEndpointAndTargetPair
	var unmatchedEndpoints []backend.NodePortEndpoint
	var unmatchedTargets []TargetInfo

	endpointsByUID := make(map[string]backend.NodePortEndpoint, len(endpoints))
	for _, endpoint := range endpoints {
		endpointUID := fmt.Sprintf("%v:%v", endpoint.InstanceID, endpoint.Port)
		endpointsByUID[endpointUID] = endpoint
	}
	targetsByUID := make(map[string]TargetInfo, len(targets))
	for _, target := range targets {
		targetUID := fmt.Sprintf("%v:%v", awssdk.ToString(target.Target.Id), awssdk.ToInt32(target.Target.Port))
		targetsByUID[targetUID] = target
	}
	endpointUIDs := sets.StringKeySet(endpointsByUID)
	targetUIDs := sets.StringKeySet(targetsByUID)
	for _, uid := range endpointUIDs.Intersection(targetUIDs).List() {
		endpoint := endpointsByUID[uid]
		target := targetsByUID[uid]
		matchedEndpointAndTargets = append(matchedEndpointAndTargets, nodePortEndpointAndTargetPair{
			endpoint: endpoint,
			target:   target,
		})
	}
	for _, uid := range endpointUIDs.Difference(targetUIDs).List() {
		unmatchedEndpoints = append(unmatchedEndpoints, endpointsByUID[uid])
	}
	for _, uid := range targetUIDs.Difference(endpointUIDs).List() {
		unmatchedTargets = append(unmatchedTargets, targetsByUID[uid])
	}
	return matchedEndpointAndTargets, unmatchedEndpoints, unmatchedTargets
}

func isELBV2TargetGroupNotFoundError(err error) bool {
	var awsErr *elbv2types.TargetGroupNotFoundException
	if errors.As(err, &awsErr) {
		return true
	}
	return false
}

func isELBV2TargetGroupARNInvalidError(err error) bool {
	var awsErr *elbv2types.InvalidTargetException
	if errors.As(err, &awsErr) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()

		return code == "ValidationError"
	}
	return false
}

func isVPCNotFoundError(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "InvalidVpcID.NotFound"
	}
	return false
}

func (m *defaultResourceManager) getMaxNewTargets(newTargetCount int, currentTargetCount int, tgbScopedLogger logr.Logger) (maxAdditions int) {
		if m.maxTargetsPerInstance > 0 && newTargetCount+currentTargetCount > m.maxTargetsPerInstance {
			maxAdditions = m.maxTargetsPerInstance - currentTargetCount
			tgbScopedLogger.Info("Limiting target additions due to max-targets-per-instance configuration",
				"currentTargets", currentTargetCount,
				"maxTargetsPerInstance", m.maxTargetsPerInstance,
				"proposedAdditions", newTargetCount)
			return maxAdditions
		}

		return newTargetCount
}
