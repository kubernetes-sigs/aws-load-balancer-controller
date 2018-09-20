package targetgroup

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	"github.com/stretchr/testify/assert"
)

func TestMerge(t *testing.T) {
	for _, tc := range []struct {
		Source         *Config
		Target         *Config
		Config         *config.Configuration
		ExpectedResult *Config
	}{
		{
			Source: &Config{
				Attributes: albelbv2.TargetGroupAttributes{
					{
						Key:   aws.String("keyA"),
						Value: aws.String("valueA"),
					},
				},
				BackendProtocol:         aws.String(elbv2.ProtocolEnumHttps),
				TargetType:              aws.String("ip"),
				SuccessCodes:            aws.String("404"),
				HealthyThresholdCount:   aws.Int64(8),
				UnhealthyThresholdCount: aws.Int64(9),
			},
			Target: &Config{
				Attributes: albelbv2.TargetGroupAttributes{
					{
						Key:   aws.String("keyB"),
						Value: aws.String("valueB"),
					},
				},
				BackendProtocol:         aws.String(elbv2.ProtocolEnumHttp),
				TargetType:              aws.String("instance"),
				SuccessCodes:            aws.String("500"),
				HealthyThresholdCount:   aws.Int64(10),
				UnhealthyThresholdCount: aws.Int64(11),
			},
			Config: &config.Configuration{
				DefaultTargetType: "instance",
			},
			ExpectedResult: &Config{
				Attributes: albelbv2.TargetGroupAttributes{
					{
						Key:   aws.String("keyA"),
						Value: aws.String("valueA"),
					},
				},
				BackendProtocol:         aws.String(elbv2.ProtocolEnumHttps),
				TargetType:              aws.String("ip"),
				SuccessCodes:            aws.String("404"),
				HealthyThresholdCount:   aws.Int64(8),
				UnhealthyThresholdCount: aws.Int64(9),
			},
		},
		{
			Source: &Config{
				Attributes:              nil,
				BackendProtocol:         aws.String(DefaultBackendProtocol),
				TargetType:              aws.String("instance"),
				SuccessCodes:            aws.String(DefaultSuccessCodes),
				HealthyThresholdCount:   aws.Int64(DefaultHealthyThresholdCount),
				UnhealthyThresholdCount: aws.Int64(DefaultUnhealthyThresholdCount),
			},
			Target: &Config{
				Attributes: albelbv2.TargetGroupAttributes{
					{
						Key:   aws.String("keyB"),
						Value: aws.String("valueB"),
					},
				},
				BackendProtocol:         aws.String(elbv2.ProtocolEnumHttp),
				TargetType:              aws.String("ip"),
				SuccessCodes:            aws.String("500"),
				HealthyThresholdCount:   aws.Int64(10),
				UnhealthyThresholdCount: aws.Int64(11),
			},
			Config: &config.Configuration{
				DefaultTargetType: "instance",
			},
			ExpectedResult: &Config{
				Attributes: albelbv2.TargetGroupAttributes{
					{
						Key:   aws.String("keyB"),
						Value: aws.String("valueB"),
					},
				},
				BackendProtocol:         aws.String(elbv2.ProtocolEnumHttp),
				TargetType:              aws.String("ip"),
				SuccessCodes:            aws.String("500"),
				HealthyThresholdCount:   aws.Int64(10),
				UnhealthyThresholdCount: aws.Int64(11),
			},
		},
	} {
		actualResult := tc.Source.Merge(tc.Target, tc.Config)
		assert.Equal(t, tc.ExpectedResult, actualResult)
	}
}
