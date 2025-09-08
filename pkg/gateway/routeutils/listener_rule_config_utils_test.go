package routeutils

import (
	"context"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	mock_client "sigs.k8s.io/aws-load-balancer-controller/mocks/controller-runtime/client"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

func TestFilterListenerRuleConfigBySecret(t *testing.T) {
	tests := []struct {
		name          string
		secret        *corev1.Secret
		mockSetup     func(*gomock.Controller) client.Client
		expectedCount int
		expectedErr   error
		expectedNames []string
	}{
		{
			name:          "Nil secret returns nil",
			secret:        nil,
			mockSetup:     func(ctrl *gomock.Controller) client.Client { return nil },
			expectedCount: 0,
			expectedErr:   nil,
		},
		{
			name: "No ListenerRuleConfigurations reference the secret",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
				},
			},
			mockSetup: func(ctrl *gomock.Controller) client.Client {
				k8sClient := testutils.GenerateTestClient()

				// Create a ListenerRuleConfiguration that doesn't reference the secret
				k8sClient.Create(context.Background(), &elbv2gw.ListenerRuleConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-1",
						Namespace: "test-ns",
					},
					Spec: elbv2gw.ListenerRuleConfigurationSpec{
						Actions: []elbv2gw.Action{
							{
								Type: elbv2gw.ActionTypeFixedResponse,
								FixedResponseConfig: &elbv2gw.FixedResponseActionConfig{
									StatusCode: 200,
								},
							},
						},
					},
				})

				return k8sClient
			},
			expectedCount: 0,
			expectedErr:   nil,
		},
		{
			name: "Single ListenerRuleConfiguration references the secret",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oidc-secret",
					Namespace: "test-ns",
				},
			},
			mockSetup: func(ctrl *gomock.Controller) client.Client {
				k8sClient := testutils.GenerateTestClient()

				// Create a ListenerRuleConfiguration that references the secret
				k8sClient.Create(context.Background(), &elbv2gw.ListenerRuleConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-with-oidc",
						Namespace: "test-ns",
					},
					Spec: elbv2gw.ListenerRuleConfigurationSpec{
						Actions: []elbv2gw.Action{
							{
								Type: elbv2gw.ActionTypeAuthenticateOIDC,
								AuthenticateOIDCConfig: &elbv2gw.AuthenticateOidcActionConfig{
									AuthorizationEndpoint: "https://example.com/auth",
									TokenEndpoint:         "https://example.com/token",
									UserInfoEndpoint:      "https://example.com/userinfo",
									Issuer:                "https://example.com",
									Secret: &elbv2gw.Secret{
										Name: "oidc-secret",
									},
								},
							},
						},
					},
				})
				// Create a ListenerRuleConfiguration that references the secret but in other namespace
				k8sClient.Create(context.Background(), &elbv2gw.ListenerRuleConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-with-oidc",
						Namespace: "other-ns",
					},
					Spec: elbv2gw.ListenerRuleConfigurationSpec{
						Actions: []elbv2gw.Action{
							{
								Type: elbv2gw.ActionTypeAuthenticateOIDC,
								AuthenticateOIDCConfig: &elbv2gw.AuthenticateOidcActionConfig{
									AuthorizationEndpoint: "https://example.com/auth",
									TokenEndpoint:         "https://example.com/token",
									UserInfoEndpoint:      "https://example.com/userinfo",
									Issuer:                "https://example.com",
									Secret: &elbv2gw.Secret{
										Name: "oidc-secret",
									},
								},
							},
						},
					},
				})

				return k8sClient
			},
			expectedCount: 1,
			expectedErr:   nil,
			expectedNames: []string{"config-with-oidc"},
		},
		{
			name: "Multiple ListenerRuleConfigurations, some reference the secret",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-secret",
					Namespace: "test-ns",
				},
			},
			mockSetup: func(ctrl *gomock.Controller) client.Client {
				k8sClient := testutils.GenerateTestClient()

				// Config 1: References the secret
				k8sClient.Create(context.Background(), &elbv2gw.ListenerRuleConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-1",
						Namespace: "test-ns",
					},
					Spec: elbv2gw.ListenerRuleConfigurationSpec{
						Actions: []elbv2gw.Action{
							{
								Type: elbv2gw.ActionTypeAuthenticateOIDC,
								AuthenticateOIDCConfig: &elbv2gw.AuthenticateOidcActionConfig{
									AuthorizationEndpoint: "https://example.com/auth",
									TokenEndpoint:         "https://example.com/token",
									UserInfoEndpoint:      "https://example.com/userinfo",
									Issuer:                "https://example.com",
									Secret: &elbv2gw.Secret{
										Name: "shared-secret",
									},
								},
							},
						},
					},
				})

				// Config 2: Does not reference the secret
				k8sClient.Create(context.Background(), &elbv2gw.ListenerRuleConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-2",
						Namespace: "test-ns",
					},
					Spec: elbv2gw.ListenerRuleConfigurationSpec{
						Actions: []elbv2gw.Action{
							{
								Type: elbv2gw.ActionTypeFixedResponse,
								FixedResponseConfig: &elbv2gw.FixedResponseActionConfig{
									StatusCode: 404,
								},
							},
						},
					},
				})

				// Config 3: References a different secret
				k8sClient.Create(context.Background(), &elbv2gw.ListenerRuleConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-3",
						Namespace: "test-ns",
					},
					Spec: elbv2gw.ListenerRuleConfigurationSpec{
						Actions: []elbv2gw.Action{
							{
								Type: elbv2gw.ActionTypeAuthenticateOIDC,
								AuthenticateOIDCConfig: &elbv2gw.AuthenticateOidcActionConfig{
									AuthorizationEndpoint: "https://example.com/auth",
									TokenEndpoint:         "https://example.com/token",
									UserInfoEndpoint:      "https://example.com/userinfo",
									Issuer:                "https://example.com",
									Secret: &elbv2gw.Secret{
										Name: "different-secret",
									},
								},
							},
						},
					},
				})

				// Config 4: Also references the shared secret
				k8sClient.Create(context.Background(), &elbv2gw.ListenerRuleConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-4",
						Namespace: "test-ns",
					},
					Spec: elbv2gw.ListenerRuleConfigurationSpec{
						Actions: []elbv2gw.Action{
							{
								Type: elbv2gw.ActionTypeAuthenticateOIDC,
								AuthenticateOIDCConfig: &elbv2gw.AuthenticateOidcActionConfig{
									AuthorizationEndpoint: "https://example.com/auth",
									TokenEndpoint:         "https://example.com/token",
									UserInfoEndpoint:      "https://example.com/userinfo",
									Issuer:                "https://example.com",
									Secret: &elbv2gw.Secret{
										Name: "shared-secret",
									},
								},
							},
						},
					},
				})

				return k8sClient
			},
			expectedCount: 2,
			expectedErr:   nil,
			expectedNames: []string{"config-1", "config-4"},
		},
		{
			name: "Secret reference with other namespace",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-name",
					Namespace: "other-ns",
				},
			},
			mockSetup: func(ctrl *gomock.Controller) client.Client {
				k8sClient := testutils.GenerateTestClient()

				k8sClient.Create(context.Background(), &elbv2gw.ListenerRuleConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-wrong-ns",
						Namespace: "config-ns",
					},
					Spec: elbv2gw.ListenerRuleConfigurationSpec{
						Actions: []elbv2gw.Action{
							{
								Type: elbv2gw.ActionTypeAuthenticateOIDC,
								AuthenticateOIDCConfig: &elbv2gw.AuthenticateOidcActionConfig{
									AuthorizationEndpoint: "https://example.com/auth",
									TokenEndpoint:         "https://example.com/token",
									UserInfoEndpoint:      "https://example.com/userinfo",
									Issuer:                "https://example.com",
									Secret: &elbv2gw.Secret{
										Name: "secret-name",
									},
								},
							},
						},
					},
				})

				return k8sClient
			},
			expectedCount: 0,
			expectedErr:   nil,
		},
		{
			name: "K8s client error",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
				},
			},
			mockSetup: func(ctrl *gomock.Controller) client.Client {
				mockClient := mock_client.NewMockClient(ctrl)
				listOpts := []client.ListOption{
					client.InNamespace("test-ns"), // namespace-scoped search
				}
				mockClient.EXPECT().List(gomock.Any(), &elbv2gw.ListenerRuleConfigurationList{}, listOpts).Return(fmt.Errorf("API error"))
				return mockClient
			},
			expectedCount: 0,
			expectedErr:   fmt.Errorf("API error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			var client client.Client
			if tt.mockSetup != nil {
				client = tt.mockSetup(ctrl)
			}

			result, err := FilterListenerRuleConfigBySecret(context.Background(), client, tt.secret)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tt.expectedCount)

				if tt.expectedNames != nil {
					actualNames := make([]string, len(result))
					for i, config := range result {
						actualNames[i] = config.Name
					}
					assert.ElementsMatch(t, tt.expectedNames, actualNames)
				}
			}
		})
	}
}

func TestIsListenerRuleConfigReferencingSecret(t *testing.T) {
	tests := []struct {
		name               string
		listenerRuleConfig *elbv2gw.ListenerRuleConfiguration
		secretKey          types.NamespacedName
		expected           bool
	}{
		{
			name: "No actions",
			listenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-1",
					Namespace: "test-ns",
				},
				Spec: elbv2gw.ListenerRuleConfigurationSpec{},
			},
			secretKey: types.NamespacedName{Name: "test-secret", Namespace: "test-ns"},
			expected:  false,
		},
		{
			name: "Actions with no OIDC config",
			listenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-1",
					Namespace: "test-ns",
				},
				Spec: elbv2gw.ListenerRuleConfigurationSpec{
					Actions: []elbv2gw.Action{
						{
							Type: elbv2gw.ActionTypeFixedResponse,
							FixedResponseConfig: &elbv2gw.FixedResponseActionConfig{
								StatusCode: 200,
							},
						},
					},
				},
			},
			secretKey: types.NamespacedName{Name: "test-secret", Namespace: "test-ns"},
			expected:  false,
		},
		{
			name: "OIDC action with no secret",
			listenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-1",
					Namespace: "test-ns",
				},
				Spec: elbv2gw.ListenerRuleConfigurationSpec{
					Actions: []elbv2gw.Action{
						{
							Type: elbv2gw.ActionTypeAuthenticateOIDC,
							AuthenticateOIDCConfig: &elbv2gw.AuthenticateOidcActionConfig{
								AuthorizationEndpoint: "https://example.com/auth",
								TokenEndpoint:         "https://example.com/token",
								UserInfoEndpoint:      "https://example.com/userinfo",
								Issuer:                "https://example.com",
								Secret:                nil,
							},
						},
					},
				},
			},
			secretKey: types.NamespacedName{Name: "test-secret", Namespace: "test-ns"},
			expected:  false,
		},
		{
			name: "OIDC action with matching secret (same namespace)",
			listenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-1",
					Namespace: "test-ns",
				},
				Spec: elbv2gw.ListenerRuleConfigurationSpec{
					Actions: []elbv2gw.Action{
						{
							Type: elbv2gw.ActionTypeAuthenticateOIDC,
							AuthenticateOIDCConfig: &elbv2gw.AuthenticateOidcActionConfig{
								AuthorizationEndpoint: "https://example.com/auth",
								TokenEndpoint:         "https://example.com/token",
								UserInfoEndpoint:      "https://example.com/userinfo",
								Issuer:                "https://example.com",
								Secret: &elbv2gw.Secret{
									Name: "test-secret",
								},
							},
						},
					},
				},
			},
			secretKey: types.NamespacedName{Name: "test-secret", Namespace: "test-ns"},
			expected:  true,
		},
		{
			name: "OIDC action with matching secret (explicit namespace)",
			listenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-1",
					Namespace: "config-ns",
				},
				Spec: elbv2gw.ListenerRuleConfigurationSpec{
					Actions: []elbv2gw.Action{
						{
							Type: elbv2gw.ActionTypeAuthenticateOIDC,
							AuthenticateOIDCConfig: &elbv2gw.AuthenticateOidcActionConfig{
								AuthorizationEndpoint: "https://example.com/auth",
								TokenEndpoint:         "https://example.com/token",
								UserInfoEndpoint:      "https://example.com/userinfo",
								Issuer:                "https://example.com",
								Secret: &elbv2gw.Secret{
									Name: "test-secret",
								},
							},
						},
					},
				},
			},
			secretKey: types.NamespacedName{Name: "test-secret", Namespace: "config-ns"},
			expected:  true,
		},
		{
			name: "OIDC action with non-matching secret name",
			listenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-1",
					Namespace: "test-ns",
				},
				Spec: elbv2gw.ListenerRuleConfigurationSpec{
					Actions: []elbv2gw.Action{
						{
							Type: elbv2gw.ActionTypeAuthenticateOIDC,
							AuthenticateOIDCConfig: &elbv2gw.AuthenticateOidcActionConfig{
								AuthorizationEndpoint: "https://example.com/auth",
								TokenEndpoint:         "https://example.com/token",
								UserInfoEndpoint:      "https://example.com/userinfo",
								Issuer:                "https://example.com",
								Secret: &elbv2gw.Secret{
									Name: "different-secret",
								},
							},
						},
					},
				},
			},
			secretKey: types.NamespacedName{Name: "test-secret", Namespace: "test-ns"},
			expected:  false,
		},
		{
			name: "Multiple actions, one matches",
			listenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-1",
					Namespace: "test-ns",
				},
				Spec: elbv2gw.ListenerRuleConfigurationSpec{
					Actions: []elbv2gw.Action{
						{
							Type: elbv2gw.ActionTypeFixedResponse,
							FixedResponseConfig: &elbv2gw.FixedResponseActionConfig{
								StatusCode: 200,
							},
						},
						{
							Type: elbv2gw.ActionTypeAuthenticateOIDC,
							AuthenticateOIDCConfig: &elbv2gw.AuthenticateOidcActionConfig{
								AuthorizationEndpoint: "https://example.com/auth",
								TokenEndpoint:         "https://example.com/token",
								UserInfoEndpoint:      "https://example.com/userinfo",
								Issuer:                "https://example.com",
								Secret: &elbv2gw.Secret{
									Name: "test-secret",
								},
							},
						},
					},
				},
			},
			secretKey: types.NamespacedName{Name: "test-secret", Namespace: "test-ns"},
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isListenerRuleConfigReferencingSecret(tt.listenerRuleConfig, tt.secretKey)
			assert.Equal(t, tt.expected, result)
		})
	}
}
