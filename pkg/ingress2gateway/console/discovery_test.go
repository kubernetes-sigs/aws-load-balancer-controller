package console

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNamespacedName(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantNS  string
		wantN   string
		wantErr string
	}{
		{
			name:   "valid",
			ref:    "my-ns/my-ingress",
			wantNS: "my-ns",
			wantN:  "my-ingress",
		},
		{
			name:    "missing slash",
			ref:     "no-slash",
			wantErr: "missing /",
		},
		{
			name:    "empty namespace",
			ref:     "/name",
			wantErr: "invalid namespaced name",
		},
		{
			name:    "empty name",
			ref:     "ns/",
			wantErr: "invalid namespaced name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, name, err := parseNamespacedName(tt.ref)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantNS, ns)
			assert.Equal(t, tt.wantN, name)
		})
	}
}

func TestInferIngressSourceFromPlan(t *testing.T) {
	tests := []struct {
		name    string
		plan    string
		wantRef string
	}{
		{
			name:    "standalone ingress tag",
			plan:    `{"id":"ns/gw","resources":{"AWS::ElasticLoadBalancingV2::LoadBalancer":{"LoadBalancer":{"spec":{"tags":{"gateway.k8s.aws/migrated-from":"ingress/my-ns/my-ingress"}}}}}}`,
			wantRef: "my-ns/my-ingress",
		},
		{
			name:    "group ingress tag",
			plan:    `{"id":"ns/gw","resources":{"AWS::ElasticLoadBalancingV2::LoadBalancer":{"LoadBalancer":{"spec":{"tags":{"gateway.k8s.aws/migrated-from":"ingress-group/my-group"}}}}}}`,
			wantRef: "",
		},
		{
			name:    "no LB resource",
			plan:    `{"id":"ns/gw","resources":{}}`,
			wantRef: "",
		},
		{
			name:    "invalid JSON",
			plan:    "not json",
			wantRef: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferIngressSourceFromPlan(tt.plan)
			assert.Equal(t, tt.wantRef, got)
		})
	}
}
