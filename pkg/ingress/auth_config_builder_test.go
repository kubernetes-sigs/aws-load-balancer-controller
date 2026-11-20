package ingress

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/annotations"
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

func Test_defaultAuthConfigBuilder_buildAuthIDPConfigOIDC_discoveryEndpoint(t *testing.T) {
	// Helper that starts a fake OIDC IdP (TLS) server returning the given body.
	makeServer := func(statusCode int, body string) *httptest.Server {
		return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/.well-known/openid-configuration" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(statusCode)
			fmt.Fprint(w, body)
		}))
	}

	tests := []struct {
		name        string
		serverBody  string
		serverCode  int
		useHTTPURL  bool // use a plain http:// discoveryEndpoint
		want        *AuthIDPConfigOIDC
		wantErrPart string // substring expected in error message, empty = no error
	}{
		{
			name:       "discoveryEndpoint populates all four fields",
			serverCode: http.StatusOK,
			serverBody: `{
				"issuer":                 "https://idp.example.com",
				"authorization_endpoint": "https://idp.example.com/auth",
				"token_endpoint":         "https://idp.example.com/token",
				"userinfo_endpoint":      "https://idp.example.com/userinfo"
			}`,
			// annotation will be built dynamically after server starts
			want: &AuthIDPConfigOIDC{
				DiscoveryEndpoint:     "", // placeholder, set in test body
				Issuer:                "https://idp.example.com",
				AuthorizationEndpoint: "https://idp.example.com/auth",
				TokenEndpoint:         "https://idp.example.com/token",
				UserInfoEndpoint:      "https://idp.example.com/userinfo",
				SecretName:            "my-k8s-secret",
			},
		},
		{
			name:       "explicit fields override discovery document values",
			serverCode: http.StatusOK,
			serverBody: `{
				"issuer":                 "https://idp.example.com",
				"authorization_endpoint": "https://idp.example.com/auth",
				"token_endpoint":         "https://idp.example.com/token",
				"userinfo_endpoint":      "https://idp.example.com/userinfo"
			}`,
			want: &AuthIDPConfigOIDC{
				DiscoveryEndpoint:     "", // placeholder
				Issuer:                "https://override.example.com",
				AuthorizationEndpoint: "https://idp.example.com/auth",
				TokenEndpoint:         "https://idp.example.com/token",
				UserInfoEndpoint:      "https://idp.example.com/userinfo",
				SecretName:            "my-k8s-secret",
			},
		},
		{
			name:        "discovery endpoint returns non-200 status",
			serverCode:  http.StatusInternalServerError,
			serverBody:  `internal error`,
			wantErrPart: "returned status 500",
		},
		{
			name:        "discovery endpoint returns invalid JSON",
			serverCode:  http.StatusOK,
			serverBody:  `not-json`,
			wantErrPart: "failed to parse OIDC discovery document",
		},
		{
			name:        "discoveryEndpoint must use https",
			serverCode:  http.StatusOK,
			serverBody:  `{}`,
			useHTTPURL:  true,
			wantErrPart: "must use https",
		},
		{
			name:       "discovery document missing issuer",
			serverCode: http.StatusOK,
			serverBody: `{
				"authorization_endpoint": "https://idp.example.com/auth",
				"token_endpoint":         "https://idp.example.com/token",
				"userinfo_endpoint":      "https://idp.example.com/userinfo"
			}`,
			wantErrPart: "missing 'issuer'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := makeServer(tt.serverCode, tt.serverBody)
			defer server.Close()

			// Build the annotation JSON using the test server's URL as discoveryEndpoint.
			discoveryURL := server.URL
			if tt.useHTTPURL {
				discoveryURL = "http://idp.example.com"
			}
			var annotationJSON string
			if tt.name == "explicit fields override discovery document values" {
				annotationJSON = fmt.Sprintf(
					`{"discoveryEndpoint":%q,"issuer":"https://override.example.com","secretName":"my-k8s-secret"}`,
					discoveryURL,
				)
			} else {
				annotationJSON = fmt.Sprintf(
					`{"discoveryEndpoint":%q,"secretName":"my-k8s-secret"}`,
					discoveryURL,
				)
			}

			svcAndIngAnnotations := map[string]string{
				"alb.ingress.kubernetes.io/auth-type":     "oidc",
				"alb.ingress.kubernetes.io/auth-idp-oidc": annotationJSON,
			}

			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			b := &defaultAuthConfigBuilder{annotationParser: annotationParser, httpClient: server.Client()}
			got, err := b.buildAuthIDPConfigOIDC(context.Background(), svcAndIngAnnotations)

			if tt.wantErrPart != "" {
				assert.ErrorContains(t, err, tt.wantErrPart)
				return
			}

			assert.NoError(t, err)
			// Patch the expected DiscoveryEndpoint with the actual test server URL.
			tt.want.DiscoveryEndpoint = server.URL
			assert.Equal(t, tt.want, got)
		})
	}
}
