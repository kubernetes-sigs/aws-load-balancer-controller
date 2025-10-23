package model

import (
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	elbv2modelk8s "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"testing"
)

func Test_buildTargetGroupBindingNetworking(t *testing.T) {
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
