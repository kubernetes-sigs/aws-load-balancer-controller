package ingress

import (
	"context"
	"github.com/pkg/errors"
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
	Build(ctx context.Context, svcAndIngAnnotations map[string]string) (AuthConfig, error)
}

// NewDefaultAuthConfigBuilder constructs new defaultAuthConfigBuilder.
func NewDefaultAuthConfigBuilder(annotationParser annotations.Parser) *defaultAuthConfigBuilder {
	return &defaultAuthConfigBuilder{
		annotationParser: annotationParser,
	}
}

var _ AuthConfigBuilder = &defaultAuthConfigBuilder{}

// default implementation for AuthConfigBuilder
type defaultAuthConfigBuilder struct {
	annotationParser annotations.Parser
}

func (b *defaultAuthConfigBuilder) Build(ctx context.Context, svcAndIngAnnotations map[string]string) (AuthConfig, error) {
	authType, err := b.buildAuthType(ctx, svcAndIngAnnotations)
	if err != nil {
		return AuthConfig{}, err
	}
	authOnUnauthenticatedRequest := b.buildAuthOnUnauthenticatedRequest(ctx, svcAndIngAnnotations)
	authScope := b.buildAuthScope(ctx, svcAndIngAnnotations)
	authSessionCookieName := b.buildAuthSessionCookieName(ctx, svcAndIngAnnotations)
	authSessionTimeout, err := b.buildAuthSessionTimeout(ctx, svcAndIngAnnotations)
	if err != nil {
		return AuthConfig{}, err
	}
	authIDPCognito, err := b.buildAuthIDPConfigCognito(ctx, svcAndIngAnnotations)
	if err != nil {
		return AuthConfig{}, err
	}
	authIDPOIDC, err := b.buildAuthIDPConfigOIDC(ctx, svcAndIngAnnotations)
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

func (b *defaultAuthConfigBuilder) buildAuthType(_ context.Context, svcAndIngAnnotations map[string]string) (AuthType, error) {
	rawAuthType := string(defaultAuthType)
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixAuthType, &rawAuthType, svcAndIngAnnotations)
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

func (b *defaultAuthConfigBuilder) buildAuthIDPConfigCognito(_ context.Context, svcAndIngAnnotations map[string]string) (*AuthIDPConfigCognito, error) {
	authIDP := AuthIDPConfigCognito{}
	exists, err := b.annotationParser.ParseJSONAnnotation(annotations.IngressSuffixAuthIDPCognito, &authIDP, svcAndIngAnnotations)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	return &authIDP, nil
}

func (b *defaultAuthConfigBuilder) buildAuthIDPConfigOIDC(_ context.Context, svcAndIngAnnotations map[string]string) (*AuthIDPConfigOIDC, error) {
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

func (b *defaultAuthConfigBuilder) buildAuthOnUnauthenticatedRequest(_ context.Context, svcAndIngAnnotations map[string]string) string {
	rawOnUnauthenticatedRequest := defaultAuthOnUnauthenticatedRequest
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixAuthOnUnauthenticatedRequest, &rawOnUnauthenticatedRequest, svcAndIngAnnotations)
	return rawOnUnauthenticatedRequest
}

func (b *defaultAuthConfigBuilder) buildAuthScope(_ context.Context, svcAndIngAnnotations map[string]string) string {
	rawAuthScope := defaultAuthScope
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixAuthScope, &rawAuthScope, svcAndIngAnnotations)
	return rawAuthScope
}

func (b *defaultAuthConfigBuilder) buildAuthSessionCookieName(_ context.Context, svcAndIngAnnotations map[string]string) string {
	rawAuthSessionCookieName := defaultAuthSessionCookieName
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixAuthSessionCookie, &rawAuthSessionCookieName, svcAndIngAnnotations)
	return rawAuthSessionCookieName
}

func (b *defaultAuthConfigBuilder) buildAuthSessionTimeout(_ context.Context, svcAndIngAnnotations map[string]string) (int64, error) {
	rawAuthSessionTimeout := int64(defaultAuthSessionTimeout)
	if _, err := b.annotationParser.ParseInt64Annotation(annotations.IngressSuffixAuthSessionTimeout, &rawAuthSessionTimeout, svcAndIngAnnotations); err != nil {
		return 0, err
	}
	return rawAuthSessionTimeout, nil
}
