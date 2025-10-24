package model

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	elbv2modelk8s "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"testing"
)

func Test_buildTargetGroupBindingNetworking_standardBuilder(t *testing.T) {
	protocolTCP := elbv2api.NetworkingProtocolTCP
	protocolUDP := elbv2api.NetworkingProtocolUDP

	intstr80 := intstr.FromInt32(80)
	intstr85 := intstr.FromInt32(85)
	intstrTrafficPort := intstr.FromString(shared_constants.HealthCheckPortTrafficPort)

	testCases := []struct {
		name                     string
		disableRestrictedSGRules bool

		targetPort intstr.IntOrString
		tgSpec     elbv2model.TargetGroupSpec

		sgOutput securityGroupOutput

		expected *elbv2modelk8s.TargetGroupBindingNetworking
	}{
		{
			name:                     "disable restricted sg rules",
			disableRestrictedSGRules: true,
			tgSpec: elbv2model.TargetGroupSpec{
				Protocol: elbv2model.ProtocolTCP,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstr80,
				},
			},
			sgOutput: securityGroupOutput{
				securityGroupTokens:           []core.StringToken{core.LiteralStringToken("sg-1")},
				backendSecurityGroupToken:     core.LiteralStringToken("foo"),
				backendSecurityGroupAllocated: false,
			},
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     nil,
							},
						},
					},
				},
			},
		},
		{
			name:                     "disable restricted sg rules - with udp",
			disableRestrictedSGRules: true,
			sgOutput: securityGroupOutput{
				securityGroupTokens:           []core.StringToken{core.LiteralStringToken("sg-1")},
				backendSecurityGroupToken:     core.LiteralStringToken("foo"),
				backendSecurityGroupAllocated: false,
			},
			tgSpec: elbv2model.TargetGroupSpec{
				Protocol: elbv2model.ProtocolUDP,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstr80,
				},
			},
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     nil,
							},
							{
								Protocol: &protocolUDP,
								Port:     nil,
							},
						},
					},
				},
			},
		},
		{
			name: "use restricted sg rules - int hc port",
			sgOutput: securityGroupOutput{
				securityGroupTokens:           []core.StringToken{core.LiteralStringToken("sg-1")},
				backendSecurityGroupToken:     core.LiteralStringToken("foo"),
				backendSecurityGroupAllocated: false,
			},
			tgSpec: elbv2model.TargetGroupSpec{
				Protocol: elbv2model.ProtocolTCP,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstr80,
				},
			},
			targetPort: intstr80,
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr80,
							},
						},
					},
				},
			},
		},
		{
			name: "use restricted sg rules - int hc port - udp traffic",
			sgOutput: securityGroupOutput{
				securityGroupTokens:           []core.StringToken{core.LiteralStringToken("sg-1")},
				backendSecurityGroupToken:     core.LiteralStringToken("foo"),
				backendSecurityGroupAllocated: false,
			},
			tgSpec: elbv2model.TargetGroupSpec{
				Protocol: elbv2model.ProtocolUDP,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstr80,
				},
			},
			targetPort: intstr80,
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolUDP,
								Port:     &intstr80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr80,
							},
						},
					},
				},
			},
		},
		{
			name: "use restricted sg rules - str hc port",
			sgOutput: securityGroupOutput{
				securityGroupTokens:           []core.StringToken{core.LiteralStringToken("sg-1")},
				backendSecurityGroupToken:     core.LiteralStringToken("foo"),
				backendSecurityGroupAllocated: false,
			},
			tgSpec: elbv2model.TargetGroupSpec{
				Protocol: elbv2model.ProtocolHTTP,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrTrafficPort,
				},
			},
			targetPort: intstr80,
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr80,
							},
						},
					},
				},
			},
		},
		{
			name: "use restricted sg rules - str hc port - udp",
			sgOutput: securityGroupOutput{
				securityGroupTokens:           []core.StringToken{core.LiteralStringToken("sg-1")},
				backendSecurityGroupToken:     core.LiteralStringToken("foo"),
				backendSecurityGroupAllocated: false,
			},
			tgSpec: elbv2model.TargetGroupSpec{
				Protocol: elbv2model.ProtocolUDP,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrTrafficPort,
				},
			},
			targetPort: intstr80,
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolUDP,
								Port:     &intstr80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr80,
							},
						},
					},
				},
			},
		},
		{
			name: "use restricted sg rules - diff hc port",
			sgOutput: securityGroupOutput{
				securityGroupTokens:           []core.StringToken{core.LiteralStringToken("sg-1")},
				backendSecurityGroupToken:     core.LiteralStringToken("foo"),
				backendSecurityGroupAllocated: false,
			},
			tgSpec: elbv2model.TargetGroupSpec{
				Protocol: elbv2model.ProtocolHTTP,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstr85,
				},
			},
			targetPort: intstr80,
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr85,
							},
						},
					},
				},
			},
		},
		{
			name: "use restricted sg rules - str hc port - udp",
			sgOutput: securityGroupOutput{
				securityGroupTokens:           []core.StringToken{core.LiteralStringToken("sg-1")},
				backendSecurityGroupToken:     core.LiteralStringToken("foo"),
				backendSecurityGroupAllocated: false,
			},
			tgSpec: elbv2model.TargetGroupSpec{
				Protocol: elbv2model.ProtocolUDP,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstr85,
				},
			},
			targetPort: intstr80,
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolUDP,
								Port:     &intstr80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken("foo"),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr85,
							},
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := &targetGroupBindingNetworkBuilderImpl{
				disableRestrictedSGRules: tc.disableRestrictedSGRules,
				sgOutput:                 tc.sgOutput,
			}

			result, err := builder.buildTargetGroupBindingNetworking(tc.tgSpec, tc.targetPort)
			assert.Equal(t, tc.expected, result)
			assert.NoError(t, err)
		})
	}
}

func Test_Test_buildTargetGroupBindingNetworking_nlbZeroSg(t *testing.T) {
	vpcId := "vpc-123"
	tcp := elbv2api.NetworkingProtocolTCP
	udp := elbv2api.NetworkingProtocolUDP

	intstrPort80 := intstr.FromInt32(80)
	intstrPort85 := intstr.FromInt32(85)

	lbSubnets := []ec2types.Subnet{
		{
			CidrBlock: awssdk.String("192.168.1.0/24"),
			Ipv6CidrBlockAssociationSet: []ec2types.SubnetIpv6CidrBlockAssociation{
				{
					Ipv6CidrBlock: awssdk.String("2600:1f13:837:8500::/64"),
					Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
						State: ec2types.SubnetCidrBlockStateCodeAssociated,
					},
				},
				{
					Ipv6CidrBlock: awssdk.String("2600:1f13:837:8504::/64"),
					Ipv6CidrBlockState: &ec2types.SubnetCidrBlockState{
						State: ec2types.SubnetCidrBlockStateCodeAssociated,
					},
				},
			},
		},
	}

	vpcInfo := networking.VPCInfo{
		VpcId: awssdk.String("vpc-2f09a348"),
		CidrBlockAssociationSet: []ec2types.VpcCidrBlockAssociation{
			{
				CidrBlock: awssdk.String("192.168.0.0/16"),
				CidrBlockState: &ec2types.VpcCidrBlockState{
					State: ec2types.VpcCidrBlockStateCodeAssociated,
				},
			},
		},
		Ipv6CidrBlockAssociationSet: []ec2types.VpcIpv6CidrBlockAssociation{
			{
				Ipv6CidrBlock: awssdk.String("2600:1f14:f8c:2700::/56"),
				Ipv6CidrBlockState: &ec2types.VpcCidrBlockState{
					State: ec2types.VpcCidrBlockStateCodeAssociated,
				},
			},
		},
	}

	type fetchVPCInfoCall struct {
		wantVPCInfo networking.VPCInfo
		err         error
	}

	testCases := []struct {
		name string

		lbScheme       elbv2model.LoadBalancerScheme
		lbSourceRanges *[]string

		tgSpec     elbv2model.TargetGroupSpec
		targetPort intstr.IntOrString

		fetchVPCInfoCalls []fetchVPCInfoCall

		expectErr bool
		expected  elbv2modelk8s.TargetGroupBindingNetworking
	}{
		{
			name:     "tcp : instance : ipv4 : internal : hc port = traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternal,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				Protocol:      elbv2model.ProtocolTCP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort80,
				},
			},
			targetPort: intstrPort80,
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: vpcInfo,
				},
			},
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.0.0/16",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
				},
			},
		},
		{
			name:     "tcp : instance : ipv4 : internet-facing : hc port = traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternetFacing,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				Protocol:      elbv2model.ProtocolTCP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort80,
				},
			},
			targetPort: intstrPort80,
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "0.0.0.0/0",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
				},
			},
		},
		{
			name:     "tcp : instance : ipv4 : internal : hc port != traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternal,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				Protocol:      elbv2model.ProtocolTCP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort85,
				},
			},
			targetPort: intstrPort80,
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: vpcInfo,
				},
			},
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.0.0/16",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.1.0/24",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort85,
							},
						},
					},
				},
			},
		},
		{
			name:     "tcp : instance : ipv4 : internet-facing : hc port != traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternetFacing,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				Protocol:      elbv2model.ProtocolTCP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort85,
				},
			},
			targetPort: intstrPort80,
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "0.0.0.0/0",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.1.0/24",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort85,
							},
						},
					},
				},
			},
		},
		{
			name:     "udp : instance : ipv4 : internal : hc port = traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternal,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				Protocol:      elbv2model.ProtocolUDP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort80,
				},
			},
			targetPort: intstrPort80,
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: vpcInfo,
				},
			},
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.0.0/16",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &udp,
								Port:     &intstrPort80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.1.0/24",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
				},
			},
		},
		{
			name:     "udp : instance : ipv4 : internet-facing : hc port = traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternetFacing,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				Protocol:      elbv2model.ProtocolUDP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort80,
				},
			},
			targetPort: intstrPort80,
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "0.0.0.0/0",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &udp,
								Port:     &intstrPort80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.1.0/24",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
				},
			},
		},
		{
			name:     "udp : instance : ipv4 : internal : hc port != traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternal,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				Protocol:      elbv2model.ProtocolUDP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort85,
				},
			},
			targetPort: intstrPort80,
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: vpcInfo,
				},
			},
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.0.0/16",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &udp,
								Port:     &intstrPort80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.1.0/24",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort85,
							},
						},
					},
				},
			},
		},
		{
			name:     "udp : instance : ipv4 : internet-facing : hc port != traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternetFacing,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				Protocol:      elbv2model.ProtocolUDP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort85,
				},
			},
			targetPort: intstrPort80,
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "0.0.0.0/0",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &udp,
								Port:     &intstrPort80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.1.0/24",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort85,
							},
						},
					},
				},
			},
		},
		{
			name:     "tcp : ip : ipv4 : internal : hc port = traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternal,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				Protocol:      elbv2model.ProtocolTCP,
				TargetType:    elbv2model.TargetTypeIP,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort80,
				},
			},
			targetPort: intstrPort80,
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.1.0/24",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
				},
			},
		},
		{
			name:     "udp : ip : ipv4 : internal : hc port = traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternal,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				Protocol:      elbv2model.ProtocolUDP,
				TargetType:    elbv2model.TargetTypeIP,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort80,
				},
			},
			targetPort: intstrPort80,
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: vpcInfo,
				},
			},
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.0.0/16",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &udp,
								Port:     &intstrPort80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.1.0/24",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
				},
			},
		},
		{
			name:     "tcp : ip : ipv6 : internal : hc port = traffic port : no source ranges : with preserve ip",
			lbScheme: elbv2model.LoadBalancerSchemeInternal,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv6,
				Protocol:      elbv2model.ProtocolTCP,
				TargetType:    elbv2model.TargetTypeIP,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort80,
				},
				TargetGroupAttributes: []elbv2model.TargetGroupAttribute{
					{
						Key:   shared_constants.TGAttributePreserveClientIPEnabled,
						Value: "true",
					},
				},
			},
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: vpcInfo,
				},
			},
			targetPort: intstrPort80,
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2600:1f14:f8c:2700::/56",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
				},
			},
		},
		{
			name:     "tcp : instance : ipv6 : internal : hc port = traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternal,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv6,
				Protocol:      elbv2model.ProtocolTCP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort80,
				},
			},
			targetPort: intstrPort80,
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: vpcInfo,
				},
			},
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2600:1f14:f8c:2700::/56",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
				},
			},
		},
		{
			name:     "tcp : instance : ipv6 : internet-facing : hc port = traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternetFacing,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv6,
				Protocol:      elbv2model.ProtocolTCP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort80,
				},
			},
			targetPort: intstrPort80,
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "::/0",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
				},
			},
		},
		{
			name:     "tcp : instance : ipv6 : internal : hc port != traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternal,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv6,
				Protocol:      elbv2model.ProtocolTCP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort85,
				},
			},
			targetPort: intstrPort80,
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: vpcInfo,
				},
			},
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2600:1f14:f8c:2700::/56",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2600:1f13:837:8500::/64",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2600:1f13:837:8504::/64",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort85,
							},
						},
					},
				},
			},
		},
		{
			name:     "tcp : instance : ipv6 : internet-facing : hc port != traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternetFacing,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv6,
				Protocol:      elbv2model.ProtocolTCP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort85,
				},
			},
			targetPort: intstrPort80,
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "::/0",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2600:1f13:837:8500::/64",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2600:1f13:837:8504::/64",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort85,
							},
						},
					},
				},
			},
		},
		{
			name:     "udp : instance : ipv6 : internal : hc port = traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternal,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv6,
				Protocol:      elbv2model.ProtocolUDP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort80,
				},
			},
			targetPort: intstrPort80,
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: vpcInfo,
				},
			},
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2600:1f14:f8c:2700::/56",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &udp,
								Port:     &intstrPort80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2600:1f13:837:8500::/64",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2600:1f13:837:8504::/64",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
				},
			},
		},
		{
			name:     "udp : instance : ipv6 : internet-facing : hc port = traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternetFacing,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv6,
				Protocol:      elbv2model.ProtocolUDP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort80,
				},
			},
			targetPort: intstrPort80,
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "::/0",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &udp,
								Port:     &intstrPort80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2600:1f13:837:8500::/64",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2600:1f13:837:8504::/64",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
				},
			},
		},
		{
			name:     "udp : instance : ipv6 : internal : hc port != traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternal,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv6,
				Protocol:      elbv2model.ProtocolUDP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort85,
				},
			},
			targetPort: intstrPort80,
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: vpcInfo,
				},
			},
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2600:1f14:f8c:2700::/56",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &udp,
								Port:     &intstrPort80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2600:1f13:837:8500::/64",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2600:1f13:837:8504::/64",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort85,
							},
						},
					},
				},
			},
		},
		{
			name:     "udp : instance : ipv4 : internet-facing : hc port != traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternetFacing,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				Protocol:      elbv2model.ProtocolUDP,
				TargetType:    elbv2model.TargetTypeInstance,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort85,
				},
			},
			targetPort: intstrPort80,
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "0.0.0.0/0",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &udp,
								Port:     &intstrPort80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.1.0/24",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort85,
							},
						},
					},
				},
			},
		},
		{
			name:     "tcp : ip : ipv4 : internal : hc port = traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternal,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				Protocol:      elbv2model.ProtocolTCP,
				TargetType:    elbv2model.TargetTypeIP,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort80,
				},
			},
			targetPort: intstrPort80,
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.1.0/24",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
				},
			},
		},
		{
			name:     "udp : ip : ipv4 : internal : hc port = traffic port : no source ranges",
			lbScheme: elbv2model.LoadBalancerSchemeInternal,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				Protocol:      elbv2model.ProtocolUDP,
				TargetType:    elbv2model.TargetTypeIP,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort80,
				},
			},
			targetPort: intstrPort80,
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: vpcInfo,
				},
			},
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.0.0/16",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &udp,
								Port:     &intstrPort80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.1.0/24",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
				},
			},
		},
		{
			name:     "tcp : ip : ipv4 : internal : hc port = traffic port : no source ranges : with preserve ip",
			lbScheme: elbv2model.LoadBalancerSchemeInternal,
			tgSpec: elbv2model.TargetGroupSpec{
				IPAddressType: elbv2model.TargetGroupIPAddressTypeIPv4,
				Protocol:      elbv2model.ProtocolTCP,
				TargetType:    elbv2model.TargetTypeIP,
				HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
					Port: &intstrPort80,
				},
				TargetGroupAttributes: []elbv2model.TargetGroupAttribute{
					{
						Key:   shared_constants.TGAttributePreserveClientIPEnabled,
						Value: "true",
					},
				},
			},
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: vpcInfo,
				},
			},
			targetPort: intstrPort80,
			expected: elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "192.168.0.0/16",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &tcp,
								Port:     &intstrPort80,
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			vpcInfoProvider := networking.NewMockVPCInfoProvider(ctrl)
			for _, call := range tc.fetchVPCInfoCalls {
				vpcInfoProvider.EXPECT().FetchVPCInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(call.wantVPCInfo, call.err)
			}

			// disableRestrictedSGRules is not used in NLB 0 SG path
			// the sg output is only checked for existence of sg tokens.
			builder := newTargetGroupBindingNetworkBuilder(false, vpcId, tc.lbScheme, tc.lbSourceRanges, securityGroupOutput{}, lbSubnets, vpcInfoProvider)

			result, err := builder.buildTargetGroupBindingNetworking(tc.tgSpec, tc.targetPort)

			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, *result)
			}
		})
	}
}
