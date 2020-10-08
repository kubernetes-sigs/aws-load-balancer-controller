package ingress

import (
	"context"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"testing"
)

func Test_defaultAuthConfigBuilder_Build(t *testing.T) {
	type args struct {
		svcAndIngAnnotations map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    AuthConfig
		wantErr error
	}{
		{
			name: "cognito auth annotation",
			args: args{
				svcAndIngAnnotations: map[string]string{
					"alb.ingress.kubernetes.io/auth-type":                       "cognito",
					"alb.ingress.kubernetes.io/auth-idp-cognito":                `{"userPoolARN":"arn:aws:cognito-idp:us-west-2:xxx:userpool/xxx","userPoolClientID":"my-clientID","userPoolDomain":"my-domain","authenticationRequestExtraParams":{"key":"value"}}`,
					"alb.ingress.kubernetes.io/auth-on-unauthenticated-request": "deny",
					"alb.ingress.kubernetes.io/auth-scope":                      "email",
					"alb.ingress.kubernetes.io/auth-session-cookie":             "my-cookie",
					"alb.ingress.kubernetes.io/auth-session-timeout":            "86400",
				},
			},
			want: AuthConfig{
				Type: AuthTypeCognito,
				IDPConfigCognito: &AuthIDPConfigCognito{
					UserPoolARN:      "arn:aws:cognito-idp:us-west-2:xxx:userpool/xxx",
					UserPoolClientID: "my-clientID",
					UserPoolDomain:   "my-domain",
					AuthenticationRequestExtraParams: map[string]string{
						"key": "value",
					},
				},
				OnUnauthenticatedRequest: "deny",
				Scope:                    "email",
				SessionCookieName:        "my-cookie",
				SessionTimeout:           86400,
			},
		},
		{
			name: "cognito auth annotation - old camelcase case json key",
			args: args{
				svcAndIngAnnotations: map[string]string{
					"alb.ingress.kubernetes.io/auth-type":                       "cognito",
					"alb.ingress.kubernetes.io/auth-idp-cognito":                `{"UserPoolArn":"arn:aws:cognito-idp:us-west-2:xxx:userpool/xxx","UserPoolClientId":"my-clientID","UserPoolDomain":"my-domain","AuthenticationRequestExtraParams":{"key":"value"}}`,
					"alb.ingress.kubernetes.io/auth-on-unauthenticated-request": "deny",
					"alb.ingress.kubernetes.io/auth-scope":                      "email",
					"alb.ingress.kubernetes.io/auth-session-cookie":             "my-cookie",
					"alb.ingress.kubernetes.io/auth-session-timeout":            "86400",
				},
			},
			want: AuthConfig{
				Type: AuthTypeCognito,
				IDPConfigCognito: &AuthIDPConfigCognito{
					UserPoolARN:      "arn:aws:cognito-idp:us-west-2:xxx:userpool/xxx",
					UserPoolClientID: "my-clientID",
					UserPoolDomain:   "my-domain",
					AuthenticationRequestExtraParams: map[string]string{
						"key": "value",
					},
				},
				OnUnauthenticatedRequest: "deny",
				Scope:                    "email",
				SessionCookieName:        "my-cookie",
				SessionTimeout:           86400,
			},
		},
		{
			name: "oidc auth annotation - old camelcase case JSON key",
			args: args{
				svcAndIngAnnotations: map[string]string{
					"alb.ingress.kubernetes.io/auth-type":                       "oidc",
					"alb.ingress.kubernetes.io/auth-idp-oidc":                   `{"issuer":"https://example.com","authorizationEndpoint":"https://authorization.example.com","tokenEndpoint":"https://token.example.com","userInfoEndpoint":"https://userinfo.example.com","secretName":"my-k8s-secret","authenticationRequestExtraParams":{"key":"value"}}`,
					"alb.ingress.kubernetes.io/auth-on-unauthenticated-request": "deny",
					"alb.ingress.kubernetes.io/auth-scope":                      "email",
					"alb.ingress.kubernetes.io/auth-session-cookie":             "my-cookie",
					"alb.ingress.kubernetes.io/auth-session-timeout":            "86400",
				},
			},
			want: AuthConfig{
				Type: AuthTypeOIDC,
				IDPConfigOIDC: &AuthIDPConfigOIDC{
					Issuer:                "https://example.com",
					AuthorizationEndpoint: "https://authorization.example.com",
					TokenEndpoint:         "https://token.example.com",
					UserInfoEndpoint:      "https://userinfo.example.com",
					SecretName:            "my-k8s-secret",
					AuthenticationRequestExtraParams: map[string]string{
						"key": "value",
					},
				},
				OnUnauthenticatedRequest: "deny",
				Scope:                    "email",
				SessionCookieName:        "my-cookie",
				SessionTimeout:           86400,
			},
		},
		{
			name: "oidc auth annotation - old camelcase case JSON key",
			args: args{
				svcAndIngAnnotations: map[string]string{
					"alb.ingress.kubernetes.io/auth-type":                       "oidc",
					"alb.ingress.kubernetes.io/auth-idp-oidc":                   `{"Issuer":"https://example.com","AuthorizationEndpoint":"https://authorization.example.com","TokenEndpoint":"https://token.example.com","UserInfoEndpoint":"https://userinfo.example.com","SecretName":"my-k8s-secret","AuthenticationRequestExtraParams":{"key":"value"}}`,
					"alb.ingress.kubernetes.io/auth-on-unauthenticated-request": "deny",
					"alb.ingress.kubernetes.io/auth-scope":                      "email",
					"alb.ingress.kubernetes.io/auth-session-cookie":             "my-cookie",
					"alb.ingress.kubernetes.io/auth-session-timeout":            "86400",
				},
			},
			want: AuthConfig{
				Type: AuthTypeOIDC,
				IDPConfigOIDC: &AuthIDPConfigOIDC{
					Issuer:                "https://example.com",
					AuthorizationEndpoint: "https://authorization.example.com",
					TokenEndpoint:         "https://token.example.com",
					UserInfoEndpoint:      "https://userinfo.example.com",
					SecretName:            "my-k8s-secret",
					AuthenticationRequestExtraParams: map[string]string{
						"key": "value",
					},
				},
				OnUnauthenticatedRequest: "deny",
				Scope:                    "email",
				SessionCookieName:        "my-cookie",
				SessionTimeout:           86400,
			},
		},
		{
			name: "no auth annotation",
			args: args{
				svcAndIngAnnotations: map[string]string{},
			},
			want: AuthConfig{
				Type:                     AuthTypeNone,
				IDPConfigCognito:         nil,
				IDPConfigOIDC:            nil,
				OnUnauthenticatedRequest: defaultAuthOnUnauthenticatedRequest,
				Scope:                    defaultAuthScope,
				SessionCookieName:        defaultAuthSessionCookieName,
				SessionTimeout:           defaultAuthSessionTimeout,
			},
		},
		{
			// The secret index functionality for Ingress/Service relies on this
			// since we allow these auth annotations be configured on either Ingress/Service
			name: "oidc configuration should still be populated even authType is unspecified",
			args: args{
				svcAndIngAnnotations: map[string]string{
					"alb.ingress.kubernetes.io/auth-idp-oidc":                   `{"issuer":"https://example.com","authorizationEndpoint":"https://authorization.example.com","tokenEndpoint":"https://token.example.com","userInfoEndpoint":"https://userinfo.example.com","secretName":"my-k8s-secret","authenticationRequestExtraParams":{"key":"value"}}`,
					"alb.ingress.kubernetes.io/auth-on-unauthenticated-request": "deny",
					"alb.ingress.kubernetes.io/auth-scope":                      "email",
					"alb.ingress.kubernetes.io/auth-session-cookie":             "my-cookie",
					"alb.ingress.kubernetes.io/auth-session-timeout":            "86400",
				},
			},
			want: AuthConfig{
				Type: AuthTypeNone,
				IDPConfigOIDC: &AuthIDPConfigOIDC{
					Issuer:                "https://example.com",
					AuthorizationEndpoint: "https://authorization.example.com",
					TokenEndpoint:         "https://token.example.com",
					UserInfoEndpoint:      "https://userinfo.example.com",
					SecretName:            "my-k8s-secret",
					AuthenticationRequestExtraParams: map[string]string{
						"key": "value",
					},
				},
				OnUnauthenticatedRequest: "deny",
				Scope:                    "email",
				SessionCookieName:        "my-cookie",
				SessionTimeout:           86400,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			b := &defaultAuthConfigBuilder{
				annotationParser: annotationParser,
			}
			got, err := b.Build(context.Background(), tt.args.svcAndIngAnnotations)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
