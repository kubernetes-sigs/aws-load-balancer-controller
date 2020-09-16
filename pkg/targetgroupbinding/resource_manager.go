package targetgroupbinding

import (
	"context"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2api "sigs.k8s.io/aws-alb-ingress-controller/apis/elbv2/v1alpha1"
	awserrors "sigs.k8s.io/aws-alb-ingress-controller/pkg/aws/errors"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/aws/services"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/backend"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/networking"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceManager manages the TargetGroupBinding resource.
type ResourceManager interface {
	Reconcile(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error
	Cleanup(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error
}

// NewDefaultResourceManager constructs new defaultResourceManager.
func NewDefaultResourceManager(k8sClient client.Client, elbv2Client services.ELBV2,
	podENIResolver networking.PodENIInfoResolver, nodeENIResolver networking.NodeENIInfoResolver,
	sgManager networking.SecurityGroupManager, sgReconciler networking.SecurityGroupReconciler,
	vpcID string, clusterName string, logger logr.Logger) *defaultResourceManager {
	targetsManager := NewCachedTargetsManager(elbv2Client, logger)
	endpointResolver := backend.NewDefaultEndpointResolver(k8sClient, logger)
	networkingManager := NewDefaultNetworkingManager(k8sClient, podENIResolver, nodeENIResolver, sgManager, sgReconciler, vpcID, clusterName, logger)
	return &defaultResourceManager{
		targetsManager:    targetsManager,
		endpointResolver:  endpointResolver,
		networkingManager: networkingManager,
		logger:            logger,
	}
}

var _ ResourceManager = &defaultResourceManager{}

// default implementation for ResourceManager.
type defaultResourceManager struct {
	targetsManager    TargetsManager
	endpointResolver  backend.EndpointResolver
	networkingManager NetworkingManager
	logger            logr.Logger
}

func (m *defaultResourceManager) Reconcile(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	if tgb.Spec.TargetType == nil {
		return errors.Errorf("targetType is not specified: %v", k8s.NamespacedName(tgb).String())
	}
	if *tgb.Spec.TargetType == elbv2api.TargetTypeIP {
		return m.reconcileWithIPTargetType(ctx, tgb)
	}
	return m.reconcileWithInstanceTargetType(ctx, tgb)
}

func (m *defaultResourceManager) Cleanup(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	targets, err := m.targetsManager.ListTargets(ctx, tgb.Spec.TargetGroupARN)
	if err != nil {
		if awserrors.IsELBV2TargetGroupNotFoundError(err) {
			return nil
		}
		return err
	}
	err = m.deregisterTargets(ctx, tgb.Spec.TargetGroupARN, targets)
	if err != nil {
		if awserrors.IsELBV2TargetGroupNotFoundError(err) {
			return nil
		}
		return err
	}
	return nil
}

func (m *defaultResourceManager) reconcileWithIPTargetType(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	svcKey := buildServiceReferenceKey(tgb, tgb.Spec.ServiceRef)
	resolveOpts := []backend.EndpointResolveOption{
		backend.WithUnreadyPodInclusionCriterion(k8s.IsPodContainersReady),
	}
	endpoints, err := m.endpointResolver.ResolvePodEndpoints(ctx, svcKey, tgb.Spec.ServiceRef.Port, resolveOpts...)
	if err != nil {
		return err
	}
	tgARN := tgb.Spec.TargetGroupARN
	targets, err := m.targetsManager.ListTargets(ctx, tgARN)
	if err != nil {
		return err
	}
	notDrainingTargets, drainingTargets := partitionTargetsByDrainingStatus(targets)
	matchedEndpointAndTargets, unmatchedEndpoints, unmatchedTargets := matchPodEndpointWithTargets(endpoints, notDrainingTargets)

	if err := m.networkingManager.ReconcileForPodEndpoints(ctx, tgb, endpoints); err != nil {
		return err
	}
	if err := m.deregisterTargets(ctx, tgARN, unmatchedTargets); err != nil {
		return err
	}
	if err := m.registerPodEndpoints(ctx, tgARN, unmatchedEndpoints); err != nil {
		return err
	}
	_ = matchedEndpointAndTargets
	_ = drainingTargets

	return nil
}

func (m *defaultResourceManager) reconcileWithInstanceTargetType(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	svcKey := buildServiceReferenceKey(tgb, tgb.Spec.ServiceRef)
	nodeSelector := backend.GetTrafficProxyNodeSelector(tgb)
	resolveOpts := []backend.EndpointResolveOption{backend.WithNodeSelector(nodeSelector)}
	endpoints, err := m.endpointResolver.ResolveNodePortEndpoints(ctx, svcKey, tgb.Spec.ServiceRef.Port, resolveOpts...)
	if err != nil {
		return err
	}
	tgARN := tgb.Spec.TargetGroupARN
	targets, err := m.targetsManager.ListTargets(ctx, tgARN)
	if err != nil {
		return err
	}
	_, unmatchedEndpoints, unmatchedTargets := matchNodePortEndpointWithTargets(endpoints, targets)

	if err := m.networkingManager.ReconcileForNodePortEndpoints(ctx, tgb, endpoints); err != nil {
		return err
	}
	if err := m.deregisterTargets(ctx, tgARN, unmatchedTargets); err != nil {
		return err
	}
	if err := m.registerNodePortEndpoints(ctx, tgARN, unmatchedEndpoints); err != nil {
		return err
	}
	return nil
}

func (m *defaultResourceManager) deregisterTargets(ctx context.Context, tgARN string, targets []TargetInfo) error {
	sdkTargets := make([]elbv2sdk.TargetDescription, 0, len(targets))
	for _, target := range targets {
		sdkTargets = append(sdkTargets, target.Target)
	}
	return m.targetsManager.DeregisterTargets(ctx, tgARN, sdkTargets)
}

func (m *defaultResourceManager) registerPodEndpoints(ctx context.Context, tgARN string, endpoints []backend.PodEndpoint) error {
	sdkTargets := make([]elbv2sdk.TargetDescription, 0, len(endpoints))
	for _, endpoint := range endpoints {
		sdkTargets = append(sdkTargets, elbv2sdk.TargetDescription{
			Id:   awssdk.String(endpoint.IP),
			Port: awssdk.Int64(endpoint.Port),
		})
	}
	return m.targetsManager.RegisterTargets(ctx, tgARN, sdkTargets)
}

func (m *defaultResourceManager) registerNodePortEndpoints(ctx context.Context, tgARN string, endpoints []backend.NodePortEndpoint) error {
	sdkTargets := make([]elbv2sdk.TargetDescription, 0, len(endpoints))
	for _, endpoint := range endpoints {
		sdkTargets = append(sdkTargets, elbv2sdk.TargetDescription{
			Id:   awssdk.String(endpoint.InstanceID),
			Port: awssdk.Int64(endpoint.Port),
		})
	}
	return m.targetsManager.RegisterTargets(ctx, tgARN, sdkTargets)
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
		targetUID := fmt.Sprintf("%v:%v", awssdk.StringValue(target.Target.Id), awssdk.Int64Value(target.Target.Port))
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
		targetUID := fmt.Sprintf("%v:%v", awssdk.StringValue(target.Target.Id), awssdk.Int64Value(target.Target.Port))
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
