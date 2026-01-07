package routeutils

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func Test_buildHttpRedirectAction(t *testing.T) {
	scheme := "https"
	expectedScheme := "HTTPS"
	invalidScheme := "invalid"

	port := int32(80)
	portString := "80"
	statusCode := 301
	query := "test-query"
	replaceFullPath := "/new-path"
	replacePrefixPath := "/new-prefix-path"
	invalidPath := "/invalid-path*"

	tests := []struct {
		name           string
		filter         *gwv1.HTTPRequestRedirectFilter
		redirectConfig *elbv2gw.RedirectActionConfig
		want           *elbv2model.Action
		wantErr        bool
	}{
		{
			name: "redirect with all fields provided",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme:     &scheme,
				Hostname:   (*gwv1.PreciseHostname)(&hostname),
				Port:       (*gwv1.PortNumber)(&port),
				StatusCode: &statusCode,
				Path: &gwv1.HTTPPathModifier{
					Type:            gwv1.FullPathHTTPPathModifier,
					ReplaceFullPath: &replaceFullPath,
				},
			},
			redirectConfig: &elbv2gw.RedirectActionConfig{
				Query: &query,
			},
			want: &elbv2model.Action{
				Type: elbv2model.ActionTypeRedirect,
				RedirectConfig: &elbv2model.RedirectActionConfig{
					Host:       &hostname,
					Path:       &replaceFullPath,
					Port:       &portString,
					Protocol:   &expectedScheme,
					StatusCode: "HTTP_301",
					Query:      &query,
				},
			},
			wantErr: false,
		},
		{
			name: "redirect with prefix  - no path in redirect config",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Hostname: (*gwv1.PreciseHostname)(&hostname),
				Path: &gwv1.HTTPPathModifier{
					Type:               gwv1.PrefixMatchHTTPPathModifier,
					ReplacePrefixMatch: &replacePrefixPath,
				},
			},
			want: &elbv2model.Action{
				Type: elbv2model.ActionTypeRedirect,
				RedirectConfig: &elbv2model.RedirectActionConfig{
					Host: &hostname,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid scheme provided",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: &invalidScheme,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "path with wildcards in ReplaceFullPath",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Path: &gwv1.HTTPPathModifier{
					Type:            gwv1.FullPathHTTPPathModifier,
					ReplaceFullPath: &invalidPath,
				},
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildHttpRedirectAction(tt.filter, tt.redirectConfig)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_BuildHttpRuleRedirectActionsBasedOnFilter(t *testing.T) {
	query := "test-query"

	redirectConfig := &elbv2gw.RedirectActionConfig{
		Query: &query,
	}

	tests := []struct {
		name           string
		filters        []gwv1.HTTPRouteFilter
		redirectConfig *elbv2gw.RedirectActionConfig
		wantErr        bool
		errContains    string
	}{
		{
			name: "request redirect filter",
			filters: []gwv1.HTTPRouteFilter{
				{
					Type: gwv1.HTTPRouteFilterRequestRedirect,
					RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
						Port: (*gwv1.PortNumber)(awssdk.Int32(80)),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "unsupported filter type",
			filters: []gwv1.HTTPRouteFilter{
				{
					Type: gwv1.HTTPRouteFilterRequestHeaderModifier,
				},
			},
			wantErr:     true,
			errContains: "Unsupported filter type",
		},
		{
			name: "single ExtensionRef filter with redirectConfig should error",
			filters: []gwv1.HTTPRouteFilter{
				{
					Type: gwv1.HTTPRouteFilterExtensionRef,
					ExtensionRef: &gwv1.LocalObjectReference{
						Kind: "ListenerRuleConfiguration",
						Name: "test-config",
					},
				},
			},
			redirectConfig: redirectConfig,
			wantErr:        true,
			errContains:    "HTTPRouteFilterRequestRedirect must be provided if RedirectActionConfig in ListenerRuleConfiguration is provided",
		},
		{
			name:           "empty filters should return nil",
			filters:        []gwv1.HTTPRouteFilter{},
			redirectConfig: nil,
			wantErr:        false,
		},
		{
			name: "multiple filters with ExtensionRef should continue processing",
			filters: []gwv1.HTTPRouteFilter{
				{
					Type: gwv1.HTTPRouteFilterExtensionRef,
					ExtensionRef: &gwv1.LocalObjectReference{
						Kind: "SomeOtherConfig",
						Name: "test-config",
					},
				},
				{
					Type: gwv1.HTTPRouteFilterRequestRedirect,
					RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
						Hostname: (*gwv1.PreciseHostname)(awssdk.String("redirect.example.com")),
					},
				},
			},
			redirectConfig: nil,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions, err := buildHttpRuleRedirectActionsBasedOnFilter(tt.filters, tt.redirectConfig)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, actions)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_buildFixedResponseRoutingAction(t *testing.T) {
	contentType := "text/plain"
	messageBody := "test-message-body"

	tests := []struct {
		name                string
		fixedResponseConfig *elbv2gw.FixedResponseActionConfig
		want                *elbv2model.Action
		wantErr             bool
	}{
		{
			name: "fixed response with all fields",
			fixedResponseConfig: &elbv2gw.FixedResponseActionConfig{
				StatusCode:  404,
				ContentType: &contentType,
				MessageBody: &messageBody,
			},
			want: &elbv2model.Action{
				Type: elbv2model.ActionTypeFixedResponse,
				FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
					StatusCode:  "404",
					ContentType: &contentType,
					MessageBody: &messageBody,
				},
			},
			wantErr: false,
		},
		{
			name: "fixed response with only status code",
			fixedResponseConfig: &elbv2gw.FixedResponseActionConfig{
				StatusCode: 503,
			},
			want: &elbv2model.Action{
				Type: elbv2model.ActionTypeFixedResponse,
				FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
					StatusCode:  "503",
					ContentType: nil,
					MessageBody: nil,
				},
			},
			wantErr: false,
		},
		{
			name: "fixed response with status code and content type only",
			fixedResponseConfig: &elbv2gw.FixedResponseActionConfig{
				StatusCode:  200,
				ContentType: &contentType,
			},
			want: &elbv2model.Action{
				Type: elbv2model.ActionTypeFixedResponse,
				FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
					StatusCode:  "200",
					ContentType: &contentType,
					MessageBody: nil,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildFixedResponseRoutingAction(tt.fixedResponseConfig)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildAuthenticateCognitoAction(t *testing.T) {
	userPoolArn := "arn:aws:cognito-idp:us-west-2:123456789012:userpool/us-west-2_EXAMPLE"
	userPoolClientID := "client123"
	userPoolDomain := "my-domain"
	scope := "openid"
	sessionCookieName := "AWSELBAuthSessionCookie"
	sessionTimeout := int64(604800)
	authRequestExtraParams := map[string]string{
		"prompt":     "login",
		"ui_locales": "en",
	}

	authenticateBehavior := elbv2gw.AuthenticateCognitoActionConditionalBehaviorEnumAuthenticate
	allowBehavior := elbv2gw.AuthenticateCognitoActionConditionalBehaviorEnumAllow
	denyBehavior := elbv2gw.AuthenticateCognitoActionConditionalBehaviorEnumDeny

	tests := []struct {
		name    string
		config  *elbv2gw.AuthenticateCognitoActionConfig
		want    *elbv2model.Action
		wantErr bool
	}{
		{
			name: "authenticate cognito with all fields",
			config: &elbv2gw.AuthenticateCognitoActionConfig{
				UserPoolArn:                      userPoolArn,
				UserPoolClientID:                 userPoolClientID,
				UserPoolDomain:                   userPoolDomain,
				AuthenticationRequestExtraParams: &authRequestExtraParams,
				OnUnauthenticatedRequest:         &authenticateBehavior,
				Scope:                            &scope,
				SessionCookieName:                &sessionCookieName,
				SessionTimeout:                   &sessionTimeout,
			},
			want: &elbv2model.Action{
				Type: elbv2model.ActionTypeAuthenticateCognito,
				AuthenticateCognitoConfig: &elbv2model.AuthenticateCognitoActionConfig{
					UserPoolARN:                      userPoolArn,
					UserPoolClientID:                 userPoolClientID,
					UserPoolDomain:                   userPoolDomain,
					AuthenticationRequestExtraParams: authRequestExtraParams,
					OnUnauthenticatedRequest:         elbv2model.AuthenticateCognitoActionConditionalBehavior(authenticateBehavior),
					Scope:                            &scope,
					SessionCookieName:                &sessionCookieName,
					SessionTimeout:                   &sessionTimeout,
				},
			},
			wantErr: false,
		},
		{
			name: "authenticate cognito with required fields only",
			config: &elbv2gw.AuthenticateCognitoActionConfig{
				UserPoolArn:                      userPoolArn,
				UserPoolClientID:                 userPoolClientID,
				UserPoolDomain:                   userPoolDomain,
				AuthenticationRequestExtraParams: &map[string]string{},
				OnUnauthenticatedRequest:         &authenticateBehavior,
			},
			want: &elbv2model.Action{
				Type: elbv2model.ActionTypeAuthenticateCognito,
				AuthenticateCognitoConfig: &elbv2model.AuthenticateCognitoActionConfig{
					UserPoolARN:                      userPoolArn,
					UserPoolClientID:                 userPoolClientID,
					UserPoolDomain:                   userPoolDomain,
					AuthenticationRequestExtraParams: map[string]string{},
					OnUnauthenticatedRequest:         elbv2model.AuthenticateCognitoActionConditionalBehavior(authenticateBehavior),
					Scope:                            nil,
					SessionCookieName:                nil,
					SessionTimeout:                   nil,
				},
			},
			wantErr: false,
		},
		{
			name: "authenticate cognito with deny behavior",
			config: &elbv2gw.AuthenticateCognitoActionConfig{
				UserPoolArn:                      userPoolArn,
				UserPoolClientID:                 userPoolClientID,
				UserPoolDomain:                   userPoolDomain,
				AuthenticationRequestExtraParams: &map[string]string{},
				OnUnauthenticatedRequest:         &denyBehavior,
			},
			want: &elbv2model.Action{
				Type: elbv2model.ActionTypeAuthenticateCognito,
				AuthenticateCognitoConfig: &elbv2model.AuthenticateCognitoActionConfig{
					UserPoolARN:                      userPoolArn,
					UserPoolClientID:                 userPoolClientID,
					UserPoolDomain:                   userPoolDomain,
					AuthenticationRequestExtraParams: map[string]string{},
					OnUnauthenticatedRequest:         elbv2model.AuthenticateCognitoActionConditionalBehaviorDeny,
					Scope:                            nil,
					SessionCookieName:                nil,
					SessionTimeout:                   nil,
				},
			},
			wantErr: false,
		},
		{
			name: "authenticate cognito with allow behavior",
			config: &elbv2gw.AuthenticateCognitoActionConfig{
				UserPoolArn:                      userPoolArn,
				UserPoolClientID:                 userPoolClientID,
				UserPoolDomain:                   userPoolDomain,
				AuthenticationRequestExtraParams: &map[string]string{"custom": "value"},
				OnUnauthenticatedRequest:         &allowBehavior,
				Scope:                            &scope,
				SessionCookieName:                awssdk.String("CustomSessionCookie"),
				SessionTimeout:                   awssdk.Int64(3600),
			},
			want: &elbv2model.Action{
				Type: elbv2model.ActionTypeAuthenticateCognito,
				AuthenticateCognitoConfig: &elbv2model.AuthenticateCognitoActionConfig{
					UserPoolARN:                      userPoolArn,
					UserPoolClientID:                 userPoolClientID,
					UserPoolDomain:                   userPoolDomain,
					AuthenticationRequestExtraParams: map[string]string{"custom": "value"},
					OnUnauthenticatedRequest:         elbv2model.AuthenticateCognitoActionConditionalBehaviorAllow,
					Scope:                            awssdk.String(scope),
					SessionCookieName:                awssdk.String("CustomSessionCookie"),
					SessionTimeout:                   awssdk.Int64(3600),
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := buildAuthenticateCognitoAction(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, got)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildAuthenticateOIDCAction(t *testing.T) {
	issuer := "https://example.okta.com"
	authzEndpoint := "https://example.okta.com/oauth2/v1/authorize"
	tokenEndpoint := "https://example.okta.com/oauth2/v1/token"
	userInfoEndpoint := "https://example.okta.com/oauth2/v1/userinfo"
	scope := "openid profile"
	sessionCookieName := "AWSELBAuthSessionCookie"
	sessionTimeout := int64(604800)
	authRequestExtraParams := map[string]string{
		"prompt":  "login",
		"display": "page",
	}

	authenticateBehavior := elbv2gw.AuthenticateOidcActionConditionalBehaviorEnumAuthenticate

	secretName := "oidc-secret"
	secretNamespace := "test-namespace"
	clientID := "test-client-id"
	clientSecret := "test-client-secret"

	tests := []struct {
		name          string
		config        *elbv2gw.AuthenticateOidcActionConfig
		secretData    map[string][]byte
		secretExists  bool
		want          *elbv2model.Action
		wantSecretRef *types.NamespacedName
		wantErr       bool
	}{
		{
			name: "authenticate OIDC with all fields",
			config: &elbv2gw.AuthenticateOidcActionConfig{
				Issuer:                issuer,
				AuthorizationEndpoint: authzEndpoint,
				TokenEndpoint:         tokenEndpoint,
				UserInfoEndpoint:      userInfoEndpoint,
				Secret: &elbv2gw.Secret{
					Name: secretName,
				},
				AuthenticationRequestExtraParams: &authRequestExtraParams,
				OnUnauthenticatedRequest:         &authenticateBehavior,
				Scope:                            &scope,
				SessionCookieName:                &sessionCookieName,
				SessionTimeout:                   &sessionTimeout,
			},
			secretData: map[string][]byte{
				"clientID":     []byte(clientID),
				"clientSecret": []byte(clientSecret),
			},
			secretExists: true,
			want: &elbv2model.Action{
				Type: elbv2model.ActionTypeAuthenticateOIDC,
				AuthenticateOIDCConfig: &elbv2model.AuthenticateOIDCActionConfig{
					Issuer:                           issuer,
					AuthorizationEndpoint:            authzEndpoint,
					TokenEndpoint:                    tokenEndpoint,
					UserInfoEndpoint:                 userInfoEndpoint,
					ClientID:                         clientID,
					ClientSecret:                     clientSecret,
					AuthenticationRequestExtraParams: authRequestExtraParams,
					OnUnauthenticatedRequest:         elbv2model.AuthenticateOIDCActionConditionalBehavior(authenticateBehavior),
					Scope:                            &scope,
					SessionCookieName:                &sessionCookieName,
					SessionTimeout:                   &sessionTimeout,
				},
			},
			wantSecretRef: &types.NamespacedName{
				Namespace: secretNamespace,
				Name:      secretName,
			},
			wantErr: false,
		},
		{
			name: "authenticate OIDC with backward compatible clientId key",
			config: &elbv2gw.AuthenticateOidcActionConfig{
				Issuer:                issuer,
				AuthorizationEndpoint: authzEndpoint,
				TokenEndpoint:         tokenEndpoint,
				UserInfoEndpoint:      userInfoEndpoint,
				Secret: &elbv2gw.Secret{
					Name: secretName,
				},
				AuthenticationRequestExtraParams: &map[string]string{},
				OnUnauthenticatedRequest:         &authenticateBehavior,
			},
			secretData: map[string][]byte{
				"clientId":     []byte(clientID), // lowercase 'd' for backward compatibility
				"clientSecret": []byte(clientSecret),
			},
			secretExists: true,
			want: &elbv2model.Action{
				Type: elbv2model.ActionTypeAuthenticateOIDC,
				AuthenticateOIDCConfig: &elbv2model.AuthenticateOIDCActionConfig{
					Issuer:                           issuer,
					AuthorizationEndpoint:            authzEndpoint,
					TokenEndpoint:                    tokenEndpoint,
					UserInfoEndpoint:                 userInfoEndpoint,
					ClientID:                         clientID,
					ClientSecret:                     clientSecret,
					AuthenticationRequestExtraParams: map[string]string{},
					OnUnauthenticatedRequest:         elbv2model.AuthenticateOIDCActionConditionalBehaviorAuthenticate,
					Scope:                            nil,
					SessionCookieName:                nil,
					SessionTimeout:                   nil,
				},
			},
			wantSecretRef: &types.NamespacedName{
				Namespace: secretNamespace, // Should use route namespace when not specified
				Name:      secretName,
			},
			wantErr: false,
		},
		{
			name: "secret not found",
			config: &elbv2gw.AuthenticateOidcActionConfig{
				Issuer:                issuer,
				AuthorizationEndpoint: authzEndpoint,
				TokenEndpoint:         tokenEndpoint,
				UserInfoEndpoint:      userInfoEndpoint,
				Secret: &elbv2gw.Secret{
					Name: "nonexistent-secret",
				},
				AuthenticationRequestExtraParams: &map[string]string{},
				OnUnauthenticatedRequest:         &authenticateBehavior,
			},
			secretExists:  false,
			want:          nil,
			wantSecretRef: nil,
			wantErr:       true,
		},
		{
			name: "missing clientID in secret",
			config: &elbv2gw.AuthenticateOidcActionConfig{
				Issuer:                issuer,
				AuthorizationEndpoint: authzEndpoint,
				TokenEndpoint:         tokenEndpoint,
				UserInfoEndpoint:      userInfoEndpoint,
				Secret: &elbv2gw.Secret{
					Name: secretName,
				},
				AuthenticationRequestExtraParams: &map[string]string{},
				OnUnauthenticatedRequest:         &authenticateBehavior,
			},
			secretData: map[string][]byte{
				"clientSecret": []byte(clientSecret),
				// missing clientID
			},
			secretExists:  true,
			want:          nil,
			wantSecretRef: nil,
			wantErr:       true,
		},
		{
			name: "missing clientSecret in secret",
			config: &elbv2gw.AuthenticateOidcActionConfig{
				Issuer:                issuer,
				AuthorizationEndpoint: authzEndpoint,
				TokenEndpoint:         tokenEndpoint,
				UserInfoEndpoint:      userInfoEndpoint,
				Secret: &elbv2gw.Secret{
					Name: secretName,
				},
				AuthenticationRequestExtraParams: &map[string]string{},
				OnUnauthenticatedRequest:         &authenticateBehavior,
			},
			secretData: map[string][]byte{
				"clientID": []byte(clientID),
				// missing clientSecret
			},
			secretExists:  true,
			want:          nil,
			wantSecretRef: nil,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock k8s client
			k8sClient := fake.NewClientBuilder().Build()

			// Create mock route
			mockRoute := &MockRoute{
				Kind:      HTTPRouteKind,
				Namespace: secretNamespace,
				Name:      "test-route",
			}

			// Create mock secretManager
			mockSecretsManager := newMockSecretsManager(k8sClient)

			// Create secret if it should exist
			if tt.secretExists {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: secretNamespace,
					},
					Data: tt.secretData,
				}
				err := k8sClient.Create(ctx, secret)
				assert.NoError(t, err)
				mockSecretsManager.MonitorSecrets("testcfg", []types.NamespacedName{k8s.NamespacedName(secret)})
			}

			got, gotSecretRef, err := buildAuthenticateOIDCAction(ctx, tt.config, mockRoute, k8sClient, mockSecretsManager)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
				assert.Nil(t, gotSecretRef)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantSecretRef, gotSecretRef)
		})
	}
}

type mockSecretsManager struct {
	cache     map[string]*corev1.Secret
	k8sClient client.Client
}

func newMockSecretsManager(k8sClient client.Client) *mockSecretsManager {
	return &mockSecretsManager{
		cache:     make(map[string]*corev1.Secret),
		k8sClient: k8sClient,
	}
}

func (m *mockSecretsManager) MonitorSecrets(consumerID string, secrets []types.NamespacedName) {
	// Simulate caching - pre-load secrets into cache
	for _, secretKey := range secrets {
		secret := &corev1.Secret{}
		if err := m.k8sClient.Get(context.Background(), secretKey, secret); err == nil {
			m.cache[secretKey.String()] = secret.DeepCopy()
		}
	}
}

func (m *mockSecretsManager) GetSecret(ctx context.Context, k8sClient client.Client, secretKey types.NamespacedName) (*corev1.Secret, error) {
	// Check cache first
	if cached, exists := m.cache[secretKey.String()]; exists {
		return cached.DeepCopy(), nil
	}

	// Fallback to API
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, secretKey, secret); err != nil {
		return nil, err
	}
	return secret, nil
}

func Test_buildJwtValidationAction(t *testing.T) {
	tests := []struct {
		name    string
		config  *elbv2gw.JwtValidationActionConfig
		want    *elbv2model.Action
		wantErr bool
	}{
		{
			name: "jwt validation with all fields",
			config: &elbv2gw.JwtValidationActionConfig{
				JwksEndpoint: "https://example.com/.well-known/jwks.json",
				Issuer:       "https://example.com",
				AdditionalClaims: []elbv2gw.JwtValidationActionAdditionalClaim{
					{
						Format: elbv2gw.FormatSingleString,
						Name:   "admin",
						Values: []string{"true"},
					},
					{
						Format: elbv2gw.FormatSpaceSeparatedValues,
						Name:   "scope",
						Values: []string{"read:api", "write:api"},
					},
					{
						Format: elbv2gw.FormatStringArray,
						Name:   "roles",
						Values: []string{"user", "admin"},
					},
				},
			},
			want: &elbv2model.Action{
				Type: elbv2model.ActionTypeJwtValidation,
				JwtValidationConfig: &elbv2model.JwtValidationConfig{
					JwksEndpoint: "https://example.com/.well-known/jwks.json",
					Issuer:       "https://example.com",
					AdditionalClaims: []elbv2model.JwtAdditionalClaim{
						{
							Format: elbv2model.FormatSingleString,
							Name:   "admin",
							Values: []string{"true"},
						},
						{
							Format: elbv2model.FormatSpaceSeparatedValues,
							Name:   "scope",
							Values: []string{"read:api", "write:api"},
						},
						{
							Format: elbv2model.FormatStringArray,
							Name:   "roles",
							Values: []string{"user", "admin"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "jwt validation without additional claims",
			config: &elbv2gw.JwtValidationActionConfig{
				JwksEndpoint:     "https://auth.example.com/jwks",
				Issuer:           "https://auth.example.com",
				AdditionalClaims: []elbv2gw.JwtValidationActionAdditionalClaim{},
			},
			want: &elbv2model.Action{
				Type: elbv2model.ActionTypeJwtValidation,
				JwtValidationConfig: &elbv2model.JwtValidationConfig{
					JwksEndpoint:     "https://auth.example.com/jwks",
					Issuer:           "https://auth.example.com",
					AdditionalClaims: []elbv2model.JwtAdditionalClaim{},
				},
			},
			wantErr: false,
		},
		{
			name: "jwt validation with single additional claim",
			config: &elbv2gw.JwtValidationActionConfig{
				JwksEndpoint: "https://example.com/jwks",
				Issuer:       "https://example.com",
				AdditionalClaims: []elbv2gw.JwtValidationActionAdditionalClaim{
					{
						Format: elbv2gw.FormatSingleString,
						Name:   "email_verified",
						Values: []string{"true"},
					},
				},
			},
			want: &elbv2model.Action{
				Type: elbv2model.ActionTypeJwtValidation,
				JwtValidationConfig: &elbv2model.JwtValidationConfig{
					JwksEndpoint: "https://example.com/jwks",
					Issuer:       "https://example.com",
					AdditionalClaims: []elbv2model.JwtAdditionalClaim{
						{
							Format: elbv2model.FormatSingleString,
							Name:   "email_verified",
							Values: []string{"true"},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := buildJwtValidationAction(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, got)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_BuildRulePreRoutingAction(t *testing.T) {
	ctx := context.Background()
	k8sClient := fake.NewClientBuilder().Build()
	mockSecretsManager := newMockSecretsManager(k8sClient)
	mockRoute := &MockRoute{
		Kind:      HTTPRouteKind,
		Namespace: "test-namespace",
		Name:      "test-route",
	}

	tests := []struct {
		name        string
		action      *elbv2gw.Action
		wantType    elbv2model.ActionType
		wantErr     bool
		errContains string
	}{
		{
			name: "jwt-validation action",
			action: &elbv2gw.Action{
				Type: elbv2gw.ActionTypeJwtValidation,
				JwtValidationConfig: &elbv2gw.JwtValidationActionConfig{
					JwksEndpoint: "https://example.com/jwks",
					Issuer:       "https://example.com",
				},
			},
			wantType: elbv2model.ActionTypeJwtValidation,
			wantErr:  false,
		},
		{
			name: "authenticate-cognito action",
			action: &elbv2gw.Action{
				Type: elbv2gw.ActionTypeAuthenticateCognito,
				AuthenticateCognitoConfig: &elbv2gw.AuthenticateCognitoActionConfig{
					UserPoolArn:                      "arn:aws:cognito-idp:us-west-2:123456789012:userpool/us-west-2_EXAMPLE",
					UserPoolClientID:                 "client123",
					UserPoolDomain:                   "my-domain",
					AuthenticationRequestExtraParams: &map[string]string{},
					OnUnauthenticatedRequest:         (*elbv2gw.AuthenticateCognitoActionConditionalBehaviorEnum)(awssdk.String(string(elbv2gw.AuthenticateCognitoActionConditionalBehaviorEnumAuthenticate))),
				},
			},
			wantType: elbv2model.ActionTypeAuthenticateCognito,
			wantErr:  false,
		},
		{
			name: "unsupported action type",
			action: &elbv2gw.Action{
				Type: elbv2gw.ActionTypeForward,
			},
			wantErr:     true,
			errContains: "unsupported action type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := BuildRulePreRoutingAction(ctx, mockRoute, tt.action, k8sClient, mockSecretsManager)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, got)
			assert.Equal(t, tt.wantType, got.Type)
		})
	}
}
