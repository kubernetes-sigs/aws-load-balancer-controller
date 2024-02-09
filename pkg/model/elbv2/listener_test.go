package elbv2

import (
	"encoding/json"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAuthenticateOIDCActionConfig_MarshalJSON(t *testing.T) {
	deny := AuthenticateOIDCActionConditionalBehaviorDeny
	type fields struct {
		AuthenticationRequestExtraParams map[string]string
		OnUnauthenticatedRequest         *AuthenticateOIDCActionConditionalBehavior
		Scope                            *string
		SessionCookieName                *string
		SessionTimeout                   *int64
		Issuer                           string
		AuthorizationEndpoint            string
		TokenEndpoint                    string
		UserInfoEndpoint                 string
		ClientID                         string
		ClientSecret                     string
	}
	tests := []struct {
		name string
		cfg  *AuthenticateOIDCActionConfig
		want string
	}{
		{
			name: "clientID and clientSecret should be redacted",
			cfg: &AuthenticateOIDCActionConfig{
				AuthenticationRequestExtraParams: map[string]string{"key": "value"},
				OnUnauthenticatedRequest:         &deny,
				Scope:                            awssdk.String("oidc"),
				SessionCookieName:                awssdk.String("my-cookie"),
				Issuer:                           "my-issuer",
				AuthorizationEndpoint:            "my-auth-endpoint",
				TokenEndpoint:                    "my-token-endpoint",
				UserInfoEndpoint:                 "my-user-endpoint",
				ClientID:                         "client-id",
				ClientSecret:                     "client-secret",
			},
			want: `{"authenticationRequestExtraParams":{"key":"value"},"onUnauthenticatedRequest":"deny","scope":"oidc","sessionCookieName":"my-cookie","issuer":"my-issuer","authorizationEndpoint":"my-auth-endpoint","tokenEndpoint":"my-token-endpoint","userInfoEndpoint":"my-user-endpoint","clientID":"[REDACTED]","clientSecret":"[REDACTED]"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, _ := json.Marshal(tt.cfg)
			got := string(payload)
			assert.JSONEq(t, tt.want, got)
		})
	}
}
