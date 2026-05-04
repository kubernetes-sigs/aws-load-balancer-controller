package console

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStack(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantErr    string
		wantFields map[string]map[string]map[string]any // resType → resID → field → value (spot checks)
	}{
		{
			name: "valid stack with LB and Listener",
			raw: `{
				"id": "ns/name",
				"resources": {
					"AWS::ElasticLoadBalancingV2::LoadBalancer": {
						"LoadBalancer": {"spec": {"name": "k8s-test-abc123", "scheme": "internet-facing", "type": "application"}}
					},
					"AWS::ElasticLoadBalancingV2::Listener": {
						"80": {"spec": {"port": 80, "protocol": "HTTP"}}
					}
				}
			}`,
			wantFields: map[string]map[string]map[string]any{
				"AWS::ElasticLoadBalancingV2::LoadBalancer": {"LoadBalancer": {"spec.name": "k8s-test-abc123", "spec.scheme": "internet-facing", "spec.type": "application"}},
				"AWS::ElasticLoadBalancingV2::Listener":     {"80": {"spec.port": float64(80), "spec.protocol": "HTTP"}},
			},
		},
		{
			name:    "empty string",
			raw:     "",
			wantErr: "empty stack JSON",
		},
		{
			name:    "invalid JSON",
			raw:     "{invalid",
			wantErr: "invalid stack JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := ParseStack(tt.raw)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			for resType, resources := range tt.wantFields {
				for resID, fields := range resources {
					for field, wantVal := range fields {
						assert.Equal(t, wantVal, tree[resType][resID][field], "field %s/%s/%s", resType, resID, field)
					}
				}
			}
		})
	}
}

func TestFlattenMap(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]any
		expect map[string]any
	}{
		{
			name:   "flat map",
			input:  map[string]any{"a": "b"},
			expect: map[string]any{"a": "b"},
		},
		{
			name:   "nested map",
			input:  map[string]any{"spec": map[string]any{"name": "test", "nested": map[string]any{"deep": "value"}}},
			expect: map[string]any{"spec.name": "test", "spec.nested.deep": "value"},
		},
		{
			name:   "empty map",
			input:  map[string]any{},
			expect: map[string]any{},
		},
		{
			name:   "array value preserved as-is",
			input:  map[string]any{"spec": map[string]any{"subnets": []any{"subnet-a", "subnet-b"}}},
			expect: map[string]any{"spec.subnets": []any{"subnet-a", "subnet-b"}},
		},
		{
			name:   "numeric value",
			input:  map[string]any{"spec": map[string]any{"port": float64(80)}},
			expect: map[string]any{"spec.port": float64(80)},
		},
		{
			name:   "boolean value",
			input:  map[string]any{"spec": map[string]any{"enabled": true}},
			expect: map[string]any{"spec.enabled": true},
		},
		{
			name:   "nil value",
			input:  map[string]any{"spec": map[string]any{"optional": nil}},
			expect: map[string]any{"spec.optional": nil},
		},
		{
			name: "mixed nested and leaf values",
			input: map[string]any{
				"spec": map[string]any{
					"name":           "lb",
					"tags":           map[string]any{"Env": "prod", "Team": "platform"},
					"securityGroups": []any{"sg-123", "sg-456"},
				},
			},
			expect: map[string]any{
				"spec.name":           "lb",
				"spec.tags.Env":       "prod",
				"spec.tags.Team":      "platform",
				"spec.securityGroups": []any{"sg-123", "sg-456"},
			},
		},
		{
			name: "deeply nested (4 levels)",
			input: map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"c": map[string]any{
							"d": "leaf",
						},
					},
				},
			},
			expect: map[string]any{"a.b.c.d": "leaf"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := make(map[string]any)
			flattenMap("", tt.input, out)
			assert.Equal(t, tt.expect, out)
		})
	}
}
