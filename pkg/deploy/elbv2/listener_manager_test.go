package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/stretchr/testify/assert"
	elbv2model "sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
	"testing"
)

func Test_isSDKListenerSettingsDrifted(t *testing.T) {
	type args struct {
		lsSpec elbv2model.ListenerSpec
		sdkLS  *elbv2sdk.Listener
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
					Port:     80,
					Protocol: elbv2model.ProtocolHTTPS,
					Certificates: []elbv2model.Certificate{
						{
							CertificateARN: awssdk.String("cert-arn1"),
						},
					},
					DefaultActions: []elbv2model.Action{
						{
							Type: elbv2model.ActionTypeFixedResponse,
							FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
								StatusCode: "404",
							},
						},
					},
					SSLPolicy:  awssdk.String("ELBSecurityPolicy-FS-1-2-Res-2019-08"),
					ALPNPolicy: []string{"HTTP2Preferred"},
				},
				sdkLS: &elbv2sdk.Listener{
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
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSDKListenerSettingsDrifted(tt.args.lsSpec, tt.args.sdkLS)
			assert.Equal(t, tt.want, got)
		})
	}
}
