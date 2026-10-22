package model

import (
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/v3/apis/gateway/v1"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
	logdeliverymodel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/logdelivery"
)

func Test_buildLogDeliveryConfigs(t *testing.T) {
	tests := []struct {
		name      string
		gwConfigs []elbv2gw.LogDeliveryConfiguration
		want      []logdeliverymodel.Config
	}{
		{
			name: "no log delivery",
			want: []logdeliverymodel.Config{},
		},
		{
			name: "every field is carried over",
			gwConfigs: []elbv2gw.LogDeliveryConfiguration{
				{
					LogType:        "ALB_ACCESS_LOGS",
					DestinationArn: awssdk.String("arn:aws:s3:::my-alb-logs"),
					OutputFormat:   awssdk.String("w3c"),
					FieldDelimiter: awssdk.String(","),
					S3DeliveryConfiguration: &elbv2gw.LogDeliveryS3Configuration{
						SuffixPath:               awssdk.String("{yyyy}/{MM}/{dd}"),
						EnableHiveCompatiblePath: awssdk.Bool(true),
					},
				},
				{
					LogType:                "ALB_CONNECTION_LOGS",
					DeliveryDestinationArn: awssdk.String("arn:aws:logs:us-west-2:444455556666:delivery-destination:central"),
				},
			},
			want: []logdeliverymodel.Config{
				{
					LogType:        "ALB_ACCESS_LOGS",
					DestinationARN: awssdk.String("arn:aws:s3:::my-alb-logs"),
					OutputFormat:   awssdk.String("w3c"),
					FieldDelimiter: awssdk.String(","),
					S3DeliveryConfiguration: &logdeliverymodel.S3DeliveryConfiguration{
						SuffixPath:               awssdk.String("{yyyy}/{MM}/{dd}"),
						EnableHiveCompatiblePath: awssdk.Bool(true),
					},
				},
				{
					LogType:                "ALB_CONNECTION_LOGS",
					DeliveryDestinationARN: awssdk.String("arn:aws:logs:us-west-2:444455556666:delivery-destination:central"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, buildLogDeliveryConfigs(tt.gwConfigs))
		})
	}
}

func Test_baseModelBuilder_buildLogDeliveries(t *testing.T) {
	for _, lbType := range []elbv2model.LoadBalancerType{elbv2model.LoadBalancerTypeApplication, elbv2model.LoadBalancerTypeNetwork} {
		t.Run(string(lbType), func(t *testing.T) {
			logType := logdeliverymodel.LogTypeALBAccessLogs
			if lbType == elbv2model.LoadBalancerTypeNetwork {
				logType = logdeliverymodel.LogTypeNLBAccessLogs
			}
			tests := []struct {
				name        string
				gateEnabled bool
				configs     []elbv2gw.LogDeliveryConfiguration
				wantCount   int
				wantErr     string
			}{
				{
					name:        "enabled gate builds valid configuration",
					gateEnabled: true,
					configs:     []elbv2gw.LogDeliveryConfiguration{{LogType: logType, DestinationArn: awssdk.String("arn:aws:s3:::my-lb-logs")}},
					wantCount:   1,
				},
				{
					name:        "enabled gate rejects invalid configuration",
					gateEnabled: true,
					configs:     []elbv2gw.LogDeliveryConfiguration{{LogType: logType, DestinationArn: awssdk.String("not-an-arn")}},
					wantErr:     `invalid destinationArn "not-an-arn"`,
				},
				{
					name:    "disabled gate ignores valid configuration",
					configs: []elbv2gw.LogDeliveryConfiguration{{LogType: logType, DestinationArn: awssdk.String("arn:aws:s3:::my-lb-logs")}},
				},
				{
					name:    "disabled gate ignores invalid configuration",
					configs: []elbv2gw.LogDeliveryConfiguration{{LogType: "UNKNOWN", DestinationArn: awssdk.String("not-an-arn")}},
				},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					stack := core.NewDefaultStack(core.StackID{Namespace: "awesome-ns", Name: "awesome-gw"})
					lb := elbv2model.NewLoadBalancer(stack, "LoadBalancer", elbv2model.LoadBalancerSpec{Type: lbType})
					featureGates := config.NewFeatureGates()
					if tt.gateEnabled {
						featureGates.Enable(config.LogDelivery)
					}
					builder := &baseModelBuilder{featureGates: featureGates, loadBalancerType: lbType}
					err := builder.buildLogDeliveries(stack, lb, tt.configs)
					if tt.wantErr != "" {
						assert.ErrorContains(t, err, tt.wantErr)
					} else {
						assert.NoError(t, err)
					}
					var deliveries []*logdeliverymodel.LogDelivery
					assert.NoError(t, stack.ListResources(&deliveries))
					assert.Len(t, deliveries, tt.wantCount)
					if len(deliveries) == 1 {
						assert.Equal(t, logType, deliveries[0].Spec.LogType)
						assert.Equal(t, []core.Resource{lb}, deliveries[0].Spec.ResourceARN.Dependencies())
					}
				})
			}
		})
	}
}
