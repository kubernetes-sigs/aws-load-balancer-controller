package console

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiff(t *testing.T) {
	tests := []struct {
		name        string
		ingress     ResourceTree
		gateway     ResourceTree
		wantSummary DiffSummary
	}{
		{
			name: "identical trees",
			ingress: ResourceTree{
				"AWS::ElasticLoadBalancingV2::LoadBalancer": {"LoadBalancer": {"spec.name": "k8s-test", "spec.scheme": "internet-facing"}},
			},
			gateway: ResourceTree{
				"AWS::ElasticLoadBalancingV2::LoadBalancer": {"LoadBalancer": {"spec.name": "k8s-test", "spec.scheme": "internet-facing"}},
			},
			wantSummary: DiffSummary{Same: 2},
		},
		{
			name: "changed field",
			ingress: ResourceTree{
				"AWS::ElasticLoadBalancingV2::LoadBalancer": {"LoadBalancer": {"spec.name": "k8s-ingress-abc", "spec.scheme": "internet-facing"}},
			},
			gateway: ResourceTree{
				"AWS::ElasticLoadBalancingV2::LoadBalancer": {"LoadBalancer": {"spec.name": "k8s-gateway-xyz", "spec.scheme": "internet-facing"}},
			},
			wantSummary: DiffSummary{Same: 1, Changed: 1},
		},
		{
			name: "removed resource",
			ingress: ResourceTree{
				"AWS::ElasticLoadBalancingV2::TargetGroup": {
					"tg-api": {"spec.targetType": "ip"},
					"tg-web": {"spec.targetType": "ip"},
				},
			},
			gateway: ResourceTree{
				"AWS::ElasticLoadBalancingV2::TargetGroup": {"tg-api": {"spec.targetType": "ip"}},
			},
			wantSummary: DiffSummary{Same: 1, Removed: 1},
		},
		{
			name: "added resource",
			ingress: ResourceTree{
				"AWS::ElasticLoadBalancingV2::Listener": {"80": {"spec.protocol": "HTTP"}},
			},
			gateway: ResourceTree{
				"AWS::ElasticLoadBalancingV2::Listener": {
					"80":  {"spec.protocol": "HTTP"},
					"443": {"spec.protocol": "HTTPS"},
				},
			},
			wantSummary: DiffSummary{Same: 1, Added: 1},
		},
		{
			name:        "empty trees",
			ingress:     ResourceTree{},
			gateway:     ResourceTree{},
			wantSummary: DiffSummary{},
		},
		{
			name: "entirely different types",
			ingress: ResourceTree{
				"AWS::ElasticLoadBalancingV2::LoadBalancer": {"LB": {"spec.name": "old"}},
			},
			gateway: ResourceTree{
				"AWS::ElasticLoadBalancingV2::Listener": {"80": {"spec.protocol": "HTTP"}},
			},
			wantSummary: DiffSummary{Removed: 1, Added: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Diff(tt.ingress, tt.gateway)
			assert.Equal(t, tt.wantSummary, result.Summary)
		})
	}
}

func TestDiff_ChangedFieldValues(t *testing.T) {
	ingress := ResourceTree{
		"AWS::ElasticLoadBalancingV2::LoadBalancer": {"LoadBalancer": {"spec.name": "k8s-ingress-abc"}},
	}
	gateway := ResourceTree{
		"AWS::ElasticLoadBalancingV2::LoadBalancer": {"LoadBalancer": {"spec.name": "k8s-gateway-xyz"}},
	}
	result := Diff(ingress, gateway)
	for _, e := range result.Entries {
		if e.Field == "spec.name" {
			assert.Equal(t, StatusChanged, e.Status)
			assert.Equal(t, "k8s-ingress-abc", e.Ingress)
			assert.Equal(t, "k8s-gateway-xyz", e.Gateway)
			return
		}
	}
	t.Fatal("expected spec.name entry not found")
}
