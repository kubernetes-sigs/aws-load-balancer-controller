package ingress

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/v3/apis/elbv2/v1beta1"
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
	classMember := func(name, className string, deliveries []elbv2api.LogDeliveryConfiguration, annotationValue *string) ClassifiedIngress {
		ing := member(name, annotationValue)
		ing.IngClassConfig.IngClassParams = &elbv2api.IngressClassParams{
			ObjectMeta: metav1.ObjectMeta{Name: className},
			Spec:       elbv2api.IngressClassParamsSpec{LogDelivery: &deliveries},
		}
		return ing
	}
	classConfig := []elbv2api.LogDeliveryConfiguration{{
		LogType:        logdeliverymodel.LogTypeALBAccessLogs,
		DestinationARN: awssdk.String("arn:aws:s3:::class-logs"),
		OutputFormat:   awssdk.String("plain"),
		FieldDelimiter: awssdk.String(","),
		S3DeliveryConfiguration: &elbv2api.LogDeliveryS3Configuration{
			SuffixPath:               awssdk.String("{yyyy}/{MM}/{dd}"),
			EnableHiveCompatiblePath: awssdk.Bool(true),
		},
	}}
	unsetClassMember := classMember("ing-0", "alb-logs", nil, awssdk.String(accessLogs))
	unsetClassMember.IngClassConfig.IngClassParams.Spec.LogDelivery = nil
	classSpecs := []logdeliverymodel.LogDeliverySpec{{
		LogType:        logdeliverymodel.LogTypeALBAccessLogs,
		DestinationARN: awssdk.String("arn:aws:s3:::class-logs"),
		OutputFormat:   awssdk.String("plain"),
		FieldDelimiter: awssdk.String(","),
		S3DeliveryConfiguration: &logdeliverymodel.S3DeliveryConfiguration{
			SuffixPath:               awssdk.String("{yyyy}/{MM}/{dd}"),
			EnableHiveCompatiblePath: awssdk.Bool(true),
		},
		Tags: map[string]string{"team": "web"},
	}}
	tests := []struct {
		gateDisabled bool
		name         string
		members      []ClassifiedIngress
		wantSpecs    []logdeliverymodel.LogDeliverySpec
		wantErr      string
	}{
		{
			name:    "absent class setting falls back to the annotation",
			members: []ClassifiedIngress{unsetClassMember},
			wantSpecs: []logdeliverymodel.LogDeliverySpec{{
				LogType:        logdeliverymodel.LogTypeALBAccessLogs,
				DestinationARN: awssdk.String("arn:aws:logs:us-west-2:111122223333:log-group:/alb/my-app"),
				OutputFormat:   awssdk.String("json"),
				Tags:           map[string]string{"team": "web"},
			}},
		},
		{
			name: "class configuration overrides conflicting group annotations",
			members: []ClassifiedIngress{
				member("ing-0", awssdk.String(accessLogs)),
				classMember("ing-1", "alb-logs", classConfig, awssdk.String("[]")),
			},
			wantSpecs: classSpecs,
		},
		{
			name: "class configuration overrides malformed annotation on another member",
			members: []ClassifiedIngress{
				member("ing-0", awssdk.String("not-json")),
				classMember("ing-1", "alb-logs", classConfig, nil),
			},
			wantSpecs: classSpecs,
		},
		{
			name: "class configuration works without annotations",
			members: []ClassifiedIngress{
				classMember("ing-0", "alb-logs", classConfig, nil),
			},
			wantSpecs: classSpecs,
		},
		{
			name: "empty class configuration disables delivery for the group",
			members: []ClassifiedIngress{
				member("ing-0", awssdk.String(accessLogs)),
				classMember("ing-1", "alb-logs", []elbv2api.LogDeliveryConfiguration{}, awssdk.String("not-json")),
			},
		},
		{
			name: "matching class configurations",
			members: []ClassifiedIngress{
				classMember("ing-0", "alb-logs", classConfig, nil),
				classMember("ing-1", "other-class", classConfig, nil),
			},
			wantSpecs: classSpecs,
		},
		{
			name: "conflicting class configurations",
			members: []ClassifiedIngress{
				classMember("ing-0", "alb-logs", classConfig, nil),
				classMember("ing-1", "other-class", []elbv2api.LogDeliveryConfiguration{}, nil),
			},
			wantErr: "conflicting log delivery configuration in IngressClassParams alb-logs and other-class",
		},
		{
			name: "invalid class configuration is rejected rather than falling back to annotations",
			members: []ClassifiedIngress{
				classMember("ing-0", "alb-logs", []elbv2api.LogDeliveryConfiguration{{LogType: "ALB_ACCESS_LOGS"}}, awssdk.String(accessLogs)),
			},
			wantErr: "exactly one of destinationArn or deliveryDestinationArn must be set",
		},
		{
			name: "class configuration references an existing destination",
			members: []ClassifiedIngress{
				classMember("ing-0", "alb-logs", []elbv2api.LogDeliveryConfiguration{{
					LogType:                logdeliverymodel.LogTypeALBConnectionLogs,
					DeliveryDestinationARN: awssdk.String("arn:aws:logs:us-west-2:111122223333:delivery-destination:shared"),
				}}, nil),
			},
			wantSpecs: []logdeliverymodel.LogDeliverySpec{{
				LogType:                logdeliverymodel.LogTypeALBConnectionLogs,
				DeliveryDestinationARN: awssdk.String("arn:aws:logs:us-west-2:111122223333:delivery-destination:shared"),
				Tags:                   map[string]string{"team": "web"},
			}},
		},
		{
			name:         "disabled gate ignores invalid and conflicting class configurations",
			gateDisabled: true,
			members: []ClassifiedIngress{
				classMember("ing-0", "alb-logs", []elbv2api.LogDeliveryConfiguration{{LogType: "invalid"}}, awssdk.String("not-json")),
				classMember("ing-1", "other-class", classConfig, nil),
			},
		},
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
			require.NoError(t, err)
			var resDeliveries []*logdeliverymodel.LogDelivery
			require.NoError(t, stack.ListResources(&resDeliveries))
			require.Len(t, resDeliveries, len(tt.wantSpecs))
			for i, resDelivery := range resDeliveries {
				want := tt.wantSpecs[i]
				assert.Equal(t, []core.Resource{lb}, resDelivery.Spec.ResourceARN.Dependencies(), "the delivery uses the load balancer's ARN")
				want.ResourceARN = resDelivery.Spec.ResourceARN
				assert.Equal(t, want, resDelivery.Spec)
			}
		})
	}
}
