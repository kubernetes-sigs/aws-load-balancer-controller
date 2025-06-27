package model

import (
	"github.com/stretchr/testify/assert"
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
