package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_defaultSecretsManager_MonitorSecrets(t *testing.T) {
	type monitorSecretsCall struct {
		groupID string
		secrets []types.NamespacedName
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
			testName: "Single group",
			monitorSecretsCall: []monitorSecretsCall{
				{
					groupID: "group1",
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
			testName: "Single group, multiple secrets",
			monitorSecretsCall: []monitorSecretsCall{
				{
					groupID: "group1",
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
			testName: "Multiple group, overlapping secrets",
			monitorSecretsCall: []monitorSecretsCall{
				{
					groupID: "group1",
					secrets: []types.NamespacedName{
						{Name: "secret-1", Namespace: "ns-1"},
						{Name: "secret-2", Namespace: "ns-2"},
						{Name: "secret-3", Namespace: "ns-3"},
					},
				},
				{
					groupID: "group2",
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
			testName: "Multiple group, with deletion",
			monitorSecretsCall: []monitorSecretsCall{
				{
					groupID: "group1",
					secrets: []types.NamespacedName{
						{Name: "secret-1", Namespace: "ns-1"},
						{Name: "secret-2", Namespace: "ns-2"},
						{Name: "secret-3", Namespace: "ns-3"},
					},
				},
				{
					groupID: "group2",
					secrets: []types.NamespacedName{
						{Name: "secret-2", Namespace: "ns-2"},
						{Name: "secret-3", Namespace: "ns-3"},
						{Name: "secret-4", Namespace: "ns-4"},
					},
				},
				{
					groupID: "group1",
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
			testName: "Multiple group, delete all",
			monitorSecretsCall: []monitorSecretsCall{
				{
					groupID: "group1",
					secrets: []types.NamespacedName{
						{Name: "secret-1", Namespace: "ns-1"},
						{Name: "secret-2", Namespace: "ns-2"},
						{Name: "secret-3", Namespace: "ns-3"},
					},
				},
				{
					groupID: "group2",
					secrets: []types.NamespacedName{
						{Name: "secret-2", Namespace: "ns-2"},
						{Name: "secret-3", Namespace: "ns-3"},
						{Name: "secret-4", Namespace: "ns-4"},
					},
				},
				{
					groupID: "group1",
					secrets: []types.NamespacedName{},
				},
				{
					groupID: "group2",
					secrets: []types.NamespacedName{},
				},
			},
			wantSecrets: []types.NamespacedName{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			secretsEventChan := make(chan event.GenericEvent, 100)
			fakeClient := fake.NewSimpleClientset()
			secretsManager := NewSecretsManager(fakeClient, secretsEventChan, &log.NullLogger{})

			for _, call := range tt.monitorSecretsCall {
				secretsManager.MonitorSecrets(call.groupID, call.secrets)
			}
			assert.Equal(t, len(tt.wantSecrets), len(secretsManager.secretMap))
			for _, want := range tt.wantSecrets {
				_, exists := secretsManager.secretMap[want]
				assert.True(t, exists)
			}
		})
	}
}
