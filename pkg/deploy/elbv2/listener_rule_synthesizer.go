package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/smithy-go"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
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
	// Find existing listener rules on the load balancer
	sdkLRs, err := s.findSDKListenersRulesOnLS(ctx, lsARN)
	if err != nil {
		return err
	}
	// Build desired rule configs for resource listener rules.
	resLRDesiredRuleConfigs := make(map[*elbv2model.ListenerRule]*resLRDesiredRuleConfig, len(resLRs))
	for _, resLR := range resLRs {
		resLRDesiredRuleConfig, err := buildResLRDesiredRuleConfig(resLR, s.featureGates)
		if err != nil {
			return err
		}
		resLRDesiredRuleConfigs[resLR] = resLRDesiredRuleConfig
	}

	// matchedResAndSDKLRsBySettings : A slice of matched resLR and SDKLR rule pairs that have matching settings like actions and conditions. These needs to be only reprioratized to their corresponding priorities
	// matchedResAndSDKLRsByPriority :  A slice of matched resLR and SDKLR rule pairs that have matching priorities but not settings like actions and conditions. These needs to be modified in place to avoid any 503 errors
	// unmatchedResLRs : A slice of resLR that do not have a corresponding match in the sdkLRs. These rules need to be created on the load balancer.
	// unmatchedSDKLRs : A slice of sdkLRs that do not have a corresponding match in the resLRs. These rules need to be first pushed down in the priority so that the new rules are created/modified at higher priority first and then deleted from the load balancer to avoid any 503 errors.
	matchedResAndSDKLRsBySettings, matchedResAndSDKLRsByPriority, unmatchedResLRs, unmatchedSDKLRs, err := s.matchResAndSDKListenerRules(resLRs, sdkLRs, resLRDesiredRuleConfigs)
	if err != nil {
		return err
	}

	s.logger.Info("unmatchedResLRs size", "size", len(unmatchedResLRs))
	for _, resLR := range unmatchedResLRs {
		s.logger.Info("Unmatched", "res lr", *resLR)
	}

	s.logger.Info("unmatchedSDKLRs size", "size", len(unmatchedSDKLRs))
	for _, sdkLr := range unmatchedSDKLRs {
		s.logger.Info("Unmatched", "res lr", *sdkLr.ListenerRule)
	}

	s.logger.Info("match priority size", "size", len(matchedResAndSDKLRsBySettings))
	for k, v := range matchedResAndSDKLRsBySettings {
		s.logger.Info("matched priority", "priority", k, "value", *v.sdkLR.ListenerRule)
	}

	// Re-prioritize matched listener rules.
	if len(matchedResAndSDKLRsBySettings) > 0 {
		err := s.lrManager.SetRulePriorities(ctx, matchedResAndSDKLRsBySettings, unmatchedSDKLRs)
		if err != nil {
			return err
		}
	}
	// Modify rules in place which are matching priorities
	for _, resAndSDKLR := range matchedResAndSDKLRsByPriority {
		lsStatus, err := s.lrManager.UpdateRules(ctx, resAndSDKLR.resLR, resAndSDKLR.sdkLR, resLRDesiredRuleConfigs[resAndSDKLR.resLR])
		if err != nil {
			return err
		}
		resAndSDKLR.resLR.SetStatus(lsStatus)
	}

	err = s.createAndDeleteRules(ctx, len(sdkLRs), resLRDesiredRuleConfigs, unmatchedResLRs, unmatchedSDKLRs)
	if err != nil {
		return err
	}

	// Update existing listener rules on the load balancer for their tags
	for _, resAndSDKLR := range matchedResAndSDKLRsBySettings {
		lsStatus, err := s.lrManager.UpdateRulesTags(ctx, resAndSDKLR.resLR, resAndSDKLR.sdkLR)
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

type resLRDesiredRuleConfig struct {
	desiredActions    []types.Action
	desiredConditions []types.RuleCondition
	desiredTransforms []types.RuleTransform
}

func (s *listenerRuleSynthesizer) matchResAndSDKListenerRules(resLRs []*elbv2model.ListenerRule, unmatchedSDKLRs []ListenerRuleWithTags, resLRDesiredRuleConfigs map[*elbv2model.ListenerRule]*resLRDesiredRuleConfig) ([]resAndSDKListenerRulePair, []resAndSDKListenerRulePair, []*elbv2model.ListenerRule, []ListenerRuleWithTags, error) {
	var matchedResAndSDKLRsBySettings []resAndSDKListenerRulePair
	var matchedResAndSDKLRsByPriority []resAndSDKListenerRulePair
	var unmatchedResLRs []*elbv2model.ListenerRule
	var resLRsToCreate []*elbv2model.ListenerRule
	var sdkLRsToDelete []ListenerRuleWithTags

	for _, resLR := range resLRs {
		resLRDesiredRuleConfig := resLRDesiredRuleConfigs[resLR]
		found := false
		for i := 0; i < len(unmatchedSDKLRs); i++ {
			sdkLR := unmatchedSDKLRs[i]

			actionsEqual := cmp.Equal(resLRDesiredRuleConfig.desiredActions, sdkLR.ListenerRule.Actions, elbv2equality.CompareOptionForActions(resLRDesiredRuleConfig.desiredActions, sdkLR.ListenerRule.Actions))
			conditionsEqual := cmp.Equal(resLRDesiredRuleConfig.desiredConditions, sdkLR.ListenerRule.Conditions, elbv2equality.CompareOptionForRuleConditions(resLRDesiredRuleConfig.desiredConditions, sdkLR.ListenerRule.Conditions))
			transformsEqual := cmp.Equal(resLRDesiredRuleConfig.desiredTransforms, sdkLR.ListenerRule.Transforms, elbv2equality.CompareOptionForTransforms(resLRDesiredRuleConfig.desiredTransforms, sdkLR.ListenerRule.Transforms))
			if actionsEqual && conditionsEqual && transformsEqual {
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
			unmatchedResLRs = append(unmatchedResLRs, resLR)
		}
	}

	resLRByPriority := mapResListenerRuleByPriority(unmatchedResLRs)
	sdkLRByPriority := mapSDKListenerRuleByPriority(unmatchedSDKLRs)
	resLRPriorities := sets.Int32KeySet(resLRByPriority)
	sdkLRPriorities := sets.Int32KeySet(sdkLRByPriority)
	for _, priority := range resLRPriorities.Intersection(sdkLRPriorities).List() {
		resLR := resLRByPriority[priority]
		sdkLR := sdkLRByPriority[priority]
		matchedResAndSDKLRsByPriority = append(matchedResAndSDKLRsByPriority, resAndSDKListenerRulePair{
			resLR: resLR,
			sdkLR: sdkLR,
		})
	}
	for _, priority := range resLRPriorities.Difference(sdkLRPriorities).List() {
		resLRsToCreate = append(resLRsToCreate, resLRByPriority[priority])
	}
	for _, priority := range sdkLRPriorities.Difference(resLRPriorities).List() {
		sdkLRsToDelete = append(sdkLRsToDelete, sdkLRByPriority[priority])
	}
	return matchedResAndSDKLRsBySettings, matchedResAndSDKLRsByPriority, resLRsToCreate, sdkLRsToDelete, nil
}

func (s *listenerRuleSynthesizer) createAndDeleteRules(ctx context.Context, initialRuleCount int, resLRDesiredActionsAndConditionsPairs map[*elbv2model.ListenerRule]*resLRDesiredRuleConfig, unmatchedResLRs []*elbv2model.ListenerRule, unmatchedSDKLRs []ListenerRuleWithTags) error {

	/*
			The Basic idea of the state machine is that we try to create all rules before attempting any deletions. By doing this,
		    we give customers maximum availability which ensures that all customer rules are accounted for at all times on the ELB listener.
			In edge cases, where we have exceeded the maximum rules on a listener, we need to delete rules before adding anymore rules.
		    To give these users maximum availability, we flip-flop between removing / adding rules.
	*/

	// Track where we are in creating / deleting rules

	// resLRIndex is the creation index
	var resLRIndex int

	// sdkLRIndex is the deletion index
	var sdkLRIndex int

	// ruleCounter tracks the number of rules (to our best knowledge) attached to a listener. This number may drift if
	// external entities are modifying the listener too (generally we say this leads to undefined behavior)
	ruleCounter := initialRuleCount

	// maxRules is the number we calculate locally which is the users' maximum allowed rules on a listener. As this is
	// a modifiable limit, we can't hardcode any value. -1 is used as a sentinel value.
	maxRules := -1

	// We should loop while we have stuff to create or delete.
	for len(unmatchedResLRs) > resLRIndex || len(unmatchedSDKLRs) > sdkLRIndex {

		if len(unmatchedResLRs) > resLRIndex && ruleCounter != maxRules {
			// We should prioritize the creation of rules. If we've reached the maximum number of rules on the listener,
			// we need to perform a deletion instead.
			resLR := unmatchedResLRs[resLRIndex]
			lrStatus, err := s.lrManager.Create(ctx, resLR, resLRDesiredActionsAndConditionsPairs[resLR])
			if err != nil {
				// Detect too many rules error, we want this exception to only trigger once.  triggering multiple times
				// either means outside forces are changing listener rules OR we have a bug.
				if isTooManyRulesErr(err) && maxRules == -1 {
					maxRules = ruleCounter
					continue
				}
				return err
			}
			// If we get here, the creation succeeded. We must advance the creation index and update the current number of rules
			// on the listener.
			resLR.SetStatus(lrStatus)
			ruleCounter++
			resLRIndex++
		} else if len(unmatchedSDKLRs) > sdkLRIndex {
			// Deletion is priority #2 to creation.
			sdkLR := unmatchedSDKLRs[sdkLRIndex]
			if err := s.lrManager.Delete(ctx, sdkLR); err != nil {
				return err
			}
			// Getting here means the deletion was successful. We need to advance the deletion index and update the current number of rules
			// on the listener.
			ruleCounter--
			sdkLRIndex++
		} else {
			// Getting here means that the user has requested more rules than what is allowed on their listener.
			return errors.New("Unable to synthesize listener rules, there's too many attached.")
		}

	}
	return nil
}

func mapResListenerRuleByPriority(resLRs []*elbv2model.ListenerRule) map[int32]*elbv2model.ListenerRule {
	resLRByPriority := make(map[int32]*elbv2model.ListenerRule, len(resLRs))
	for _, resLR := range resLRs {
		resLRByPriority[resLR.Spec.Priority] = resLR
	}
	return resLRByPriority
}

func mapSDKListenerRuleByPriority(sdkLRs []ListenerRuleWithTags) map[int32]ListenerRuleWithTags {
	sdkLRByPriority := make(map[int32]ListenerRuleWithTags, len(sdkLRs))
	for _, sdkLR := range sdkLRs {
		priority, _ := strconv.ParseInt(awssdk.ToString(sdkLR.ListenerRule.Priority), 10, 64)
		sdkLRByPriority[int32(priority)] = sdkLR
	}
	return sdkLRByPriority
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

func isTooManyRulesErr(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "TooManyRules"
	}
	return false
}
