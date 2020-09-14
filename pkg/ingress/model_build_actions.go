package ingress

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/algorithm"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
	"strings"
	"unicode"
)

func (b *defaultModelBuilder) buildActions(ctx context.Context, stack core.Stack, ingGroupID GroupID, tgByID map[string]*elbv2model.TargetGroup,
	protocol elbv2model.Protocol, ing *networking.Ingress, backend EnhancedBackend) ([]elbv2model.Action, error) {
	var actions []elbv2model.Action
	if protocol == elbv2model.ProtocolHTTPS {
		authAction, err := b.buildAuthAction(ctx, ing, backend)
		if err != nil {
			return nil, err
		}
		if authAction != nil {
			actions = append(actions, *authAction)
		}
	}
	backendAction, err := b.buildBackendAction(ctx, stack, ingGroupID, tgByID, ing, backend.Action)
	if err != nil {
		return nil, err
	}
	actions = append(actions, backendAction)
	return actions, nil
}

func (b *defaultModelBuilder) buildBackendAction(ctx context.Context, stack core.Stack, ingGroupID GroupID,
	tgByID map[string]*elbv2model.TargetGroup, ing *networking.Ingress, actionCfg Action) (elbv2model.Action, error) {
	switch actionCfg.Type {
	case ActionTypeFixedResponse:
		return b.buildFixedResponseAction(ctx, actionCfg)
	case ActionTypeRedirect:
		return b.buildRedirectAction(ctx, actionCfg)
	case ActionTypeForward:
		return b.buildForwardAction(ctx, stack, ingGroupID, tgByID, ing, actionCfg)
	}
	return elbv2model.Action{}, errors.Errorf("unknown action type: %v", actionCfg.Type)
}

func (b *defaultModelBuilder) buildAuthAction(ctx context.Context, ing *networking.Ingress, backend EnhancedBackend) (*elbv2model.Action, error) {
	// if a single service is used as backend, then it's auth configuration via annotation will take priority than ingress.
	svcAndIngAnnotations := ing.Annotations
	if backend.Action.Type == ActionTypeForward && len(backend.Action.ForwardConfig.TargetGroups) == 1 && backend.Action.ForwardConfig.TargetGroups[0].ServiceName != nil {
		svcName := awssdk.StringValue(backend.Action.ForwardConfig.TargetGroups[0].ServiceName)
		svcKey := types.NamespacedName{
			Namespace: ing.Namespace,
			Name:      svcName,
		}
		svc := &corev1.Service{}
		if err := b.k8sClient.Get(ctx, svcKey, svc); err != nil {
			return nil, err
		}
		svcAndIngAnnotations = algorithm.MergeStringMap(svc.Annotations, svcAndIngAnnotations)
	}
	authCfg, err := b.authConfigBuilder.Build(ctx, svcAndIngAnnotations)
	if err != nil {
		return nil, err
	}
	switch authCfg.Type {
	case AuthTypeCognito:
		action, err := b.buildAuthenticateCognitoAction(ctx, authCfg)
		if err != nil {
			return nil, err
		}
		return &action, nil
	case AuthTypeOIDC:
		action, err := b.buildAuthenticateOIDCAction(ctx, authCfg, ing.Namespace)
		if err != nil {
			return nil, err
		}
		return &action, nil
	default:
		return nil, nil
	}
}

func (b *defaultModelBuilder) buildFixedResponseAction(ctx context.Context, actionCfg Action) (elbv2model.Action, error) {
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

func (b *defaultModelBuilder) buildRedirectAction(ctx context.Context, actionCfg Action) (elbv2model.Action, error) {
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

func (b *defaultModelBuilder) buildForwardAction(ctx context.Context, stack core.Stack, ingGroupID GroupID,
	tgByID map[string]*elbv2model.TargetGroup, ing *networking.Ingress, actionCfg Action) (elbv2model.Action, error) {
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
				Namespace: ing.Namespace,
				Name:      awssdk.StringValue(tgt.ServiceName),
			}
			svc := &corev1.Service{}
			if err := b.k8sClient.Get(ctx, svcKey, svc); err != nil {
				return elbv2model.Action{}, err
			}
			tg, err := b.buildTargetGroup(ctx, stack, ingGroupID, tgByID, ing, svc, *tgt.ServicePort)
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

func (b *defaultModelBuilder) buildAuthenticateCognitoAction(ctx context.Context, authCfg AuthConfig) (elbv2model.Action, error) {
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

func (b *defaultModelBuilder) buildAuthenticateOIDCAction(ctx context.Context, authCfg AuthConfig, namespace string) (elbv2model.Action, error) {
	if authCfg.IDPConfigOIDC == nil {
		return elbv2model.Action{}, errors.New("missing IDPConfigOIDC")
	}
	onUnauthenticatedRequest := elbv2model.AuthenticateOIDCActionConditionalBehavior(authCfg.OnUnauthenticatedRequest)
	secretKey := types.NamespacedName{
		Namespace: namespace,
		Name:      authCfg.IDPConfigOIDC.SecretName,
	}
	secret := &corev1.Secret{}
	if err := b.k8sClient.Get(ctx, secretKey, secret); err != nil {
		return elbv2model.Action{}, err
	}
	clientID := strings.TrimRightFunc(string(secret.Data["clientId"]), unicode.IsSpace)
	clientSecret := string(secret.Data["clientSecret"])
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

func (b *defaultModelBuilder) build404Action(ctx context.Context) elbv2model.Action {
	return elbv2model.Action{
		Type: elbv2model.ActionTypeFixedResponse,
		FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
			ContentType: awssdk.String("text/plain"),
			StatusCode:  "404",
		},
	}
}
