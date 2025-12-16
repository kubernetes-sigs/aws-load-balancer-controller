package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCompareOptionsForMTLS(t *testing.T) {
	testCases := []struct {
		name     string
		expected bool
		attrs1   *elbv2types.MutualAuthenticationAttributes
		attrs2   *elbv2types.MutualAuthenticationAttributes
	}{
		{
			name:     "both nil should be equal",
			expected: true,
			attrs1:   nil,
			attrs2:   nil,
		},
		{
			name:     "attrs1 nil, attrs2 empty should not be equal",
			expected: false,
			attrs1:   nil,
			attrs2:   &elbv2types.MutualAuthenticationAttributes{},
		},
		{
			name:     "attrs1 empty, attrs2 empty should be equal",
			expected: true,
			attrs1:   &elbv2types.MutualAuthenticationAttributes{},
			attrs2:   &elbv2types.MutualAuthenticationAttributes{},
		},
		{
			name:     "same mode should be equal",
			expected: true,
			attrs1: &elbv2types.MutualAuthenticationAttributes{
				Mode: awssdk.String("verify"),
			},
			attrs2: &elbv2types.MutualAuthenticationAttributes{
				Mode: awssdk.String("verify"),
			},
		},
		{
			name:     "different mode should not be equal",
			expected: false,
			attrs1: &elbv2types.MutualAuthenticationAttributes{
				Mode: awssdk.String("verify"),
			},
			attrs2: &elbv2types.MutualAuthenticationAttributes{
				Mode: awssdk.String("passthrough"),
			},
		},
		{
			name:     "same trust store arn should be equal",
			expected: true,
			attrs1: &elbv2types.MutualAuthenticationAttributes{
				Mode:          awssdk.String("verify"),
				TrustStoreArn: awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:truststore/my-trust-store/1234567890123456"),
			},
			attrs2: &elbv2types.MutualAuthenticationAttributes{
				Mode:          awssdk.String("verify"),
				TrustStoreArn: awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:truststore/my-trust-store/1234567890123456"),
			},
		},
		{
			name:     "different trust store arn should not be equal",
			expected: false,
			attrs1: &elbv2types.MutualAuthenticationAttributes{
				Mode:          awssdk.String("verify"),
				TrustStoreArn: awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:truststore/my-trust-store-1/1234567890123456"),
			},
			attrs2: &elbv2types.MutualAuthenticationAttributes{
				Mode:          awssdk.String("verify"),
				TrustStoreArn: awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:truststore/my-trust-store-2/1234567890123456"),
			},
		},
		{
			name:     "trust store association status should be ignored",
			expected: true,
			attrs1: &elbv2types.MutualAuthenticationAttributes{
				Mode:                        awssdk.String("verify"),
				TrustStoreArn:               awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:truststore/my-trust-store/1234567890123456"),
				TrustStoreAssociationStatus: elbv2types.TrustStoreAssociationStatusEnumActive,
			},
			attrs2: &elbv2types.MutualAuthenticationAttributes{
				Mode:          awssdk.String("verify"),
				TrustStoreArn: awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:truststore/my-trust-store/1234567890123456"),
			},
		},
		{
			name:     "ignore client certificate expiry should be compared",
			expected: true,
			attrs1: &elbv2types.MutualAuthenticationAttributes{
				Mode:                          awssdk.String("verify"),
				IgnoreClientCertificateExpiry: awssdk.Bool(true),
			},
			attrs2: &elbv2types.MutualAuthenticationAttributes{
				Mode:                          awssdk.String("verify"),
				IgnoreClientCertificateExpiry: awssdk.Bool(true),
			},
		},
		{
			name:     "different ignore client certificate expiry should not be equal",
			expected: false,
			attrs1: &elbv2types.MutualAuthenticationAttributes{
				Mode:                          awssdk.String("verify"),
				IgnoreClientCertificateExpiry: awssdk.Bool(true),
			},
			attrs2: &elbv2types.MutualAuthenticationAttributes{
				Mode:                          awssdk.String("verify"),
				IgnoreClientCertificateExpiry: awssdk.Bool(false),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := cmp.Equal(tc.attrs1, tc.attrs2, CompareOptionsForMTLS())
			assert.Equal(t, tc.expected, res)
		})
	}
}
