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
	mergeModeGWC := elbv2gw.MergeModePreferGatewayClass
	mergeModeGW := elbv2gw.MergeModePreferGateway
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
		{
			name: "full config in gw class and gw. merge mode prefers gatewayclass",
			gwClassLbConfig: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					MergingMode:      &mergeModeGWC,
					LoadBalancerName: awssdk.String("gwclass-name"),
					Scheme:           &internalScheme,
					IpAddressType:    &ipv4AddrType,
					EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic: awssdk.String("off"),
					CustomerOwnedIpv4Pool:                                awssdk.String("coipv4-gwclass"),
					IPv4IPAMPoolId:                                       awssdk.String("ipam pool gwclass"),
					LoadBalancerSubnets: &[]elbv2gw.SubnetConfiguration{
						{
							Identifier: "subnet-1a",
						},
						{
							Identifier: "subnet-1b",
						},
					},
					LoadBalancerSubnetsSelector: &map[string][]string{
						"gwclass-v": {"k1", "k2", "k3"},
					},
					SecurityGroups:        &[]string{"sg1"},
					SecurityGroupPrefixes: &[]string{"pl1"},
					SourceRanges:          &[]string{"127.0.0.0/20"},
					VpcId:                 awssdk.String("vpc-gw-class"),
					ListenerConfigurations: &[]elbv2gw.ListenerConfiguration{
						{
							ProtocolPort:       "pp1-gwclass",
							DefaultCertificate: awssdk.String("default-cert-gwclass"),
						},
						{
							ProtocolPort:       "pp1-common",
							DefaultCertificate: awssdk.String("common-gwclass"),
						},
					},
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   "lb-gw-class-k-1",
							Value: "lb-gw-class-v-1",
						},
						{
							Key:   "common",
							Value: "gwclass",
						},
					},
					Tags: &map[string]string{
						"gwclass":   "key1",
						"commonTag": "gwclass",
					},
					EnableICMP:                      awssdk.Bool(true),
					ManageBackendSecurityGroupRules: awssdk.Bool(true),
				},
			},
			gwLbConfig: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerName: awssdk.String("gw-name"),
					Scheme:           &internalScheme,
					IpAddressType:    &ipv4AddrType,
					EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic: awssdk.String("on"),
					CustomerOwnedIpv4Pool:                                awssdk.String("coipv4-class"),
					IPv4IPAMPoolId:                                       awssdk.String("ipam pool class"),
					LoadBalancerSubnets: &[]elbv2gw.SubnetConfiguration{
						{
							Identifier: "subnet-1c",
						},
					},
					LoadBalancerSubnetsSelector: &map[string][]string{
						"gw-v": {"k1", "k2", "k3"},
					},
					SecurityGroups:        &[]string{"sg1-gw"},
					SecurityGroupPrefixes: &[]string{"pl1-gw"},
					SourceRanges:          &[]string{"127.0.0.10/20"},
					VpcId:                 awssdk.String("vpc-gw"),
					ListenerConfigurations: &[]elbv2gw.ListenerConfiguration{
						{
							ProtocolPort:       "pp1-gw",
							DefaultCertificate: awssdk.String("default-cert-gw"),
						},
						{
							ProtocolPort:       "pp1-common",
							DefaultCertificate: awssdk.String("common-gw"),
						},
					},
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   "lb-gw-k-1",
							Value: "lb-gw-v-1",
						},
						{
							Key:   "common",
							Value: "gw",
						},
					},
					Tags: &map[string]string{
						"gw":        "key2",
						"commonTag": "gw",
					},
					EnableICMP:                      awssdk.Bool(true),
					ManageBackendSecurityGroupRules: awssdk.Bool(true),
				},
			},
			expected: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerName: awssdk.String("gwclass-name"),
					Scheme:           &internalScheme,
					IpAddressType:    &ipv4AddrType,
					EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic: awssdk.String("off"),
					CustomerOwnedIpv4Pool:                                awssdk.String("coipv4-gwclass"),
					IPv4IPAMPoolId:                                       awssdk.String("ipam pool gwclass"),
					LoadBalancerSubnets: &[]elbv2gw.SubnetConfiguration{
						{
							Identifier: "subnet-1a",
						},
						{
							Identifier: "subnet-1b",
						},
					},
					LoadBalancerSubnetsSelector: &map[string][]string{
						"gwclass-v": {"k1", "k2", "k3"},
					},
					SecurityGroups:        &[]string{"sg1"},
					SecurityGroupPrefixes: &[]string{"pl1"},
					SourceRanges:          &[]string{"127.0.0.0/20"},
					VpcId:                 awssdk.String("vpc-gw-class"),
					ListenerConfigurations: &[]elbv2gw.ListenerConfiguration{
						{
							ProtocolPort:       "pp1-common",
							DefaultCertificate: awssdk.String("common-gwclass"),
						},
						{
							ProtocolPort:       "pp1-gw",
							DefaultCertificate: awssdk.String("default-cert-gw"),
						},
						{
							ProtocolPort:       "pp1-gwclass",
							DefaultCertificate: awssdk.String("default-cert-gwclass"),
						},
					},
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   "common",
							Value: "gwclass",
						},
						{
							Key:   "lb-gw-class-k-1",
							Value: "lb-gw-class-v-1",
						},
						{
							Key:   "lb-gw-k-1",
							Value: "lb-gw-v-1",
						},
					},
					Tags: &map[string]string{
						"gwclass":   "key1",
						"commonTag": "gwclass",
						"gw":        "key2",
					},
					EnableICMP:                      awssdk.Bool(true),
					ManageBackendSecurityGroupRules: awssdk.Bool(true),
				},
			},
		},
		{
			name: "full config in gw class and gw. merge mode prefers gateway",
			gwClassLbConfig: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					MergingMode:      &mergeModeGW,
					LoadBalancerName: awssdk.String("gwclass-name"),
					Scheme:           &internalScheme,
					IpAddressType:    &ipv4AddrType,
					EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic: awssdk.String("off"),
					CustomerOwnedIpv4Pool:                                awssdk.String("coipv4-gwclass"),
					IPv4IPAMPoolId:                                       awssdk.String("ipam pool gwclass"),
					LoadBalancerSubnets: &[]elbv2gw.SubnetConfiguration{
						{
							Identifier: "subnet-1a",
						},
						{
							Identifier: "subnet-1b",
						},
					},
					LoadBalancerSubnetsSelector: &map[string][]string{
						"gwclass-v": {"k1", "k2", "k3"},
					},
					SecurityGroups:        &[]string{"sg1"},
					SecurityGroupPrefixes: &[]string{"pl1"},
					SourceRanges:          &[]string{"127.0.0.0/20"},
					VpcId:                 awssdk.String("vpc-gw-class"),
					ListenerConfigurations: &[]elbv2gw.ListenerConfiguration{
						{
							ProtocolPort:       "pp1-gwclass",
							DefaultCertificate: awssdk.String("default-cert-gwclass"),
						},
						{
							ProtocolPort:       "pp1-common",
							DefaultCertificate: awssdk.String("common-gwclass"),
						},
					},
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   "lb-gw-class-k-1",
							Value: "lb-gw-class-v-1",
						},
						{
							Key:   "common",
							Value: "gwclass",
						},
					},
					Tags: &map[string]string{
						"gwclass":   "key1",
						"commonTag": "gwclass",
					},
					EnableICMP:                      awssdk.Bool(true),
					ManageBackendSecurityGroupRules: awssdk.Bool(true),
				},
			},
			gwLbConfig: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerName: awssdk.String("gw-name"),
					Scheme:           &internalScheme,
					IpAddressType:    &ipv4AddrType,
					EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic: awssdk.String("on"),
					CustomerOwnedIpv4Pool:                                awssdk.String("coipv4-class"),
					IPv4IPAMPoolId:                                       awssdk.String("ipam pool class"),
					LoadBalancerSubnets: &[]elbv2gw.SubnetConfiguration{
						{
							Identifier: "subnet-1c",
						},
					},
					LoadBalancerSubnetsSelector: &map[string][]string{
						"gw-v": {"k1", "k2", "k3"},
					},
					SecurityGroups:        &[]string{"sg1-gw"},
					SecurityGroupPrefixes: &[]string{"pl1-gw"},
					SourceRanges:          &[]string{"127.0.0.10/20"},
					VpcId:                 awssdk.String("vpc-gw"),
					ListenerConfigurations: &[]elbv2gw.ListenerConfiguration{
						{
							ProtocolPort:       "pp1-gw",
							DefaultCertificate: awssdk.String("default-cert-gw"),
						},
						{
							ProtocolPort:       "pp1-common",
							DefaultCertificate: awssdk.String("common-gw"),
						},
					},
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   "lb-gw-k-1",
							Value: "lb-gw-v-1",
						},
						{
							Key:   "common",
							Value: "gw",
						},
					},
					Tags: &map[string]string{
						"gw":        "key2",
						"commonTag": "gw",
					},
					EnableICMP:                      awssdk.Bool(true),
					ManageBackendSecurityGroupRules: awssdk.Bool(true),
				},
			},
			expected: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerName: awssdk.String("gw-name"),
					Scheme:           &internalScheme,
					IpAddressType:    &ipv4AddrType,
					EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic: awssdk.String("on"),
					CustomerOwnedIpv4Pool:                                awssdk.String("coipv4-class"),
					IPv4IPAMPoolId:                                       awssdk.String("ipam pool class"),
					LoadBalancerSubnets: &[]elbv2gw.SubnetConfiguration{
						{
							Identifier: "subnet-1c",
						},
					},
					LoadBalancerSubnetsSelector: &map[string][]string{
						"gw-v": {"k1", "k2", "k3"},
					},
					SecurityGroups:        &[]string{"sg1-gw"},
					SecurityGroupPrefixes: &[]string{"pl1-gw"},
					SourceRanges:          &[]string{"127.0.0.10/20"},
					VpcId:                 awssdk.String("vpc-gw"),
					ListenerConfigurations: &[]elbv2gw.ListenerConfiguration{
						{
							ProtocolPort:       "pp1-common",
							DefaultCertificate: awssdk.String("common-gw"),
						},
						{
							ProtocolPort:       "pp1-gw",
							DefaultCertificate: awssdk.String("default-cert-gw"),
						},
						{
							ProtocolPort:       "pp1-gwclass",
							DefaultCertificate: awssdk.String("default-cert-gwclass"),
						},
					},
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   "common",
							Value: "gw",
						},
						{
							Key:   "lb-gw-class-k-1",
							Value: "lb-gw-class-v-1",
						},
						{
							Key:   "lb-gw-k-1",
							Value: "lb-gw-v-1",
						},
					},
					Tags: &map[string]string{
						"gw":        "key2",
						"commonTag": "gw",
						"gwclass":   "key1",
					},
					EnableICMP:                      awssdk.Bool(true),
					ManageBackendSecurityGroupRules: awssdk.Bool(true),
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
