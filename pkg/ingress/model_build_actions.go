package ingress

import (
	"context"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"strings"
	"unicode"
)

func (t *defaultModelBuildTask) buildActions(ctx context.Context, protocol elbv2model.Protocol, ing ClassifiedIngress, backend EnhancedBackend) ([]elbv2model.Action, error) {
	var actions []elbv2model.Action
	if protocol == elbv2model.ProtocolHTTPS {
		authAction, err := t.buildAuthAction(ctx, ing.Ing.Namespace, backend)
		if err != nil {
			return nil, err
		}
		if authAction != nil {
			actions = append(actions, *authAction)
		}
	}
	backendAction, err := t.buildBackendAction(ctx, ing, backend.Action)
	if err != nil {
		return nil, err
	}
	actions = append(actions, backendAction)
	return actions, nil
}

func (t *defaultModelBuildTask) buildBackendAction(ctx context.Context, ing ClassifiedIngress, actionCfg Action) (elbv2model.Action, error) {
	switch actionCfg.Type {
	case ActionTypeFixedResponse:
		return t.buildFixedResponseAction(ctx, actionCfg)
	case ActionTypeRedirect:
		return t.buildRedirectAction(ctx, actionCfg)
	case ActionTypeForward:
		return t.buildForwardAction(ctx, ing, actionCfg)
	}
	return elbv2model.Action{}, errors.Errorf("unknown action type: %v", actionCfg.Type)
}

func (t *defaultModelBuildTask) buildAuthAction(ctx context.Context, namespace string, backend EnhancedBackend) (*elbv2model.Action, error) {
	authCfg := backend.AuthConfig
	switch authCfg.Type {
	case AuthTypeCognito:
		action, err := t.buildAuthenticateCognitoAction(ctx, authCfg)
		if err != nil {
			return nil, err
		}
		return &action, nil
	case AuthTypeOIDC:
		action, err := t.buildAuthenticateOIDCAction(ctx, namespace, authCfg)
		if err != nil {
			return nil, err
		}
		return &action, nil
	default:
		return nil, nil
	}
}

func (t *defaultModelBuildTask) buildFixedResponseAction(_ context.Context, actionCfg Action) (elbv2model.Action, error) {
	if actionCfg.FixedResponseConfig == nil {
		return elbv2model.Action{}, errors.New("missing FixedResponseConfig")
	}
	return elbv2model.Action{
		Type: elbv2model.ActionTypeFixedResponse,
		FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
			ContentType: actionCfg.FixedResponseConfig.ContentType,
			MessageBody: actionCfg.FixedResponseConfig.MessageBody,
			StatusCode:  actionCfg.FixedResponseConfig.StatusCode,
		},
	}, nil
}

func (t *defaultModelBuildTask) buildRedirectAction(_ context.Context, actionCfg Action) (elbv2model.Action, error) {
	if actionCfg.RedirectConfig == nil {
		return elbv2model.Action{}, errors.New("missing RedirectConfig")
	}
	return elbv2model.Action{
		Type: elbv2model.ActionTypeRedirect,
		RedirectConfig: &elbv2model.RedirectActionConfig{
			Host:       actionCfg.RedirectConfig.Host,
			Path:       actionCfg.RedirectConfig.Path,
			Port:       actionCfg.RedirectConfig.Port,
			Protocol:   actionCfg.RedirectConfig.Protocol,
			Query:      actionCfg.RedirectConfig.Query,
			StatusCode: actionCfg.RedirectConfig.StatusCode,
		},
	}, nil
}

func (t *defaultModelBuildTask) buildForwardAction(ctx context.Context, ing ClassifiedIngress, actionCfg Action) (elbv2model.Action, error) {
	if actionCfg.ForwardConfig == nil {
		return elbv2model.Action{}, errors.New("missing ForwardConfig")
	}

	var targetGroupTuples []elbv2model.TargetGroupTuple
	for _, tgt := range actionCfg.ForwardConfig.TargetGroups {
		var tgARN core.StringToken
		if tgt.TargetGroupARN != nil {
			tgARN = core.LiteralStringToken(*tgt.TargetGroupARN)
		} else {
			svcKey := types.NamespacedName{
				Namespace: ing.Ing.Namespace,
				Name:      awssdk.StringValue(tgt.ServiceName),
			}
			svc := t.backendServices[svcKey]
			tg, err := t.buildTargetGroup(ctx, ing, svc, *tgt.ServicePort)
			if err != nil {
				return elbv2model.Action{}, err
			}
			tgARN = tg.TargetGroupARN()
		}
		targetGroupTuples = append(targetGroupTuples, elbv2model.TargetGroupTuple{
			TargetGroupARN: tgARN,
			Weight:         tgt.Weight,
		})
	}
	var stickinessCfg *elbv2model.TargetGroupStickinessConfig
	if actionCfg.ForwardConfig.TargetGroupStickinessConfig != nil {
		stickinessCfg = &elbv2model.TargetGroupStickinessConfig{
			Enabled:         actionCfg.ForwardConfig.TargetGroupStickinessConfig.Enabled,
			DurationSeconds: actionCfg.ForwardConfig.TargetGroupStickinessConfig.DurationSeconds,
		}
	}

	return elbv2model.Action{
		Type: elbv2model.ActionTypeForward,
		ForwardConfig: &elbv2model.ForwardActionConfig{
			TargetGroups:                targetGroupTuples,
			TargetGroupStickinessConfig: stickinessCfg,
		},
	}, nil
}

func (t *defaultModelBuildTask) buildAuthenticateCognitoAction(_ context.Context, authCfg AuthConfig) (elbv2model.Action, error) {
	if authCfg.IDPConfigCognito == nil {
		return elbv2model.Action{}, errors.New("missing IDPConfigCognito")
	}
	onUnauthenticatedRequest := elbv2model.AuthenticateCognitoActionConditionalBehavior(authCfg.OnUnauthenticatedRequest)
	return elbv2model.Action{
		Type: elbv2model.ActionTypeAuthenticateCognito,
		AuthenticateCognitoConfig: &elbv2model.AuthenticateCognitoActionConfig{
			UserPoolARN:                      authCfg.IDPConfigCognito.UserPoolARN,
			UserPoolClientID:                 authCfg.IDPConfigCognito.UserPoolClientID,
			UserPoolDomain:                   authCfg.IDPConfigCognito.UserPoolDomain,
			AuthenticationRequestExtraParams: authCfg.IDPConfigCognito.AuthenticationRequestExtraParams,
			OnUnauthenticatedRequest:         &onUnauthenticatedRequest,
			Scope:                            &authCfg.Scope,
			SessionCookieName:                &authCfg.SessionCookieName,
			SessionTimeout:                   &authCfg.SessionTimeout,
		},
	}, nil
}

func (t *defaultModelBuildTask) buildAuthenticateOIDCAction(ctx context.Context, namespace string, authCfg AuthConfig) (elbv2model.Action, error) {
	if authCfg.IDPConfigOIDC == nil {
		return elbv2model.Action{}, errors.New("missing IDPConfigOIDC")
	}
	onUnauthenticatedRequest := elbv2model.AuthenticateOIDCActionConditionalBehavior(authCfg.OnUnauthenticatedRequest)
	secretKey := types.NamespacedName{
		Namespace: namespace,
		Name:      authCfg.IDPConfigOIDC.SecretName,
	}
	secret := &corev1.Secret{}
	if err := t.k8sClient.Get(ctx, secretKey, secret); err != nil {
		return elbv2model.Action{}, err
	}
	rawClientID, ok := secret.Data["clientID"]
	// AWSALBIngressController looks for clientId, we should be backwards-compatible here.
	if !ok {
		rawClientID, ok = secret.Data["clientId"]
	}
	if !ok {
		return elbv2model.Action{}, errors.Errorf("missing clientID, secret: %v", secretKey)
	}
	rawClientSecret, ok := secret.Data["clientSecret"]
	if !ok {
		return elbv2model.Action{}, errors.Errorf("missing clientSecret, secret: %v", secretKey)
	}

	t.secretKeys = append(t.secretKeys, secretKey)
	clientID := strings.TrimRightFunc(string(rawClientID), unicode.IsSpace)
	clientSecret := string(rawClientSecret)
	return elbv2model.Action{
		Type: elbv2model.ActionTypeAuthenticateOIDC,
		AuthenticateOIDCConfig: &elbv2model.AuthenticateOIDCActionConfig{
			Issuer:                           authCfg.IDPConfigOIDC.Issuer,
			AuthorizationEndpoint:            authCfg.IDPConfigOIDC.AuthorizationEndpoint,
			TokenEndpoint:                    authCfg.IDPConfigOIDC.TokenEndpoint,
			UserInfoEndpoint:                 authCfg.IDPConfigOIDC.UserInfoEndpoint,
			ClientID:                         clientID,
			ClientSecret:                     clientSecret,
			AuthenticationRequestExtraParams: authCfg.IDPConfigOIDC.AuthenticationRequestExtraParams,
			OnUnauthenticatedRequest:         &onUnauthenticatedRequest,
			Scope:                            &authCfg.Scope,
			SessionCookieName:                &authCfg.SessionCookieName,
			SessionTimeout:                   &authCfg.SessionTimeout,
		},
	}, nil
}

func (t *defaultModelBuildTask) build404Action(_ context.Context) elbv2model.Action {
	return elbv2model.Action{
		Type: elbv2model.ActionTypeFixedResponse,
		FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
			ContentType: awssdk.String("text/plain"),
			StatusCode:  "404",
		},
	}
}

func (t *defaultModelBuildTask) buildSSLRedirectAction(_ context.Context, sslRedirectConfig SSLRedirectConfig) elbv2model.Action {
	return elbv2model.Action{
		Type: elbv2model.ActionTypeRedirect,
		RedirectConfig: &elbv2model.RedirectActionConfig{
			Port:       awssdk.String(fmt.Sprintf("%v", sslRedirectConfig.SSLPort)),
			Protocol:   awssdk.String(string(elbv2model.ProtocolHTTPS)),
			StatusCode: sslRedirectConfig.StatusCode,
		},
	}
}
