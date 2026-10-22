package ingress

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
	logdeliverymodel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/logdelivery"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/shared_constants"
)

func Test_defaultModelBuildTask_buildLogDeliveries(t *testing.T) {
	const accessLogs = `[{"logType":"ALB_ACCESS_LOGS","destinationArn":"arn:aws:logs:us-west-2:111122223333:log-group:/alb/my-app","outputFormat":"json"}]`
	member := func(name string, annotationValue *string) ClassifiedIngress {
		ingAnnotations := map[string]string{}
		if annotationValue != nil {
			ingAnnotations["alb.ingress.kubernetes.io/log-delivery"] = *annotationValue
		}
		return ClassifiedIngress{
			Ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{Namespace: "awesome-ns", Name: name, Annotations: ingAnnotations},
			},
		}
	}
	tests := []struct {
		gateDisabled bool
		name         string
		members      []ClassifiedIngress
		wantSpecs    []logdeliverymodel.LogDeliverySpec
		wantErr      string
	}{
		{
			name:    "no ingress sets the annotation",
			members: []ClassifiedIngress{member("ing-0", nil), member("ing-1", nil)},
		},
		{
			name:    "one ingress in the group sets the annotation",
			members: []ClassifiedIngress{member("ing-0", nil), member("ing-1", awssdk.String(accessLogs))},
			wantSpecs: []logdeliverymodel.LogDeliverySpec{{
				LogType:        logdeliverymodel.LogTypeALBAccessLogs,
				DestinationARN: awssdk.String("arn:aws:logs:us-west-2:111122223333:log-group:/alb/my-app"),
				OutputFormat:   awssdk.String("json"),
				Tags:           map[string]string{"team": "web"},
			}},
		},
		{
			name:    "ingresses set the same value",
			members: []ClassifiedIngress{member("ing-0", awssdk.String(accessLogs)), member("ing-1", awssdk.String(accessLogs))},
			wantSpecs: []logdeliverymodel.LogDeliverySpec{{
				LogType:        logdeliverymodel.LogTypeALBAccessLogs,
				DestinationARN: awssdk.String("arn:aws:logs:us-west-2:111122223333:log-group:/alb/my-app"),
				OutputFormat:   awssdk.String("json"),
				Tags:           map[string]string{"team": "web"},
			}},
		},
		{
			name:    "an empty list turns log delivery off",
			members: []ClassifiedIngress{member("ing-0", awssdk.String("[]"))},
		},
		{
			name: "ingresses set different values",
			members: []ClassifiedIngress{
				member("ing-0", awssdk.String(accessLogs)),
				member("ing-1", awssdk.String(`[{"logType":"ALB_ACCESS_LOGS","destinationArn":"arn:aws:s3:::other-bucket"}]`)),
			},
			wantErr: "conflicting log delivery configuration in ingresses awesome-ns/ing-0 and awesome-ns/ing-1",
		},
		{
			name:    "malformed JSON",
			members: []ClassifiedIngress{member("ing-0", awssdk.String(`{"logType":"ALB_ACCESS_LOGS"}`))},
			wantErr: "failed to parse json annotation, alb.ingress.kubernetes.io/log-delivery",
		},
		{
			name:    "NLB log type on an ALB",
			members: []ClassifiedIngress{member("ing-0", awssdk.String(`[{"logType":"NLB_ACCESS_LOGS","destinationArn":"arn:aws:s3:::my-logs"}]`))},
			wantErr: `logType "NLB_ACCESS_LOGS" is not supported for application load balancers`,
		},
		{
			name:         "disabled gate ignores valid configuration",
			gateDisabled: true,
			members:      []ClassifiedIngress{member("ing-0", awssdk.String(accessLogs))},
		},
		{
			name:         "disabled gate ignores malformed JSON",
			gateDisabled: true,
			members:      []ClassifiedIngress{member("ing-0", awssdk.String(`not-json`))},
		},
		{
			name:         "disabled gate ignores invalid configuration",
			gateDisabled: true,
			members:      []ClassifiedIngress{member("ing-0", awssdk.String(`[{"logType":"NLB_ACCESS_LOGS","destinationArn":"not-an-arn"}]`))},
		},
		{
			name:         "disabled gate ignores conflicting group annotations",
			gateDisabled: true,
			members: []ClassifiedIngress{
				member("ing-0", awssdk.String(accessLogs)),
				member("ing-1", awssdk.String(`[]`)),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := core.NewDefaultStack(core.StackID{Namespace: "awesome-ns", Name: "ing-0"})
			lb := elbv2model.NewLoadBalancer(stack, shared_constants.ResourceIDLoadBalancer, elbv2model.LoadBalancerSpec{
				Type: elbv2model.LoadBalancerTypeApplication,
				Tags: map[string]string{"team": "web"},
			})
			featureGates := config.NewFeatureGates()
			if !tt.gateDisabled {
				featureGates.Enable(config.LogDelivery)
			}
			task := &defaultModelBuildTask{
				featureGates:     featureGates,
				ingGroup:         Group{Members: tt.members},
				stack:            stack,
				annotationParser: annotations.NewSuffixAnnotationParser(annotations.AnnotationPrefixIngress),
			}
			err := task.buildLogDeliveries(context.Background(), lb)
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
