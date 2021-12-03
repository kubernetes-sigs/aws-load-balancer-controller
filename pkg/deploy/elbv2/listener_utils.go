package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"time"
)

const (
	defaultWaitLSExistencePollInterval = 2 * time.Second
	defaultWaitLSExistenceTimeout      = 20 * time.Second
)

func buildSDKActions(modelActions []elbv2model.Action, featureGates config.FeatureGates) ([]*elbv2sdk.Action, error) {
	var sdkActions []*elbv2sdk.Action
	if len(modelActions) != 0 {
		sdkActions = make([]*elbv2sdk.Action, 0, len(modelActions))
		for index, modelAction := range modelActions {
			sdkAction, err := buildSDKAction(modelAction, featureGates)
			if err != nil {
				return nil, err
			}
			sdkAction.Order = awssdk.Int64(int64(index) + 1)
			sdkActions = append(sdkActions, sdkAction)
		}
	}
	return sdkActions, nil
}

func buildSDKAction(modelAction elbv2model.Action, featureGates config.FeatureGates) (*elbv2sdk.Action, error) {
	sdkObj := &elbv2sdk.Action{}
	sdkObj.Type = awssdk.String(string(modelAction.Type))
	if modelAction.AuthenticateCognitoConfig != nil {
		sdkObj.AuthenticateCognitoConfig = buildSDKAuthenticateCognitoActionConfig(*modelAction.AuthenticateCognitoConfig)
	}
	if modelAction.AuthenticateOIDCConfig != nil {
		sdkObj.AuthenticateOidcConfig = buildSDKAuthenticateOidcActionConfig(*modelAction.AuthenticateOIDCConfig)
	}
	if modelAction.FixedResponseConfig != nil {
		sdkObj.FixedResponseConfig = buildSDKFixedResponseActionConfig(*modelAction.FixedResponseConfig)
	}
	if modelAction.RedirectConfig != nil {
		sdkObj.RedirectConfig = buildSDKRedirectActionConfig(*modelAction.RedirectConfig)
	}
	if modelAction.ForwardConfig != nil {
		forwardConfig, err := buildSDKForwardActionConfig(*modelAction.ForwardConfig)
		if err != nil {
			return nil, err
		}
		if !featureGates.Enabled(config.WeightedTargetGroups) {
			if len(forwardConfig.TargetGroups) == 1 {
				sdkObj.TargetGroupArn = forwardConfig.TargetGroups[0].TargetGroupArn
			} else {
				return nil, errors.New("weighted target groups feature is disabled")
			}
		} else {
			sdkObj.ForwardConfig = forwardConfig
		}
	}
	return sdkObj, nil
}

func buildSDKAuthenticateCognitoActionConfig(modelCfg elbv2model.AuthenticateCognitoActionConfig) *elbv2sdk.AuthenticateCognitoActionConfig {
	return &elbv2sdk.AuthenticateCognitoActionConfig{
		AuthenticationRequestExtraParams: awssdk.StringMap(modelCfg.AuthenticationRequestExtraParams),
		OnUnauthenticatedRequest:         (*string)(modelCfg.OnUnauthenticatedRequest),
		Scope:                            modelCfg.Scope,
		SessionCookieName:                modelCfg.SessionCookieName,
		SessionTimeout:                   modelCfg.SessionTimeout,
		UserPoolArn:                      awssdk.String(modelCfg.UserPoolARN),
		UserPoolClientId:                 awssdk.String(modelCfg.UserPoolClientID),
		UserPoolDomain:                   awssdk.String(modelCfg.UserPoolDomain),
	}
}

func buildSDKAuthenticateOidcActionConfig(modelCfg elbv2model.AuthenticateOIDCActionConfig) *elbv2sdk.AuthenticateOidcActionConfig {
	return &elbv2sdk.AuthenticateOidcActionConfig{
		AuthenticationRequestExtraParams: awssdk.StringMap(modelCfg.AuthenticationRequestExtraParams),
		OnUnauthenticatedRequest:         (*string)(modelCfg.OnUnauthenticatedRequest),
		Scope:                            modelCfg.Scope,
		SessionCookieName:                modelCfg.SessionCookieName,
		SessionTimeout:                   modelCfg.SessionTimeout,
		ClientId:                         awssdk.String(modelCfg.ClientID),
		ClientSecret:                     awssdk.String(modelCfg.ClientSecret),
		Issuer:                           awssdk.String(modelCfg.Issuer),
		AuthorizationEndpoint:            awssdk.String(modelCfg.AuthorizationEndpoint),
		TokenEndpoint:                    awssdk.String(modelCfg.TokenEndpoint),
		UserInfoEndpoint:                 awssdk.String(modelCfg.UserInfoEndpoint),
	}
}

func buildSDKFixedResponseActionConfig(modelCfg elbv2model.FixedResponseActionConfig) *elbv2sdk.FixedResponseActionConfig {
	return &elbv2sdk.FixedResponseActionConfig{
		ContentType: modelCfg.ContentType,
		MessageBody: modelCfg.MessageBody,
		StatusCode:  awssdk.String(modelCfg.StatusCode),
	}
}

func buildSDKRedirectActionConfig(modelCfg elbv2model.RedirectActionConfig) *elbv2sdk.RedirectActionConfig {
	return &elbv2sdk.RedirectActionConfig{
		Host:       modelCfg.Host,
		Path:       modelCfg.Path,
		Port:       modelCfg.Port,
		Protocol:   modelCfg.Protocol,
		Query:      modelCfg.Query,
		StatusCode: awssdk.String(modelCfg.StatusCode),
	}
}

func buildSDKForwardActionConfig(modelCfg elbv2model.ForwardActionConfig) (*elbv2sdk.ForwardActionConfig, error) {
	ctx := context.Background()
	sdkObj := &elbv2sdk.ForwardActionConfig{}
	var tgTuples []*elbv2sdk.TargetGroupTuple
	for _, tgt := range modelCfg.TargetGroups {
		tgARN, err := tgt.TargetGroupARN.Resolve(ctx)
		if err != nil {
			return nil, err
		}
		tgTuples = append(tgTuples, &elbv2sdk.TargetGroupTuple{
			TargetGroupArn: awssdk.String(tgARN),
			Weight:         tgt.Weight,
		})
	}
	sdkObj.TargetGroups = tgTuples
	if modelCfg.TargetGroupStickinessConfig != nil {
		sdkObj.TargetGroupStickinessConfig = &elbv2sdk.TargetGroupStickinessConfig{
			DurationSeconds: modelCfg.TargetGroupStickinessConfig.DurationSeconds,
			Enabled:         modelCfg.TargetGroupStickinessConfig.Enabled,
		}
	}

	return sdkObj, nil
}

func buildSDKRuleConditions(modelConditions []elbv2model.RuleCondition) []*elbv2sdk.RuleCondition {
	var sdkConditions []*elbv2sdk.RuleCondition
	if len(modelConditions) != 0 {
		sdkConditions = make([]*elbv2sdk.RuleCondition, 0, len(modelConditions))
		for _, modelCondition := range modelConditions {
			sdkCondition := buildSDKRuleCondition(modelCondition)
			sdkConditions = append(sdkConditions, sdkCondition)
		}
	}
	return sdkConditions
}

func buildSDKRuleCondition(modelCondition elbv2model.RuleCondition) *elbv2sdk.RuleCondition {
	sdkObj := &elbv2sdk.RuleCondition{}
	sdkObj.Field = awssdk.String(string(modelCondition.Field))
	if modelCondition.HostHeaderConfig != nil {
		sdkObj.HostHeaderConfig = buildSDKHostHeaderConditionConfig(*modelCondition.HostHeaderConfig)
	}
	if modelCondition.HTTPHeaderConfig != nil {
		sdkObj.HttpHeaderConfig = buildSDKHTTPHeaderConditionConfig(*modelCondition.HTTPHeaderConfig)
	}
	if modelCondition.HTTPRequestMethodConfig != nil {
		sdkObj.HttpRequestMethodConfig = buildSDKHTTPRequestMethodConditionConfig(*modelCondition.HTTPRequestMethodConfig)
	}
	if modelCondition.PathPatternConfig != nil {
		sdkObj.PathPatternConfig = buildSDKPathPatternConditionConfig(*modelCondition.PathPatternConfig)
	}
	if modelCondition.QueryStringConfig != nil {
		sdkObj.QueryStringConfig = buildSDKQueryStringConditionConfig(*modelCondition.QueryStringConfig)
	}
	if modelCondition.SourceIPConfig != nil {
		sdkObj.SourceIpConfig = buildSDKSourceIpConditionConfig(*modelCondition.SourceIPConfig)
	}
	return sdkObj
}

func buildSDKHostHeaderConditionConfig(modelCfg elbv2model.HostHeaderConditionConfig) *elbv2sdk.HostHeaderConditionConfig {
	return &elbv2sdk.HostHeaderConditionConfig{
		Values: awssdk.StringSlice(modelCfg.Values),
	}
}

func buildSDKHTTPHeaderConditionConfig(modelCfg elbv2model.HTTPHeaderConditionConfig) *elbv2sdk.HttpHeaderConditionConfig {
	return &elbv2sdk.HttpHeaderConditionConfig{
		HttpHeaderName: awssdk.String(modelCfg.HTTPHeaderName),
		Values:         awssdk.StringSlice(modelCfg.Values),
	}
}

func buildSDKHTTPRequestMethodConditionConfig(modelCfg elbv2model.HTTPRequestMethodConditionConfig) *elbv2sdk.HttpRequestMethodConditionConfig {
	return &elbv2sdk.HttpRequestMethodConditionConfig{
		Values: awssdk.StringSlice(modelCfg.Values),
	}
}

func buildSDKPathPatternConditionConfig(modelCfg elbv2model.PathPatternConditionConfig) *elbv2sdk.PathPatternConditionConfig {
	return &elbv2sdk.PathPatternConditionConfig{
		Values: awssdk.StringSlice(modelCfg.Values),
	}
}

func buildSDKQueryStringConditionConfig(modelCfg elbv2model.QueryStringConditionConfig) *elbv2sdk.QueryStringConditionConfig {
	kvPairs := make([]*elbv2sdk.QueryStringKeyValuePair, 0, len(modelCfg.Values))
	for _, value := range modelCfg.Values {
		kvPairs = append(kvPairs, &elbv2sdk.QueryStringKeyValuePair{
			Key:   value.Key,
			Value: awssdk.String(value.Value),
		})
	}
	return &elbv2sdk.QueryStringConditionConfig{
		Values: kvPairs,
	}
}

func buildSDKSourceIpConditionConfig(modelCfg elbv2model.SourceIPConditionConfig) *elbv2sdk.SourceIpConditionConfig {
	return &elbv2sdk.SourceIpConditionConfig{
		Values: awssdk.StringSlice(modelCfg.Values),
	}
}

func isListenerNotFoundError(err error) bool {
	var awsErr awserr.Error
	if errors.As(err, &awsErr) {
		return awsErr.Code() == "ListenerNotFound"
	}
	return false
}
