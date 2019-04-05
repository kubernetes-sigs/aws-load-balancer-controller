package auth

import (
	"context"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

const (
	DefaultAuthType                     = TypeNone
	DefaultAuthScope                    = "openid"
	DefaultAuthSessionCookie            = "AWSELBAuthSessionCookie"
	DefaultAuthSessionTimeout           = 604800
	DefaultAuthOnUnauthenticatedRequest = api.OnUnauthenticatedRequestActionAuthenticate
)

type ActionBuilder interface {
	Build(ctx context.Context, ing *extensions.Ingress, svcAnnotations map[string]string,
		protocol api.Protocol) ([]api.ListenerAction, error)
}

func NewActionBuilder(cache cache.Cache, annotationParser k8s.AnnotationParser) ActionBuilder {
	return &defaultBuilder{
		cache:            cache,
		annotationParser: annotationParser,
	}
}

type defaultBuilder struct {
	cache            cache.Cache
	annotationParser k8s.AnnotationParser
}

func (m *defaultBuilder) Build(ctx context.Context, ing *extensions.Ingress, svcAnnotations map[string]string,
	protocol api.Protocol) ([]api.ListenerAction, error) {

	if protocol != api.ProtocolHTTPS {
		return nil, nil
	}

	authType := DefaultAuthType
	authCfg := api.AuthenticateConfig{
		OnUnauthenticatedRequest: DefaultAuthOnUnauthenticatedRequest,
		Scope:                    DefaultAuthScope,
		SessionCookieName:        DefaultAuthSessionCookie,
		SessionTimeout:           DefaultAuthSessionTimeout,
	}
	ingAnnotations := ing.Annotations
	_ = m.annotationParser.ParseStringAnnotation(k8s.AnnotationSuffixAuthType, (*string)(&authType), svcAnnotations, ingAnnotations)
	_ = m.annotationParser.ParseStringAnnotation(k8s.AnnotationSuffixAuthOnUnauthenticatedRequest, (*string)(&authCfg.OnUnauthenticatedRequest), svcAnnotations, ingAnnotations)
	_ = m.annotationParser.ParseStringAnnotation(k8s.AnnotationSuffixAuthScope, &authCfg.Scope, svcAnnotations, ingAnnotations)
	_ = m.annotationParser.ParseStringAnnotation(k8s.AnnotationSuffixAuthSessionCookie, &authCfg.SessionCookieName, svcAnnotations, ingAnnotations)
	if _, err := m.annotationParser.ParseInt64Annotation(k8s.AnnotationSuffixAuthSessionTimeout, &authCfg.SessionTimeout, svcAnnotations, ingAnnotations); err != nil {
		return nil, err
	}

	switch authType {
	case TypeCognito:
		{
			authIDP := IDPCognito{}
			exists, err := m.annotationParser.ParseJSONAnnotation(k8s.AnnotationSuffixAuthIDPCognito, &authIDP, svcAnnotations, ingAnnotations)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, errors.Errorf("annotation %s is required when authType == %s", k8s.AnnotationSuffixAuthIDPCognito, TypeCognito)
			}
			return []api.ListenerAction{
				{
					Type: api.ListenerActionTypeAuthenticateCognito,
					AuthenticateCognito: &api.AuthenticateCognitoConfig{
						AuthenticateConfig: authCfg,
						UserPoolARN:        authIDP.UserPoolArn,
						UserPoolClientID:   authIDP.UserPoolClientId,
						UserPoolDomain:     authIDP.UserPoolDomain,
					},
				},
			}, nil
		}
	case TypeOIDC:
		{
			authIDP := IDPOIDC{}
			exists, err := m.loadIDPOIDC(ctx, &authIDP, ing.Namespace, svcAnnotations, ingAnnotations)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, errors.Errorf("annotation %s is required when authType == %s", k8s.AnnotationSuffixAuthIDPOIDC, TypeOIDC)
			}
			return []api.ListenerAction{
				{
					Type: api.ListenerActionTypeAuthenticateOIDC,
					AuthenticateOIDC: &api.AuthenticateOIDCConfig{
						AuthenticateConfig:    authCfg,
						Issuer:                authIDP.Issuer,
						AuthorizationEndpoint: authIDP.AuthorizationEndpoint,
						TokenEndpoint:         authIDP.TokenEndpoint,
						UserInfoEndpoint:      authIDP.UserInfoEndpoint,
						ClientID:              authIDP.ClientId,
						ClientSecret:          authIDP.ClientSecret,
					},
				},
			}, nil
		}
	}
	return nil, nil
}

func (m *defaultBuilder) loadIDPOIDC(ctx context.Context, idpOIDC *IDPOIDC, namespace string, svcAnnotations map[string]string, ingAnnotations map[string]string) (bool, error) {
	annoIDPOIDC := AnnotationSchemaIDPOIDC{}
	exists, err := m.annotationParser.ParseJSONAnnotation(k8s.AnnotationSuffixAuthIDPOIDC, &annoIDPOIDC, svcAnnotations, ingAnnotations)
	if err != nil {
		return true, errors.Wrapf(err, "failed to load configuration for IDP OIDC")
	}
	if !exists {
		return false, nil
	}

	secretKey := types.NamespacedName{
		Namespace: namespace,
		Name:      annoIDPOIDC.SecretName,
	}
	k8sSecret := corev1.Secret{}
	if err := m.cache.Get(ctx, secretKey, &k8sSecret); err != nil {
		return true, errors.Wrapf(err, "failed to load k8s secret: %v", secretKey)
	}
	clientId := string(k8sSecret.Data["clientId"])
	clientSecret := string(k8sSecret.Data["clientSecret"])
	*idpOIDC = IDPOIDC{
		Issuer:                annoIDPOIDC.Issuer,
		AuthorizationEndpoint: annoIDPOIDC.AuthorizationEndpoint,
		TokenEndpoint:         annoIDPOIDC.TokenEndpoint,
		UserInfoEndpoint:      annoIDPOIDC.UserInfoEndpoint,
		ClientId:              clientId,
		ClientSecret:          clientSecret,
	}
	return true, nil
}

func (m *defaultBuilder) buildOIDCSecretIndex(namespace string, annos map[string]string) []string {
	annoIDPOIDC := AnnotationSchemaIDPOIDC{}
	exists, err := m.annotationParser.ParseJSONAnnotation(k8s.AnnotationSuffixAuthIDPOIDC, &annoIDPOIDC, annos)
	if !exists || err != nil {
		return nil
	}

	secretKey := types.NamespacedName{
		Namespace: namespace,
		Name:      annoIDPOIDC.SecretName,
	}.String()
	return []string{secretKey}
}
