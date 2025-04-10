package ingress

import (
	"context"

	"github.com/pkg/errors"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
)

const (
	defaultAuthType                     = AuthTypeNone
	defaultAuthScope                    = "openid"
	defaultAuthSessionCookieName        = "AWSELBAuthSessionCookie"
	defaultAuthSessionTimeout           = 604800
	defaultAuthOnUnauthenticatedRequest = "authenticate"
)

// Auth config for Service / Ingresses
type AuthConfig struct {
	Type                     AuthType
	IDPConfigCognito         *AuthIDPConfigCognito
	IDPConfigOIDC            *AuthIDPConfigOIDC
	OnUnauthenticatedRequest string
	Scope                    string
	SessionCookieName        string
	SessionTimeout           int64
}

// AuthConfig builder can build auth configuration for service or ingresses.
type AuthConfigBuilder interface {
	Build(ctx context.Context, ingressClassParams *elbv2api.IngressClassParams, svcAndIngAnnotations map[string]string) (AuthConfig, error)
}

// NewDefaultAuthConfigBuilder constructs new defaultAuthConfigBuilder.
func NewDefaultAuthConfigBuilder(annotationParser annotations.Parser) *defaultAuthConfigBuilder {
	return &defaultAuthConfigBuilder{
		annotationParser: annotationParser,
	}
}

var _ AuthConfigBuilder = &defaultAuthConfigBuilder{}

// Default implementation for AuthConfigBuilder
type defaultAuthConfigBuilder struct {
	annotationParser annotations.Parser
}

func (b *defaultAuthConfigBuilder) Build(ctx context.Context, ingressClassParams *elbv2api.IngressClassParams, svcAndIngAnnotations map[string]string) (AuthConfig, error) {
	err := b.validateIngressClassParamsAuthConfig(ingressClassParams)
	if err != nil {
		return AuthConfig{}, err
	}

	authType, err := b.buildAuthType(ctx, ingressClassParams, svcAndIngAnnotations)
	if err != nil {
		return AuthConfig{}, err
	}
	authOnUnauthenticatedRequest := b.buildAuthOnUnauthenticatedRequest(ctx, ingressClassParams, svcAndIngAnnotations)
	authScope := b.buildAuthScope(ctx, ingressClassParams, svcAndIngAnnotations)
	authSessionCookieName := b.buildAuthSessionCookieName(ctx, ingressClassParams, svcAndIngAnnotations)
	authSessionTimeout, err := b.buildAuthSessionTimeout(ctx, ingressClassParams, svcAndIngAnnotations)
	if err != nil {
		return AuthConfig{}, err
	}
	authIDPCognito, err := b.buildAuthIDPConfigCognito(ctx, ingressClassParams, svcAndIngAnnotations)
	if err != nil {
		return AuthConfig{}, err
	}
	authIDPOIDC, err := b.buildAuthIDPConfigOIDC(ctx, ingressClassParams, svcAndIngAnnotations)
	if err != nil {
		return AuthConfig{}, err
	}

	authConfig := AuthConfig{
		Type:                     authType,
		OnUnauthenticatedRequest: authOnUnauthenticatedRequest,
		Scope:                    authScope,
		SessionCookieName:        authSessionCookieName,
		SessionTimeout:           authSessionTimeout,
		IDPConfigOIDC:            authIDPOIDC,
		IDPConfigCognito:         authIDPCognito,
	}

	return authConfig, nil
}

func (b *defaultAuthConfigBuilder) validateIngressClassParamsAuthConfig(ingressClassParams *elbv2api.IngressClassParams) error {
	if ingressClassParams == nil || ingressClassParams.Spec.AuthConfig == nil {
		return nil
	}

	// Verify that idpCognitoConfiguration exists when auth type is "cognito"
	if string(ingressClassParams.Spec.AuthConfig.Type) == string(AuthTypeCognito) && ingressClassParams.Spec.AuthConfig.IDPConfigCognito == nil {
		return errors.Errorf("idpCognitoConfiguration is required when authenticationConfiguration type is %s", string(AuthTypeCognito))
	}

	// Verify that idpOidcConfiguration exists when auth type is "oidc"
	if string(ingressClassParams.Spec.AuthConfig.Type) == string(AuthTypeOIDC) && ingressClassParams.Spec.AuthConfig.IDPConfigOIDC == nil {
		return errors.Errorf("idpOidcConfiguration is required when authenticationConfiguration type is %s", string(AuthTypeOIDC))
	}

	return nil
}

// AuthConfig precedence rules:
// In the following functions, the AuthConfig in IngressClassParams takes precedence if specified.
// Otherwise, the Ingress annotations are used.

func (b *defaultAuthConfigBuilder) buildAuthType(_ context.Context, ingressClassParams *elbv2api.IngressClassParams, svcAndIngAnnotations map[string]string) (AuthType, error) {
	rawAuthType := string(defaultAuthType)

	if ingressClassParams != nil && ingressClassParams.Spec.AuthConfig != nil {
		rawAuthType = string(ingressClassParams.Spec.AuthConfig.Type)
	} else {
		_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixAuthType, &rawAuthType, svcAndIngAnnotations)
	}

	switch rawAuthType {
	case string(AuthTypeCognito):
		return AuthTypeCognito, nil
	case string(AuthTypeOIDC):
		return AuthTypeOIDC, nil
	case string(AuthTypeNone):
		return AuthTypeNone, nil
	default:
		return "", errors.Errorf("unknown authType: %v", rawAuthType)
	}
}

func (b *defaultAuthConfigBuilder) buildAuthIDPConfigCognito(_ context.Context, ingressClassParams *elbv2api.IngressClassParams, svcAndIngAnnotations map[string]string) (*AuthIDPConfigCognito, error) {
	// If using ingressClassParams authenticationConfiguration, only build if AuthType is "cognito"
	if ingressClassParams != nil && ingressClassParams.Spec.AuthConfig != nil && ingressClassParams.Spec.AuthConfig.Type != elbv2api.AuthTypeCognito {
		return nil, nil
	}

	authIDP := AuthIDPConfigCognito{}

	if ingressClassParams != nil && ingressClassParams.Spec.AuthConfig != nil && ingressClassParams.Spec.AuthConfig.IDPConfigCognito != nil {
		config := ingressClassParams.Spec.AuthConfig.IDPConfigCognito

		authIDP = AuthIDPConfigCognito{
			UserPoolARN:                      config.UserPoolARN,
			UserPoolClientID:                 config.UserPoolClientID,
			UserPoolDomain:                   config.UserPoolDomain,
			AuthenticationRequestExtraParams: config.AuthenticationRequestExtraParams,
		}

		return &authIDP, nil
	}

	exists, err := b.annotationParser.ParseJSONAnnotation(annotations.IngressSuffixAuthIDPCognito, &authIDP, svcAndIngAnnotations)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	return &authIDP, nil
}

func (b *defaultAuthConfigBuilder) buildAuthIDPConfigOIDC(_ context.Context, ingressClassParams *elbv2api.IngressClassParams, svcAndIngAnnotations map[string]string) (*AuthIDPConfigOIDC, error) {
	// When using ingressClassParams authenticationConfiguration, only build if AuthType is "oidc"
	if ingressClassParams != nil && ingressClassParams.Spec.AuthConfig != nil && ingressClassParams.Spec.AuthConfig.Type != elbv2api.AuthTypeOIDC {
		return nil, nil
	}

	if ingressClassParams != nil && ingressClassParams.Spec.AuthConfig != nil && ingressClassParams.Spec.AuthConfig.IDPConfigOIDC != nil {
		config := ingressClassParams.Spec.AuthConfig.IDPConfigOIDC
		authIDP := AuthIDPConfigOIDC{
			Issuer:                           config.Issuer,
			AuthorizationEndpoint:            config.AuthorizationEndpoint,
			TokenEndpoint:                    config.TokenEndpoint,
			UserInfoEndpoint:                 config.UserInfoEndpoint,
			SecretName:                       config.SecretName,
			AuthenticationRequestExtraParams: config.AuthenticationRequestExtraParams,
		}
		return &authIDP, nil
	}

	authIDP := AuthIDPConfigOIDC{}
	exists, err := b.annotationParser.ParseJSONAnnotation(annotations.IngressSuffixAuthIDPOIDC, &authIDP, svcAndIngAnnotations)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	return &authIDP, nil
}

func (b *defaultAuthConfigBuilder) buildAuthOnUnauthenticatedRequest(_ context.Context, ingressClassParams *elbv2api.IngressClassParams, svcAndIngAnnotations map[string]string) string {
	if ingressClassParams != nil && ingressClassParams.Spec.AuthConfig != nil {
		if ingressClassParams.Spec.AuthConfig.Type == elbv2api.AuthTypeNone || ingressClassParams.Spec.AuthConfig.OnUnauthenticatedRequest == "" {
			return defaultAuthOnUnauthenticatedRequest
		}

		return ingressClassParams.Spec.AuthConfig.OnUnauthenticatedRequest
	}

	rawOnUnauthenticatedRequest := defaultAuthOnUnauthenticatedRequest
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixAuthOnUnauthenticatedRequest, &rawOnUnauthenticatedRequest, svcAndIngAnnotations)
	return rawOnUnauthenticatedRequest
}

func (b *defaultAuthConfigBuilder) buildAuthScope(_ context.Context, ingressClassParams *elbv2api.IngressClassParams, svcAndIngAnnotations map[string]string) string {
	if ingressClassParams != nil && ingressClassParams.Spec.AuthConfig != nil {
		if ingressClassParams.Spec.AuthConfig.Type == elbv2api.AuthTypeNone || ingressClassParams.Spec.AuthConfig.Scope == "" {
			return defaultAuthScope
		}

		return ingressClassParams.Spec.AuthConfig.Scope
	}

	rawAuthScope := defaultAuthScope
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixAuthScope, &rawAuthScope, svcAndIngAnnotations)
	return rawAuthScope
}

func (b *defaultAuthConfigBuilder) buildAuthSessionCookieName(_ context.Context, ingressClassParams *elbv2api.IngressClassParams, svcAndIngAnnotations map[string]string) string {
	if ingressClassParams != nil && ingressClassParams.Spec.AuthConfig != nil {
		if ingressClassParams.Spec.AuthConfig.Type == elbv2api.AuthTypeNone || ingressClassParams.Spec.AuthConfig.SessionCookieName == "" {
			return defaultAuthSessionCookieName
		}

		return ingressClassParams.Spec.AuthConfig.SessionCookieName
	}

	rawAuthSessionCookieName := defaultAuthSessionCookieName
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixAuthSessionCookie, &rawAuthSessionCookieName, svcAndIngAnnotations)
	return rawAuthSessionCookieName
}

func (b *defaultAuthConfigBuilder) buildAuthSessionTimeout(_ context.Context, ingressClassParams *elbv2api.IngressClassParams, svcAndIngAnnotations map[string]string) (int64, error) {
	if ingressClassParams != nil && ingressClassParams.Spec.AuthConfig != nil {
		if ingressClassParams.Spec.AuthConfig.Type == elbv2api.AuthTypeNone || ingressClassParams.Spec.AuthConfig.SessionTimeout == nil {
			return int64(defaultAuthSessionTimeout), nil
		}

		return int64(*ingressClassParams.Spec.AuthConfig.SessionTimeout), nil
	}

	rawAuthSessionTimeout := int64(defaultAuthSessionTimeout)
	if _, err := b.annotationParser.ParseInt64Annotation(annotations.IngressSuffixAuthSessionTimeout, &rawAuthSessionTimeout, svcAndIngAnnotations); err != nil {
		return 0, err
	}
	return rawAuthSessionTimeout, nil
}
