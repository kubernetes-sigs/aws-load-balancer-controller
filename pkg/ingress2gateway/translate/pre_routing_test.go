package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
)

func TestBuildAuthAction_None(t *testing.T) {
	annos := map[string]string{
		"alb.ingress.kubernetes.io/auth-type": "none",
	}
	action, err := buildAuthAction(annos)
	require.NoError(t, err)
	assert.Nil(t, action)
}

func TestBuildAuthAction_NoAnnotation(t *testing.T) {
	action, err := buildAuthAction(map[string]string{})
	require.NoError(t, err)
	assert.Nil(t, action)
}

func TestBuildAuthAction_UnknownType(t *testing.T) {
	annos := map[string]string{
		"alb.ingress.kubernetes.io/auth-type": "foobar",
	}
	_, err := buildAuthAction(annos)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported auth-type")
}

func TestBuildAuthAction_CognitoFull(t *testing.T) {
	annos := map[string]string{
		"alb.ingress.kubernetes.io/auth-type":                       "cognito",
		"alb.ingress.kubernetes.io/auth-idp-cognito":                `{"userPoolARN":"arn:aws:cognito-idp:us-west-2:123456789:userpool/us-west-2_abc","userPoolClientID":"my-client-id","userPoolDomain":"my-domain"}`,
		"alb.ingress.kubernetes.io/auth-scope":                      "email openid",
		"alb.ingress.kubernetes.io/auth-on-unauthenticated-request": "deny",
		"alb.ingress.kubernetes.io/auth-session-cookie":             "my-cookie",
		"alb.ingress.kubernetes.io/auth-session-timeout":            "86400",
	}
	action, err := buildAuthAction(annos)
	require.NoError(t, err)
	require.NotNil(t, action)

	assert.Equal(t, gatewayv1beta1.ActionTypeAuthenticateCognito, action.Type)
	require.NotNil(t, action.AuthenticateCognitoConfig)

	cfg := action.AuthenticateCognitoConfig
	assert.Equal(t, "arn:aws:cognito-idp:us-west-2:123456789:userpool/us-west-2_abc", cfg.UserPoolArn)
	assert.Equal(t, "my-client-id", cfg.UserPoolClientID)
	assert.Equal(t, "my-domain", cfg.UserPoolDomain)

	require.NotNil(t, cfg.Scope)
	assert.Equal(t, "email openid", *cfg.Scope)

	require.NotNil(t, cfg.OnUnauthenticatedRequest)
	assert.Equal(t, gatewayv1beta1.AuthenticateCognitoActionConditionalBehaviorEnumDeny, *cfg.OnUnauthenticatedRequest)

	require.NotNil(t, cfg.SessionCookieName)
	assert.Equal(t, "my-cookie", *cfg.SessionCookieName)

	require.NotNil(t, cfg.SessionTimeout)
	assert.Equal(t, int64(86400), *cfg.SessionTimeout)

	assert.Nil(t, cfg.AuthenticationRequestExtraParams)
}

func TestBuildAuthAction_CognitoExtraParams(t *testing.T) {
	annos := map[string]string{
		"alb.ingress.kubernetes.io/auth-type":        "cognito",
		"alb.ingress.kubernetes.io/auth-idp-cognito": `{"userPoolARN":"arn:pool","userPoolClientID":"cid","userPoolDomain":"dom","authenticationRequestExtraParams":{"display":"page","prompt":"login"}}`,
	}
	action, err := buildAuthAction(annos)
	require.NoError(t, err)
	require.NotNil(t, action)

	cfg := action.AuthenticateCognitoConfig
	require.NotNil(t, cfg.AuthenticationRequestExtraParams)
	assert.Equal(t, "page", (*cfg.AuthenticationRequestExtraParams)["display"])
	assert.Equal(t, "login", (*cfg.AuthenticationRequestExtraParams)["prompt"])
}

func TestBuildAuthAction_CognitoMissingIDP(t *testing.T) {
	annos := map[string]string{
		"alb.ingress.kubernetes.io/auth-type": "cognito",
	}
	_, err := buildAuthAction(annos)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestBuildAuthAction_OIDCFull(t *testing.T) {
	annos := map[string]string{
		"alb.ingress.kubernetes.io/auth-type":                       "oidc",
		"alb.ingress.kubernetes.io/auth-idp-oidc":                   `{"issuer":"https://example.com","authorizationEndpoint":"https://auth.example.com","tokenEndpoint":"https://token.example.com","userInfoEndpoint":"https://userinfo.example.com","secretName":"my-k8s-secret"}`,
		"alb.ingress.kubernetes.io/auth-scope":                      "email openid profile",
		"alb.ingress.kubernetes.io/auth-on-unauthenticated-request": "allow",
		"alb.ingress.kubernetes.io/auth-session-cookie":             "oidc-cookie",
		"alb.ingress.kubernetes.io/auth-session-timeout":            "7200",
	}
	action, err := buildAuthAction(annos)
	require.NoError(t, err)
	require.NotNil(t, action)

	assert.Equal(t, gatewayv1beta1.ActionTypeAuthenticateOIDC, action.Type)
	require.NotNil(t, action.AuthenticateOIDCConfig)

	cfg := action.AuthenticateOIDCConfig
	assert.Equal(t, "https://example.com", cfg.Issuer)
	assert.Equal(t, "https://auth.example.com", cfg.AuthorizationEndpoint)
	assert.Equal(t, "https://token.example.com", cfg.TokenEndpoint)
	assert.Equal(t, "https://userinfo.example.com", cfg.UserInfoEndpoint)

	// Secret reference preserved by name
	require.NotNil(t, cfg.Secret)
	assert.Equal(t, "my-k8s-secret", cfg.Secret.Name)

	require.NotNil(t, cfg.Scope)
	assert.Equal(t, "email openid profile", *cfg.Scope)

	require.NotNil(t, cfg.OnUnauthenticatedRequest)
	assert.Equal(t, gatewayv1beta1.AuthenticateOidcActionConditionalBehaviorEnumAllow, *cfg.OnUnauthenticatedRequest)

	require.NotNil(t, cfg.SessionCookieName)
	assert.Equal(t, "oidc-cookie", *cfg.SessionCookieName)

	require.NotNil(t, cfg.SessionTimeout)
	assert.Equal(t, int64(7200), *cfg.SessionTimeout)

	assert.Nil(t, cfg.AuthenticationRequestExtraParams)
}

func TestBuildAuthAction_OIDCExtraParams(t *testing.T) {
	annos := map[string]string{
		"alb.ingress.kubernetes.io/auth-type":     "oidc",
		"alb.ingress.kubernetes.io/auth-idp-oidc": `{"issuer":"https://ex.com","authorizationEndpoint":"https://auth.ex.com","tokenEndpoint":"https://tok.ex.com","userInfoEndpoint":"https://ui.ex.com","secretName":"sec","authenticationRequestExtraParams":{"display":"page","prompt":"consent"}}`,
	}
	action, err := buildAuthAction(annos)
	require.NoError(t, err)
	require.NotNil(t, action)

	cfg := action.AuthenticateOIDCConfig
	require.NotNil(t, cfg.AuthenticationRequestExtraParams)
	assert.Equal(t, "page", (*cfg.AuthenticationRequestExtraParams)["display"])
	assert.Equal(t, "consent", (*cfg.AuthenticationRequestExtraParams)["prompt"])
}

func TestBuildAuthAction_OIDCMissingIDP(t *testing.T) {
	annos := map[string]string{
		"alb.ingress.kubernetes.io/auth-type": "oidc",
	}
	_, err := buildAuthAction(annos)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestBuildAuthAction_OIDCSecretNamePreserved(t *testing.T) {
	annos := map[string]string{
		"alb.ingress.kubernetes.io/auth-type":     "oidc",
		"alb.ingress.kubernetes.io/auth-idp-oidc": `{"issuer":"https://ex.com","authorizationEndpoint":"https://auth.ex.com","tokenEndpoint":"https://tok.ex.com","userInfoEndpoint":"https://ui.ex.com","secretName":"my-special-secret"}`,
	}
	action, err := buildAuthAction(annos)
	require.NoError(t, err)
	require.NotNil(t, action)
	require.NotNil(t, action.AuthenticateOIDCConfig)
	require.NotNil(t, action.AuthenticateOIDCConfig.Secret)
	assert.Equal(t, "my-special-secret", action.AuthenticateOIDCConfig.Secret.Name)
}

func TestBuildJwtValidationAction_NoAnnotation(t *testing.T) {
	action, err := buildJwtValidationAction(map[string]string{})
	require.NoError(t, err)
	assert.Nil(t, action)
}

func TestBuildJwtValidationAction_Basic(t *testing.T) {
	annos := map[string]string{
		"alb.ingress.kubernetes.io/jwt-validation": `{"jwksEndpoint":"https://example.com/.well-known/jwks.json","issuer":"https://example.com"}`,
	}
	action, err := buildJwtValidationAction(annos)
	require.NoError(t, err)
	require.NotNil(t, action)

	assert.Equal(t, gatewayv1beta1.ActionTypeJwtValidation, action.Type)
	require.NotNil(t, action.JwtValidationConfig)

	cfg := action.JwtValidationConfig
	assert.Equal(t, "https://example.com/.well-known/jwks.json", cfg.JwksEndpoint)
	assert.Equal(t, "https://example.com", cfg.Issuer)
	assert.Empty(t, cfg.AdditionalClaims)
}

func TestBuildJwtValidationAction_WithAdditionalClaims(t *testing.T) {
	annos := map[string]string{
		"alb.ingress.kubernetes.io/jwt-validation": `{"jwksEndpoint":"https://example-endpoint.com","issuer":"https://example-issuer.com","additionalClaims":[{"name":"admin","format":"single-string","values":["true"]},{"name":"ver","format":"string-array","values":["6","19"]},{"name":"scope","format":"space-separated-values","values":["read:api","write","email"]}]}`,
	}
	action, err := buildJwtValidationAction(annos)
	require.NoError(t, err)
	require.NotNil(t, action)

	cfg := action.JwtValidationConfig
	assert.Equal(t, "https://example-endpoint.com", cfg.JwksEndpoint)
	assert.Equal(t, "https://example-issuer.com", cfg.Issuer)
	require.Len(t, cfg.AdditionalClaims, 3)

	assert.Equal(t, "admin", cfg.AdditionalClaims[0].Name)
	assert.Equal(t, gatewayv1beta1.FormatSingleString, cfg.AdditionalClaims[0].Format)
	assert.Equal(t, []string{"true"}, cfg.AdditionalClaims[0].Values)

	assert.Equal(t, "ver", cfg.AdditionalClaims[1].Name)
	assert.Equal(t, gatewayv1beta1.FormatStringArray, cfg.AdditionalClaims[1].Format)
	assert.Equal(t, []string{"6", "19"}, cfg.AdditionalClaims[1].Values)

	assert.Equal(t, "scope", cfg.AdditionalClaims[2].Name)
	assert.Equal(t, gatewayv1beta1.FormatSpaceSeparatedValues, cfg.AdditionalClaims[2].Format)
	assert.Equal(t, []string{"read:api", "write", "email"}, cfg.AdditionalClaims[2].Values)
}

func TestBuildJwtValidationAction_InvalidJSON(t *testing.T) {
	annos := map[string]string{
		"alb.ingress.kubernetes.io/jwt-validation": `{invalid json}`,
	}
	_, err := buildJwtValidationAction(annos)
	require.Error(t, err)
}
