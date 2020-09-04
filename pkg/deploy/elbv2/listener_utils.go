package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	elbv2model "sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
)

func buildSDKActions(modelActions []elbv2model.Action) ([]*elbv2sdk.Action, error) {
	var sdkActions []*elbv2sdk.Action
	if len(modelActions) != 0 {
		sdkActions = make([]*elbv2sdk.Action, 0, len(modelActions))
		for index, modelAction := range modelActions {
			sdkAction, err := buildSDKAction(modelAction)
			sdkAction.Order = awssdk.Int64(int64(index) + 1)
			if err != nil {
				return nil, err
			}
			sdkActions = append(sdkActions, sdkAction)
		}
	}
	return sdkActions, nil
}

func buildSDKAction(modelAction elbv2model.Action) (*elbv2sdk.Action, error) {
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
		sdkObj.ForwardConfig = forwardConfig
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
