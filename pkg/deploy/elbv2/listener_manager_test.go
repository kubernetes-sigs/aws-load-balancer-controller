package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/stretchr/testify/assert"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

func Test_isSDKListenerSettingsDrifted(t *testing.T) {
	type args struct {
		lsSpec                             elbv2model.ListenerSpec
		sdkLS                              ListenerWithTags
		desiredDefaultActions              []elbv2types.Action
		desiredDefaultCerts                []elbv2types.Certificate
		desiredDefaultMutualAuthentication *elbv2types.MutualAuthenticationAttributes
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "listener hasn't drifted",
			args: args{
				lsSpec: elbv2model.ListenerSpec{
					Port:       80,
					Protocol:   elbv2model.ProtocolHTTPS,
					SSLPolicy:  awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
					ALPNPolicy: []string{"HTTP2Preferred"},
				},
				sdkLS: ListenerWithTags{
					Listener: &elbv2types.Listener{
						Port:     awssdk.Int32(80),
						Protocol: elbv2types.ProtocolEnum("HTTPS"),
						Certificates: []elbv2types.Certificate{
							{
								CertificateArn: awssdk.String("cert-arn1"),
								IsDefault:      awssdk.Bool(true),
							},
						},
						DefaultActions: []elbv2types.Action{
							{
								Type: elbv2types.ActionTypeEnum("fixed-response"),
								FixedResponseConfig: &elbv2types.FixedResponseActionConfig{
									StatusCode: awssdk.String("404"),
								},
							},
						},
						SslPolicy:  awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
						AlpnPolicy: []string{"HTTP2Preferred"},
						MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
							Mode: awssdk.String("off"),
						},
					},
				},
				desiredDefaultCerts: []elbv2types.Certificate{
					{
						CertificateArn: awssdk.String("cert-arn1"),
						IsDefault:      awssdk.Bool(true),
					},
				},
				desiredDefaultActions: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("fixed-response"),
						FixedResponseConfig: &elbv2types.FixedResponseActionConfig{
							StatusCode: awssdk.String("404"),
						},
					},
				},
				desiredDefaultMutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
					Mode: awssdk.String("off"),
				},
			},
		},
		{
			name: "listener hasn't drifted if multiple acm specified",
			args: args{
				lsSpec: elbv2model.ListenerSpec{
					Port:       80,
					Protocol:   elbv2model.ProtocolHTTPS,
					SSLPolicy:  awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
					ALPNPolicy: []string{"HTTP2Preferred"},
					Certificates: []elbv2model.Certificate{
						{
							CertificateARN: awssdk.String("cert-arn1"),
						},
						{
							CertificateARN: awssdk.String("cert-arn2"),
						},
					},
				},
				sdkLS: ListenerWithTags{
					Listener: &elbv2types.Listener{
						Port:     awssdk.Int32(80),
						Protocol: elbv2types.ProtocolEnum("HTTPS"),
						Certificates: []elbv2types.Certificate{
							{
								CertificateArn: awssdk.String("cert-arn1"),
								IsDefault:      awssdk.Bool(true),
							},
						},
						DefaultActions: []elbv2types.Action{
							{
								Type: elbv2types.ActionTypeEnum("fixed-response"),
								FixedResponseConfig: &elbv2types.FixedResponseActionConfig{
									StatusCode: awssdk.String("404"),
								},
							},
						},
						SslPolicy:  awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
						AlpnPolicy: []string{"HTTP2Preferred"},
						MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
							Mode: awssdk.String("off"),
						},
					},
				},
				desiredDefaultCerts: []elbv2types.Certificate{
					{
						CertificateArn: awssdk.String("cert-arn1"),
						IsDefault:      awssdk.Bool(true),
					},
				},
				desiredDefaultActions: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("fixed-response"),
						FixedResponseConfig: &elbv2types.FixedResponseActionConfig{
							StatusCode: awssdk.String("404"),
						},
					},
				},
				desiredDefaultMutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
					Mode: awssdk.String("off"),
				},
			},
		},
		{
			name: "Ignore ALPN configuration if not specified in model",
			args: args{
				lsSpec: elbv2model.ListenerSpec{
					Port:      80,
					Protocol:  elbv2model.ProtocolHTTPS,
					SSLPolicy: awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
				},
				sdkLS: ListenerWithTags{
					Listener: &elbv2types.Listener{
						Port:     awssdk.Int32(80),
						Protocol: elbv2types.ProtocolEnum("HTTPS"),
						Certificates: []elbv2types.Certificate{
							{
								CertificateArn: awssdk.String("cert-arn1"),
								IsDefault:      awssdk.Bool(true),
							},
						},
						DefaultActions: []elbv2types.Action{
							{
								Type: elbv2types.ActionTypeEnum("forward-config"),
								ForwardConfig: &elbv2types.ForwardActionConfig{
									TargetGroups: []elbv2types.TargetGroupTuple{
										{
											TargetGroupArn: awssdk.String("target-group"),
										},
									},
								},
							},
						},
						SslPolicy:  awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
						AlpnPolicy: []string{"HTTP2Preferred"},
						MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
							Mode: awssdk.String("off"),
						},
					},
				},
				desiredDefaultCerts: []elbv2types.Certificate{
					{
						CertificateArn: awssdk.String("cert-arn1"),
						IsDefault:      awssdk.Bool(true),
					},
				},
				desiredDefaultActions: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("forward-config"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("target-group"),
								},
							},
						},
					},
				},
				desiredDefaultMutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
					Mode: awssdk.String("off"),
				},
			},
		},
		{
			name: "listener hasn't drifted if mutualAuthentication verify mode specified",
			args: args{
				lsSpec: elbv2model.ListenerSpec{
					Port:      80,
					Protocol:  elbv2model.ProtocolHTTPS,
					SSLPolicy: awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
					MutualAuthentication: &elbv2model.MutualAuthenticationAttributes{
						Mode:          "verify",
						TrustStoreArn: awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789123:truststore/ts-1/8786hghf"),
					},
				},
				sdkLS: ListenerWithTags{
					Listener: &elbv2types.Listener{
						Port:     awssdk.Int32(80),
						Protocol: elbv2types.ProtocolEnum("HTTPS"),
						Certificates: []elbv2types.Certificate{
							{
								CertificateArn: awssdk.String("cert-arn1"),
								IsDefault:      awssdk.Bool(true),
							},
						},
						DefaultActions: []elbv2types.Action{
							{
								Type: elbv2types.ActionTypeEnum("forward-config"),
								ForwardConfig: &elbv2types.ForwardActionConfig{
									TargetGroups: []elbv2types.TargetGroupTuple{
										{
											TargetGroupArn: awssdk.String("target-group"),
										},
									},
								},
							},
						},
						SslPolicy:  awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
						AlpnPolicy: []string{"HTTP2Preferred"},
						MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
							Mode:                          awssdk.String("verify"),
							TrustStoreArn:                 awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789123:truststore/ts-1/8786hghf"),
							IgnoreClientCertificateExpiry: awssdk.Bool(false),
						},
					},
				},
				desiredDefaultCerts: []elbv2types.Certificate{
					{
						CertificateArn: awssdk.String("cert-arn1"),
						IsDefault:      awssdk.Bool(true),
					},
				},
				desiredDefaultActions: []elbv2types.Action{
					{
						Type: elbv2types.ActionTypeEnum("forward-config"),
						ForwardConfig: &elbv2types.ForwardActionConfig{
							TargetGroups: []elbv2types.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("target-group"),
								},
							},
						},
					},
				},
				desiredDefaultMutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
					Mode:                          awssdk.String("verify"),
					TrustStoreArn:                 awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789123:truststore/ts-1/8786hghf"),
					IgnoreClientCertificateExpiry: awssdk.Bool(false),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSDKListenerSettingsDrifted(tt.args.lsSpec, tt.args.sdkLS, tt.args.desiredDefaultActions, tt.args.desiredDefaultCerts, tt.args.desiredDefaultMutualAuthentication)
			assert.Equal(t, tt.want, got)
		})
	}
}
