package ingress

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func Test_defaultModelBuildTask_buildAuthenticateOIDCAction(t *testing.T) {
	type env struct {
		secrets []*corev1.Secret
	}
	type args struct {
		authCfg   AuthConfig
		namespace string
	}
	authBehaviorAuthenticate := elbv2model.AuthenticateOIDCActionConditionalBehaviorAuthenticate
	tests := []struct {
		name    string
		env     env
		args    args
		want    elbv2model.Action
		wantErr error
	}{
		{
			name: "clientID & clientSecret configured",
			env: env{
				secrets: []*corev1.Secret{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "my-ns",
							Name:      "my-k8s-secret",
						},
						Data: map[string][]byte{
							"clientID":     []byte("my-client-id"),
							"clientSecret": []byte("my-client-secret"),
						},
					},
				},
			},
			args: args{
				authCfg: AuthConfig{
					Type: AuthTypeCognito,
					IDPConfigOIDC: &AuthIDPConfigOIDC{
						Issuer:                "https://example.com",
						AuthorizationEndpoint: "https://authorization.example.com",
						TokenEndpoint:         "https://token.example.com",
						UserInfoEndpoint:      "https://userinfo.example.co",
						SecretName:            "my-k8s-secret",
						AuthenticationRequestExtraParams: map[string]string{
							"key1": "value1",
						},
					},
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "email",
					SessionCookieName:        "my-session-cookie",
					SessionTimeout:           65536,
				},
				namespace: "my-ns",
			},
			want: elbv2model.Action{
				Type: elbv2model.ActionTypeAuthenticateOIDC,
				AuthenticateOIDCConfig: &elbv2model.AuthenticateOIDCActionConfig{
					Issuer:                "https://example.com",
					AuthorizationEndpoint: "https://authorization.example.com",
					TokenEndpoint:         "https://token.example.com",
					UserInfoEndpoint:      "https://userinfo.example.co",
					ClientID:              "my-client-id",
					ClientSecret:          "my-client-secret",
					AuthenticationRequestExtraParams: map[string]string{
						"key1": "value1",
					},
					OnUnauthenticatedRequest: &authBehaviorAuthenticate,
					Scope:                    awssdk.String("email"),
					SessionCookieName:        awssdk.String("my-session-cookie"),
					SessionTimeout:           awssdk.Int64(65536),
				},
			},
		},
		{
			name: "clientID & clientSecret configured - legacy clientId",
			env: env{
				secrets: []*corev1.Secret{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "my-ns",
							Name:      "my-k8s-secret",
						},
						Data: map[string][]byte{
							"clientId":     []byte("my-client-id"),
							"clientSecret": []byte("my-client-secret"),
						},
					},
				},
			},
			args: args{
				authCfg: AuthConfig{
					Type: AuthTypeCognito,
					IDPConfigOIDC: &AuthIDPConfigOIDC{
						Issuer:                "https://example.com",
						AuthorizationEndpoint: "https://authorization.example.com",
						TokenEndpoint:         "https://token.example.com",
						UserInfoEndpoint:      "https://userinfo.example.co",
						SecretName:            "my-k8s-secret",
						AuthenticationRequestExtraParams: map[string]string{
							"key1": "value1",
						},
					},
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "email",
					SessionCookieName:        "my-session-cookie",
					SessionTimeout:           65536,
				},
				namespace: "my-ns",
			},
			want: elbv2model.Action{
				Type: elbv2model.ActionTypeAuthenticateOIDC,
				AuthenticateOIDCConfig: &elbv2model.AuthenticateOIDCActionConfig{
					Issuer:                "https://example.com",
					AuthorizationEndpoint: "https://authorization.example.com",
					TokenEndpoint:         "https://token.example.com",
					UserInfoEndpoint:      "https://userinfo.example.co",
					ClientID:              "my-client-id",
					ClientSecret:          "my-client-secret",
					AuthenticationRequestExtraParams: map[string]string{
						"key1": "value1",
					},
					OnUnauthenticatedRequest: &authBehaviorAuthenticate,
					Scope:                    awssdk.String("email"),
					SessionCookieName:        awssdk.String("my-session-cookie"),
					SessionTimeout:           awssdk.Int64(65536),
				},
			},
		},
		{
			name: "missing IDPConfigOIDC",
			args: args{
				authCfg: AuthConfig{
					Type:          AuthTypeCognito,
					IDPConfigOIDC: nil,
				},
			},
			wantErr: errors.New("missing IDPConfigOIDC"),
		},
		{
			name: "missing clientID",
			env: env{
				secrets: []*corev1.Secret{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "my-ns",
							Name:      "my-k8s-secret",
						},
						Data: map[string][]byte{
							"clientSecret": []byte("my-client-secret"),
						},
					},
				},
			},
			args: args{
				authCfg: AuthConfig{
					Type: AuthTypeCognito,
					IDPConfigOIDC: &AuthIDPConfigOIDC{
						Issuer:                "https://example.com",
						AuthorizationEndpoint: "https://authorization.example.com",
						TokenEndpoint:         "https://token.example.com",
						UserInfoEndpoint:      "https://userinfo.example.co",
						SecretName:            "my-k8s-secret",
						AuthenticationRequestExtraParams: map[string]string{
							"key1": "value1",
						},
					},
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "email",
					SessionCookieName:        "my-session-cookie",
					SessionTimeout:           65536,
				},
				namespace: "my-ns",
			},
			wantErr: errors.New("missing clientID, secret: my-ns/my-k8s-secret"),
		},
		{
			name: "missing clientSecret",
			env: env{
				secrets: []*corev1.Secret{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "my-ns",
							Name:      "my-k8s-secret",
						},
						Data: map[string][]byte{
							"clientID": []byte("my-client-id"),
						},
					},
				},
			},
			args: args{
				authCfg: AuthConfig{
					Type: AuthTypeCognito,
					IDPConfigOIDC: &AuthIDPConfigOIDC{
						Issuer:                "https://example.com",
						AuthorizationEndpoint: "https://authorization.example.com",
						TokenEndpoint:         "https://token.example.com",
						UserInfoEndpoint:      "https://userinfo.example.co",
						SecretName:            "my-k8s-secret",
						AuthenticationRequestExtraParams: map[string]string{
							"key1": "value1",
						},
					},
					OnUnauthenticatedRequest: "authenticate",
					Scope:                    "email",
					SessionCookieName:        "my-session-cookie",
					SessionTimeout:           65536,
				},
				namespace: "my-ns",
			},
			wantErr: errors.New("missing clientSecret, secret: my-ns/my-k8s-secret"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			for _, secret := range tt.env.secrets {
				err := k8sClient.Create(context.Background(), secret.DeepCopy())
				assert.NoError(t, err)
			}

			task := &defaultModelBuildTask{
				k8sClient: k8sClient,
			}
			got, err := task.buildAuthenticateOIDCAction(context.Background(), tt.args.authCfg, tt.args.namespace)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildSSLRedirectAction(t *testing.T) {
	type args struct {
		sslRedirectConfig SSLRedirectConfig
	}
	tests := []struct {
		name string
		args args
		want elbv2model.Action
	}{
		{
			name: "SSLRedirect to 443 with 301",
			args: args{
				sslRedirectConfig: SSLRedirectConfig{
					SSLPort:    443,
					StatusCode: "HTTP_301",
				},
			},
			want: elbv2model.Action{
				Type: elbv2model.ActionTypeRedirect,
				RedirectConfig: &elbv2model.RedirectActionConfig{
					Port:       awssdk.String("443"),
					Protocol:   awssdk.String("HTTPS"),
					StatusCode: "HTTP_301",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
			task := &defaultModelBuildTask{}
			got := task.buildSSLRedirectAction(context.Background(), tt.args.sslRedirectConfig)
			assert.Equal(t, tt.want, got)
		})
	}
}
