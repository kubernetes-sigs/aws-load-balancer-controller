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
func TestDiff_SliceOrderInvariance(t *testing.T) {
	tests := []struct {
		name    string
		ingress any
		gateway any
		want    DiffStatus
	}{
		{
			name:    "scalar slice — identical order",
			ingress: []any{"a", "b", "c"},
			gateway: []any{"a", "b", "c"},
			want:    StatusSame,
		},
		{
			name:    "scalar slice — reversed order",
			ingress: []any{"a", "b", "c"},
			gateway: []any{"c", "b", "a"},
			want:    StatusSame,
		},
		{
			name:    "scalar slice — different elements",
			ingress: []any{"a", "b", "c"},
			gateway: []any{"a", "b", "d"},
			want:    StatusChanged,
		},
		{
			name: "slice of maps — reversed order",
			ingress: []any{
				map[string]any{"fromPort": float64(81), "ipProtocol": "tcp", "toPort": float64(81),
					"ipRanges": []any{map[string]any{"cidrIP": "0.0.0.0/0"}}},
				map[string]any{"fromPort": float64(80), "ipProtocol": "tcp", "toPort": float64(80),
					"ipRanges": []any{map[string]any{"cidrIP": "0.0.0.0/0"}}},
			},
			gateway: []any{
				map[string]any{"fromPort": float64(80), "ipProtocol": "tcp", "toPort": float64(80),
					"ipRanges": []any{map[string]any{"cidrIP": "0.0.0.0/0"}}},
				map[string]any{"fromPort": float64(81), "ipProtocol": "tcp", "toPort": float64(81),
					"ipRanges": []any{map[string]any{"cidrIP": "0.0.0.0/0"}}},
			},
			want: StatusSame,
		},
		{
			name: "slice of maps — different element",
			ingress: []any{
				map[string]any{"fromPort": float64(80), "toPort": float64(80)},
				map[string]any{"fromPort": float64(81), "toPort": float64(81)},
			},
			gateway: []any{
				map[string]any{"fromPort": float64(80), "toPort": float64(80)},
				map[string]any{"fromPort": float64(82), "toPort": float64(82)},
			},
			want: StatusChanged,
		},
		{
			name:    "scalar unchanged",
			ingress: "hello",
			gateway: "hello",
			want:    StatusSame,
		},
		{
			name:    "scalar changed",
			ingress: "hello",
			gateway: "world",
			want:    StatusChanged,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingress := ResourceTree{
				"AWS::EC2::SecurityGroup": {"SG": {"spec.ingress": tt.ingress}},
			}
			gateway := ResourceTree{
				"AWS::EC2::SecurityGroup": {"SG": {"spec.ingress": tt.gateway}},
			}
			result := Diff(ingress, gateway)
			var got DiffStatus
			for _, e := range result.Entries {
				if e.Field == "spec.ingress" {
					got = e.Status
					break
				}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGroupByCorrelation(t *testing.T) {
	const (
		tgType  = "AWS::ElasticLoadBalancingV2::TargetGroup"
		tgbType = "K8S::ElasticLoadBalancingV2::TargetGroupBinding"
		lbType  = "AWS::ElasticLoadBalancingV2::LoadBalancer"

		tgRawID = "console-test/echo-echoserver:80"
		lbRawID = "LoadBalancer"
	)

	// validTGB is a fully formed TargetGroupBinding whose serviceRef lets the
	// correlator derive a stable join key.
	validTGB := map[string]any{
		"spec.template.spec.serviceRef.name": "echoserver",
		"spec.template.spec.serviceRef.port": float64(80),
	}

	tests := []struct {
		name    string
		resType string
		tree    ResourceTree
		want    map[string]correlated
	}{
		{
			name:    "non-correlated type uses raw ID as key",
			resType: lbType,
			tree: ResourceTree{
				lbType: {
					lbRawID: {"spec.name": "k8s-test"},
				},
			},
			want: map[string]correlated{
				lbRawID: {rawID: lbRawID, fields: map[string]any{"spec.name": "k8s-test"}},
			},
		},
		{
			name:    "TargetGroup correlates via serviceRef",
			resType: tgType,
			tree: ResourceTree{
				tgType: {
					tgRawID: {"spec.targetType": "ip"},
				},
				// Same raw key surfaces the TGB carrying the serviceRef.
				tgbType: {
					tgRawID: validTGB,
				},
			},
			want: map[string]correlated{
				"echoserver:80": {
					rawID:  tgRawID,
					fields: map[string]any{"spec.targetType": "ip"},
				},
			},
		},
		{
			name:    "TargetGroupBinding correlates via its own serviceRef",
			resType: tgbType,
			tree: ResourceTree{
				tgbType: {
					tgRawID: validTGB,
				},
			},
			want: map[string]correlated{
				"echoserver:80": {rawID: tgRawID, fields: validTGB},
			},
		},
		{
			name:    "TargetGroup without a paired TGB falls back to raw ID",
			resType: tgType,
			tree: ResourceTree{
				tgType: {
					tgRawID: {"spec.targetType": "ip"},
				},
				// No tgbType entry at all — correlator can't resolve a serviceRef.
			},
			want: map[string]correlated{
				tgRawID: {rawID: tgRawID, fields: map[string]any{"spec.targetType": "ip"}},
			},
		},
		{
			name:    "TGB with missing serviceRef.name falls back to raw ID",
			resType: tgType,
			tree: ResourceTree{
				tgType: {
					tgRawID: {"spec.targetType": "ip"},
				},
				tgbType: {
					tgRawID: map[string]any{
						"spec.template.spec.serviceRef.port": float64(80),
						// no serviceRef.name → correlation ID falls back to rawID
					},
				},
			},
			want: map[string]correlated{
				tgRawID: {rawID: tgRawID, fields: map[string]any{"spec.targetType": "ip"}},
			},
		},
		{
			name:    "resource type absent from tree yields empty map",
			resType: tgType,
			tree:    ResourceTree{lbType: {lbRawID: {"spec.name": "k8s-test"}}},
			want:    map[string]correlated{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupByCorrelation(tt.resType, tt.tree)
			assert.Equal(t, tt.want, got)
		})
	}
}
