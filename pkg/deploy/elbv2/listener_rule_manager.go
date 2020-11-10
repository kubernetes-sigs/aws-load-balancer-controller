package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	elbv2equality "sigs.k8s.io/aws-load-balancer-controller/pkg/equality/elbv2"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
	"time"
)

// ListenerRuleManager is responsible for create/update/delete ListenerRule resources.
type ListenerRuleManager interface {
	Create(ctx context.Context, resLR *elbv2model.ListenerRule) (elbv2model.ListenerRuleStatus, error)

	Update(ctx context.Context, resLR *elbv2model.ListenerRule, sdkLR *elbv2sdk.Rule) (elbv2model.ListenerRuleStatus, error)

	Delete(ctx context.Context, sdkLR *elbv2sdk.Rule) error
}

// NewDefaultListenerRuleManager constructs new defaultListenerRuleManager.
func NewDefaultListenerRuleManager(elbv2Client services.ELBV2, logger logr.Logger) *defaultListenerRuleManager {
	return &defaultListenerRuleManager{
		elbv2Client:                 elbv2Client,
		logger:                      logger,
		waitLSExistencePollInterval: defaultWaitLSExistencePollInterval,
		waitLSExistenceTimeout:      defaultWaitLSExistenceTimeout,
	}
}

// default implementation for ListenerRuleManager.
type defaultListenerRuleManager struct {
	elbv2Client services.ELBV2
	logger      logr.Logger

	waitLSExistencePollInterval time.Duration
	waitLSExistenceTimeout      time.Duration
}

func (m *defaultListenerRuleManager) Create(ctx context.Context, resLR *elbv2model.ListenerRule) (elbv2model.ListenerRuleStatus, error) {
	req, err := buildSDKCreateListenerRuleInput(resLR.Spec)
	if err != nil {
		return elbv2model.ListenerRuleStatus{}, err
	}

	m.logger.Info("creating listener rule",
		"stackID", resLR.Stack().StackID(),
		"resourceID", resLR.ID())
	var sdkLR *elbv2sdk.Rule
	if err := runtime.RetryImmediateOnError(m.waitLSExistencePollInterval, m.waitLSExistenceTimeout, isListenerNotFoundError, func() error {
		resp, err := m.elbv2Client.CreateRuleWithContext(ctx, req)
		if err != nil {
			return err
		}
		sdkLR = resp.Rules[0]
		return nil
	}); err != nil {
		return elbv2model.ListenerRuleStatus{}, errors.Wrap(err, "failed to create listener rule")
	}
	m.logger.Info("created listener rule",
		"stackID", resLR.Stack().StackID(),
		"resourceID", resLR.ID(),
		"arn", awssdk.StringValue(sdkLR.RuleArn))

	return buildResListenerRuleStatus(sdkLR), nil
}

func (m *defaultListenerRuleManager) Update(ctx context.Context, resLR *elbv2model.ListenerRule, sdkLR *elbv2sdk.Rule) (elbv2model.ListenerRuleStatus, error) {
	if err := m.updateSDKListenerRuleWithSettings(ctx, resLR, sdkLR); err != nil {
		return elbv2model.ListenerRuleStatus{}, err
	}
	return buildResListenerRuleStatus(sdkLR), nil
}

func (m *defaultListenerRuleManager) Delete(ctx context.Context, sdkLR *elbv2sdk.Rule) error {
	req := &elbv2sdk.DeleteRuleInput{
		RuleArn: sdkLR.RuleArn,
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

func (m *defaultListenerRuleManager) updateSDKListenerRuleWithSettings(ctx context.Context, resLR *elbv2model.ListenerRule, sdkLR *elbv2sdk.Rule) error {
	desiredActions, err := buildSDKActions(resLR.Spec.Actions)
	if err != nil {
		return err
	}
	desiredConditions := buildSDKRuleConditions(resLR.Spec.Conditions)
	if !isSDKListenerRuleSettingsDrifted(resLR.Spec, sdkLR, desiredActions, desiredConditions) {
		return nil
	}

	req := buildSDKModifyListenerRuleInput(resLR.Spec, desiredActions, desiredConditions)
	req.RuleArn = sdkLR.RuleArn
	m.logger.Info("modifying listener rule",
		"stackID", resLR.Stack().StackID(),
		"resourceID", resLR.ID(),
		"arn", awssdk.StringValue(sdkLR.RuleArn))
	if _, err := m.elbv2Client.ModifyRuleWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("modified listener rule",
		"stackID", resLR.Stack().StackID(),
		"resourceID", resLR.ID(),
		"arn", awssdk.StringValue(sdkLR.RuleArn))
	return nil
}

func isSDKListenerRuleSettingsDrifted(lrSpec elbv2model.ListenerRuleSpec, sdkLR *elbv2sdk.Rule,
	desiredActions []*elbv2sdk.Action, desiredConditions []*elbv2sdk.RuleCondition) bool {

	if !cmp.Equal(desiredActions, sdkLR.Actions, elbv2equality.CompareOptionForActions()) {
		return true
	}
	if !cmp.Equal(desiredConditions, sdkLR.Conditions, elbv2equality.CompareOptionForRuleConditions()) {
		return true
	}

	return false
}

func buildSDKCreateListenerRuleInput(lrSpec elbv2model.ListenerRuleSpec) (*elbv2sdk.CreateRuleInput, error) {
	ctx := context.Background()
	lsARN, err := lrSpec.ListenerARN.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	sdkObj := &elbv2sdk.CreateRuleInput{}
	sdkObj.ListenerArn = awssdk.String(lsARN)
	sdkObj.Priority = awssdk.Int64(lrSpec.Priority)
	actions, err := buildSDKActions(lrSpec.Actions)
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

func buildResListenerRuleStatus(sdkLR *elbv2sdk.Rule) elbv2model.ListenerRuleStatus {
	return elbv2model.ListenerRuleStatus{
		RuleARN: awssdk.StringValue(sdkLR.RuleArn),
	}
}
