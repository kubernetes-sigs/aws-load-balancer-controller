package k8s

import (
	"context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_defaultSecretsManager_MonitorSecrets(t *testing.T) {
	type monitorSecretsCall struct {
		consumerID string
		secrets    []types.NamespacedName
	}
	tests := []struct {
		testName           string
		monitorSecretsCall []monitorSecretsCall
		wantSecrets        []types.NamespacedName
	}{
		{
			testName: "No secrets",
		},
		{
			testName: "Single Ingress consumer",
			monitorSecretsCall: []monitorSecretsCall{
				{
					consumerID: "ig-group1",
					secrets: []types.NamespacedName{
						{Name: "secret-1", Namespace: "ns-1"},
					},
				},
			},
			wantSecrets: []types.NamespacedName{
				{Name: "secret-1", Namespace: "ns-1"},
			},
		},
		{
			testName: "Single ingress consumer, multiple secrets",
			monitorSecretsCall: []monitorSecretsCall{
				{
					consumerID: "ig-group1",
					secrets: []types.NamespacedName{
						{Name: "secret-1", Namespace: "ns-1"},
						{Name: "secret-2", Namespace: "ns-2"},
						{Name: "secret-3", Namespace: "ns-3"},
					},
				},
			},
			wantSecrets: []types.NamespacedName{
				{Name: "secret-1", Namespace: "ns-1"},
				{Name: "secret-2", Namespace: "ns-2"},
				{Name: "secret-3", Namespace: "ns-3"},
			},
		},
		{
			testName: "Multiple consumers, mix of ingress and gateway, overlapping secrets",
			monitorSecretsCall: []monitorSecretsCall{
				{
					consumerID: "ig-group1",
					secrets: []types.NamespacedName{
						{Name: "secret-1", Namespace: "ns-1"},
						{Name: "secret-2", Namespace: "ns-2"},
						{Name: "secret-3", Namespace: "ns-3"},
					},
				},
				{
					consumerID: "gw-1",
					secrets: []types.NamespacedName{
						{Name: "secret-2", Namespace: "ns-2"},
						{Name: "secret-3", Namespace: "ns-3"},
						{Name: "secret-4", Namespace: "ns-4"},
					},
				},
			},
			wantSecrets: []types.NamespacedName{
				{Name: "secret-1", Namespace: "ns-1"},
				{Name: "secret-2", Namespace: "ns-2"},
				{Name: "secret-3", Namespace: "ns-3"},
				{Name: "secret-4", Namespace: "ns-4"},
			},
		},
		{
			testName: "Multiple ingress consumers, with deletion",
			monitorSecretsCall: []monitorSecretsCall{
				{
					consumerID: "group1",
					secrets: []types.NamespacedName{
						{Name: "secret-1", Namespace: "ns-1"},
						{Name: "secret-2", Namespace: "ns-2"},
						{Name: "secret-3", Namespace: "ns-3"},
					},
				},
				{
					consumerID: "group2",
					secrets: []types.NamespacedName{
						{Name: "secret-2", Namespace: "ns-2"},
						{Name: "secret-3", Namespace: "ns-3"},
						{Name: "secret-4", Namespace: "ns-4"},
					},
				},
				{
					consumerID: "group1",
					secrets: []types.NamespacedName{
						{Name: "secret-2", Namespace: "ns-2"},
						{Name: "secret-3", Namespace: "ns-3"},
					},
				},
			},
			wantSecrets: []types.NamespacedName{
				{Name: "secret-2", Namespace: "ns-2"},
				{Name: "secret-3", Namespace: "ns-3"},
				{Name: "secret-4", Namespace: "ns-4"},
			},
		},
		{
			testName: "Multiple ingress consumers, delete all",
			monitorSecretsCall: []monitorSecretsCall{
				{
					consumerID: "group1",
					secrets: []types.NamespacedName{
						{Name: "secret-1", Namespace: "ns-1"},
						{Name: "secret-2", Namespace: "ns-2"},
						{Name: "secret-3", Namespace: "ns-3"},
					},
				},
				{
					consumerID: "group2",
					secrets: []types.NamespacedName{
						{Name: "secret-2", Namespace: "ns-2"},
						{Name: "secret-3", Namespace: "ns-3"},
						{Name: "secret-4", Namespace: "ns-4"},
					},
				},
				{
					consumerID: "group1",
					secrets:    []types.NamespacedName{},
				},
				{
					consumerID: "group2",
					secrets:    []types.NamespacedName{},
				},
			},
			wantSecrets: []types.NamespacedName{},
		},
		{
			testName: "multiple gateways, overlapping secrets",
			monitorSecretsCall: []monitorSecretsCall{
				{
					consumerID: "gw-1",
					secrets: []types.NamespacedName{
						{Name: "oidc-secret-1", Namespace: "auth-ns"},
						{Name: "shared-secret", Namespace: "shared-ns"},
					},
				},
				{
					consumerID: "gw-2",
					secrets: []types.NamespacedName{
						{Name: "shared-secret", Namespace: "shared-ns"},
						{Name: "oidc-secret-2", Namespace: "auth-ns"},
					},
				},
				{
					consumerID: "gw-3",
					secrets: []types.NamespacedName{
						{Name: "prod-secret", Namespace: "production"},
						{Name: "shared-secret", Namespace: "shared-ns"},
					},
				},
			},
			wantSecrets: []types.NamespacedName{
				{Name: "oidc-secret-1", Namespace: "auth-ns"},
				{Name: "shared-secret", Namespace: "shared-ns"},
				{Name: "oidc-secret-2", Namespace: "auth-ns"},
				{Name: "prod-secret", Namespace: "production"},
			},
		},
		{
			testName: "multiple gateways, with deletion",
			monitorSecretsCall: []monitorSecretsCall{
				{
					consumerID: "gw-1",
					secrets: []types.NamespacedName{
						{Name: "oidc-secret-1", Namespace: "auth-ns"},
						{Name: "shared-secret", Namespace: "shared-ns"},
					},
				},
				{
					consumerID: "gw-2",
					secrets: []types.NamespacedName{
						{Name: "shared-secret", Namespace: "shared-ns"},
						{Name: "oidc-secret-2", Namespace: "auth-ns"},
					},
				},
				{
					consumerID: "gw-3",
					secrets: []types.NamespacedName{
						{Name: "prod-secret", Namespace: "production"},
						{Name: "shared-secret", Namespace: "shared-ns"},
					},
				},
				// Delete the first gateway configuration
				{
					consumerID: "gw-1",
					secrets:    []types.NamespacedName{},
				},
			},
			wantSecrets: []types.NamespacedName{
				{Name: "shared-secret", Namespace: "shared-ns"}, // Still used by gw-2 and gw-3
				{Name: "oidc-secret-2", Namespace: "auth-ns"},   // Still used by gw-2
				{Name: "prod-secret", Namespace: "production"},  // Still used by gw-3
			},
		},
		{
			testName: "Cross-controller cleanup - ingress removed, multiple gateways remain",
			monitorSecretsCall: []monitorSecretsCall{
				{
					consumerID: "ig-group1",
					secrets: []types.NamespacedName{
						{Name: "shared-secret", Namespace: "ns-1"},
						{Name: "ingress-only-secret", Namespace: "ns-1"},
					},
				},
				{
					consumerID: "gw-1",
					secrets: []types.NamespacedName{
						{Name: "shared-secret", Namespace: "ns-1"},
						{Name: "gateway-secret-1", Namespace: "ns-2"},
					},
				},
				{
					consumerID: "gw-2",
					secrets: []types.NamespacedName{
						{Name: "shared-secret", Namespace: "ns-1"},
						{Name: "gateway-secret-2", Namespace: "ns-3"},
					},
				},
				// Remove ingress consumer
				{
					consumerID: "ig-group1",
					secrets:    []types.NamespacedName{},
				},
			},
			wantSecrets: []types.NamespacedName{
				{Name: "shared-secret", Namespace: "ns-1"},    // Should still exist for both gateways
				{Name: "gateway-secret-1", Namespace: "ns-2"}, // Should still exist for gw 1
				{Name: "gateway-secret-2", Namespace: "ns-3"}, // Should still exist for gw 2
			},
		},
		{
			testName: "Cross-controller cleanup - one gateway removed, ingress and other gateway remain",
			monitorSecretsCall: []monitorSecretsCall{
				{
					consumerID: "ig-group1",
					secrets: []types.NamespacedName{
						{Name: "shared-secret", Namespace: "ns-1"},
						{Name: "ingress-only-secret", Namespace: "ns-1"},
					},
				},
				{
					consumerID: "gw-1",
					secrets: []types.NamespacedName{
						{Name: "shared-secret", Namespace: "ns-1"},
						{Name: "gateway-secret-1", Namespace: "ns-2"},
					},
				},
				{
					consumerID: "gw-2",
					secrets: []types.NamespacedName{
						{Name: "shared-secret", Namespace: "ns-1"},
						{Name: "gateway-secret-2", Namespace: "ns-3"},
					},
				},
				// Remove one gateway consumer
				{
					consumerID: "gw-1",
					secrets:    []types.NamespacedName{},
				},
			},
			wantSecrets: []types.NamespacedName{
				{Name: "shared-secret", Namespace: "ns-1"},       // Should still exist for ingress and gw 2
				{Name: "ingress-only-secret", Namespace: "ns-1"}, // Should still exist for ingress
				{Name: "gateway-secret-2", Namespace: "ns-3"},    // Should still exist for gw 2
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			secretsEventChan := make(chan event.TypedGenericEvent[*corev1.Secret], 100)
			fakeClient := fake.NewSimpleClientset()
			secretsManager := NewSecretsManager(fakeClient, secretsEventChan, logr.New(&log.NullLogSink{}))

			for _, call := range tt.monitorSecretsCall {
				secretsManager.MonitorSecrets(call.consumerID, call.secrets)
			}
			assert.Equal(t, len(tt.wantSecrets), len(secretsManager.secretMap))
			for _, want := range tt.wantSecrets {
				_, exists := secretsManager.secretMap[want]
				assert.True(t, exists)
			}
		})
	}
}

func Test_defaultSecretsManager_GetSecret(t *testing.T) {
	secretNamespace := "test-namespace"
	secretName := "test-secret"
	secretKey := types.NamespacedName{
		Namespace: secretNamespace,
		Name:      secretName,
	}

	clientID := "test-client-id"
	clientSecret := "test-client-secret"

	testSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: secretNamespace,
		},
		Data: map[string][]byte{
			"clientID":     []byte(clientID),
			"clientSecret": []byte(clientSecret),
		},
	}

	tests := []struct {
		name           string
		setupSecrets   []*corev1.Secret       // Secrets to create in fake client
		monitorSecrets []types.NamespacedName // Secrets to monitor (cached)
		secretKey      types.NamespacedName   // Secret to retrieve
		want           *corev1.Secret         // Expected secret
		wantErr        bool                   // Expect error
		errContains    string                 // Error should contain this text
	}{
		{
			name:           "cache hit - monitored secret exists in cache",
			setupSecrets:   []*corev1.Secret{testSecret},
			monitorSecrets: []types.NamespacedName{secretKey},
			secretKey:      secretKey,
			want:           testSecret,
			wantErr:        false,
		},
		{
			name:           "cache miss - monitored secret but not in store",
			setupSecrets:   []*corev1.Secret{}, // Secret doesn't exist in API either
			monitorSecrets: []types.NamespacedName{secretKey},
			secretKey:      secretKey,
			want:           nil,
			wantErr:        true,
			errContains:    "not found",
		},
		{
			name:           "fallback to API - secret not monitored but exists",
			setupSecrets:   []*corev1.Secret{testSecret},
			monitorSecrets: []types.NamespacedName{}, // Not monitoring this secret
			secretKey:      secretKey,
			want:           testSecret,
			wantErr:        false,
		},
		{
			name:           "API error - secret not monitored and not found",
			setupSecrets:   []*corev1.Secret{},
			monitorSecrets: []types.NamespacedName{},
			secretKey:      secretKey,
			want:           nil,
			wantErr:        true,
			errContains:    "not found",
		},
		{
			name: "cache hit with different secret",
			setupSecrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "different-secret",
						Namespace: secretNamespace,
					},
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				},
			},
			monitorSecrets: []types.NamespacedName{
				{Namespace: secretNamespace, Name: "different-secret"},
			},
			secretKey: types.NamespacedName{
				Namespace: secretNamespace,
				Name:      "different-secret",
			},
			want: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "different-secret",
					Namespace: secretNamespace,
				},
				Data: map[string][]byte{
					"key": []byte("value"),
				},
			},
			wantErr: false,
		},
		{
			name:           "monitored secret with multiple consumers",
			setupSecrets:   []*corev1.Secret{testSecret},
			monitorSecrets: []types.NamespacedName{secretKey},
			secretKey:      secretKey,
			want:           testSecret,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create fake k8s client
			k8sClient := testutils.GenerateTestClient()

			// Add secrets to fake client
			for _, secret := range tt.setupSecrets {
				err := k8sClient.Create(ctx, secret.DeepCopy())
				assert.NoError(t, err)
			}

			// Create SecretsManager
			secretsEventChan := make(chan event.TypedGenericEvent[*corev1.Secret], 100)
			fakeClientset := fake.NewSimpleClientset()
			secretsManager := NewSecretsManager(fakeClientset, secretsEventChan, logr.New(&log.NullLogSink{}))

			// Monitor secrets if specified
			if len(tt.monitorSecrets) > 0 {
				secretsManager.MonitorSecrets("test-consumer", tt.monitorSecrets)

				// Simulate cache population by manually adding to cache
				for _, monitoredSecret := range tt.monitorSecrets {
					if secretItem, exists := secretsManager.secretMap[monitoredSecret]; exists {
						// Find the corresponding secret from setup
						for _, setupSecret := range tt.setupSecrets {
							if setupSecret.Name == monitoredSecret.Name &&
								setupSecret.Namespace == monitoredSecret.Namespace {
								// Manually add to store (simulating reflector behavior)
								err := secretItem.store.Add(setupSecret.DeepCopy())
								assert.NoError(t, err)
							}
						}
					}
				}
			}

			// Call GetSecret
			got, err := secretsManager.GetSecret(ctx, k8sClient, tt.secretKey)

			// Assertions
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, got)

				// Compare secret content
				assert.Equal(t, tt.want.Name, got.Name)
				assert.Equal(t, tt.want.Namespace, got.Namespace)
				assert.Equal(t, tt.want.Data, got.Data)

				// Ensure it's a deep copy (different memory addresses)
				if len(tt.monitorSecrets) > 0 {
					// Only check for cache hits
					for _, monitoredSecret := range tt.monitorSecrets {
						if monitoredSecret == tt.secretKey {
							assert.NotSame(t, tt.want, got, "GetSecret should return a deep copy")
						}
					}
				}
			}
		})
	}
}

func Test_defaultSecretsManager_GetSecret_CacheInvalidation(t *testing.T) {
	secretNamespace := "test-namespace"
	secretName := "test-secret"
	secretKey := types.NamespacedName{
		Namespace: secretNamespace,
		Name:      secretName,
	}

	originalSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: secretNamespace,
		},
		Data: map[string][]byte{
			"clientID": []byte("original-client-id"),
		},
	}

	updatedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: secretNamespace,
		},
		Data: map[string][]byte{
			"clientID": []byte("updated-client-id"),
		},
	}

	tests := []struct {
		name         string
		setupPhase   func(t *testing.T, sm *defaultSecretsManager, k8sClient client.Client)
		invalidation func(t *testing.T, sm *defaultSecretsManager, k8sClient client.Client)
		verification func(t *testing.T, sm *defaultSecretsManager, k8sClient client.Client)
	}{
		{
			name: "cache invalidation on secret update",
			setupPhase: func(t *testing.T, sm *defaultSecretsManager, k8sClient client.Client) {
				// Create original secret
				err := k8sClient.Create(context.Background(), originalSecret.DeepCopy())
				assert.NoError(t, err)

				// Monitor and populate cache
				sm.MonitorSecrets("test-consumer", []types.NamespacedName{secretKey})
				if secretItem, exists := sm.secretMap[secretKey]; exists {
					err := secretItem.store.Add(originalSecret.DeepCopy())
					assert.NoError(t, err)
				}

				// Verify original value is cached
				secret, err := sm.GetSecret(context.Background(), k8sClient, secretKey)
				assert.NoError(t, err)
				assert.Equal(t, "original-client-id", string(secret.Data["clientID"]))
			},
			invalidation: func(t *testing.T, sm *defaultSecretsManager, k8sClient client.Client) {
				// Update secret in API
				err := k8sClient.Update(context.Background(), updatedSecret.DeepCopy())
				assert.NoError(t, err)

				// Simulate reflector cache update (normally done by watch events)
				if secretItem, exists := sm.secretMap[secretKey]; exists {
					err := secretItem.store.Update(updatedSecret.DeepCopy())
					assert.NoError(t, err)
				}
			},
			verification: func(t *testing.T, sm *defaultSecretsManager, k8sClient client.Client) {
				// Verify updated value is returned from cache
				secret, err := sm.GetSecret(context.Background(), k8sClient, secretKey)
				assert.NoError(t, err)
				assert.Equal(t, "updated-client-id", string(secret.Data["clientID"]))
			},
		},
		{
			name: "cache invalidation on secret deletion",
			setupPhase: func(t *testing.T, sm *defaultSecretsManager, k8sClient client.Client) {
				// Create and cache secret
				err := k8sClient.Create(context.Background(), originalSecret.DeepCopy())
				assert.NoError(t, err)

				sm.MonitorSecrets("test-consumer", []types.NamespacedName{secretKey})
				if secretItem, exists := sm.secretMap[secretKey]; exists {
					err := secretItem.store.Add(originalSecret.DeepCopy())
					assert.NoError(t, err)
				}

				// Verify it's cached
				secret, err := sm.GetSecret(context.Background(), k8sClient, secretKey)
				assert.NoError(t, err)
				assert.NotNil(t, secret)
			},
			invalidation: func(t *testing.T, sm *defaultSecretsManager, k8sClient client.Client) {
				// Delete secret from API
				err := k8sClient.Delete(context.Background(), originalSecret.DeepCopy())
				assert.NoError(t, err)

				//Simulate reflector cache deletion (normally done by watch events)
				if secretItem, exists := sm.secretMap[secretKey]; exists {
					err := secretItem.store.Delete(originalSecret.DeepCopy())
					assert.NoError(t, err)
				}
			},
			verification: func(t *testing.T, sm *defaultSecretsManager, k8sClient client.Client) {
				// Verify GetSecret falls back to API and returns NotFound error
				secret, err := sm.GetSecret(context.Background(), k8sClient, secretKey)
				assert.Error(t, err)
				assert.True(t, apierrors.IsNotFound(err))
				assert.Nil(t, secret)
			},
		},
		{
			name: "stop monitoring removes from cache",
			setupPhase: func(t *testing.T, sm *defaultSecretsManager, k8sClient client.Client) {
				// Create and cache secret
				err := k8sClient.Create(context.Background(), originalSecret.DeepCopy())
				assert.NoError(t, err)

				sm.MonitorSecrets("test-consumer", []types.NamespacedName{secretKey})
				if secretItem, exists := sm.secretMap[secretKey]; exists {
					err := secretItem.store.Add(originalSecret.DeepCopy())
					assert.NoError(t, err)
				}

				// Verify it's monitored and cached
				assert.Contains(t, sm.secretMap, secretKey)
				secret, err := sm.GetSecret(context.Background(), k8sClient, secretKey)
				assert.NoError(t, err)
				assert.Equal(t, "original-client-id", string(secret.Data["clientID"]))
			},
			invalidation: func(t *testing.T, sm *defaultSecretsManager, k8sClient client.Client) {
				// Stop monitoring (garbage collection)
				sm.MonitorSecrets("test-consumer", []types.NamespacedName{})
			},
			verification: func(t *testing.T, sm *defaultSecretsManager, k8sClient client.Client) {
				// Verify secret is no longer monitored
				assert.NotContains(t, sm.secretMap, secretKey)

				// GetSecret should now fallback to API
				secret, err := sm.GetSecret(context.Background(), k8sClient, secretKey)
				assert.NoError(t, err)
				assert.Equal(t, "original-client-id", string(secret.Data["clientID"]))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh SecretsManager and client for each test
			k8sClient := testutils.GenerateTestClient()
			secretsEventChan := make(chan event.TypedGenericEvent[*corev1.Secret], 100)
			fakeClientset := fake.NewSimpleClientset()
			secretsManager := NewSecretsManager(fakeClientset, secretsEventChan, logr.New(&log.NullLogSink{}))

			// Run test phases
			tt.setupPhase(t, secretsManager, k8sClient)
			tt.invalidation(t, secretsManager, k8sClient)
			tt.verification(t, secretsManager, k8sClient)
		})
	}
}
