package logdelivery

import (
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
)

const (
	testLogGroupARN            = "arn:aws:logs:us-west-2:111122223333:log-group:/alb/my-app"
	testBucketARN              = "arn:aws:s3:::my-alb-logs"
	testFirehoseARN            = "arn:aws:firehose:us-west-2:111122223333:deliverystream/alb-logs"
	testDeliveryDestinationARN = "arn:aws:logs:us-west-2:444455556666:delivery-destination:central-alb-logs"
)

func Test_BuildLogDeliveries(t *testing.T) {
	tests := []struct {
		name      string
		lbType    elbv2model.LoadBalancerType
		configs   []Config
		tags      map[string]string
		wantSpecs []LogDeliverySpec
		wantErr   string
	}{
		{
			name:   "no configs",
			lbType: elbv2model.LoadBalancerTypeApplication,
		},
		{
			name:   "ALB deliveries to a controller-owned and an existing destination",
			lbType: elbv2model.LoadBalancerTypeApplication,
			configs: []Config{
				{LogType: LogTypeALBAccessLogs, DestinationARN: awssdk.String(testLogGroupARN), OutputFormat: awssdk.String("json")},
				{
					LogType:                 LogTypeALBHealthCheckLogs,
					DestinationARN:          awssdk.String(testBucketARN + "/prefix"),
					OutputFormat:            awssdk.String("parquet"),
					S3DeliveryConfiguration: &S3DeliveryConfiguration{SuffixPath: awssdk.String("{yyyy}/{MM}/{dd}"), EnableHiveCompatiblePath: awssdk.Bool(true)},
				},
				{LogType: LogTypeALBConnectionLogs, DeliveryDestinationARN: awssdk.String(testDeliveryDestinationARN)},
				{LogType: LogTypeALBAccessLogs, DestinationARN: awssdk.String(testFirehoseARN), FieldDelimiter: awssdk.String(",")},
			},
			tags: map[string]string{"team": "web"},
			wantSpecs: []LogDeliverySpec{
				{LogType: LogTypeALBAccessLogs, DestinationARN: awssdk.String(testLogGroupARN), OutputFormat: awssdk.String("json"), Tags: map[string]string{"team": "web"}},
				{
					LogType:                 LogTypeALBHealthCheckLogs,
					DestinationARN:          awssdk.String(testBucketARN + "/prefix"),
					OutputFormat:            awssdk.String("parquet"),
					S3DeliveryConfiguration: &S3DeliveryConfiguration{SuffixPath: awssdk.String("{yyyy}/{MM}/{dd}"), EnableHiveCompatiblePath: awssdk.Bool(true)},
					Tags:                    map[string]string{"team": "web"},
				},
				{LogType: LogTypeALBConnectionLogs, DeliveryDestinationARN: awssdk.String(testDeliveryDestinationARN), Tags: map[string]string{"team": "web"}},
				{LogType: LogTypeALBAccessLogs, DestinationARN: awssdk.String(testFirehoseARN), FieldDelimiter: awssdk.String(","), Tags: map[string]string{"team": "web"}},
			},
		},
		{
			name:   "NLB access logs",
			lbType: elbv2model.LoadBalancerTypeNetwork,
			configs: []Config{
				{LogType: LogTypeNLBAccessLogs, DestinationARN: awssdk.String(testBucketARN)},
			},
			wantSpecs: []LogDeliverySpec{
				{LogType: LogTypeNLBAccessLogs, DestinationARN: awssdk.String(testBucketARN)},
			},
		},
		{
			name:    "ALB log type on an NLB",
			lbType:  elbv2model.LoadBalancerTypeNetwork,
			configs: []Config{{LogType: LogTypeALBAccessLogs, DestinationARN: awssdk.String(testBucketARN)}},
			wantErr: `invalid log delivery configuration at index 0: logType "ALB_ACCESS_LOGS" is not supported for network load balancers, supported values: [NLB_ACCESS_LOGS]`,
		},
		{
			name:    "unknown log type",
			lbType:  elbv2model.LoadBalancerTypeApplication,
			configs: []Config{{LogType: "ACCESS_LOGS", DestinationARN: awssdk.String(testBucketARN)}},
			wantErr: `invalid log delivery configuration at index 0: logType "ACCESS_LOGS" is not supported for application load balancers, supported values: [ALB_ACCESS_LOGS ALB_CONNECTION_LOGS ALB_HEALTH_CHECK_LOGS]`,
		},
		{
			name:    "no destination",
			lbType:  elbv2model.LoadBalancerTypeApplication,
			configs: []Config{{LogType: LogTypeALBAccessLogs}},
			wantErr: "invalid log delivery configuration at index 0: exactly one of destinationArn or deliveryDestinationArn must be set",
		},
		{
			name:   "both destinations",
			lbType: elbv2model.LoadBalancerTypeApplication,
			configs: []Config{{
				LogType:                LogTypeALBAccessLogs,
				DestinationARN:         awssdk.String(testBucketARN),
				DeliveryDestinationARN: awssdk.String(testDeliveryDestinationARN),
			}},
			wantErr: "invalid log delivery configuration at index 0: exactly one of destinationArn or deliveryDestinationArn must be set",
		},
		{
			name:    "delivery destination passed as destinationArn",
			lbType:  elbv2model.LoadBalancerTypeApplication,
			configs: []Config{{LogType: LogTypeALBAccessLogs, DestinationARN: awssdk.String(testDeliveryDestinationARN)}},
			wantErr: `invalid log delivery configuration at index 0: destinationArn "` + testDeliveryDestinationARN + `" is a delivery destination, set it as deliveryDestinationArn instead`,
		},
		{
			name:    "unsupported destination service",
			lbType:  elbv2model.LoadBalancerTypeApplication,
			configs: []Config{{LogType: LogTypeALBAccessLogs, DestinationARN: awssdk.String("arn:aws:sqs:us-west-2:111122223333:queue")}},
			wantErr: `invalid log delivery configuration at index 0: destinationArn "arn:aws:sqs:us-west-2:111122223333:queue" must be a CloudWatch Logs log group, an S3 bucket or a Firehose delivery stream`,
		},
		{
			name:    "malformed destination ARN",
			lbType:  elbv2model.LoadBalancerTypeApplication,
			configs: []Config{{LogType: LogTypeALBAccessLogs, DestinationARN: awssdk.String("my-bucket")}},
			wantErr: `invalid log delivery configuration at index 0: invalid destinationArn "my-bucket": arn: invalid prefix`,
		},
		{
			name:    "deliveryDestinationArn that isn't a delivery destination",
			lbType:  elbv2model.LoadBalancerTypeApplication,
			configs: []Config{{LogType: LogTypeALBAccessLogs, DeliveryDestinationARN: awssdk.String(testLogGroupARN)}},
			wantErr: `invalid log delivery configuration at index 0: deliveryDestinationArn "` + testLogGroupARN + `" must be the ARN of a CloudWatch Logs delivery destination`,
		},
		{
			name:   "output format with an existing delivery destination",
			lbType: elbv2model.LoadBalancerTypeApplication,
			configs: []Config{{
				LogType:                LogTypeALBAccessLogs,
				DeliveryDestinationARN: awssdk.String(testDeliveryDestinationARN),
				OutputFormat:           awssdk.String("json"),
			}},
			wantErr: "invalid log delivery configuration at index 0: outputFormat is set by the delivery destination and cannot be used with deliveryDestinationArn",
		},
		{
			name:    "output format the destination type doesn't accept",
			lbType:  elbv2model.LoadBalancerTypeApplication,
			configs: []Config{{LogType: LogTypeALBAccessLogs, DestinationARN: awssdk.String(testLogGroupARN), OutputFormat: awssdk.String("parquet")}},
			wantErr: `invalid log delivery configuration at index 0: outputFormat "parquet" is not supported for CWL destinations, supported values: [plain json]`,
		},
		{
			name:    "unsupported field delimiter",
			lbType:  elbv2model.LoadBalancerTypeApplication,
			configs: []Config{{LogType: LogTypeALBAccessLogs, DestinationARN: awssdk.String(testLogGroupARN), FieldDelimiter: awssdk.String(";")}},
			wantErr: `invalid log delivery configuration at index 0: fieldDelimiter ";" is not supported, supported values: ["\t" " " ","]`,
		},
		{
			name:   "S3 settings on a log group",
			lbType: elbv2model.LoadBalancerTypeApplication,
			configs: []Config{{
				LogType:                 LogTypeALBAccessLogs,
				DestinationARN:          awssdk.String(testLogGroupARN),
				S3DeliveryConfiguration: &S3DeliveryConfiguration{SuffixPath: awssdk.String("{yyyy}")},
			}},
			wantErr: "invalid log delivery configuration at index 0: s3DeliveryConfiguration can only be used with S3 destinations",
		},
		{
			name:   "duplicate log type and destination",
			lbType: elbv2model.LoadBalancerTypeApplication,
			configs: []Config{
				{LogType: LogTypeALBAccessLogs, DestinationARN: awssdk.String(testLogGroupARN)},
				{LogType: LogTypeALBAccessLogs, DestinationARN: awssdk.String(testLogGroupARN), OutputFormat: awssdk.String("plain")},
			},
			wantErr: "duplicate log delivery configuration for logType ALB_ACCESS_LOGS and destination " + testLogGroupARN,
		},
		{
			name:   "error in a later config",
			lbType: elbv2model.LoadBalancerTypeApplication,
			configs: []Config{
				{LogType: LogTypeALBAccessLogs, DestinationARN: awssdk.String(testLogGroupARN)},
				{LogType: LogTypeNLBAccessLogs, DestinationARN: awssdk.String(testLogGroupARN)},
			},
			wantErr: `invalid log delivery configuration at index 1: logType "NLB_ACCESS_LOGS" is not supported for application load balancers, supported values: [ALB_ACCESS_LOGS ALB_CONNECTION_LOGS ALB_HEALTH_CHECK_LOGS]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := core.NewDefaultStack(core.StackID{Namespace: "awesome-ns", Name: "awesome-ing"})
			lbARN := core.LiteralStringToken("arn:aws:elasticloadbalancing:us-west-2:111122223333:loadbalancer/app/my-lb/0123456789abcdef")
			got, err := BuildLogDeliveries(stack, tt.lbType, lbARN, tt.configs, tt.tags)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				var resources []*LogDelivery
				assert.NoError(t, stack.ListResources(&resources))
				assert.Empty(t, resources, "no resources are added when a config is invalid")
				return
			}
			assert.NoError(t, err)
			assert.Len(t, got, len(tt.wantSpecs))
			for i, delivery := range got {
				want := tt.wantSpecs[i]
				want.ResourceARN = lbARN
				assert.Equal(t, want, delivery.Spec)
				assert.Equal(t, "AWS::Logs::Delivery", delivery.Type())
			}
			var resources []*LogDelivery
			assert.NoError(t, stack.ListResources(&resources))
			assert.Len(t, resources, len(tt.wantSpecs))
		})
	}
}

func Test_BuildLogDeliveries_resourceIDs(t *testing.T) {
	lbARN := core.LiteralStringToken("arn:aws:elasticloadbalancing:us-west-2:111122223333:loadbalancer/app/my-lb/0123456789abcdef")
	access := Config{LogType: LogTypeALBAccessLogs, DestinationARN: awssdk.String(testLogGroupARN)}
	health := Config{LogType: LogTypeALBHealthCheckLogs, DestinationARN: awssdk.String(testLogGroupARN)}

	first, err := BuildLogDeliveries(core.NewDefaultStack(core.StackID{Name: "group"}), elbv2model.LoadBalancerTypeApplication, lbARN, []Config{access, health}, nil)
	assert.NoError(t, err)
	reordered, err := BuildLogDeliveries(core.NewDefaultStack(core.StackID{Name: "group"}), elbv2model.LoadBalancerTypeApplication, lbARN, []Config{health, access}, nil)
	assert.NoError(t, err)

	assert.Equal(t, first[0].ID(), reordered[1].ID(), "IDs don't depend on the order of configs")
	assert.Equal(t, first[1].ID(), reordered[0].ID())
	assert.NotEqual(t, first[0].ID(), first[1].ID())
	assert.Regexp(t, `^ALB_ACCESS_LOGS-[0-9a-f]{16}$`, first[0].ID())
}

func Test_BuildLogDeliveries_tagsAreCopied(t *testing.T) {
	tags := map[string]string{"team": "web"}
	got, err := BuildLogDeliveries(core.NewDefaultStack(core.StackID{Name: "group"}), elbv2model.LoadBalancerTypeApplication,
		core.LiteralStringToken("lb-arn"), []Config{{LogType: LogTypeALBAccessLogs, DestinationARN: awssdk.String(testBucketARN)}}, tags)
	assert.NoError(t, err)
	tags["team"] = "changed"
	assert.Equal(t, map[string]string{"team": "web"}, got[0].Spec.Tags)
}

func Test_DestinationType(t *testing.T) {
	tests := []struct {
		arn     string
		want    string
		wantErr bool
	}{
		{arn: testLogGroupARN, want: DestinationTypeCloudWatchLogs},
		{arn: testLogGroupARN + ":*", want: DestinationTypeCloudWatchLogs},
		{arn: testBucketARN, want: DestinationTypeS3},
		{arn: testBucketARN + "/AWSLogs/prefix", want: DestinationTypeS3},
		{arn: "arn:aws-us-gov:s3:::gov-bucket", want: DestinationTypeS3},
		{arn: "arn:aws-cn:s3:::cn-bucket/prefix", want: DestinationTypeS3},
		{arn: "arn:aws:s3:us-west-2:111122223333:accesspoint/example", wantErr: true},
		{arn: "arn:aws:s3::111122223333:accesspoint/example.mrap", wantErr: true},
		{arn: "arn:aws:s3:us-west-2::my-alb-logs", wantErr: true},
		{arn: "arn:aws:s3::111122223333:my-alb-logs", wantErr: true},
		{arn: "arn:aws:s3:::", wantErr: true},
		{arn: "arn:aws:s3:::/prefix", wantErr: true},
		{arn: "arn:aws:s3:::accesspoint:example", wantErr: true},
		{arn: "arn:aws:s3:::*", wantErr: true},
		{arn: testFirehoseARN, want: DestinationTypeFirehose},
		{arn: "arn:aws:firehose:us-west-2:111122223333:role/x", wantErr: true},
		{arn: "arn:aws:logs:us-west-2:111122223333:destination:x", wantErr: true},
		{arn: testDeliveryDestinationARN, wantErr: true},
		{arn: "not-an-arn", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.arn, func(t *testing.T) {
			got, err := DestinationType(tt.arn)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_BuildLogDeliveries_fieldDelimiterOutputFormat(t *testing.T) {
	for _, format := range []string{"", "plain", "w3c", "json", "parquet", "raw"} {
		t.Run(format, func(t *testing.T) {
			cfg := Config{
				LogType:        LogTypeALBAccessLogs,
				DestinationARN: awssdk.String(testBucketARN),
				FieldDelimiter: awssdk.String(","),
			}
			if format != "" {
				cfg.OutputFormat = awssdk.String(format)
			}
			if format == "raw" {
				cfg.DestinationARN = awssdk.String(testFirehoseARN)
			}
			stack := core.NewDefaultStack(core.StackID{Name: "group"})
			_, err := BuildLogDeliveries(stack, elbv2model.LoadBalancerTypeApplication, core.LiteralStringToken("lb-arn"), []Config{cfg}, nil)
			var resources []*LogDelivery
			assert.NoError(t, stack.ListResources(&resources))
			if format == "json" || format == "parquet" || format == "raw" {
				assert.EqualError(t, err, `invalid log delivery configuration at index 0: fieldDelimiter can only be used with plain or w3c outputFormat, got "`+format+`"`)
				assert.Empty(t, resources)
			} else {
				assert.NoError(t, err)
				assert.Len(t, resources, 1)
			}
		})
	}
	t.Run("existing destination determines the output format", func(t *testing.T) {
		_, err := BuildLogDeliveries(core.NewDefaultStack(core.StackID{Name: "group"}), elbv2model.LoadBalancerTypeApplication,
			core.LiteralStringToken("lb-arn"), []Config{{LogType: LogTypeALBAccessLogs, DeliveryDestinationARN: awssdk.String(testDeliveryDestinationARN), FieldDelimiter: awssdk.String(",")}}, nil)
		assert.NoError(t, err)
	})
}
