package ingress

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
)

func Test_defaultAuthConfigBuilder_Build(t *testing.T) {
	type args struct {
		ingressClassParams   elbv2api.IngressClassParams
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
		{
			name: "cognito authentication configuration via ingress class params",
			args: args{
				ingressClassParams: elbv2api.IngressClassParams{
					Spec: elbv2api.IngressClassParamsSpec{
						AuthConfig: &elbv2api.AuthConfig{
							Type: "cognito",
							IDPConfigCognito: &elbv2api.AuthIDPConfigCognito{
								UserPoolARN:      "arn:aws:cognito-idp:us-east-1:xxx:userpool/xxx",
								UserPoolClientID: "client1234",
								UserPoolDomain:   "https://us-east-1xxx.auth.us-east-1.amazoncognito.com",
								AuthenticationRequestExtraParams: map[string]string{
									"key": "value",
								},
							},
							OnUnauthenticatedRequest: "deny",
							Scope:                    "aws.cognito.signin.user.admin email phone",
							SessionCookieName:        "my-session-cookie",
							SessionTimeout:           aws.Int64(1234),
						},
					},
				},
			},
			want: AuthConfig{
				Type: AuthTypeCognito,
				IDPConfigCognito: &AuthIDPConfigCognito{
					UserPoolARN:      "arn:aws:cognito-idp:us-east-1:xxx:userpool/xxx",
					UserPoolClientID: "client1234",
					UserPoolDomain:   "https://us-east-1xxx.auth.us-east-1.amazoncognito.com",
					AuthenticationRequestExtraParams: map[string]string{
						"key": "value",
					},
				},
				OnUnauthenticatedRequest: "deny",
				Scope:                    "aws.cognito.signin.user.admin email phone",
				SessionCookieName:        "my-session-cookie",
				SessionTimeout:           1234,
			},
		},
		{
			name: "cognito authentication configuration via ingress class params - should take precendence over annotations",
			args: args{
				ingressClassParams: elbv2api.IngressClassParams{
					Spec: elbv2api.IngressClassParamsSpec{
						AuthConfig: &elbv2api.AuthConfig{
							Type: "cognito",
							IDPConfigCognito: &elbv2api.AuthIDPConfigCognito{
								UserPoolARN:      "arn:aws:cognito-idp:us-east-1:xxx:userpool/xxx",
								UserPoolClientID: "client1234",
								UserPoolDomain:   "https://us-east-1xxx.auth.us-east-1.amazoncognito.com",
								AuthenticationRequestExtraParams: map[string]string{
									"key": "value",
								},
							},
							OnUnauthenticatedRequest: "deny",
							Scope:                    "aws.cognito.signin.user.admin email phone",
							SessionCookieName:        "my-session-cookie",
							SessionTimeout:           aws.Int64(1234),
						},
					},
				},
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
					UserPoolARN:      "arn:aws:cognito-idp:us-east-1:xxx:userpool/xxx",
					UserPoolClientID: "client1234",
					UserPoolDomain:   "https://us-east-1xxx.auth.us-east-1.amazoncognito.com",
					AuthenticationRequestExtraParams: map[string]string{
						"key": "value",
					},
				},
				OnUnauthenticatedRequest: "deny",
				Scope:                    "aws.cognito.signin.user.admin email phone",
				SessionCookieName:        "my-session-cookie",
				SessionTimeout:           1234,
			},
		},
		{
			name: "oidc authentication configuration via ingress class params",
			args: args{
				ingressClassParams: elbv2api.IngressClassParams{
					Spec: elbv2api.IngressClassParamsSpec{
						AuthConfig: &elbv2api.AuthConfig{
							Type: "oidc",
							IDPConfigOIDC: &elbv2api.AuthIDPConfigOIDC{
								Issuer:                "https://my-site.com",
								AuthorizationEndpoint: "https://super-strong-auth.my-site.com",
								TokenEndpoint:         "https://token.my-site.com",
								UserInfoEndpoint:      "https://user.my-site.com",
								SecretName:            "top-secret",
								AuthenticationRequestExtraParams: map[string]string{
									"key": "value",
								},
							},
							OnUnauthenticatedRequest: "deny",
							Scope:                    "email phone",
							SessionCookieName:        "my-session-cookie",
							SessionTimeout:           aws.Int64(1234),
						},
					},
				},
			},
			want: AuthConfig{
				Type: AuthTypeOIDC,
				IDPConfigOIDC: &AuthIDPConfigOIDC{
					Issuer:                "https://my-site.com",
					AuthorizationEndpoint: "https://super-strong-auth.my-site.com",
					TokenEndpoint:         "https://token.my-site.com",
					UserInfoEndpoint:      "https://user.my-site.com",
					SecretName:            "top-secret",
					AuthenticationRequestExtraParams: map[string]string{
						"key": "value",
					},
				},
				OnUnauthenticatedRequest: "deny",
				Scope:                    "email phone",
				SessionCookieName:        "my-session-cookie",
				SessionTimeout:           1234,
			},
		},
		{
			name: "oidc authentication configuration via ingress class params - should take precedence over annotations",
			args: args{
				ingressClassParams: elbv2api.IngressClassParams{
					Spec: elbv2api.IngressClassParamsSpec{
						AuthConfig: &elbv2api.AuthConfig{
							Type: "oidc",
							IDPConfigOIDC: &elbv2api.AuthIDPConfigOIDC{
								Issuer:                "https://my-site.com",
								AuthorizationEndpoint: "https://super-strong-auth.my-site.com",
								TokenEndpoint:         "https://token.my-site.com",
								UserInfoEndpoint:      "https://user.my-site.com",
								SecretName:            "top-secret",
								AuthenticationRequestExtraParams: map[string]string{
									"key": "value",
								},
							},
							OnUnauthenticatedRequest: "deny",
							Scope:                    "email phone",
							SessionCookieName:        "my-session-cookie",
							SessionTimeout:           aws.Int64(1234),
						},
					},
				},
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
					Issuer:                "https://my-site.com",
					AuthorizationEndpoint: "https://super-strong-auth.my-site.com",
					TokenEndpoint:         "https://token.my-site.com",
					UserInfoEndpoint:      "https://user.my-site.com",
					SecretName:            "top-secret",
					AuthenticationRequestExtraParams: map[string]string{
						"key": "value",
					},
				},
				OnUnauthenticatedRequest: "deny",
				Scope:                    "email phone",
				SessionCookieName:        "my-session-cookie",
				SessionTimeout:           1234,
			},
		},
		{
			name: "authentication configuration set to 'none' in ingress class params, should take precedence over annotations",
			args: args{
				ingressClassParams: elbv2api.IngressClassParams{
					Spec: elbv2api.IngressClassParamsSpec{
						AuthConfig: &elbv2api.AuthConfig{
							Type: "none",
						},
					},
				},
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
				Type:                     AuthTypeNone,
				IDPConfigCognito:         nil,
				IDPConfigOIDC:            nil,
				OnUnauthenticatedRequest: defaultAuthOnUnauthenticatedRequest,
				Scope:                    defaultAuthScope,
				SessionCookieName:        defaultAuthSessionCookieName,
				SessionTimeout:           defaultAuthSessionTimeout,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			b := &defaultAuthConfigBuilder{
				annotationParser: annotationParser,
			}
			got, err := b.Build(context.Background(), &tt.args.ingressClassParams, tt.args.svcAndIngAnnotations)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
