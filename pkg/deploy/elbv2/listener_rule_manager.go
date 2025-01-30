package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
	"slices"
	"strconv"
	"time"
)

// ListenerRuleManager is responsible for create/update/delete ListenerRule resources.
type ListenerRuleManager interface {
	Create(ctx context.Context, resLR *elbv2model.ListenerRule, desiredActionsAndConditions *resLRDesiredActionsAndConditionsPair) (elbv2model.ListenerRuleStatus, error)

	UpdateRules(ctx context.Context, resLR *elbv2model.ListenerRule, sdkLR ListenerRuleWithTags, desiredActionsAndConditions *resLRDesiredActionsAndConditionsPair) (elbv2model.ListenerRuleStatus, error)

	UpdateRulesTags(ctx context.Context, resLR *elbv2model.ListenerRule, sdkLR ListenerRuleWithTags) (elbv2model.ListenerRuleStatus, error)

	Delete(ctx context.Context, sdkLR ListenerRuleWithTags) error

	SetRulePriorities(ctx context.Context, matchedResAndSDKLRsBySettings []resAndSDKListenerRulePair, unmatchedSDKLRs []ListenerRuleWithTags) error
}

// NewDefaultListenerRuleManager constructs new defaultListenerRuleManager.
func NewDefaultListenerRuleManager(elbv2Client services.ELBV2, trackingProvider tracking.Provider,
	taggingManager TaggingManager, externalManagedTags []string, featureGates config.FeatureGates, logger logr.Logger) *defaultListenerRuleManager {
	return &defaultListenerRuleManager{
		elbv2Client:                 elbv2Client,
		trackingProvider:            trackingProvider,
		taggingManager:              taggingManager,
		externalManagedTags:         externalManagedTags,
		featureGates:                featureGates,
		logger:                      logger,
		waitLSExistencePollInterval: defaultWaitLSExistencePollInterval,
		waitLSExistenceTimeout:      defaultWaitLSExistenceTimeout,
	}
}

// default implementation for ListenerRuleManager.
type defaultListenerRuleManager struct {
	elbv2Client         services.ELBV2
	trackingProvider    tracking.Provider
	taggingManager      TaggingManager
	externalManagedTags []string
	featureGates        config.FeatureGates
	logger              logr.Logger

	waitLSExistencePollInterval time.Duration
	waitLSExistenceTimeout      time.Duration
}

func (m *defaultListenerRuleManager) Create(ctx context.Context, resLR *elbv2model.ListenerRule, desiredActionsAndConditions *resLRDesiredActionsAndConditionsPair) (elbv2model.ListenerRuleStatus, error) {
	req, err := buildSDKCreateListenerRuleInput(resLR.Spec, desiredActionsAndConditions, m.featureGates)
	if err != nil {
		return elbv2model.ListenerRuleStatus{}, err
	}
	var ruleTags map[string]string
	if m.featureGates.Enabled(config.ListenerRulesTagging) {
		ruleTags = m.trackingProvider.ResourceTags(resLR.Stack(), resLR, resLR.Spec.Tags)
	}
	req.Tags = convertTagsToSDKTags(ruleTags)

	m.logger.Info("creating listener rule",
		"stackID", resLR.Stack().StackID(),
		"resourceID", resLR.ID())
	var sdkLR ListenerRuleWithTags
	if err := runtime.RetryImmediateOnError(m.waitLSExistencePollInterval, m.waitLSExistenceTimeout, isListenerNotFoundError, func() error {
		resp, err := m.elbv2Client.CreateRuleWithContext(ctx, req)
		if err != nil {
			return err
		}
		sdkLR = ListenerRuleWithTags{
			ListenerRule: &resp.Rules[0],
			Tags:         ruleTags,
		}
		return nil
	}); err != nil {
		return elbv2model.ListenerRuleStatus{}, errors.Wrap(err, "failed to create listener rule")
	}
	m.logger.Info("created listener rule",
		"stackID", resLR.Stack().StackID(),
		"resourceID", resLR.ID(),
		"arn", awssdk.ToString(sdkLR.ListenerRule.RuleArn))

	return buildResListenerRuleStatus(sdkLR), nil
}

func (m *defaultListenerRuleManager) UpdateRulesTags(ctx context.Context, resLR *elbv2model.ListenerRule, sdkLR ListenerRuleWithTags) (elbv2model.ListenerRuleStatus, error) {
	if m.featureGates.Enabled(config.ListenerRulesTagging) {
		if err := m.updateSDKListenerRuleWithTags(ctx, resLR, sdkLR); err != nil {
			return elbv2model.ListenerRuleStatus{}, err
		}
	}
	return buildResListenerRuleStatus(sdkLR), nil
}

func (m *defaultListenerRuleManager) UpdateRules(ctx context.Context, resLR *elbv2model.ListenerRule, sdkLR ListenerRuleWithTags, desiredActionsAndConditions *resLRDesiredActionsAndConditionsPair) (elbv2model.ListenerRuleStatus, error) {
	if err := m.updateSDKListenerRuleWithSettings(ctx, resLR, sdkLR, desiredActionsAndConditions); err != nil {
		return elbv2model.ListenerRuleStatus{}, err
	}
	return buildResListenerRuleStatus(sdkLR), nil
}

func (m *defaultListenerRuleManager) Delete(ctx context.Context, sdkLR ListenerRuleWithTags) error {
	req := &elbv2sdk.DeleteRuleInput{
		RuleArn: sdkLR.ListenerRule.RuleArn,
	}
	m.logger.Info("deleting listener rule",
		"arn", awssdk.ToString(req.RuleArn))
	if _, err := m.elbv2Client.DeleteRuleWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("deleted listener rule",
		"arn", awssdk.ToString(req.RuleArn))
	return nil
}

func (m *defaultListenerRuleManager) SetRulePriorities(ctx context.Context, matchedResAndSDKLRsBySettings []resAndSDKListenerRulePair, unmatchedSDKLRs []ListenerRuleWithTags) error {
	var lastAvailablePriority int32 = 50000
	var sdkLRs []ListenerRuleWithTags

	// Push down all the unmatched existing SDK rules on load balancer so that updated rules can take their place
	for _, sdkLR := range slices.Backward(unmatchedSDKLRs) {
		sdkLR.ListenerRule.Priority = awssdk.String(strconv.Itoa(int(lastAvailablePriority)))
		sdkLRs = append(sdkLRs, sdkLR)
		lastAvailablePriority--
	}
	//Reprioratize matched rules by settings
	for _, resAndSDKLR := range matchedResAndSDKLRsBySettings {
		resAndSDKLR.sdkLR.ListenerRule.Priority = awssdk.String(strconv.Itoa(int(resAndSDKLR.resLR.Spec.Priority)))
		sdkLRs = append(sdkLRs, resAndSDKLR.sdkLR)
	}
	req := buildSDKSetRulePrioritiesInput(sdkLRs)
	m.logger.Info("setting listener rule priorities",
		"rule priority pairs", req.RulePriorities)
	if _, err := m.elbv2Client.SetRulePrioritiesWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("setting listener rule priorities complete",
		"rule priority pairs", req.RulePriorities)
	return nil
}

func (m *defaultListenerRuleManager) updateSDKListenerRuleWithTags(ctx context.Context, resLR *elbv2model.ListenerRule, sdkLR ListenerRuleWithTags) error {
	desiredTags := m.trackingProvider.ResourceTags(resLR.Stack(), resLR, resLR.Spec.Tags)
	return m.taggingManager.ReconcileTags(ctx, awssdk.ToString(sdkLR.ListenerRule.RuleArn), desiredTags,
		WithCurrentTags(sdkLR.Tags),
		WithIgnoredTagKeys(m.externalManagedTags))
}

func (m *defaultListenerRuleManager) updateSDKListenerRuleWithSettings(ctx context.Context, resLR *elbv2model.ListenerRule, sdkLR ListenerRuleWithTags, desiredActionsAndConditions *resLRDesiredActionsAndConditionsPair) error {
	desiredActions := desiredActionsAndConditions.desiredActions
	desiredConditions := desiredActionsAndConditions.desiredConditions

	req := buildSDKModifyListenerRuleInput(resLR.Spec, desiredActions, desiredConditions)
	req.RuleArn = sdkLR.ListenerRule.RuleArn
	m.logger.Info("modifying listener rule",
		"stackID", resLR.Stack().StackID(),
		"resourceID", resLR.ID(),
		"arn", awssdk.ToString(sdkLR.ListenerRule.RuleArn))
	if _, err := m.elbv2Client.ModifyRuleWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("modified listener rule",
		"stackID", resLR.Stack().StackID(),
		"resourceID", resLR.ID(),
		"arn", awssdk.ToString(sdkLR.ListenerRule.RuleArn))
	return nil
}

func buildSDKCreateListenerRuleInput(lrSpec elbv2model.ListenerRuleSpec, desiredActionsAndConditions *resLRDesiredActionsAndConditionsPair, featureGates config.FeatureGates) (*elbv2sdk.CreateRuleInput, error) {
	ctx := context.Background()
	lsARN, err := lrSpec.ListenerARN.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	sdkObj := &elbv2sdk.CreateRuleInput{}
	sdkObj.ListenerArn = awssdk.String(lsARN)
	sdkObj.Priority = awssdk.Int32(lrSpec.Priority)
	if desiredActionsAndConditions != nil && desiredActionsAndConditions.desiredActions != nil {
		sdkObj.Actions = desiredActionsAndConditions.desiredActions
	} else {
		actions, err := buildSDKActions(lrSpec.Actions, featureGates)
		if err != nil {
			return nil, err
		}
		sdkObj.Actions = actions
	}
	if desiredActionsAndConditions != nil && desiredActionsAndConditions.desiredConditions != nil {
		sdkObj.Conditions = desiredActionsAndConditions.desiredConditions
	} else {
		sdkObj.Conditions = buildSDKRuleConditions(lrSpec.Conditions)
	}
	return sdkObj, nil
}

func buildSDKModifyListenerRuleInput(_ elbv2model.ListenerRuleSpec, desiredActions []elbv2types.Action, desiredConditions []elbv2types.RuleCondition) *elbv2sdk.ModifyRuleInput {
	sdkObj := &elbv2sdk.ModifyRuleInput{}
	sdkObj.Actions = desiredActions
	sdkObj.Conditions = desiredConditions
	return sdkObj
}

func buildSDKSetRulePrioritiesInput(sdkLRs []ListenerRuleWithTags) *elbv2sdk.SetRulePrioritiesInput {
	var rulePriorities []elbv2types.RulePriorityPair
	for _, sdkLR := range sdkLRs {
		p, _ := strconv.ParseInt(awssdk.ToString(sdkLR.ListenerRule.Priority), 10, 32)
		rulePriorityPair := elbv2types.RulePriorityPair{
			RuleArn:  sdkLR.ListenerRule.RuleArn,
			Priority: awssdk.Int32(int32(p)),
		}
		rulePriorities = append(rulePriorities, rulePriorityPair)
	}
	sdkObj := &elbv2sdk.SetRulePrioritiesInput{
		RulePriorities: rulePriorities,
	}
	return sdkObj
}
func buildResListenerRuleStatus(sdkLR ListenerRuleWithTags) elbv2model.ListenerRuleStatus {
	return elbv2model.ListenerRuleStatus{
		RuleARN: awssdk.ToString(sdkLR.ListenerRule.RuleArn),
	}
}
