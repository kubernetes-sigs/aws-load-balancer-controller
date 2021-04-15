package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/stretchr/testify/assert"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

func Test_isSDKListenerSettingsDrifted(t *testing.T) {
	type args struct {
		lsSpec                elbv2model.ListenerSpec
		sdkLS                 ListenerWithTags
		desiredDefaultActions []*elbv2sdk.Action
		desiredDefaultCerts   []*elbv2sdk.Certificate
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
					Listener: &elbv2sdk.Listener{
						Port:     awssdk.Int64(80),
						Protocol: awssdk.String("HTTPS"),
						Certificates: []*elbv2sdk.Certificate{
							{
								CertificateArn: awssdk.String("cert-arn1"),
								IsDefault:      awssdk.Bool(true),
							},
						},
						DefaultActions: []*elbv2sdk.Action{
							{
								Type: awssdk.String("fixed-response"),
								FixedResponseConfig: &elbv2sdk.FixedResponseActionConfig{
									StatusCode: awssdk.String("404"),
								},
							},
						},
						SslPolicy:  awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
						AlpnPolicy: awssdk.StringSlice([]string{"HTTP2Preferred"}),
					},
				},
				desiredDefaultCerts: []*elbv2sdk.Certificate{
					{
						CertificateArn: awssdk.String("cert-arn1"),
						IsDefault:      awssdk.Bool(true),
					},
				},
				desiredDefaultActions: []*elbv2sdk.Action{
					{
						Type: awssdk.String("fixed-response"),
						FixedResponseConfig: &elbv2sdk.FixedResponseActionConfig{
							StatusCode: awssdk.String("404"),
						},
					},
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
					Listener: &elbv2sdk.Listener{
						Port:     awssdk.Int64(80),
						Protocol: awssdk.String("HTTPS"),
						Certificates: []*elbv2sdk.Certificate{
							{
								CertificateArn: awssdk.String("cert-arn1"),
								IsDefault:      awssdk.Bool(true),
							},
						},
						DefaultActions: []*elbv2sdk.Action{
							{
								Type: awssdk.String("fixed-response"),
								FixedResponseConfig: &elbv2sdk.FixedResponseActionConfig{
									StatusCode: awssdk.String("404"),
								},
							},
						},
						SslPolicy:  awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
						AlpnPolicy: awssdk.StringSlice([]string{"HTTP2Preferred"}),
					},
				},
				desiredDefaultCerts: []*elbv2sdk.Certificate{
					{
						CertificateArn: awssdk.String("cert-arn1"),
						IsDefault:      awssdk.Bool(true),
					},
				},
				desiredDefaultActions: []*elbv2sdk.Action{
					{
						Type: awssdk.String("fixed-response"),
						FixedResponseConfig: &elbv2sdk.FixedResponseActionConfig{
							StatusCode: awssdk.String("404"),
						},
					},
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
					Listener: &elbv2sdk.Listener{
						Port:     awssdk.Int64(80),
						Protocol: awssdk.String("HTTPS"),
						Certificates: []*elbv2sdk.Certificate{
							{
								CertificateArn: awssdk.String("cert-arn1"),
								IsDefault:      awssdk.Bool(true),
							},
						},
						DefaultActions: []*elbv2sdk.Action{
							{
								Type: awssdk.String("forward-config"),
								ForwardConfig: &elbv2sdk.ForwardActionConfig{
									TargetGroups: []*elbv2sdk.TargetGroupTuple{
										{
											TargetGroupArn: awssdk.String("target-group"),
										},
									},
								},
							},
						},
						SslPolicy:  awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
						AlpnPolicy: awssdk.StringSlice([]string{"HTTP2Preferred"}),
					},
				},
				desiredDefaultCerts: []*elbv2sdk.Certificate{
					{
						CertificateArn: awssdk.String("cert-arn1"),
						IsDefault:      awssdk.Bool(true),
					},
				},
				desiredDefaultActions: []*elbv2sdk.Action{
					{
						Type: awssdk.String("forward-config"),
						ForwardConfig: &elbv2sdk.ForwardActionConfig{
							TargetGroups: []*elbv2sdk.TargetGroupTuple{
								{
									TargetGroupArn: awssdk.String("target-group"),
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSDKListenerSettingsDrifted(tt.args.lsSpec, tt.args.sdkLS, tt.args.desiredDefaultActions, tt.args.desiredDefaultCerts)
			assert.Equal(t, tt.want, got)
		})
	}
}
