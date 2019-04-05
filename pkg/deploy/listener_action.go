package deploy

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/pkg/errors"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/build"
)

func (a *loadBalancerActuator) buildELBV2ListenerActions(ctx context.Context, actions []api.ListenerAction) ([]*elbv2.Action, error) {
	result := make([]*elbv2.Action, 0, len(actions))
	for index, action := range actions {
		elbv2Action, err := a.buildELBV2ListenerAction(ctx, action)
		if err != nil {
			return nil, err
		}
		order := int64(index + 1)
		elbv2Action.Order = aws.Int64(order)
		result = append(result, elbv2Action)
	}
	return result, nil
}

func (a *loadBalancerActuator) buildELBV2ListenerAction(ctx context.Context, action api.ListenerAction) (*elbv2.Action, error) {
	switch action.Type {
	case api.ListenerActionTypeAuthenticateCognito:
		return &elbv2.Action{
			Type: aws.String(elbv2.ActionTypeEnumAuthenticateCognito),
			AuthenticateCognitoConfig: &elbv2.AuthenticateCognitoActionConfig{
				AuthenticationRequestExtraParams: aws.StringMap(action.AuthenticateCognito.AuthenticationRequestExtraParams),
				OnUnauthenticatedRequest:         aws.String(action.AuthenticateCognito.OnUnauthenticatedRequest.String()),
				Scope:                            aws.String(action.AuthenticateCognito.Scope),
				SessionCookieName:                aws.String(action.AuthenticateCognito.SessionCookieName),
				SessionTimeout:                   aws.Int64(action.AuthenticateCognito.SessionTimeout),
				UserPoolArn:                      aws.String(action.AuthenticateCognito.UserPoolARN),
				UserPoolClientId:                 aws.String(action.AuthenticateCognito.UserPoolClientID),
				UserPoolDomain:                   aws.String(action.AuthenticateCognito.UserPoolDomain),
			},
		}, nil
	case api.ListenerActionTypeAuthenticateOIDC:
		return &elbv2.Action{
			Type: aws.String(elbv2.ActionTypeEnumAuthenticateOidc),
			AuthenticateOidcConfig: &elbv2.AuthenticateOidcActionConfig{
				AuthenticationRequestExtraParams: aws.StringMap(action.AuthenticateOIDC.AuthenticationRequestExtraParams),
				OnUnauthenticatedRequest:         aws.String(action.AuthenticateOIDC.OnUnauthenticatedRequest.String()),
				Scope:                            aws.String(action.AuthenticateOIDC.Scope),
				SessionCookieName:                aws.String(action.AuthenticateOIDC.SessionCookieName),
				SessionTimeout:                   aws.Int64(action.AuthenticateOIDC.SessionTimeout),
				Issuer:                           aws.String(action.AuthenticateOIDC.Issuer),
				AuthorizationEndpoint:            aws.String(action.AuthenticateOIDC.AuthorizationEndpoint),
				TokenEndpoint:                    aws.String(action.AuthenticateOIDC.TokenEndpoint),
				UserInfoEndpoint:                 aws.String(action.AuthenticateOIDC.UserInfoEndpoint),
				ClientId:                         aws.String(action.AuthenticateOIDC.ClientID),
				ClientSecret:                     aws.String(action.AuthenticateOIDC.ClientSecret),
			},
		}, nil
	case api.ListenerActionTypeFixedResponse:
		return &elbv2.Action{
			Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
			FixedResponseConfig: &elbv2.FixedResponseActionConfig{
				ContentType: aws.String(action.FixedResponse.ContentType),
				MessageBody: aws.String(action.FixedResponse.MessageBody),
				StatusCode:  aws.String(action.FixedResponse.StatusCode),
			},
		}, nil
	case api.ListenerActionTypeRedirect:
		return &elbv2.Action{
			Type: aws.String(elbv2.ActionTypeEnumRedirect),
			RedirectConfig: &elbv2.RedirectActionConfig{
				Host:       aws.String(action.Redirect.Host),
				Path:       aws.String(action.Redirect.Path),
				Port:       aws.String(action.Redirect.Port),
				Protocol:   aws.String(action.Redirect.Protocol),
				Query:      aws.String(action.Redirect.Query),
				StatusCode: aws.String(action.Redirect.StatusCode),
			},
		}, nil
	case api.ListenerActionTypeForward:
		tgArn, err := resolveTargetGroupReference(ctx, a.stack, action.Forward.TargetGroup)
		if err != nil {
			return nil, err
		}
		return &elbv2.Action{
			Type:           aws.String(elbv2.ActionTypeEnumForward),
			TargetGroupArn: aws.String(tgArn),
		}, nil
	}
	return nil, errors.Errorf("unknown action type: %v", action.Type)
}

func resolveTargetGroupReference(ctx context.Context, stack *build.LoadBalancingStack, tgRef api.TargetGroupReference) (string, error) {
	if len(tgRef.TargetGroupARN) != 0 {
		return tgRef.TargetGroupARN, nil
	}
	tg, exists := stack.FindTargetGroup(tgRef.TargetGroupRef.Name)
	// should never happen under current code
	if !exists || len(tg.Status.ARN) == 0 {
		return "", errors.Errorf("failed to resolve targetGroup: %v", tgRef.TargetGroupRef.Name)
	}
	return tg.Status.ARN, nil
}
