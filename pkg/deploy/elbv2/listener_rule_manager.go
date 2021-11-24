package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	elbv2equality "sigs.k8s.io/aws-load-balancer-controller/pkg/equality/elbv2"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
	"time"
)

// ListenerRuleManager is responsible for create/update/delete ListenerRule resources.
type ListenerRuleManager interface {
	Create(ctx context.Context, resLR *elbv2model.ListenerRule) (elbv2model.ListenerRuleStatus, error)

	Update(ctx context.Context, resLR *elbv2model.ListenerRule, sdkLR ListenerRuleWithTags) (elbv2model.ListenerRuleStatus, error)

	Delete(ctx context.Context, sdkLR ListenerRuleWithTags) error
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

func (m *defaultListenerRuleManager) Create(ctx context.Context, resLR *elbv2model.ListenerRule) (elbv2model.ListenerRuleStatus, error) {
	req, err := buildSDKCreateListenerRuleInput(resLR.Spec, m.featureGates)
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
			ListenerRule: resp.Rules[0],
			Tags:         ruleTags,
		}
		return nil
	}); err != nil {
		return elbv2model.ListenerRuleStatus{}, errors.Wrap(err, "failed to create listener rule")
	}
	m.logger.Info("created listener rule",
		"stackID", resLR.Stack().StackID(),
		"resourceID", resLR.ID(),
		"arn", awssdk.StringValue(sdkLR.ListenerRule.RuleArn))

	return buildResListenerRuleStatus(sdkLR), nil
}

func (m *defaultListenerRuleManager) Update(ctx context.Context, resLR *elbv2model.ListenerRule, sdkLR ListenerRuleWithTags) (elbv2model.ListenerRuleStatus, error) {
	if m.featureGates.Enabled(config.ListenerRulesTagging) {
		if err := m.updateSDKListenerRuleWithTags(ctx, resLR, sdkLR); err != nil {
			return elbv2model.ListenerRuleStatus{}, err
		}
	}
	if err := m.updateSDKListenerRuleWithSettings(ctx, resLR, sdkLR); err != nil {
		return elbv2model.ListenerRuleStatus{}, err
	}
	return buildResListenerRuleStatus(sdkLR), nil
}

func (m *defaultListenerRuleManager) Delete(ctx context.Context, sdkLR ListenerRuleWithTags) error {
	req := &elbv2sdk.DeleteRuleInput{
		RuleArn: sdkLR.ListenerRule.RuleArn,
	}
	m.logger.Info("deleting listener rule",
		"arn", awssdk.StringValue(req.RuleArn))
	if _, err := m.elbv2Client.DeleteRuleWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("deleted listener rule",
		"arn", awssdk.StringValue(req.RuleArn))
	return nil
}

func (m *defaultListenerRuleManager) updateSDKListenerRuleWithSettings(ctx context.Context, resLR *elbv2model.ListenerRule, sdkLR ListenerRuleWithTags) error {
	desiredActions, err := buildSDKActions(resLR.Spec.Actions, m.featureGates)
	if err != nil {
		return err
	}
	desiredConditions := buildSDKRuleConditions(resLR.Spec.Conditions)
	if !isSDKListenerRuleSettingsDrifted(resLR.Spec, sdkLR, desiredActions, desiredConditions) {
		return nil
	}

	req := buildSDKModifyListenerRuleInput(resLR.Spec, desiredActions, desiredConditions)
	req.RuleArn = sdkLR.ListenerRule.RuleArn
	m.logger.Info("modifying listener rule",
		"stackID", resLR.Stack().StackID(),
		"resourceID", resLR.ID(),
		"arn", awssdk.StringValue(sdkLR.ListenerRule.RuleArn))
	if _, err := m.elbv2Client.ModifyRuleWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("modified listener rule",
		"stackID", resLR.Stack().StackID(),
		"resourceID", resLR.ID(),
		"arn", awssdk.StringValue(sdkLR.ListenerRule.RuleArn))
	return nil
}

func (m *defaultListenerRuleManager) updateSDKListenerRuleWithTags(ctx context.Context, resLR *elbv2model.ListenerRule, sdkLR ListenerRuleWithTags) error {
	desiredTags := m.trackingProvider.ResourceTags(resLR.Stack(), resLR, resLR.Spec.Tags)
	return m.taggingManager.ReconcileTags(ctx, awssdk.StringValue(sdkLR.ListenerRule.RuleArn), desiredTags,
		WithCurrentTags(sdkLR.Tags),
		WithIgnoredTagKeys(m.externalManagedTags))
}

func isSDKListenerRuleSettingsDrifted(lrSpec elbv2model.ListenerRuleSpec, sdkLR ListenerRuleWithTags,
	desiredActions []*elbv2sdk.Action, desiredConditions []*elbv2sdk.RuleCondition) bool {

	if !cmp.Equal(desiredActions, sdkLR.ListenerRule.Actions, elbv2equality.CompareOptionForActions()) {
		return true
	}
	if !cmp.Equal(desiredConditions, sdkLR.ListenerRule.Conditions, elbv2equality.CompareOptionForRuleConditions()) {
		return true
	}

	return false
}

func buildSDKCreateListenerRuleInput(lrSpec elbv2model.ListenerRuleSpec, featureGates config.FeatureGates) (*elbv2sdk.CreateRuleInput, error) {
	ctx := context.Background()
	lsARN, err := lrSpec.ListenerARN.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	sdkObj := &elbv2sdk.CreateRuleInput{}
	sdkObj.ListenerArn = awssdk.String(lsARN)
	sdkObj.Priority = awssdk.Int64(lrSpec.Priority)
	actions, err := buildSDKActions(lrSpec.Actions, featureGates)
	if err != nil {
		return nil, err
	}
	sdkObj.Actions = actions
	sdkObj.Conditions = buildSDKRuleConditions(lrSpec.Conditions)
	return sdkObj, nil
}

func buildSDKModifyListenerRuleInput(_ elbv2model.ListenerRuleSpec, desiredActions []*elbv2sdk.Action, desiredConditions []*elbv2sdk.RuleCondition) *elbv2sdk.ModifyRuleInput {
	sdkObj := &elbv2sdk.ModifyRuleInput{}
	sdkObj.Actions = desiredActions
	sdkObj.Conditions = desiredConditions
	return sdkObj
}

func buildResListenerRuleStatus(sdkLR ListenerRuleWithTags) elbv2model.ListenerRuleStatus {
	return elbv2model.ListenerRuleStatus{
		RuleARN: awssdk.StringValue(sdkLR.ListenerRule.RuleArn),
	}
}
