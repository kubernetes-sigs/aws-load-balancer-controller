package model

import (
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

// Test IsIPv6Supported
func Test_IsIPv6Supported(t *testing.T) {
	tests := []struct {
		name          string
		ipAddressType elbv2model.IPAddressType
		expected      bool
	}{
		{
			name:          "DualStack should support IPv6",
			ipAddressType: elbv2model.IPAddressTypeDualStack,
			expected:      true,
		},
		{
			name:          "DualStackWithoutPublicIPV4 should support IPv6",
			ipAddressType: elbv2model.IPAddressTypeDualStackWithoutPublicIPV4,
			expected:      true,
		},
		{
			name:          "IPv4 should not support IPv6",
			ipAddressType: elbv2model.IPAddressTypeIPV4,
			expected:      false,
		},
		{
			name:          "Empty address type should not support IPv6",
			ipAddressType: "",
			expected:      false,
		},
		{
			name:          "Unknown address type should not support IPv6",
			ipAddressType: "unknown",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isIPv6Supported(tt.ipAddressType)
			assert.Equal(t, tt.expected, got, "isIPv6Supported() = %v, want %v", got, tt.expected)
		})
	}
}

// Test SortRoutesByHostnamePrecedence
func Test_SortRoutesByHostnamePrecedence(t *testing.T) {
	tests := []struct {
		name     string
		input    []routeutils.RouteDescriptor
		expected []routeutils.RouteDescriptor
	}{
		{
			name:     "empty routes",
			input:    []routeutils.RouteDescriptor{},
			expected: []routeutils.RouteDescriptor{},
		},
		{
			name: "routes with no hostnames",
			input: []routeutils.RouteDescriptor{
				&routeutils.MockRoute{Hostnames: []string{}},
				&routeutils.MockRoute{Hostnames: []string{}},
			},
			expected: []routeutils.RouteDescriptor{
				&routeutils.MockRoute{Hostnames: []string{}},
				&routeutils.MockRoute{Hostnames: []string{}},
			},
		},
		{
			name: "mix of empty and non-empty hostnames",
			input: []routeutils.RouteDescriptor{
				&routeutils.MockRoute{Hostnames: []string{}},
				&routeutils.MockRoute{Hostnames: []string{"example.com"}},
				&routeutils.MockRoute{Hostnames: []string{"test.com"}},
			},
			expected: []routeutils.RouteDescriptor{
				&routeutils.MockRoute{Hostnames: []string{"example.com"}},
				&routeutils.MockRoute{Hostnames: []string{"test.com"}},
				&routeutils.MockRoute{Hostnames: []string{}},
			},
		},
		{
			name: "with and without wildcard hostnames",
			input: []routeutils.RouteDescriptor{
				&routeutils.MockRoute{Hostnames: []string{"*.example.com"}},
				&routeutils.MockRoute{Hostnames: []string{"test.example.com"}},
			},
			expected: []routeutils.RouteDescriptor{
				&routeutils.MockRoute{Hostnames: []string{"test.example.com"}},
				&routeutils.MockRoute{Hostnames: []string{"*.example.com"}},
			},
		},
		{
			name: "complex mixed hostnames",
			input: []routeutils.RouteDescriptor{
				&routeutils.MockRoute{Hostnames: []string{"*.example.com"}},
				&routeutils.MockRoute{Hostnames: []string{}},
				&routeutils.MockRoute{Hostnames: []string{"test.example.com"}},
				&routeutils.MockRoute{Hostnames: []string{"another.example.com"}},
			},
			expected: []routeutils.RouteDescriptor{
				&routeutils.MockRoute{Hostnames: []string{"another.example.com"}},
				&routeutils.MockRoute{Hostnames: []string{"test.example.com"}},
				&routeutils.MockRoute{Hostnames: []string{"*.example.com"}},
				&routeutils.MockRoute{Hostnames: []string{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy of input to avoid modifying the test case data
			actual := make([]routeutils.RouteDescriptor, len(tt.input))
			copy(actual, tt.input)

			// Execute the sort
			sortRoutesByHostnamePrecedence(actual)

			// Verify the result
			assert.Equal(t, tt.expected, actual, "sorted routes should match expected order")

			// Verify stability of sort
			sortRoutesByHostnamePrecedence(actual)
			assert.Equal(t, tt.expected, actual, "second sort should maintain the same order (stable sort)")
		})
	}
}
