package shared_utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

func TestMakeLoadBalancerAttributeSliceFromMap(t *testing.T) {
	tests := []struct {
		name         string
		attributeMap map[string]string
		expected     []elbv2model.LoadBalancerAttribute
	}{
		{
			name:         "empty map",
			attributeMap: map[string]string{},
			expected:     []elbv2model.LoadBalancerAttribute{},
		},
		{
			name:         "single attribute",
			attributeMap: map[string]string{"key1": "value1"},
			expected:     []elbv2model.LoadBalancerAttribute{{Key: "key1", Value: "value1"}},
		},
		{
			name:         "multiple attributes",
			attributeMap: map[string]string{"key2": "value2", "key1": "value1", "key3": "value3"},
			expected: []elbv2model.LoadBalancerAttribute{
				{Key: "key1", Value: "value1"},
				{Key: "key2", Value: "value2"},
				{Key: "key3", Value: "value3"},
			},
		},
		{
			name:         "attributes with special characters",
			attributeMap: map[string]string{"key-with-dash": "value with spaces", "key_with_underscore": "value=with=equals"},
			expected: []elbv2model.LoadBalancerAttribute{
				{Key: "key-with-dash", Value: "value with spaces"},
				{Key: "key_with_underscore", Value: "value=with=equals"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MakeLoadBalancerAttributeSliceFromMap(tt.attributeMap)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMakeLoadBalancerAttributeSliceFromMap_Sorting(t *testing.T) {
	// Test that attributes are properly sorted by key
	attributeMap := map[string]string{
		"zebra":  "last",
		"apple":  "first",
		"banana": "middle",
	}

	result := MakeLoadBalancerAttributeSliceFromMap(attributeMap)

	expected := []elbv2model.LoadBalancerAttribute{
		{Key: "apple", Value: "first"},
		{Key: "banana", Value: "middle"},
		{Key: "zebra", Value: "last"},
	}

	assert.Equal(t, expected, result)
}
