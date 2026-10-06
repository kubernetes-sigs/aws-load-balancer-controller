package service

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
	logdeliverymodel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/logdelivery"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/shared_constants"
)

func Test_defaultModelBuildTask_buildLogDeliveries(t *testing.T) {
	tests := []struct {
		gateDisabled bool
		name         string
		annotations  map[string]string
		wantSpecs    []logdeliverymodel.LogDeliverySpec
		wantErr      string
	}{
		{
			name: "annotation not set",
		},
		{
			name: "NLB access logs to S3",
			annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-log-delivery": `[{"logType":"NLB_ACCESS_LOGS","destinationArn":"arn:aws:s3:::my-nlb-logs","outputFormat":"parquet"}]`,
			},
			wantSpecs: []logdeliverymodel.LogDeliverySpec{{
				LogType:        logdeliverymodel.LogTypeNLBAccessLogs,
				DestinationARN: awssdk.String("arn:aws:s3:::my-nlb-logs"),
				OutputFormat:   awssdk.String("parquet"),
				Tags:           map[string]string{"team": "web"},
			}},
		},
		{
			name: "ALB log type on an NLB",
			annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-log-delivery": `[{"logType":"ALB_ACCESS_LOGS","destinationArn":"arn:aws:s3:::my-nlb-logs"}]`,
			},
			wantErr: `logType "ALB_ACCESS_LOGS" is not supported for network load balancers`,
		},
		{
			name: "malformed JSON",
			annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-log-delivery": `NLB_ACCESS_LOGS`,
			},
			wantErr: "failed to parse json annotation, service.beta.kubernetes.io/aws-load-balancer-log-delivery",
		},
		{
			name:         "disabled gate ignores valid configuration",
			gateDisabled: true,
			annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-log-delivery": `[{"logType":"NLB_ACCESS_LOGS","destinationArn":"arn:aws:s3:::my-nlb-logs"}]`,
			},
		},
		{
			name:         "disabled gate ignores malformed JSON",
			gateDisabled: true,
			annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-log-delivery": `not-json`,
			},
		},
		{
			name:         "disabled gate ignores invalid configuration",
			gateDisabled: true,
			annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-log-delivery": `[{"logType":"ALB_ACCESS_LOGS","destinationArn":"not-an-arn"}]`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := core.NewDefaultStack(core.StackID{Namespace: "awesome-ns", Name: "awesome-svc"})
			lb := elbv2model.NewLoadBalancer(stack, shared_constants.ResourceIDLoadBalancer, elbv2model.LoadBalancerSpec{
				Type: elbv2model.LoadBalancerTypeNetwork,
				Tags: map[string]string{"team": "web"},
			})
			featureGates := config.NewFeatureGates()
			if !tt.gateDisabled {
				featureGates.Enable(config.LogDelivery)
			}
			task := &defaultModelBuildTask{
				featureGates: featureGates,
				service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Namespace: "awesome-ns", Name: "awesome-svc", Annotations: tt.annotations},
				},
				stack:            stack,
				loadBalancer:     lb,
				annotationParser: annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io"),
			}
			err := task.buildLogDeliveries(context.Background())
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			var resDeliveries []*logdeliverymodel.LogDelivery
			assert.NoError(t, stack.ListResources(&resDeliveries))
			assert.Len(t, resDeliveries, len(tt.wantSpecs))
			for i, resDelivery := range resDeliveries {
				want := tt.wantSpecs[i]
				assert.Equal(t, []core.Resource{lb}, resDelivery.Spec.ResourceARN.Dependencies(), "the delivery uses the load balancer's ARN")
				want.ResourceARN = resDelivery.Spec.ResourceARN
				assert.Equal(t, want, resDelivery.Spec)
			}
		})
	}
}
