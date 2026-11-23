package elbv2

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/config"
	acmModel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/acm"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
)

const nlbMTLSTrustStoreARN = "arn:aws:elasticloadbalancing:us-east-1:123456789123:truststore/ts-1/8786hghf"

// getMockClientCertIdentityFilter returns a single filter for subject CN with the given value
func getMockClientCertIdentityFilter(value string) []elbv2types.ClientCertificateIdentityFilter {
	return []elbv2types.ClientCertificateIdentityFilter{
		{
			Action:    elbv2types.ClientCertificateIdentityFilterActionAccept,
			Field:     elbv2types.ClientCertificateIdentityFilterFieldSubjectCn,
			MatchType: elbv2types.ClientCertificateIdentityFilterMatchTypeExact,
			Value:     awssdk.String(value),
		},
	}
}

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
							CertificateARN: acmModel.NewExistingCertificate("cert-arn1").CertificateARN(),
						},
						{
							CertificateARN: acmModel.NewExistingCertificate("cert-arn2").CertificateARN(),
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
		{
			name: "null desired client certificate identity filters ignores existing filters on the listener",
			args: args{
				lsSpec: elbv2model.ListenerSpec{
					Port:     443,
					Protocol: elbv2model.ProtocolTLS,
				},
				sdkLS: ListenerWithTags{
					Listener: &elbv2types.Listener{
						Port:     awssdk.Int32(443),
						Protocol: elbv2types.ProtocolEnum("TLS"),
						MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
							Mode:                             awssdk.String("verify"),
							TrustStoreArn:                    awssdk.String(nlbMTLSTrustStoreARN),
							IgnoreClientCertificateExpiry:    awssdk.Bool(false),
							AdvertiseTrustStoreCaNames:       elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
							ClientCertificateIdentityFilters: getMockClientCertIdentityFilter("www.example.com"),
						},
					},
				},
				desiredDefaultMutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
					Mode:                             awssdk.String("verify"),
					TrustStoreArn:                    awssdk.String(nlbMTLSTrustStoreARN),
					IgnoreClientCertificateExpiry:    awssdk.Bool(false),
					AdvertiseTrustStoreCaNames:       elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
					ClientCertificateIdentityFilters: nil,
				},
			},
			wantEnhanced: false,
			wantLegacy:   false,
		},
		{
			name: "empty desired client certificate identity filters drift from existing filters on the listener",
			args: args{
				lsSpec: elbv2model.ListenerSpec{
					Port:     443,
					Protocol: elbv2model.ProtocolTLS,
				},
				sdkLS: ListenerWithTags{
					Listener: &elbv2types.Listener{
						Port:     awssdk.Int32(443),
						Protocol: elbv2types.ProtocolEnum("TLS"),
						MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
							Mode:                             awssdk.String("verify"),
							TrustStoreArn:                    awssdk.String(nlbMTLSTrustStoreARN),
							IgnoreClientCertificateExpiry:    awssdk.Bool(false),
							AdvertiseTrustStoreCaNames:       elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
							ClientCertificateIdentityFilters: getMockClientCertIdentityFilter("www.example.com"),
						},
					},
				},
				desiredDefaultMutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
					Mode:                             awssdk.String("verify"),
					TrustStoreArn:                    awssdk.String(nlbMTLSTrustStoreARN),
					IgnoreClientCertificateExpiry:    awssdk.Bool(false),
					AdvertiseTrustStoreCaNames:       elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
					ClientCertificateIdentityFilters: []elbv2types.ClientCertificateIdentityFilter{},
				},
			},
			wantEnhanced: true,
			wantLegacy:   true,
		},
		{
			name: "same client certificate identity filters have not drifted",
			args: args{
				lsSpec: elbv2model.ListenerSpec{
					Port:     443,
					Protocol: elbv2model.ProtocolTLS,
				},
				sdkLS: ListenerWithTags{
					Listener: &elbv2types.Listener{
						Port:     awssdk.Int32(443),
						Protocol: elbv2types.ProtocolEnum("TLS"),
						MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
							Mode:                             awssdk.String("verify"),
							TrustStoreArn:                    awssdk.String(nlbMTLSTrustStoreARN),
							IgnoreClientCertificateExpiry:    awssdk.Bool(false),
							AdvertiseTrustStoreCaNames:       elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
							ClientCertificateIdentityFilters: getMockClientCertIdentityFilter("www.example.com"),
						},
					},
				},
				desiredDefaultMutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
					Mode:                             awssdk.String("verify"),
					TrustStoreArn:                    awssdk.String(nlbMTLSTrustStoreARN),
					IgnoreClientCertificateExpiry:    awssdk.Bool(false),
					AdvertiseTrustStoreCaNames:       elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
					ClientCertificateIdentityFilters: getMockClientCertIdentityFilter("www.example.com"),
				},
			},
			wantEnhanced: false,
			wantLegacy:   false,
		},
		{
			name: "different client certificate identity filters have drifted",
			args: args{
				lsSpec: elbv2model.ListenerSpec{
					Port:     443,
					Protocol: elbv2model.ProtocolTLS,
				},
				sdkLS: ListenerWithTags{
					Listener: &elbv2types.Listener{
						Port:     awssdk.Int32(443),
						Protocol: elbv2types.ProtocolEnum("TLS"),
						MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
							Mode:                             awssdk.String("verify"),
							TrustStoreArn:                    awssdk.String(nlbMTLSTrustStoreARN),
							IgnoreClientCertificateExpiry:    awssdk.Bool(false),
							AdvertiseTrustStoreCaNames:       elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
							ClientCertificateIdentityFilters: getMockClientCertIdentityFilter("www.example.com"),
						},
					},
				},
				desiredDefaultMutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
					Mode:                             awssdk.String("verify"),
					TrustStoreArn:                    awssdk.String(nlbMTLSTrustStoreARN),
					IgnoreClientCertificateExpiry:    awssdk.Bool(false),
					AdvertiseTrustStoreCaNames:       elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
					ClientCertificateIdentityFilters: getMockClientCertIdentityFilter("other.example.com"),
				},
			},
			wantEnhanced: true,
			wantLegacy:   true,
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
		name                                   string
		lsSpec                                 elbv2model.ListenerSpec
		desiredDefaultActions                  []elbv2types.Action
		desiredDefaultCerts                    []elbv2types.Certificate
		removeMTLS                             bool
		removeALPN                             bool
		removeClientCertificateIdentityFilters bool
		expected                               elbv2sdk.ModifyListenerInput
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
		{
			name:                                   "remove client certificate identity filters sends an empty list",
			removeClientCertificateIdentityFilters: true,
			lsSpec: elbv2model.ListenerSpec{
				Port:     443,
				Protocol: elbv2model.ProtocolTLS,
				MutualAuthentication: &elbv2model.MutualAuthenticationAttributes{
					Mode:                          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStoreArn:                 awssdk.String(nlbMTLSTrustStoreARN),
					IgnoreClientCertificateExpiry: awssdk.Bool(false),
				},
			},
			expected: elbv2sdk.ModifyListenerInput{
				Port:     awssdk.Int32(443),
				Protocol: elbv2types.ProtocolEnumTls,
				MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
					Mode:                             awssdk.String(string(elbv2model.MutualAuthenticationVerifyMode)),
					TrustStoreArn:                    awssdk.String(nlbMTLSTrustStoreARN),
					IgnoreClientCertificateExpiry:    awssdk.Bool(false),
					AdvertiseTrustStoreCaNames:       elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
					ClientCertificateIdentityFilters: []elbv2types.ClientCertificateIdentityFilter{},
				},
			},
		},
		{
			name: "null identity filters are omitted without the remove flag",
			lsSpec: elbv2model.ListenerSpec{
				Port:     443,
				Protocol: elbv2model.ProtocolTLS,
				MutualAuthentication: &elbv2model.MutualAuthenticationAttributes{
					Mode:                          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStoreArn:                 awssdk.String(nlbMTLSTrustStoreARN),
					IgnoreClientCertificateExpiry: awssdk.Bool(false),
				},
			},
			expected: elbv2sdk.ModifyListenerInput{
				Port:     awssdk.Int32(443),
				Protocol: elbv2types.ProtocolEnumTls,
				MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
					Mode:                          awssdk.String(string(elbv2model.MutualAuthenticationVerifyMode)),
					TrustStoreArn:                 awssdk.String(nlbMTLSTrustStoreARN),
					IgnoreClientCertificateExpiry: awssdk.Bool(false),
					AdvertiseTrustStoreCaNames:    elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := buildSDKModifyListenerInput(tc.lsSpec, tc.desiredDefaultActions, tc.desiredDefaultCerts, tc.removeMTLS, tc.removeALPN, tc.removeClientCertificateIdentityFilters)
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

func Test_areListenerAttributesSupported(t *testing.T) {
	testCases := []struct {
		name     string
		protocol elbv2model.Protocol
		result   bool
	}{
		{
			name:     "protocol HTTP",
			protocol: elbv2model.ProtocolHTTP,
			result:   true,
		},
		{
			name:     "protocol HTTPS",
			protocol: elbv2model.ProtocolHTTPS,
			result:   true,
		},
		{
			name:     "protocol TCP",
			protocol: elbv2model.ProtocolTCP,
			result:   true,
		},
		{
			name:     "protocol UDP",
			protocol: elbv2model.ProtocolUDP,
			result:   true,
		},
		{
			name:     "protocol TCP_UDP",
			protocol: elbv2model.ProtocolTCP_UDP,
			result:   true,
		},
		{
			name:     "protocol QUIC",
			protocol: elbv2model.ProtocolQUIC,
			result:   true,
		},
		{
			name:     "protocol TCP_QUIC",
			protocol: elbv2model.ProtocolTCP_QUIC,
			result:   true,
		},
		{
			name:     "protocol TLS",
			protocol: elbv2model.ProtocolTLS,
			result:   false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := areListenerAttributesSupported(tc.protocol)
			assert.Equal(t, tc.result, res)
		})
	}
}

func Test_translateClientCertificateIdentityFiltersToSdk(t *testing.T) {
	tests := []struct {
		name  string
		input []elbv2model.ClientCertificateIdentityFilter
		want  []elbv2types.ClientCertificateIdentityFilter
	}{
		{
			name:  "nil filters",
			input: nil,
			want:  nil,
		},
		{
			name:  "empty filters",
			input: []elbv2model.ClientCertificateIdentityFilter{},
			want:  []elbv2types.ClientCertificateIdentityFilter{},
		},
		{
			name: "multiple filters",
			input: []elbv2model.ClientCertificateIdentityFilter{
				{
					Action:    elbv2model.ClientCertificateIdentityFilterActionAccept,
					Field:     elbv2model.ClientCertificateIdentityFilterFieldSubjectCn,
					MatchType: elbv2model.MatchTypeExact,
					Value:     "www.example.com",
				},
				{
					Action:    elbv2model.ClientCertificateIdentityFilterActionAccept,
					Field:     elbv2model.ClientCertificateIdentityFilterFieldSanDns,
					MatchType: elbv2model.MatchTypePrefix,
					Value:     "api.",
				},
			},
			want: []elbv2types.ClientCertificateIdentityFilter{
				{
					Action:    elbv2types.ClientCertificateIdentityFilterAction("accept"),
					Field:     elbv2types.ClientCertificateIdentityFilterField("subject-cn"),
					MatchType: elbv2types.ClientCertificateIdentityFilterMatchType("exact"),
					Value:     awssdk.String("www.example.com"),
				},
				{
					Action:    elbv2types.ClientCertificateIdentityFilterAction("accept"),
					Field:     elbv2types.ClientCertificateIdentityFilterField("san-dns"),
					MatchType: elbv2types.ClientCertificateIdentityFilterMatchType("prefix"),
					Value:     awssdk.String("api."),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, translateClientCertificateIdentityFiltersToSdk(tt.input))
		})
	}
}

func Test_isRemoveClientCertificateIdentityFilters(t *testing.T) {
	tests := []struct {
		name    string
		desired *elbv2types.MutualAuthenticationAttributes
		want    bool
	}{
		{
			name:    "no mutual authentication",
			desired: nil,
			want:    false,
		},
		{
			name:    "off mode",
			desired: &elbv2types.MutualAuthenticationAttributes{Mode: awssdk.String("off")},
			want:    false,
		},
		{
			name:    "verify mode with no desired filters",
			desired: &elbv2types.MutualAuthenticationAttributes{Mode: awssdk.String("verify")},
			want:    true,
		},
		{
			name: "verify mode with empty list of filters",
			desired: &elbv2types.MutualAuthenticationAttributes{
				Mode:                             awssdk.String("verify"),
				ClientCertificateIdentityFilters: []elbv2types.ClientCertificateIdentityFilter{},
			},
			want: false,
		},
		{
			name: "verify mode with filters",
			desired: &elbv2types.MutualAuthenticationAttributes{
				Mode:                             awssdk.String("verify"),
				ClientCertificateIdentityFilters: getMockClientCertIdentityFilter("www.example.com"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isRemoveClientCertificateIdentityFilters(tt.desired))
		})
	}
}

func Test_updateSDKListenerWithSettings_clientCertificateIdentityFilters(t *testing.T) {
	tests := []struct {
		name                   string
		enhancedDefaulting     bool
		actualFilters          []elbv2types.ClientCertificateIdentityFilter
		wantModify             bool
		wantModifyFiltersEmpty bool
	}{
		{
			name:                   "enhanced defaulting on clears existing filters",
			enhancedDefaulting:     true,
			actualFilters:          getMockClientCertIdentityFilter("www.example.com"),
			wantModify:             true,
			wantModifyFiltersEmpty: true,
		},
		{
			name:               "enhanced defaulting on does nothing once filters are cleared",
			enhancedDefaulting: true,
			actualFilters:      nil,
			wantModify:         false,
		},
		{
			name:               "enhanced defaulting off leaves existing filters alone",
			enhancedDefaulting: false,
			actualFilters:      getMockClientCertIdentityFilter("www.example.com"),
			wantModify:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)

			stack := core.NewDefaultStack(core.StackID{Namespace: "default", Name: "svc"})
			resLS := elbv2model.NewListener(stack, "443", elbv2model.ListenerSpec{
				LoadBalancerARN: core.LiteralStringToken("lb-arn"),
				Port:            443,
				Protocol:        elbv2model.ProtocolTLS,
				MutualAuthentication: &elbv2model.MutualAuthenticationAttributes{
					Mode:                          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStoreArn:                 awssdk.String(nlbMTLSTrustStoreARN),
					IgnoreClientCertificateExpiry: awssdk.Bool(false),
				},
			})
			sdkLS := ListenerWithTags{
				Listener: &elbv2types.Listener{
					ListenerArn: awssdk.String("listener-arn"),
					Port:        awssdk.Int32(443),
					Protocol:    elbv2types.ProtocolEnumTls,
					MutualAuthentication: &elbv2types.MutualAuthenticationAttributes{
						Mode:                             awssdk.String(string(elbv2model.MutualAuthenticationVerifyMode)),
						TrustStoreArn:                    awssdk.String(nlbMTLSTrustStoreARN),
						IgnoreClientCertificateExpiry:    awssdk.Bool(false),
						AdvertiseTrustStoreCaNames:       elbv2types.AdvertiseTrustStoreCaNamesEnumOff,
						ClientCertificateIdentityFilters: tt.actualFilters,
					},
				},
			}

			if tt.wantModify {
				elbv2Client.EXPECT().ModifyListenerWithContext(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, req *elbv2sdk.ModifyListenerInput) (*elbv2sdk.ModifyListenerOutput, error) {
						require.NotNil(t, req.MutualAuthentication)
						if tt.wantModifyFiltersEmpty {
							assert.NotNil(t, req.MutualAuthentication.ClientCertificateIdentityFilters)
							assert.Empty(t, req.MutualAuthentication.ClientCertificateIdentityFilters)
						}
						return &elbv2sdk.ModifyListenerOutput{}, nil
					})
			}

			m := &defaultListenerManager{
				elbv2Client:                     elbv2Client,
				featureGates:                    config.NewFeatureGates(),
				enhancedDefaultingPolicyEnabled: tt.enhancedDefaulting,
				logger:                          logr.Discard(),
			}
			err := m.updateSDKListenerWithSettings(context.Background(), resLS, sdkLS)
			assert.NoError(t, err)
		})
	}
}
