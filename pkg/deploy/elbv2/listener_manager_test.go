package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
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
		name         string
		args         args
		wantEnhanced bool
		wantLegacy   bool
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
			name:         "ALPN configuration if not specified in model",
			wantLegacy:   false,
			wantEnhanced: true,
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
							AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
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
					AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
				},
			},
		},
		{
			name: "desired = nil, sdk = nil.",
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
			},
		},
		{
			name: "desired = nil, sdk = mtls off.",
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
							Mode:                          awssdk.String("off"),
							TrustStoreArn:                 awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789123:truststore/ts-1/8786hghf"),
							IgnoreClientCertificateExpiry: awssdk.Bool(false),
							AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
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
			},
		},
		{
			name:         "desired = nil, sdk = mtls on.",
			wantEnhanced: true,
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
							AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
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
			},
		},
		{
			name: "desired = mtls off, sdk = nil",
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
					Mode:                          awssdk.String("off"),
					TrustStoreArn:                 awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789123:truststore/ts-1/8786hghf"),
					IgnoreClientCertificateExpiry: awssdk.Bool(false),
					AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
				},
			},
		},
		{
			name: "desired = mtls off, sdk = off, result = no drift",
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
							Mode:                          awssdk.String("off"),
							TrustStoreArn:                 awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789123:truststore/ts-1/8786hghf"),
							IgnoreClientCertificateExpiry: awssdk.Bool(false),
							AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
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
					Mode:                          awssdk.String("off"),
					TrustStoreArn:                 awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789123:truststore/ts-1/8786hghf"),
					IgnoreClientCertificateExpiry: awssdk.Bool(false),
					AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
				},
			},
		},
		{
			name:         "desired = mtls on, sdk = nil. result = drift",
			wantLegacy:   true,
			wantEnhanced: true,
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
					AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
				},
			},
		},
		{
			name:         "desired = mtls on, sdk = mtls on. result = drift when values change",
			wantEnhanced: true,
			wantLegacy:   true,
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
							TrustStoreArn:                 awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789123:truststore/ts-1/8786hghf--2"),
							IgnoreClientCertificateExpiry: awssdk.Bool(false),
							AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
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
					AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
				},
			},
		},
		{
			name: "desired = mtls on, sdk = mtls on. result = no drift because no change",
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
							AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
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
					AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
				},
			},
		},
		{
			name: "desired = mtls on, sdk = mtls on. result = no drift because no change (association status set)",
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
							AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
							TrustStoreAssociationStatus:   elbv2types.TrustStoreAssociationStatusEnumActive,
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
					AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mEnhanced := &defaultListenerManager{
				enhancedDefaultingPolicyEnabled: true,
			}
			gotEnhanced := mEnhanced.isSDKListenerSettingsDrifted(tt.args.lsSpec, tt.args.sdkLS, tt.args.desiredDefaultActions, tt.args.desiredDefaultCerts, tt.args.desiredDefaultMutualAuthentication)
			assert.Equal(t, tt.wantEnhanced, gotEnhanced)

			mLegacy := &defaultListenerManager{}
			gotLegacy := mLegacy.isSDKListenerSettingsDrifted(tt.args.lsSpec, tt.args.sdkLS, tt.args.desiredDefaultActions, tt.args.desiredDefaultCerts, tt.args.desiredDefaultMutualAuthentication)
			assert.Equal(t, tt.wantLegacy, gotLegacy)
		})
	}
}

func Test_buildSDKModifyListenerInput(t *testing.T) {
	testCases := []struct {
		name                  string
		lsSpec                elbv2model.ListenerSpec
		desiredDefaultActions []elbv2types.Action
		desiredDefaultCerts   []elbv2types.Certificate
		removeMTLS            bool
		removeALPN            bool
		expected              elbv2sdk.ModifyListenerInput
	}{
		{
			name: "all fields populated",
			lsSpec: elbv2model.ListenerSpec{
				Port:       80,
				Protocol:   elbv2model.ProtocolHTTPS,
				SSLPolicy:  awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
				ALPNPolicy: []string{"HTTP2Preferred"},
				MutualAuthentication: &elbv2model.MutualAuthenticationAttributes{
					Mode:                          "verify",
					TrustStoreArn:                 awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789123:truststore/ts-1/8786hghf"),
					IgnoreClientCertificateExpiry: awssdk.Bool(false),
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
			desiredDefaultCerts: []elbv2types.Certificate{
				{
					CertificateArn: awssdk.String("cert-arn1"),
				},
			},
			expected: elbv2sdk.ModifyListenerInput{
				Port:     awssdk.Int32(80),
				Protocol: elbv2types.ProtocolEnumHttps,
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
				Certificates: []elbv2types.Certificate{
					{
						CertificateArn: awssdk.String("cert-arn1"),
					},
				},
				AlpnPolicy: []string{"HTTP2Preferred"},
				SslPolicy:  awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
				MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
					Mode:                          awssdk.String("verify"),
					TrustStoreArn:                 awssdk.String("arn:aws:elasticloadbalancing:us-east-1:123456789123:truststore/ts-1/8786hghf"),
					IgnoreClientCertificateExpiry: awssdk.Bool(false),
					AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
				},
			},
		},
		{
			name: "no alpn policy",
			lsSpec: elbv2model.ListenerSpec{
				Port:      80,
				Protocol:  elbv2model.ProtocolHTTPS,
				SSLPolicy: awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
			},
			desiredDefaultActions: []elbv2types.Action{},
			desiredDefaultCerts:   []elbv2types.Certificate{},
			expected: elbv2sdk.ModifyListenerInput{
				Port:           awssdk.Int32(80),
				Protocol:       elbv2types.ProtocolEnumHttps,
				DefaultActions: []elbv2types.Action{},
				Certificates:   []elbv2types.Certificate{},
				SslPolicy:      awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
			},
		},
		{
			name:       "with explicit remove mtls flag on",
			removeMTLS: true,
			lsSpec: elbv2model.ListenerSpec{
				Port:      80,
				Protocol:  elbv2model.ProtocolHTTPS,
				SSLPolicy: awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
			},
			desiredDefaultActions: []elbv2types.Action{},
			desiredDefaultCerts:   []elbv2types.Certificate{},
			expected: elbv2sdk.ModifyListenerInput{
				Port:           awssdk.Int32(80),
				Protocol:       elbv2types.ProtocolEnumHttps,
				DefaultActions: []elbv2types.Action{},
				Certificates:   []elbv2types.Certificate{},
				SslPolicy:      awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
				MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
					Mode: awssdk.String(string(elbv2model.MutualAuthenticationOffMode)),
				},
			},
		},
		{
			name:       "with explicit remove alpn flag on",
			removeALPN: true,
			lsSpec: elbv2model.ListenerSpec{
				Port:      80,
				Protocol:  elbv2model.ProtocolHTTPS,
				SSLPolicy: awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
			},
			desiredDefaultActions: []elbv2types.Action{},
			desiredDefaultCerts:   []elbv2types.Certificate{},
			expected: elbv2sdk.ModifyListenerInput{
				Port:           awssdk.Int32(80),
				Protocol:       elbv2types.ProtocolEnumHttps,
				DefaultActions: []elbv2types.Action{},
				Certificates:   []elbv2types.Certificate{},
				SslPolicy:      awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
				AlpnPolicy: []string{
					string(elbv2model.ALPNPolicyNone),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := buildSDKModifyListenerInput(tc.lsSpec, tc.desiredDefaultActions, tc.desiredDefaultCerts, tc.removeMTLS, tc.removeALPN)
			assert.Equal(t, tc.expected, *res)
		})
	}
}

func Test_isRemoveMTLS(t *testing.T) {
	testCases := []struct {
		name                               string
		sdkLS                              ListenerWithTags
		desiredDefaultMutualAuthentication *elbv2types.MutualAuthenticationAttributes
		expected                           bool
	}{
		{
			name: "desired = nil, sdk = nil",
			sdkLS: ListenerWithTags{
				Listener: &elbv2types.Listener{},
			},
			desiredDefaultMutualAuthentication: nil,
		},
		{
			name: "desired = nil, sdk = none",
			sdkLS: ListenerWithTags{
				Listener: &elbv2types.Listener{
					MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
						Mode: awssdk.String("off"),
					},
				},
			},
			desiredDefaultMutualAuthentication: nil,
		},
		{
			name: "desired = nil, sdk = verify",
			sdkLS: ListenerWithTags{
				Listener: &elbv2types.Listener{
					MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
						Mode: awssdk.String("verify"),
					},
				},
			},
			desiredDefaultMutualAuthentication: nil,
			expected:                           true,
		},
		{
			name: "desired = verify, sdk = verify",
			sdkLS: ListenerWithTags{
				Listener: &elbv2types.Listener{
					MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
						Mode: awssdk.String("verify"),
					},
				},
			},
			desiredDefaultMutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
				Mode: awssdk.String("verify"),
			},
		},
		{
			name: "desired = passthrough, sdk = verify",
			sdkLS: ListenerWithTags{
				Listener: &elbv2types.Listener{
					MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
						Mode: awssdk.String("verify"),
					},
				},
			},
			desiredDefaultMutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
				Mode: awssdk.String("passthrough"),
			},
		},
		{
			name: "desired = verify, sdk = nil",
			sdkLS: ListenerWithTags{
				Listener: &elbv2types.Listener{},
			},
			desiredDefaultMutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
				Mode: awssdk.String("passthrough"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := isRemoveMTLS(tc.sdkLS, tc.desiredDefaultMutualAuthentication)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func Test_isRemoveALPN(t *testing.T) {
	testCases := []struct {
		name         string
		sdkLS        ListenerWithTags
		listenerSpec elbv2model.ListenerSpec
		expected     bool
	}{
		{
			name: "desired = nil, sdk = nil",
			sdkLS: ListenerWithTags{
				Listener: &elbv2types.Listener{},
			},
			listenerSpec: elbv2model.ListenerSpec{},
		},
		{
			name: "desired = nil, sdk = none",
			sdkLS: ListenerWithTags{
				Listener: &elbv2types.Listener{
					AlpnPolicy: []string{"None"},
				},
			},
			listenerSpec: elbv2model.ListenerSpec{},
		},
		{
			name: "desired = nil, sdk = HTTP2Only",
			sdkLS: ListenerWithTags{
				Listener: &elbv2types.Listener{
					AlpnPolicy: []string{"HTTP2Only"},
				},
			},
			listenerSpec: elbv2model.ListenerSpec{},
			expected:     true,
		},
		{
			name: "desired = HTTP2Only, sdk = HTTP2Only",
			sdkLS: ListenerWithTags{
				Listener: &elbv2types.Listener{
					AlpnPolicy: []string{"HTTP2Only"},
				},
			},
			listenerSpec: elbv2model.ListenerSpec{
				ALPNPolicy: []string{"HTTP2Only"},
			},
		},
		{
			name: "desired = HTTP2Optional, sdk = HTTP2Only",
			sdkLS: ListenerWithTags{
				Listener: &elbv2types.Listener{
					AlpnPolicy: []string{"HTTP2Only"},
				},
			},
			listenerSpec: elbv2model.ListenerSpec{
				ALPNPolicy: []string{"HTTP2Optional"},
			},
		},
		{
			name: "desired = HTTP2Only, sdk = nil",
			sdkLS: ListenerWithTags{
				Listener: &elbv2types.Listener{},
			},
			listenerSpec: elbv2model.ListenerSpec{
				ALPNPolicy: []string{"HTTP2Optional"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := isRemoveALPN(tc.sdkLS, tc.listenerSpec)
			assert.Equal(t, tc.expected, res)
		})
	}
}
