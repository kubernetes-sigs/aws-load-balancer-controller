package gateway

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"testing"
)

func Test_Merge(t *testing.T) {
	internalScheme := elbv2gw.LoadBalancerSchemeInternal
	ipv4AddrType := elbv2gw.LoadBalancerIpAddressTypeIPv4
	testCases := []struct {
		name            string
		gwClassLbConfig elbv2gw.LoadBalancerConfiguration
		gwLbConfig      elbv2gw.LoadBalancerConfiguration
		expected        elbv2gw.LoadBalancerConfiguration
	}{
		{
			name:            "both blank",
			gwClassLbConfig: elbv2gw.LoadBalancerConfiguration{},
			gwLbConfig:      elbv2gw.LoadBalancerConfiguration{},
			expected:        elbv2gw.LoadBalancerConfiguration{},
		},
		{
			name: "full config in gw class. empty gw config",
			gwClassLbConfig: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerName: awssdk.String("lb name"),
					Scheme:           &internalScheme,
					IpAddressType:    &ipv4AddrType,
					EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic: awssdk.String("on"),
					CustomerOwnedIpv4Pool:                                awssdk.String("coipv4"),
					IPv4IPAMPoolId:                                       awssdk.String("ipam pool"),
					LoadBalancerSubnets: &[]elbv2gw.SubnetConfiguration{
						{
							Identifier: "subnet-1a",
						},
					},
					LoadBalancerSubnetsSelector: &map[string][]string{
						"v1": {"k1", "k2", "k3"},
					},
					SecurityGroups:        &[]string{"sg1", "sg2", "s3"},
					SecurityGroupPrefixes: &[]string{"pl1"},
					SourceRanges:          &[]string{"127.0.0.0/20"},
					VpcId:                 awssdk.String("vpc-1234"),
					ListenerConfigurations: &[]elbv2gw.ListenerConfiguration{
						{
							ProtocolPort:       "pp1",
							DefaultCertificate: awssdk.String("default-cert"),
						},
					},
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   "lb-k-1",
							Value: "lb-v-1",
						},
					},
					Tags: &map[string]string{
						"tag1": "key1",
						"tag2": "key2",
					},
					EnableICMP:                      awssdk.Bool(false),
					ManageBackendSecurityGroupRules: awssdk.Bool(false),
				},
			},
			gwLbConfig: elbv2gw.LoadBalancerConfiguration{},
			expected: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerName: awssdk.String("lb name"),
					Scheme:           &internalScheme,
					IpAddressType:    &ipv4AddrType,
					EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic: awssdk.String("on"),
					CustomerOwnedIpv4Pool:                                awssdk.String("coipv4"),
					IPv4IPAMPoolId:                                       awssdk.String("ipam pool"),
					LoadBalancerSubnets: &[]elbv2gw.SubnetConfiguration{
						{
							Identifier: "subnet-1a",
						},
					},
					LoadBalancerSubnetsSelector: &map[string][]string{
						"v1": {"k1", "k2", "k3"},
					},
					SecurityGroups:        &[]string{"sg1", "sg2", "s3"},
					SecurityGroupPrefixes: &[]string{"pl1"},
					SourceRanges:          &[]string{"127.0.0.0/20"},
					VpcId:                 awssdk.String("vpc-1234"),
					ListenerConfigurations: &[]elbv2gw.ListenerConfiguration{
						{
							ProtocolPort:       "pp1",
							DefaultCertificate: awssdk.String("default-cert"),
						},
					},
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   "lb-k-1",
							Value: "lb-v-1",
						},
					},
					Tags: &map[string]string{
						"tag1": "key1",
						"tag2": "key2",
					},
					EnableICMP:                      awssdk.Bool(false),
					ManageBackendSecurityGroupRules: awssdk.Bool(false),
				},
			},
		},
		{
			name: "full config in gw. empty gw class config",
			gwLbConfig: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerName: awssdk.String("lb name"),
					Scheme:           &internalScheme,
					IpAddressType:    &ipv4AddrType,
					EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic: awssdk.String("on"),
					CustomerOwnedIpv4Pool:                                awssdk.String("coipv4"),
					IPv4IPAMPoolId:                                       awssdk.String("ipam pool"),
					LoadBalancerSubnets: &[]elbv2gw.SubnetConfiguration{
						{
							Identifier: "subnet-1a",
						},
					},
					LoadBalancerSubnetsSelector: &map[string][]string{
						"v1": {"k1", "k2", "k3"},
					},
					SecurityGroups:        &[]string{"sg1", "sg2", "s3"},
					SecurityGroupPrefixes: &[]string{"pl1"},
					SourceRanges:          &[]string{"127.0.0.0/20"},
					VpcId:                 awssdk.String("vpc-1234"),
					ListenerConfigurations: &[]elbv2gw.ListenerConfiguration{
						{
							ProtocolPort:       "pp1",
							DefaultCertificate: awssdk.String("default-cert"),
						},
					},
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   "lb-k-1",
							Value: "lb-v-1",
						},
					},
					Tags: &map[string]string{
						"tag1": "key1",
						"tag2": "key2",
					},
					EnableICMP:                      awssdk.Bool(false),
					ManageBackendSecurityGroupRules: awssdk.Bool(false),
				},
			},
			gwClassLbConfig: elbv2gw.LoadBalancerConfiguration{},
			expected: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerName: awssdk.String("lb name"),
					Scheme:           &internalScheme,
					IpAddressType:    &ipv4AddrType,
					EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic: awssdk.String("on"),
					CustomerOwnedIpv4Pool:                                awssdk.String("coipv4"),
					IPv4IPAMPoolId:                                       awssdk.String("ipam pool"),
					LoadBalancerSubnets: &[]elbv2gw.SubnetConfiguration{
						{
							Identifier: "subnet-1a",
						},
					},
					LoadBalancerSubnetsSelector: &map[string][]string{
						"v1": {"k1", "k2", "k3"},
					},
					SecurityGroups:        &[]string{"sg1", "sg2", "s3"},
					SecurityGroupPrefixes: &[]string{"pl1"},
					SourceRanges:          &[]string{"127.0.0.0/20"},
					VpcId:                 awssdk.String("vpc-1234"),
					ListenerConfigurations: &[]elbv2gw.ListenerConfiguration{
						{
							ProtocolPort:       "pp1",
							DefaultCertificate: awssdk.String("default-cert"),
						},
					},
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   "lb-k-1",
							Value: "lb-v-1",
						},
					},
					Tags: &map[string]string{
						"tag1": "key1",
						"tag2": "key2",
					},
					EnableICMP:                      awssdk.Bool(false),
					ManageBackendSecurityGroupRules: awssdk.Bool(false),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			merger := NewConfigMerger()
			result := merger.Merge(tc.gwClassLbConfig, tc.gwLbConfig)
			assert.Equal(t, tc.expected, result)
		})
	}
}
