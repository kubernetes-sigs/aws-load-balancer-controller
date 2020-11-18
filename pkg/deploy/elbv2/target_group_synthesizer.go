package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

// NewTargetGroupSynthesizer constructs targetGroupSynthesizer
func NewTargetGroupSynthesizer(elbv2Client services.ELBV2, trackingProvider tracking.Provider, taggingManager TaggingManager,
	tgManager TargetGroupManager, logger logr.Logger, stack core.Stack) *targetGroupSynthesizer {
	return &targetGroupSynthesizer{
		elbv2Client:      elbv2Client,
		trackingProvider: trackingProvider,
		taggingManager:   taggingManager,
		tgManager:        tgManager,
		logger:           logger,
		stack:            stack,
		unmatchedSDKTGs:  nil,
	}
}

// targetGroupSynthesizer is responsible for synthesize TargetGroup resources types for certain stack.
type targetGroupSynthesizer struct {
	elbv2Client      services.ELBV2
	trackingProvider tracking.Provider
	taggingManager   TaggingManager
	tgManager        TargetGroupManager
	logger           logr.Logger

	stack           core.Stack
	unmatchedSDKTGs []TargetGroupWithTags
}

func (s *targetGroupSynthesizer) Synthesize(ctx context.Context) error {
	var resTGs []*elbv2model.TargetGroup
	s.stack.ListResources(&resTGs)
	sdkTGs, err := s.findSDKTargetGroups(ctx)
	if err != nil {
		return err
	}
	matchedResAndSDKTGs, unmatchedResTGs, unmatchedSDKTGs, err := matchResAndSDKTargetGroups(resTGs, sdkTGs, s.trackingProvider.ResourceIDTagKey())
	if err != nil {
		return err
	}

	// For TargetGroups, we delete unmatched ones during post synthesize given below facts:
	// * unmatched targetGroups might still be use by a listener rule.
	s.unmatchedSDKTGs = unmatchedSDKTGs

	for _, resTG := range unmatchedResTGs {
		tgStatus, err := s.tgManager.Create(ctx, resTG)
		if err != nil {
			return err
		}
		resTG.SetStatus(tgStatus)
	}
	for _, resAndSDKTG := range matchedResAndSDKTGs {
		tgStatus, err := s.tgManager.Update(ctx, resAndSDKTG.resTG, resAndSDKTG.sdkTG)
		if err != nil {
			return err
		}
		resAndSDKTG.resTG.SetStatus(tgStatus)
	}
	return nil
}

func (s *targetGroupSynthesizer) PostSynthesize(ctx context.Context) error {
	for _, sdkTG := range s.unmatchedSDKTGs {
		if err := s.tgManager.Delete(ctx, sdkTG); err != nil {
			return err
		}
	}
	return nil
}

// findSDKTargetGroups will find all AWS TargetGroups created for stack.
func (s *targetGroupSynthesizer) findSDKTargetGroups(ctx context.Context) ([]TargetGroupWithTags, error) {
	stackTags := s.trackingProvider.StackTags(s.stack)
	stackTagsLegacy := s.trackingProvider.StackTagsLegacy(s.stack)
	return s.taggingManager.ListTargetGroups(ctx,
		tracking.TagsAsTagFilter(stackTags),
		tracking.TagsAsTagFilter(stackTagsLegacy))
}

type resAndSDKTargetGroupPair struct {
	resTG *elbv2model.TargetGroup
	sdkTG TargetGroupWithTags
}

func matchResAndSDKTargetGroups(resTGs []*elbv2model.TargetGroup, sdkTGs []TargetGroupWithTags,
	resourceIDTagKey string) ([]resAndSDKTargetGroupPair, []*elbv2model.TargetGroup, []TargetGroupWithTags, error) {
	var matchedResAndSDKTGs []resAndSDKTargetGroupPair
	var unmatchedResTGs []*elbv2model.TargetGroup
	var unmatchedSDKTGs []TargetGroupWithTags

	resTGsByID := mapResTargetGroupByResourceID(resTGs)
	sdkTGsByID, err := mapSDKTargetGroupByResourceID(sdkTGs, resourceIDTagKey)
	if err != nil {
		return nil, nil, nil, err
	}

	resTGIDs := sets.StringKeySet(resTGsByID)
	sdkTGIDs := sets.StringKeySet(sdkTGsByID)
	for _, resID := range resTGIDs.Intersection(sdkTGIDs).List() {
		resTG := resTGsByID[resID]
		sdkTGs := sdkTGsByID[resID]
		foundMatch := false
		for _, sdkTG := range sdkTGs {
			if isSDKTargetGroupRequiresReplacement(sdkTG, resTG) {
				unmatchedSDKTGs = append(unmatchedSDKTGs, sdkTG)
				continue
			}
			matchedResAndSDKTGs = append(matchedResAndSDKTGs, resAndSDKTargetGroupPair{
				resTG: resTG,
				sdkTG: sdkTG,
			})
			foundMatch = true
		}
		if !foundMatch {
			unmatchedResTGs = append(unmatchedResTGs, resTG)
		}
	}
	for _, resID := range resTGIDs.Difference(sdkTGIDs).List() {
		unmatchedResTGs = append(unmatchedResTGs, resTGsByID[resID])
	}
	for _, resID := range sdkTGIDs.Difference(resTGIDs).List() {
		unmatchedSDKTGs = append(unmatchedSDKTGs, sdkTGsByID[resID]...)
	}

	return matchedResAndSDKTGs, unmatchedResTGs, unmatchedSDKTGs, nil
}

func mapResTargetGroupByResourceID(resTGs []*elbv2model.TargetGroup) map[string]*elbv2model.TargetGroup {
	resTGsByID := make(map[string]*elbv2model.TargetGroup, len(resTGs))
	for _, resTG := range resTGs {
		resTGsByID[resTG.ID()] = resTG
	}
	return resTGsByID
}

func mapSDKTargetGroupByResourceID(sdkTGs []TargetGroupWithTags, resourceIDTagKey string) (map[string][]TargetGroupWithTags, error) {
	sdkTGsByID := make(map[string][]TargetGroupWithTags, len(sdkTGs))
	for _, sdkTG := range sdkTGs {
		resourceID, ok := sdkTG.Tags[resourceIDTagKey]
		if !ok {
			return nil, errors.Errorf("unexpected targetGroup with no resourceID: %v", awssdk.StringValue(sdkTG.TargetGroup.TargetGroupArn))
		}
		sdkTGsByID[resourceID] = append(sdkTGsByID[resourceID], sdkTG)
	}
	return sdkTGsByID, nil
}

// isSDKTargetGroupRequiresReplacement checks whether a sdk TargetGroup requires replacement to fulfill a TargetGroup resource.
func isSDKTargetGroupRequiresReplacement(sdkTG TargetGroupWithTags, resTG *elbv2model.TargetGroup) bool {
	if string(resTG.Spec.TargetType) != awssdk.StringValue(sdkTG.TargetGroup.TargetType) {
		return true
	}
	if string(resTG.Spec.Protocol) != awssdk.StringValue(sdkTG.TargetGroup.Protocol) {
		return true
	}
	if resTG.Spec.ProtocolVersion != nil {
		if string(*resTG.Spec.ProtocolVersion) != awssdk.StringValue(sdkTG.TargetGroup.ProtocolVersion) {
			return true
		}
	}

	return isSDKTargetGroupRequiresReplacementDueToNLBHealthCheck(sdkTG, resTG)
}

// most of the healthCheck settings for NLB targetGroups cannot be changed for now.
func isSDKTargetGroupRequiresReplacementDueToNLBHealthCheck(sdkTG TargetGroupWithTags, resTG *elbv2model.TargetGroup) bool {
	if resTG.Spec.HealthCheckConfig == nil {
		return false
	}
	if resTG.Spec.Protocol != elbv2model.ProtocolTCP && resTG.Spec.Protocol != elbv2model.ProtocolUDP &&
		resTG.Spec.Protocol != elbv2model.ProtocolTCP_UDP && resTG.Spec.Protocol != elbv2model.ProtocolTLS {
		return false
	}
	sdkObj := sdkTG.TargetGroup
	hcConfig := *resTG.Spec.HealthCheckConfig
	if hcConfig.Protocol != nil && string(*hcConfig.Protocol) != awssdk.StringValue(sdkObj.HealthCheckProtocol) {
		return true
	}
	if hcConfig.Matcher != nil && (sdkObj.Matcher == nil || awssdk.StringValue(hcConfig.Matcher.GRPCCode) != awssdk.StringValue(sdkObj.Matcher.GrpcCode) || awssdk.StringValue(hcConfig.Matcher.HTTPCode) != awssdk.StringValue(sdkObj.Matcher.HttpCode)) {
		return true
	}
	if hcConfig.IntervalSeconds != nil && awssdk.Int64Value(hcConfig.IntervalSeconds) != awssdk.Int64Value(sdkObj.HealthCheckIntervalSeconds) {
		return true
	}
	if hcConfig.TimeoutSeconds != nil && awssdk.Int64Value(hcConfig.TimeoutSeconds) != awssdk.Int64Value(sdkObj.HealthCheckTimeoutSeconds) {
		return true
	}
	return false
}
