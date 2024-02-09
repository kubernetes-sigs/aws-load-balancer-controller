package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

func Test_isSDKTargetGroupHealthCheckDrifted(t *testing.T) {
	port9090 := intstr.FromInt(9090)
	protocolHTTP := elbv2model.ProtocolHTTP
	type args struct {
		tgSpec elbv2model.TargetGroupSpec
		sdkTG  TargetGroupWithTags
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "healthCheck isn't drifted",
			args: args{
				tgSpec: elbv2model.TargetGroupSpec{
					Name:       "my-tg",
					TargetType: elbv2model.TargetTypeIP,
					Port:       8080,
					Protocol:   elbv2model.ProtocolHTTP,
					HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
						Port:                    &port9090,
						Protocol:                &protocolHTTP,
						Path:                    awssdk.String("/healthcheck"),
						Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
						IntervalSeconds:         awssdk.Int64(10),
						TimeoutSeconds:          awssdk.Int64(5),
						HealthyThresholdCount:   awssdk.Int64(3),
						UnhealthyThresholdCount: awssdk.Int64(2),
					},
				},
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2sdk.TargetGroup{
						HealthCheckEnabled:         awssdk.Bool(true),
						HealthCheckIntervalSeconds: awssdk.Int64(10),
						HealthCheckPath:            awssdk.String("/healthcheck"),
						HealthCheckPort:            awssdk.String("9090"),
						HealthCheckProtocol:        awssdk.String("HTTP"),
						HealthCheckTimeoutSeconds:  awssdk.Int64(5),
						HealthyThresholdCount:      awssdk.Int64(3),
						Matcher:                    &elbv2sdk.Matcher{HttpCode: awssdk.String("200")},
						UnhealthyThresholdCount:    awssdk.Int64(2),
					},
				},
			},
			want: false,
		},
		{
			name: "port changed",
			args: args{
				tgSpec: elbv2model.TargetGroupSpec{
					Name:       "my-tg",
					TargetType: elbv2model.TargetTypeIP,
					Port:       8080,
					Protocol:   elbv2model.ProtocolHTTP,
					HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
						Port:                    &port9090,
						Protocol:                &protocolHTTP,
						Path:                    awssdk.String("/healthcheck"),
						Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
						IntervalSeconds:         awssdk.Int64(10),
						TimeoutSeconds:          awssdk.Int64(5),
						HealthyThresholdCount:   awssdk.Int64(3),
						UnhealthyThresholdCount: awssdk.Int64(2),
					},
				},
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2sdk.TargetGroup{
						HealthCheckEnabled:         awssdk.Bool(true),
						HealthCheckIntervalSeconds: awssdk.Int64(10),
						HealthCheckPath:            awssdk.String("/healthcheck"),
						HealthCheckPort:            awssdk.String("9091"),
						HealthCheckProtocol:        awssdk.String("HTTP"),
						HealthCheckTimeoutSeconds:  awssdk.Int64(5),
						HealthyThresholdCount:      awssdk.Int64(3),
						Matcher:                    &elbv2sdk.Matcher{HttpCode: awssdk.String("200")},
						UnhealthyThresholdCount:    awssdk.Int64(2),
					},
				},
			},
			want: true,
		},
		{
			name: "protocol changed",
			args: args{
				tgSpec: elbv2model.TargetGroupSpec{
					Name:       "my-tg",
					TargetType: elbv2model.TargetTypeIP,
					Port:       8080,
					Protocol:   elbv2model.ProtocolHTTP,
					HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
						Port:                    &port9090,
						Protocol:                &protocolHTTP,
						Path:                    awssdk.String("/healthcheck"),
						Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
						IntervalSeconds:         awssdk.Int64(10),
						TimeoutSeconds:          awssdk.Int64(5),
						HealthyThresholdCount:   awssdk.Int64(3),
						UnhealthyThresholdCount: awssdk.Int64(2),
					},
				},
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2sdk.TargetGroup{
						HealthCheckEnabled:         awssdk.Bool(true),
						HealthCheckIntervalSeconds: awssdk.Int64(10),
						HealthCheckPath:            awssdk.String("/healthcheck"),
						HealthCheckPort:            awssdk.String("9090"),
						HealthCheckProtocol:        awssdk.String("TCP"),
						HealthCheckTimeoutSeconds:  awssdk.Int64(5),
						HealthyThresholdCount:      awssdk.Int64(3),
						Matcher:                    &elbv2sdk.Matcher{HttpCode: awssdk.String("200")},
						UnhealthyThresholdCount:    awssdk.Int64(2),
					},
				},
			},
			want: true,
		},
		{
			name: "HealthCheckPath changed",
			args: args{
				tgSpec: elbv2model.TargetGroupSpec{
					Name:       "my-tg",
					TargetType: elbv2model.TargetTypeIP,
					Port:       8080,
					Protocol:   elbv2model.ProtocolHTTP,
					HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
						Port:                    &port9090,
						Protocol:                &protocolHTTP,
						Path:                    awssdk.String("/healthcheck"),
						Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
						IntervalSeconds:         awssdk.Int64(10),
						TimeoutSeconds:          awssdk.Int64(5),
						HealthyThresholdCount:   awssdk.Int64(3),
						UnhealthyThresholdCount: awssdk.Int64(2),
					},
				},
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2sdk.TargetGroup{
						HealthCheckEnabled:         awssdk.Bool(true),
						HealthCheckIntervalSeconds: awssdk.Int64(10),
						HealthCheckPath:            awssdk.String("/some-other-path"),
						HealthCheckPort:            awssdk.String("9090"),
						HealthCheckProtocol:        awssdk.String("HTTP"),
						HealthCheckTimeoutSeconds:  awssdk.Int64(5),
						HealthyThresholdCount:      awssdk.Int64(3),
						Matcher:                    &elbv2sdk.Matcher{HttpCode: awssdk.String("200")},
						UnhealthyThresholdCount:    awssdk.Int64(2),
					},
				},
			},
			want: true,
		},
		{
			name: "matcher changed",
			args: args{
				tgSpec: elbv2model.TargetGroupSpec{
					Name:       "my-tg",
					TargetType: elbv2model.TargetTypeIP,
					Port:       8080,
					Protocol:   elbv2model.ProtocolHTTP,
					HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
						Port:                    &port9090,
						Protocol:                &protocolHTTP,
						Path:                    awssdk.String("/healthcheck"),
						Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
						IntervalSeconds:         awssdk.Int64(10),
						TimeoutSeconds:          awssdk.Int64(5),
						HealthyThresholdCount:   awssdk.Int64(3),
						UnhealthyThresholdCount: awssdk.Int64(2),
					},
				},
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2sdk.TargetGroup{
						HealthCheckEnabled:         awssdk.Bool(true),
						HealthCheckIntervalSeconds: awssdk.Int64(10),
						HealthCheckPath:            awssdk.String("/healthcheck"),
						HealthCheckPort:            awssdk.String("9090"),
						HealthCheckProtocol:        awssdk.String("HTTP"),
						HealthCheckTimeoutSeconds:  awssdk.Int64(5),
						HealthyThresholdCount:      awssdk.Int64(3),
						Matcher:                    &elbv2sdk.Matcher{HttpCode: awssdk.String("503")},
						UnhealthyThresholdCount:    awssdk.Int64(2),
					},
				},
			},
			want: true,
		},
		{
			name: "matcher GrpcCode changed",
			args: args{
				tgSpec: elbv2model.TargetGroupSpec{
					Name:       "my-tg",
					TargetType: elbv2model.TargetTypeIP,
					Port:       8080,
					Protocol:   elbv2model.ProtocolHTTP,
					HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
						Port:                    &port9090,
						Protocol:                &protocolHTTP,
						Path:                    awssdk.String("/healthcheck"),
						Matcher:                 &elbv2model.HealthCheckMatcher{GRPCCode: awssdk.String("200")},
						IntervalSeconds:         awssdk.Int64(10),
						TimeoutSeconds:          awssdk.Int64(5),
						HealthyThresholdCount:   awssdk.Int64(3),
						UnhealthyThresholdCount: awssdk.Int64(2),
					},
				},
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2sdk.TargetGroup{
						HealthCheckEnabled:         awssdk.Bool(true),
						HealthCheckIntervalSeconds: awssdk.Int64(10),
						HealthCheckPath:            awssdk.String("/healthcheck"),
						HealthCheckPort:            awssdk.String("9090"),
						HealthCheckProtocol:        awssdk.String("HTTP"),
						HealthCheckTimeoutSeconds:  awssdk.Int64(5),
						HealthyThresholdCount:      awssdk.Int64(3),
						Matcher:                    &elbv2sdk.Matcher{GrpcCode: awssdk.String("503")},
						UnhealthyThresholdCount:    awssdk.Int64(2),
					},
				},
			},
			want: true,
		},
		{
			name: "intervalSeconds changed",
			args: args{
				tgSpec: elbv2model.TargetGroupSpec{
					Name:       "my-tg",
					TargetType: elbv2model.TargetTypeIP,
					Port:       8080,
					Protocol:   elbv2model.ProtocolHTTP,
					HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
						Port:                    &port9090,
						Protocol:                &protocolHTTP,
						Path:                    awssdk.String("/healthcheck"),
						Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
						IntervalSeconds:         awssdk.Int64(10),
						TimeoutSeconds:          awssdk.Int64(5),
						HealthyThresholdCount:   awssdk.Int64(3),
						UnhealthyThresholdCount: awssdk.Int64(2),
					},
				},
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2sdk.TargetGroup{
						HealthCheckEnabled:         awssdk.Bool(true),
						HealthCheckIntervalSeconds: awssdk.Int64(11),
						HealthCheckPath:            awssdk.String("/healthcheck"),
						HealthCheckPort:            awssdk.String("9090"),
						HealthCheckProtocol:        awssdk.String("HTTP"),
						HealthCheckTimeoutSeconds:  awssdk.Int64(5),
						HealthyThresholdCount:      awssdk.Int64(3),
						Matcher:                    &elbv2sdk.Matcher{HttpCode: awssdk.String("200")},
						UnhealthyThresholdCount:    awssdk.Int64(2),
					},
				},
			},
			want: true,
		},
		{
			name: "timeoutSeconds changed",
			args: args{
				tgSpec: elbv2model.TargetGroupSpec{
					Name:       "my-tg",
					TargetType: elbv2model.TargetTypeIP,
					Port:       8080,
					Protocol:   elbv2model.ProtocolHTTP,
					HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
						Port:                    &port9090,
						Protocol:                &protocolHTTP,
						Path:                    awssdk.String("/healthcheck"),
						Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
						IntervalSeconds:         awssdk.Int64(10),
						TimeoutSeconds:          awssdk.Int64(5),
						HealthyThresholdCount:   awssdk.Int64(3),
						UnhealthyThresholdCount: awssdk.Int64(2),
					},
				},
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2sdk.TargetGroup{
						HealthCheckEnabled:         awssdk.Bool(true),
						HealthCheckIntervalSeconds: awssdk.Int64(10),
						HealthCheckPath:            awssdk.String("/healthcheck"),
						HealthCheckPort:            awssdk.String("9090"),
						HealthCheckProtocol:        awssdk.String("HTTP"),
						HealthCheckTimeoutSeconds:  awssdk.Int64(6),
						HealthyThresholdCount:      awssdk.Int64(3),
						Matcher:                    &elbv2sdk.Matcher{HttpCode: awssdk.String("200")},
						UnhealthyThresholdCount:    awssdk.Int64(2),
					},
				},
			},
			want: true,
		},
		{
			name: "healthyThresholdCount changed",
			args: args{
				tgSpec: elbv2model.TargetGroupSpec{
					Name:       "my-tg",
					TargetType: elbv2model.TargetTypeIP,
					Port:       8080,
					Protocol:   elbv2model.ProtocolHTTP,
					HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
						Port:                    &port9090,
						Protocol:                &protocolHTTP,
						Path:                    awssdk.String("/healthcheck"),
						Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
						IntervalSeconds:         awssdk.Int64(10),
						TimeoutSeconds:          awssdk.Int64(5),
						HealthyThresholdCount:   awssdk.Int64(3),
						UnhealthyThresholdCount: awssdk.Int64(2),
					},
				},
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2sdk.TargetGroup{
						HealthCheckEnabled:         awssdk.Bool(true),
						HealthCheckIntervalSeconds: awssdk.Int64(10),
						HealthCheckPath:            awssdk.String("/healthcheck"),
						HealthCheckPort:            awssdk.String("9090"),
						HealthCheckProtocol:        awssdk.String("HTTP"),
						HealthCheckTimeoutSeconds:  awssdk.Int64(5),
						HealthyThresholdCount:      awssdk.Int64(4),
						Matcher:                    &elbv2sdk.Matcher{HttpCode: awssdk.String("200")},
						UnhealthyThresholdCount:    awssdk.Int64(2),
					},
				},
			},
			want: true,
		},
		{
			name: "UnhealthyThresholdCount changed",
			args: args{
				tgSpec: elbv2model.TargetGroupSpec{
					Name:       "my-tg",
					TargetType: elbv2model.TargetTypeIP,
					Port:       8080,
					Protocol:   elbv2model.ProtocolHTTP,
					HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
						Port:                    &port9090,
						Protocol:                &protocolHTTP,
						Path:                    awssdk.String("/healthcheck"),
						Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
						IntervalSeconds:         awssdk.Int64(10),
						TimeoutSeconds:          awssdk.Int64(5),
						HealthyThresholdCount:   awssdk.Int64(3),
						UnhealthyThresholdCount: awssdk.Int64(2),
					},
				},
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2sdk.TargetGroup{
						HealthCheckEnabled:         awssdk.Bool(true),
						HealthCheckIntervalSeconds: awssdk.Int64(10),
						HealthCheckPath:            awssdk.String("/healthcheck"),
						HealthCheckPort:            awssdk.String("9090"),
						HealthCheckProtocol:        awssdk.String("HTTP"),
						HealthCheckTimeoutSeconds:  awssdk.Int64(5),
						HealthyThresholdCount:      awssdk.Int64(3),
						Matcher:                    &elbv2sdk.Matcher{HttpCode: awssdk.String("200")},
						UnhealthyThresholdCount:    awssdk.Int64(3),
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSDKTargetGroupHealthCheckDrifted(tt.args.tgSpec, tt.args.sdkTG)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildSDKCreateTargetGroupInput(t *testing.T) {
	port9090 := intstr.FromInt(9090)
	protocolHTTP := elbv2model.ProtocolHTTP
	protocolVersionHTTP2 := elbv2model.ProtocolVersionHTTP2
	ipAddressTypeIPv4 := elbv2model.TargetGroupIPAddressTypeIPv4
	ipAddressTypeIPv6 := elbv2model.TargetGroupIPAddressTypeIPv6
	type args struct {
		tgSpec elbv2model.TargetGroupSpec
	}
	tests := []struct {
		name string
		args args
		want *elbv2sdk.CreateTargetGroupInput
	}{
		{
			name: "standard case",
			args: args{
				tgSpec: elbv2model.TargetGroupSpec{
					Name:          "my-tg",
					TargetType:    elbv2model.TargetTypeIP,
					Port:          8080,
					Protocol:      elbv2model.ProtocolHTTP,
					IPAddressType: &ipAddressTypeIPv4,
					HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
						Port:                    &port9090,
						Protocol:                &protocolHTTP,
						Path:                    awssdk.String("/healthcheck"),
						Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
						IntervalSeconds:         awssdk.Int64(10),
						TimeoutSeconds:          awssdk.Int64(5),
						HealthyThresholdCount:   awssdk.Int64(3),
						UnhealthyThresholdCount: awssdk.Int64(2),
					},
				},
			},
			want: &elbv2sdk.CreateTargetGroupInput{
				HealthCheckEnabled:         awssdk.Bool(true),
				HealthCheckIntervalSeconds: awssdk.Int64(10),
				HealthCheckPath:            awssdk.String("/healthcheck"),
				HealthCheckPort:            awssdk.String("9090"),
				HealthCheckProtocol:        awssdk.String("HTTP"),
				HealthCheckTimeoutSeconds:  awssdk.Int64(5),
				HealthyThresholdCount:      awssdk.Int64(3),
				Matcher:                    &elbv2sdk.Matcher{HttpCode: awssdk.String("200")},
				UnhealthyThresholdCount:    awssdk.Int64(2),
				Name:                       awssdk.String("my-tg"),
				Port:                       awssdk.Int64(8080),
				Protocol:                   awssdk.String("HTTP"),
				TargetType:                 awssdk.String("ip"),
			},
		},
		{
			name: "standard case with protocol version",
			args: args{
				tgSpec: elbv2model.TargetGroupSpec{
					Name:            "my-tg",
					TargetType:      elbv2model.TargetTypeIP,
					Port:            8080,
					Protocol:        elbv2model.ProtocolHTTP,
					ProtocolVersion: &protocolVersionHTTP2,
					HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
						Port:                    &port9090,
						Protocol:                &protocolHTTP,
						Path:                    awssdk.String("/healthcheck"),
						Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
						IntervalSeconds:         awssdk.Int64(10),
						TimeoutSeconds:          awssdk.Int64(5),
						HealthyThresholdCount:   awssdk.Int64(3),
						UnhealthyThresholdCount: awssdk.Int64(2),
					},
				},
			},
			want: &elbv2sdk.CreateTargetGroupInput{
				HealthCheckEnabled:         awssdk.Bool(true),
				HealthCheckIntervalSeconds: awssdk.Int64(10),
				HealthCheckPath:            awssdk.String("/healthcheck"),
				HealthCheckPort:            awssdk.String("9090"),
				HealthCheckProtocol:        awssdk.String("HTTP"),
				HealthCheckTimeoutSeconds:  awssdk.Int64(5),
				HealthyThresholdCount:      awssdk.Int64(3),
				Matcher:                    &elbv2sdk.Matcher{HttpCode: awssdk.String("200")},
				UnhealthyThresholdCount:    awssdk.Int64(2),
				Name:                       awssdk.String("my-tg"),
				Port:                       awssdk.Int64(8080),
				Protocol:                   awssdk.String("HTTP"),
				ProtocolVersion:            awssdk.String("HTTP2"),
				TargetType:                 awssdk.String("ip"),
			},
		},
		{
			name: "standard case ipv6 address",
			args: args{
				tgSpec: elbv2model.TargetGroupSpec{
					Name:          "my-tg",
					TargetType:    elbv2model.TargetTypeIP,
					Port:          8080,
					Protocol:      elbv2model.ProtocolHTTP,
					IPAddressType: &ipAddressTypeIPv6,
					HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
						Port:                    &port9090,
						Protocol:                &protocolHTTP,
						Path:                    awssdk.String("/healthcheck"),
						Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
						IntervalSeconds:         awssdk.Int64(10),
						TimeoutSeconds:          awssdk.Int64(5),
						HealthyThresholdCount:   awssdk.Int64(3),
						UnhealthyThresholdCount: awssdk.Int64(2),
					},
				},
			},
			want: &elbv2sdk.CreateTargetGroupInput{
				HealthCheckEnabled:         awssdk.Bool(true),
				HealthCheckIntervalSeconds: awssdk.Int64(10),
				HealthCheckPath:            awssdk.String("/healthcheck"),
				HealthCheckPort:            awssdk.String("9090"),
				HealthCheckProtocol:        awssdk.String("HTTP"),
				HealthCheckTimeoutSeconds:  awssdk.Int64(5),
				HealthyThresholdCount:      awssdk.Int64(3),
				Matcher:                    &elbv2sdk.Matcher{HttpCode: awssdk.String("200")},
				UnhealthyThresholdCount:    awssdk.Int64(2),
				Name:                       awssdk.String("my-tg"),
				Port:                       awssdk.Int64(8080),
				Protocol:                   awssdk.String("HTTP"),
				TargetType:                 awssdk.String("ip"),
				IpAddressType:              awssdk.String("ipv6"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSDKCreateTargetGroupInput(tt.args.tgSpec)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildSDKModifyTargetGroupInput(t *testing.T) {
	port9090 := intstr.FromInt(9090)
	protocolHTTP := elbv2model.ProtocolHTTP
	type args struct {
		tgSpec elbv2model.TargetGroupSpec
	}
	tests := []struct {
		name string
		args args
		want *elbv2sdk.ModifyTargetGroupInput
	}{
		{
			name: "standard case",
			args: args{
				tgSpec: elbv2model.TargetGroupSpec{
					Name:       "my-tg",
					TargetType: elbv2model.TargetTypeIP,
					Port:       8080,
					Protocol:   elbv2model.ProtocolHTTP,
					HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
						Port:                    &port9090,
						Protocol:                &protocolHTTP,
						Path:                    awssdk.String("/healthcheck"),
						Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
						IntervalSeconds:         awssdk.Int64(10),
						TimeoutSeconds:          awssdk.Int64(5),
						HealthyThresholdCount:   awssdk.Int64(3),
						UnhealthyThresholdCount: awssdk.Int64(2),
					},
				},
			},
			want: &elbv2sdk.ModifyTargetGroupInput{
				HealthCheckEnabled:         awssdk.Bool(true),
				HealthCheckIntervalSeconds: awssdk.Int64(10),
				HealthCheckPath:            awssdk.String("/healthcheck"),
				HealthCheckPort:            awssdk.String("9090"),
				HealthCheckProtocol:        awssdk.String("HTTP"),
				HealthCheckTimeoutSeconds:  awssdk.Int64(5),
				HealthyThresholdCount:      awssdk.Int64(3),
				Matcher:                    &elbv2sdk.Matcher{HttpCode: awssdk.String("200")},
				UnhealthyThresholdCount:    awssdk.Int64(2),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSDKModifyTargetGroupInput(tt.args.tgSpec)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildSDKMatcher(t *testing.T) {
	type args struct {
		modelMatcher    elbv2model.HealthCheckMatcher
		protocolVersion elbv2model.ProtocolVersion
	}
	tests := []struct {
		name string
		args args
		want *elbv2sdk.Matcher
	}{
		{
			name: "standard case",
			args: args{
				modelMatcher: elbv2model.HealthCheckMatcher{
					HTTPCode: awssdk.String("200"),
				},
				protocolVersion: elbv2model.ProtocolVersionHTTP1,
			},
			want: &elbv2sdk.Matcher{
				HttpCode: awssdk.String("200"),
			},
		},
		{
			name: "grpc case",
			args: args{
				modelMatcher: elbv2model.HealthCheckMatcher{
					GRPCCode: awssdk.String("2"),
				},
				protocolVersion: elbv2model.ProtocolVersionGRPC,
			},
			want: &elbv2sdk.Matcher{
				GrpcCode: awssdk.String("2"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSDKMatcher(tt.args.modelMatcher)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildResTargetGroupStatus(t *testing.T) {
	type args struct {
		sdkTG TargetGroupWithTags
	}
	tests := []struct {
		name string
		args args
		want elbv2model.TargetGroupStatus
	}{
		{
			name: "standard case",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2sdk.TargetGroup{
						TargetGroupArn: awssdk.String("my-arn"),
					},
				},
			},
			want: elbv2model.TargetGroupStatus{TargetGroupARN: "my-arn"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildResTargetGroupStatus(tt.args.sdkTG)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_isTargetGroupResourceInUseError(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "is ResourceInUse error",
			args: args{
				err: awserr.New("ResourceInUse", "some message", nil),
			},
			want: true,
		},
		{
			name: "wraps ResourceInUse error",
			args: args{
				err: errors.Wrap(awserr.New("ResourceInUse", "some message", nil), "wrapped message"),
			},
			want: true,
		},
		{
			name: "isn't ResourceInUse error",
			args: args{
				err: errors.New("some other error"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTargetGroupResourceInUseError(tt.args.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
