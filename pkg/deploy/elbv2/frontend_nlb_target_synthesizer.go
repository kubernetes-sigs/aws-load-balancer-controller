package elbv2

import (
	"context"
	ctrlerrors "sigs.k8s.io/aws-load-balancer-controller/pkg/error"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TargetGroupsResult struct {
	TargetGroups []TargetGroupWithTags
	Err          error
}

func NewFrontendNlbTargetSynthesizer(k8sClient client.Client, trackingProvider tracking.Provider, taggingManager TaggingManager, frontendNlbTargetsManager FrontendNlbTargetsManager, logger logr.Logger, featureGates config.FeatureGates, stack core.Stack, frontendNlbTargetGroupDesiredState *core.FrontendNlbTargetGroupDesiredState, findSDKTargetGroups func() TargetGroupsResult) *frontendNlbTargetSynthesizer {
	return &frontendNlbTargetSynthesizer{
		k8sClient:                          k8sClient,
		trackingProvider:                   trackingProvider,
		taggingManager:                     taggingManager,
		frontendNlbTargetsManager:          frontendNlbTargetsManager,
		featureGates:                       featureGates,
		logger:                             logger,
		stack:                              stack,
		frontendNlbTargetGroupDesiredState: frontendNlbTargetGroupDesiredState,
		findSDKTargetGroups:                findSDKTargetGroups,
	}
}

type frontendNlbTargetSynthesizer struct {
	k8sClient                          client.Client
	trackingProvider                   tracking.Provider
	taggingManager                     TaggingManager
	frontendNlbTargetsManager          FrontendNlbTargetsManager
	featureGates                       config.FeatureGates
	logger                             logr.Logger
	stack                              core.Stack
	frontendNlbTargetGroupDesiredState *core.FrontendNlbTargetGroupDesiredState
	findSDKTargetGroups                func() TargetGroupsResult
}

// Synthesize processes AWS target groups and deregisters ALB targets based on the desired state.
func (s *frontendNlbTargetSynthesizer) Synthesize(ctx context.Context) error {
	var resTGs []*elbv2model.TargetGroup
	s.stack.ListResources(&resTGs)
	res := s.findSDKTargetGroups()
	if res.Err != nil {
		return res.Err
	}
	sdkTGs := res.TargetGroups
	_, _, unmatchedSDKTGs, err := matchResAndSDKTargetGroups(resTGs, sdkTGs,
		s.trackingProvider.ResourceIDTagKey(), s.featureGates)
	if err != nil {
		return err
	}

	for _, sdkTG := range unmatchedSDKTGs {
		if sdkTG.TargetGroup.TargetType != elbv2types.TargetTypeEnumAlb {
			continue
		}

		err := s.deregisterCurrentTarget(ctx, sdkTG)
		if err != nil {
			return errors.Wrapf(err, "failed to deregister target for the target group: %s", *sdkTG.TargetGroup.TargetGroupArn)
		}
	}

	return nil

}

func (s *frontendNlbTargetSynthesizer) deregisterCurrentTarget(ctx context.Context, sdkTG TargetGroupWithTags) error {
	// Retrieve the current targets for the target group
	currentTargets, err := s.frontendNlbTargetsManager.ListTargets(ctx, *sdkTG.TargetGroup.TargetGroupArn)
	if err != nil {
		return errors.Wrapf(err, "failed to list current target for target group: %s", *sdkTG.TargetGroup.TargetGroupArn)
	}

	// If there is no target, nothing to deregister
	if len(currentTargets) == 0 {
		return nil
	}

	// Deregister current target
	s.logger.Info("Deregistering current target",
		"targetGroupARN", *sdkTG.TargetGroup.TargetGroupArn,
		"target", currentTargets[0].Target.Id,
		"port", currentTargets[0].Target.Port,
	)

	err = s.frontendNlbTargetsManager.DeregisterTargets(ctx, *sdkTG.TargetGroup.TargetGroupArn, elbv2types.TargetDescription{
		Id:   awssdk.String(*currentTargets[0].Target.Id),
		Port: awssdk.Int32(*currentTargets[0].Target.Port),
	})

	if err != nil {
		return errors.Wrapf(err, "failed to deregister targets for target group: %s", *sdkTG.TargetGroup.TargetGroupArn)
	}

	return nil
}

func (s *frontendNlbTargetSynthesizer) PostSynthesize(ctx context.Context) error {
	var resTGs []*elbv2model.TargetGroup
	s.stack.ListResources(&resTGs)

	// Filter desired target group to include only ALB type target group
	albResTGs := filterALBTargetGroups(resTGs)

	for _, resTG := range albResTGs {

		// Skip target group that are not yet created
		if resTG.Status.TargetGroupARN == "" {
			continue
		}

		// List current targets
		currentTargets, err := s.frontendNlbTargetsManager.ListTargets(ctx, resTG.Status.TargetGroupARN)

		if err != nil {
			return err
		}

		desiredTarget, err := s.frontendNlbTargetGroupDesiredState.TargetGroups[resTG.Spec.Name].TargetARN.Resolve(ctx)
		desiredTargetPort := s.frontendNlbTargetGroupDesiredState.TargetGroups[resTG.Spec.Name].TargetPort

		if err != nil {
			return errors.Wrapf(err, "failed to resolve the desiredTarget for target group: %s", desiredTarget)
		}

		if len(currentTargets) == 0 ||
			currentTargets[0].Target == nil ||
			currentTargets[0].Target.Id == nil ||
			*currentTargets[0].Target.Id != desiredTarget {
			err = s.frontendNlbTargetsManager.RegisterTargets(ctx, resTG.Status.TargetGroupARN, elbv2types.TargetDescription{
				Id:   awssdk.String(desiredTarget),
				Port: awssdk.Int32(desiredTargetPort),
			})

			if err != nil {
				requeueMsg := "Failed to register target, retrying after deplay for target group: " + resTG.Status.TargetGroupARN
				return ctrlerrors.NewRequeueNeededAfter(requeueMsg, 15*time.Second)
			}

		}

	}

	return nil
}

func filterALBTargetGroups(targetGroups []*elbv2model.TargetGroup) []*elbv2model.TargetGroup {
	var filteredTargetGroups []*elbv2model.TargetGroup
	for _, tg := range targetGroups {
		if elbv2types.TargetTypeEnum(tg.Spec.TargetType) == elbv2types.TargetTypeEnumAlb {
			filteredTargetGroups = append(filteredTargetGroups, tg)
		}
	}
	return filteredTargetGroups
}
