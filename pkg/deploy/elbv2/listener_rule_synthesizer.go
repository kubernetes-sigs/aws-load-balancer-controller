package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	elbv2equality "sigs.k8s.io/aws-load-balancer-controller/pkg/equality/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"strconv"
)

// NewListenerRuleSynthesizer constructs new listenerRuleSynthesizer.
func NewListenerRuleSynthesizer(elbv2Client services.ELBV2, taggingManager TaggingManager,
	lrManager ListenerRuleManager, logger logr.Logger, featureGates config.FeatureGates, stack core.Stack) *listenerRuleSynthesizer {
	return &listenerRuleSynthesizer{
		elbv2Client:    elbv2Client,
		lrManager:      lrManager,
		logger:         logger,
		featureGates:   featureGates,
		taggingManager: taggingManager,
		stack:          stack,
	}
}

type listenerRuleSynthesizer struct {
	elbv2Client    services.ELBV2
	lrManager      ListenerRuleManager
	featureGates   config.FeatureGates
	logger         logr.Logger
	taggingManager TaggingManager

	stack core.Stack
}

func (s *listenerRuleSynthesizer) Synthesize(ctx context.Context) error {
	var resLRs []*elbv2model.ListenerRule
	s.stack.ListResources(&resLRs)
	resLRsByLSARN, err := mapResListenerRuleByListenerARN(resLRs)
	if err != nil {
		return err
	}

	var resLSs []*elbv2model.Listener
	s.stack.ListResources(&resLSs)
	for _, resLS := range resLSs {
		lsARN, err := resLS.ListenerARN().Resolve(ctx)
		if err != nil {
			return err
		}
		resLRs := resLRsByLSARN[lsARN]
		if err := s.synthesizeListenerRulesOnListener(ctx, lsARN, resLRs); err != nil {
			return err
		}
	}
	return nil
}

func (s *listenerRuleSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here.
	return nil
}

func (s *listenerRuleSynthesizer) synthesizeListenerRulesOnListener(ctx context.Context, lsARN string, resLRs []*elbv2model.ListenerRule) error {
	// Build desired actions and conditions pairs for resource listener rules.
	resLRDesiredActionsAndConditionsPairs := make(map[*elbv2model.ListenerRule]*resLRDesiredActionsAndConditionsPair, len(resLRs))
	for _, resLR := range resLRs {
		resLRDesiredActionsAndConditionsPair, err := buildResLRDesiredActionsAndConditionsPair(resLR, s.featureGates)
		if err != nil {
			return err
		}
		resLRDesiredActionsAndConditionsPairs[resLR] = resLRDesiredActionsAndConditionsPair
	}
	// Find existing listener rules on the load balancer
	sdkLRs, err := s.findSDKListenersRulesOnLS(ctx, lsARN)
	if err != nil {
		return err
	}
	// matchedResAndSDKLRsBySettings : A slice of matched resLR and SDKLR rule pairs that have matching settings like actions and conditions
	// unmatchedResLRs : A slice of resLR) that do not have a corresponding match in the sdkLRs. These rules need to be created on the load balancer.
	// unmatchedSDKLRs : A slice of sdkLRs that do not have a corresponding match in the resLRs. These rules need to be deleted from the load balancer.
	matchedResAndSDKLRsBySettings, unmatchedResLRs, unmatchedSDKLRs, err := s.matchResAndSDKListenerRules(resLRs, sdkLRs, resLRDesiredActionsAndConditionsPairs)
	if err != nil {
		return err
	}
	for _, sdkLR := range unmatchedSDKLRs {
		if err := s.lrManager.Delete(ctx, sdkLR); err != nil {
			return err
		}
	}
	// Re-prioritize matched listener rules.
	if len(matchedResAndSDKLRsBySettings) > 0 {
		err := s.lrManager.SetRulePriorities(ctx, matchedResAndSDKLRsBySettings)
		if err != nil {
			return err
		}
	}
	// Create all the new rules on the LB
	for _, resLR := range unmatchedResLRs {
		lrStatus, err := s.lrManager.Create(ctx, resLR, resLRDesiredActionsAndConditionsPairs[resLR])
		if err != nil {
			return err
		}
		resLR.SetStatus(lrStatus)
	}
	// Update existing listener rules on the load balancer for their tags
	for _, resAndSDKLR := range matchedResAndSDKLRsBySettings {
		lsStatus, err := s.lrManager.Update(ctx, resAndSDKLR.resLR, resAndSDKLR.sdkLR)
		if err != nil {
			return err
		}
		resAndSDKLR.resLR.SetStatus(lsStatus)
	}
	return nil
}

// findSDKListenersRulesOnLS returns the listenerRules configured on Listener.
func (s *listenerRuleSynthesizer) findSDKListenersRulesOnLS(ctx context.Context, lsARN string) ([]ListenerRuleWithTags, error) {
	sdkLRs, err := s.taggingManager.ListListenerRules(ctx, lsARN)
	if err != nil {
		return nil, err
	}
	nonDefaultRules := make([]ListenerRuleWithTags, 0, len(sdkLRs))
	for _, rule := range sdkLRs {
		if awssdk.ToBool(rule.ListenerRule.IsDefault) {
			continue
		}
		nonDefaultRules = append(nonDefaultRules, rule)
	}
	return nonDefaultRules, nil
}

type resAndSDKListenerRulePair struct {
	resLR *elbv2model.ListenerRule
	sdkLR ListenerRuleWithTags
}

type resLRDesiredActionsAndConditionsPair struct {
	desiredActions    []types.Action
	desiredConditions []types.RuleCondition
}

func (s *listenerRuleSynthesizer) matchResAndSDKListenerRules(unmatchedResLRs []*elbv2model.ListenerRule, unmatchedSDKLRs []ListenerRuleWithTags, resLRDesiredActionsAndConditionsPairs map[*elbv2model.ListenerRule]*resLRDesiredActionsAndConditionsPair) ([]resAndSDKListenerRulePair, []*elbv2model.ListenerRule, []ListenerRuleWithTags, error) {
	var matchedResAndSDKLRsBySettings []resAndSDKListenerRulePair
	var resLRsToCreate []*elbv2model.ListenerRule

	for _, resLR := range unmatchedResLRs {
		resLRDesiredActionsAndConditionsPair := resLRDesiredActionsAndConditionsPairs[resLR]
		found := false
		for i := 0; i < len(unmatchedSDKLRs); i++ {
			sdkLR := unmatchedSDKLRs[i]
			if cmp.Equal(resLRDesiredActionsAndConditionsPair.desiredActions, sdkLR.ListenerRule.Actions, elbv2equality.CompareOptionForActions()) &&
				cmp.Equal(resLRDesiredActionsAndConditionsPair.desiredConditions, sdkLR.ListenerRule.Conditions, elbv2equality.CompareOptionForRuleConditions()) {
				sdkLRPriority, _ := strconv.ParseInt(awssdk.ToString(sdkLR.ListenerRule.Priority), 10, 64)
				if resLR.Spec.Priority != int32(sdkLRPriority) {
					matchedResAndSDKLRsBySettings = append(matchedResAndSDKLRsBySettings, resAndSDKListenerRulePair{
						resLR: resLR,
						sdkLR: sdkLR,
					})
				}
				unmatchedSDKLRs = append(unmatchedSDKLRs[:i], unmatchedSDKLRs[i+1:]...)
				i--
				found = true
				break
			}
		}
		if !found {
			resLRsToCreate = append(resLRsToCreate, resLR)
		}
	}
	return matchedResAndSDKLRsBySettings, resLRsToCreate, unmatchedSDKLRs, nil
}

func mapResListenerRuleByListenerARN(resLRs []*elbv2model.ListenerRule) (map[string][]*elbv2model.ListenerRule, error) {
	resLRsByLSARN := make(map[string][]*elbv2model.ListenerRule, len(resLRs))
	ctx := context.Background()
	for _, lr := range resLRs {
		lsARN, err := lr.Spec.ListenerARN.Resolve(ctx)
		if err != nil {
			return nil, err
		}
		resLRsByLSARN[lsARN] = append(resLRsByLSARN[lsARN], lr)
	}
	return resLRsByLSARN, nil
}
