package translate

import (
	"fmt"

	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	annotations "sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
)

const (
	authTypeNone    = "none"
	authTypeCognito = "cognito"
	authTypeOIDC    = "oidc"
)

// buildAuthAction reads auth-type and related annotations and returns a
// ListenerRuleConfiguration Action for authenticate-cognito or authenticate-oidc.
// Returns (nil, nil) when auth-type is "none" or absent.
func buildAuthAction(annos map[string]string) (*gatewayv1beta1.Action, error) {
	authType := getString(annos, annotations.IngressSuffixAuthType)
	if authType == "" || authType == authTypeNone {
		return nil, nil
	}

	switch authType {
	case authTypeCognito:
		return buildAuthCognitoAction(annos)
	case authTypeOIDC:
		return buildOIDCAction(annos)
	default:
		return nil, fmt.Errorf("unsupported auth-type %q", authType)
	}
}

// buildAuthCognitoAction parses auth-idp-cognito JSON and shared auth annotations
// into an authenticate-cognito Action.
func buildAuthCognitoAction(annos map[string]string) (*gatewayv1beta1.Action, error) {
	var idp ingress.AuthIDPConfigCognito
	exists, err := ingressAnnotationParser.ParseJSONAnnotation(annotations.IngressSuffixAuthIDPCognito, &idp, annos)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s annotation: %w", annotations.IngressSuffixAuthIDPCognito, err)
	}
	if !exists {
		return nil, fmt.Errorf("auth-type is %q but %s annotation is missing", authTypeCognito, annotations.IngressSuffixAuthIDPCognito)
	}

	scope := getOptionalAuthScope(annos)
	onUnauth := getOptionalAuthOnUnauthenticatedRequest(annos)
	sessionCookie := getOptionalAuthSessionCookieName(annos)
	sessionTimeout := getOptionalAuthSessionTimeout(annos)

	cfg := &gatewayv1beta1.AuthenticateCognitoActionConfig{
		UserPoolArn:      idp.UserPoolARN,
		UserPoolClientID: idp.UserPoolClientID,
		UserPoolDomain:   idp.UserPoolDomain,
	}

	if scope != nil {
		cfg.Scope = scope
	}
	if onUnauth != nil {
		cognitoEnum := gatewayv1beta1.AuthenticateCognitoActionConditionalBehaviorEnum(*onUnauth)
		cfg.OnUnauthenticatedRequest = &cognitoEnum
	}
	if sessionCookie != nil {
		cfg.SessionCookieName = sessionCookie
	}
	if sessionTimeout != nil {
		cfg.SessionTimeout = sessionTimeout
	}

	if len(idp.AuthenticationRequestExtraParams) > 0 {
		params := make(map[string]string, len(idp.AuthenticationRequestExtraParams))
		for k, v := range idp.AuthenticationRequestExtraParams {
			params[k] = v
		}
		cfg.AuthenticationRequestExtraParams = &params
	}

	return &gatewayv1beta1.Action{
		Type:                      gatewayv1beta1.ActionTypeAuthenticateCognito,
		AuthenticateCognitoConfig: cfg,
	}, nil
}

// buildOIDCAction parses auth-idp-oidc JSON and shared auth annotations
// into an authenticate-oidc Action. The Secret reference (secretName) from
// the annotation JSON is preserved as Secret.Name on the CRD.
func buildOIDCAction(annos map[string]string) (*gatewayv1beta1.Action, error) {
	var idp ingress.AuthIDPConfigOIDC
	exists, err := ingressAnnotationParser.ParseJSONAnnotation(annotations.IngressSuffixAuthIDPOIDC, &idp, annos)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s annotation: %w", annotations.IngressSuffixAuthIDPOIDC, err)
	}
	if !exists {
		return nil, fmt.Errorf("auth-type is %q but %s annotation is missing", authTypeOIDC, annotations.IngressSuffixAuthIDPOIDC)
	}

	scope := getOptionalAuthScope(annos)
	onUnauth := getOptionalAuthOnUnauthenticatedRequest(annos)
	sessionCookie := getOptionalAuthSessionCookieName(annos)
	sessionTimeout := getOptionalAuthSessionTimeout(annos)

	cfg := &gatewayv1beta1.AuthenticateOidcActionConfig{
		Issuer:                idp.Issuer,
		AuthorizationEndpoint: idp.AuthorizationEndpoint,
		TokenEndpoint:         idp.TokenEndpoint,
		UserInfoEndpoint:      idp.UserInfoEndpoint,
		Secret:                &gatewayv1beta1.Secret{Name: idp.SecretName},
	}

	if scope != nil {
		cfg.Scope = scope
	}
	if onUnauth != nil {
		oidcEnum := gatewayv1beta1.AuthenticateOidcActionConditionalBehaviorEnum(*onUnauth)
		cfg.OnUnauthenticatedRequest = &oidcEnum
	}
	if sessionCookie != nil {
		cfg.SessionCookieName = sessionCookie
	}
	if sessionTimeout != nil {
		cfg.SessionTimeout = sessionTimeout
	}

	if len(idp.AuthenticationRequestExtraParams) > 0 {
		params := make(map[string]string, len(idp.AuthenticationRequestExtraParams))
		for k, v := range idp.AuthenticationRequestExtraParams {
			params[k] = v
		}
		cfg.AuthenticationRequestExtraParams = &params
	}

	return &gatewayv1beta1.Action{
		Type:                   gatewayv1beta1.ActionTypeAuthenticateOIDC,
		AuthenticateOIDCConfig: cfg,
	}, nil
}

// getOptionalAuthScope returns the auth-scope annotation value, or nil if not set.
// The CRD has kubebuilder:default="openid" so omitting it lets the webhook fill the default.
func getOptionalAuthScope(annos map[string]string) *string {
	v := getString(annos, annotations.IngressSuffixAuthScope)
	if v == "" {
		return nil
	}
	return &v
}

// getOptionalAuthOnUnauthenticatedRequest returns the annotation value, or nil if not set.
// The CRD has kubebuilder:default="authenticate".
func getOptionalAuthOnUnauthenticatedRequest(annos map[string]string) *string {
	v := getString(annos, annotations.IngressSuffixAuthOnUnauthenticatedRequest)
	if v == "" {
		return nil
	}
	return &v
}

// getOptionalAuthSessionCookieName returns the annotation value, or nil if not set.
// The CRD has kubebuilder:default="AWSELBAuthSessionCookie".
func getOptionalAuthSessionCookieName(annos map[string]string) *string {
	v := getString(annos, annotations.IngressSuffixAuthSessionCookie)
	if v == "" {
		return nil
	}
	return &v
}

// getOptionalAuthSessionTimeout returns the annotation value, or nil if not set.
// The CRD has kubebuilder:default=604800.
func getOptionalAuthSessionTimeout(annos map[string]string) *int64 {
	return getInt64(annos, annotations.IngressSuffixAuthSessionTimeout)
}
