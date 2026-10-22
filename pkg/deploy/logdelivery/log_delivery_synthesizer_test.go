package logdelivery

import (
	"context"
	"errors"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	cloudwatchlogssdk "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
	logdeliverymodel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/logdelivery"
)

const (
	testClusterName            = "my-cluster"
	testLBARN                  = "arn:aws:elasticloadbalancing:us-west-2:111122223333:loadbalancer/app/k8s-awesomen-awesomei-0123456789/1111111111111111"
	testReplacedLBARN          = "arn:aws:elasticloadbalancing:us-west-2:111122223333:loadbalancer/app/k8s-awesomen-awesomei-0123456789/2222222222222222"
	testLogGroupARN            = "arn:aws:logs:us-west-2:111122223333:log-group:/alb/my-app"
	testBucketARN              = "arn:aws:s3:::my-alb-logs"
	testExternalDestinationARN = "arn:aws:logs:us-west-2:444455556666:delivery-destination:central-alb-logs"
	testDeliveryDestinationARN = "arn:aws:logs:us-west-2:111122223333:delivery-destination:"
)

type synthesizerTestEnv struct {
	stackID            core.StackID
	prefix             string
	accessSource       string
	stackTags          map[string]string
	userTags           map[string]string
	jsonDestination    string
	jsonDestinationARN string
	plainDestination   string
	bucketDestination  string
}

func newSynthesizerTestEnv() synthesizerTestEnv {
	stackID := core.StackID{Namespace: "awesome-ns", Name: "awesome-ing"}
	prefix := namePrefix(testClusterName, "ingress.k8s.aws/resource", stackID)
	jsonDestination := deliveryDestinationName(prefix, logdeliverymodel.DestinationTypeCloudWatchLogs, testLogGroupARN, "json")
	return synthesizerTestEnv{
		stackID:      stackID,
		prefix:       prefix,
		accessSource: deliverySourceName(prefix, logdeliverymodel.LogTypeALBAccessLogs),
		stackTags: map[string]string{
			"elbv2.k8s.aws/cluster": testClusterName,
			"ingress.k8s.aws/stack": "awesome-ns/awesome-ing",
		},
		userTags:           map[string]string{"team": "web"},
		jsonDestination:    jsonDestination,
		jsonDestinationARN: testDeliveryDestinationARN + jsonDestination,
		plainDestination:   deliveryDestinationName(prefix, logdeliverymodel.DestinationTypeCloudWatchLogs, testLogGroupARN, "plain"),
		bucketDestination:  deliveryDestinationName(prefix, logdeliverymodel.DestinationTypeS3, testBucketARN, ""),
	}
}

func (e synthesizerTestEnv) tags(extra ...map[string]string) map[string]string {
	tags := map[string]string{}
	for _, m := range append([]map[string]string{e.stackTags, e.userTags}, extra...) {
		for k, v := range m {
			tags[k] = v
		}
	}
	return tags
}

func (e synthesizerTestEnv) ownedSource(resourceARN string) cwltypes.DeliverySource {
	return cwltypes.DeliverySource{
		Name:         awssdk.String(e.accessSource),
		LogType:      awssdk.String(logdeliverymodel.LogTypeALBAccessLogs),
		ResourceArns: []string{resourceARN},
		Service:      awssdk.String("elasticloadbalancing"),
	}
}

func (e synthesizerTestEnv) ownedDestination(name string, outputFormat cwltypes.OutputFormat) cwltypes.DeliveryDestination {
	return cwltypes.DeliveryDestination{
		Name:         awssdk.String(name),
		Arn:          awssdk.String(testDeliveryDestinationARN + name),
		OutputFormat: outputFormat,
		DeliveryDestinationConfiguration: &cwltypes.DeliveryDestinationConfiguration{
			DestinationResourceArn: awssdk.String(testLogGroupARN),
		},
	}
}

var unrelatedSource = cwltypes.DeliverySource{
	Name:         awssdk.String("cloudfront-standard-logs"),
	LogType:      awssdk.String("ACCESS_LOGS"),
	ResourceArns: []string{"arn:aws:cloudfront::111122223333:distribution/E1"},
}

var unrelatedDelivery = cwltypes.Delivery{
	Id:                     awssdk.String("unrelated"),
	DeliverySourceName:     unrelatedSource.Name,
	DeliveryDestinationArn: awssdk.String(testDeliveryDestinationARN + "someone-else"),
}

func Test_logDeliverySynthesizer_Synthesize(t *testing.T) {
	env := newSynthesizerTestEnv()
	notFound := &cwltypes.ResourceNotFoundException{Message: awssdk.String("not found")}

	tests := []struct {
		name    string
		configs []logdeliverymodel.Config
		setup   func(m *services.MockCloudWatchLogs, resourceIDs []string)
		wantErr string
	}{
		{
			name: "stack without log delivery and nothing to clean up only lists delivery sources",
			setup: func(m *services.MockCloudWatchLogs, _ []string) {
				m.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliverySource{unrelatedSource}, nil)
			},
		},
		{
			name: "creates the source first, then the destination, then the deliveries",
			configs: []logdeliverymodel.Config{
				{LogType: logdeliverymodel.LogTypeALBAccessLogs, DestinationARN: awssdk.String(testLogGroupARN), OutputFormat: awssdk.String("json")},
				{LogType: logdeliverymodel.LogTypeALBAccessLogs, DeliveryDestinationARN: awssdk.String(testExternalDestinationARN)},
			},
			setup: func(m *services.MockCloudWatchLogs, resourceIDs []string) {
				m.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliverySource{unrelatedSource}, nil)
				m.EXPECT().DescribeDeliveryDestinationsAsList(gomock.Any(), gomock.Any()).Return(nil, nil)
				m.EXPECT().DescribeDeliveriesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.Delivery{unrelatedDelivery}, nil)
				putSource := m.EXPECT().PutDeliverySourceWithContext(gomock.Any(), &cloudwatchlogssdk.PutDeliverySourceInput{
					Name:        awssdk.String(env.accessSource),
					ResourceArn: awssdk.String(testLBARN),
					LogType:     awssdk.String(logdeliverymodel.LogTypeALBAccessLogs),
					Tags:        env.tags(),
				}).Return(&cloudwatchlogssdk.PutDeliverySourceOutput{}, nil)
				putDestination := m.EXPECT().PutDeliveryDestinationWithContext(gomock.Any(), &cloudwatchlogssdk.PutDeliveryDestinationInput{
					Name: awssdk.String(env.jsonDestination),
					DeliveryDestinationConfiguration: &cwltypes.DeliveryDestinationConfiguration{
						DestinationResourceArn: awssdk.String(testLogGroupARN),
					},
					OutputFormat: cwltypes.OutputFormatJson,
					Tags:         env.tags(),
				}).Return(&cloudwatchlogssdk.PutDeliveryDestinationOutput{
					DeliveryDestination: &cwltypes.DeliveryDestination{Arn: awssdk.String(env.jsonDestinationARN)},
				}, nil).After(putSource)
				m.EXPECT().CreateDeliveryWithContext(gomock.Any(), &cloudwatchlogssdk.CreateDeliveryInput{
					DeliverySourceName:     awssdk.String(env.accessSource),
					DeliveryDestinationArn: awssdk.String(env.jsonDestinationARN),
					Tags:                   env.tags(map[string]string{"ingress.k8s.aws/resource": resourceIDs[0]}),
				}).Return(&cloudwatchlogssdk.CreateDeliveryOutput{Delivery: &cwltypes.Delivery{Id: awssdk.String("d1")}}, nil).After(putDestination)
				m.EXPECT().CreateDeliveryWithContext(gomock.Any(), &cloudwatchlogssdk.CreateDeliveryInput{
					DeliverySourceName:     awssdk.String(env.accessSource),
					DeliveryDestinationArn: awssdk.String(testExternalDestinationARN),
					Tags:                   env.tags(map[string]string{"ingress.k8s.aws/resource": resourceIDs[1]}),
				}).Return(&cloudwatchlogssdk.CreateDeliveryOutput{Delivery: &cwltypes.Delivery{Id: awssdk.String("d2")}}, nil).After(putDestination)
			},
		},
		{
			name: "makes no changes when everything matches",
			configs: []logdeliverymodel.Config{
				{LogType: logdeliverymodel.LogTypeALBAccessLogs, DestinationARN: awssdk.String(testLogGroupARN), OutputFormat: awssdk.String("json")},
				{LogType: logdeliverymodel.LogTypeALBAccessLogs, DeliveryDestinationARN: awssdk.String(testExternalDestinationARN)},
			},
			setup: func(m *services.MockCloudWatchLogs, _ []string) {
				m.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliverySource{unrelatedSource, env.ownedSource(testLBARN)}, nil)
				m.EXPECT().DescribeDeliveryDestinationsAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliveryDestination{
					env.ownedDestination(env.jsonDestination, cwltypes.OutputFormatJson),
				}, nil)
				m.EXPECT().DescribeDeliveriesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.Delivery{
					unrelatedDelivery,
					{Id: awssdk.String("d1"), DeliverySourceName: awssdk.String(env.accessSource), DeliveryDestinationArn: awssdk.String(env.jsonDestinationARN)},
					{Id: awssdk.String("d2"), DeliverySourceName: awssdk.String(env.accessSource), DeliveryDestinationArn: awssdk.String(testExternalDestinationARN)},
				}, nil)
			},
		},
		{
			name: "removes deliveries, then destinations, then sources when the configuration is removed",
			setup: func(m *services.MockCloudWatchLogs, _ []string) {
				m.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliverySource{env.ownedSource(testLBARN), unrelatedSource}, nil)
				m.EXPECT().DescribeDeliveryDestinationsAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliveryDestination{
					env.ownedDestination(env.jsonDestination, cwltypes.OutputFormatJson),
					{Name: awssdk.String("someone-else"), Arn: awssdk.String(testDeliveryDestinationARN + "someone-else")},
				}, nil)
				m.EXPECT().DescribeDeliveriesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.Delivery{
					{Id: awssdk.String("d1"), DeliverySourceName: awssdk.String(env.accessSource), DeliveryDestinationArn: awssdk.String(env.jsonDestinationARN)},
					unrelatedDelivery,
					{Id: awssdk.String("d2"), DeliverySourceName: awssdk.String(env.accessSource), DeliveryDestinationArn: awssdk.String(testExternalDestinationARN)},
				}, nil)
				gomock.InOrder(
					m.EXPECT().DeleteDeliveryWithContext(gomock.Any(), &cloudwatchlogssdk.DeleteDeliveryInput{Id: awssdk.String("d1")}).Return(&cloudwatchlogssdk.DeleteDeliveryOutput{}, nil),
					m.EXPECT().DeleteDeliveryWithContext(gomock.Any(), &cloudwatchlogssdk.DeleteDeliveryInput{Id: awssdk.String("d2")}).Return(&cloudwatchlogssdk.DeleteDeliveryOutput{}, nil),
					m.EXPECT().DeleteDeliveryDestinationWithContext(gomock.Any(), &cloudwatchlogssdk.DeleteDeliveryDestinationInput{Name: awssdk.String(env.jsonDestination)}).Return(&cloudwatchlogssdk.DeleteDeliveryDestinationOutput{}, nil),
					m.EXPECT().DeleteDeliverySourceWithContext(gomock.Any(), &cloudwatchlogssdk.DeleteDeliverySourceInput{Name: awssdk.String(env.accessSource)}).Return(&cloudwatchlogssdk.DeleteDeliverySourceOutput{}, nil),
				)
			},
		},
		{
			name: "leaves alone resources with the stack's prefix that another stack tagged",
			setup: func(m *services.MockCloudWatchLogs, _ []string) {
				foreignSource := env.ownedSource(testLBARN)
				foreignSource.Tags = map[string]string{"elbv2.k8s.aws/cluster": testClusterName, "ingress.k8s.aws/stack": "other-group"}
				m.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliverySource{foreignSource}, nil)
			},
		},
		{
			name: "treats tagged resources of its own stack as owned",
			setup: func(m *services.MockCloudWatchLogs, _ []string) {
				taggedSource := env.ownedSource(testLBARN)
				taggedSource.Tags = env.tags()
				m.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliverySource{taggedSource}, nil)
				m.EXPECT().DescribeDeliveryDestinationsAsList(gomock.Any(), gomock.Any()).Return(nil, nil)
				m.EXPECT().DescribeDeliveriesAsList(gomock.Any(), gomock.Any()).Return(nil, nil)
				m.EXPECT().DeleteDeliverySourceWithContext(gomock.Any(), &cloudwatchlogssdk.DeleteDeliverySourceInput{Name: awssdk.String(env.accessSource)}).Return(&cloudwatchlogssdk.DeleteDeliverySourceOutput{}, nil)
			},
		},
		{
			name: "ignores resources that are already gone",
			setup: func(m *services.MockCloudWatchLogs, _ []string) {
				m.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliverySource{env.ownedSource(testLBARN)}, nil)
				m.EXPECT().DescribeDeliveryDestinationsAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliveryDestination{
					env.ownedDestination(env.jsonDestination, cwltypes.OutputFormatJson),
				}, nil)
				m.EXPECT().DescribeDeliveriesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.Delivery{
					{Id: awssdk.String("d1"), DeliverySourceName: awssdk.String(env.accessSource), DeliveryDestinationArn: awssdk.String(env.jsonDestinationARN)},
				}, nil)
				m.EXPECT().DeleteDeliveryWithContext(gomock.Any(), gomock.Any()).Return(nil, notFound)
				m.EXPECT().DeleteDeliveryDestinationWithContext(gomock.Any(), gomock.Any()).Return(nil, notFound)
				m.EXPECT().DeleteDeliverySourceWithContext(gomock.Any(), gomock.Any()).Return(nil, notFound)
			},
		},
		{
			name: "replaces the source when the load balancer was replaced",
			configs: []logdeliverymodel.Config{
				{LogType: logdeliverymodel.LogTypeALBAccessLogs, DeliveryDestinationARN: awssdk.String(testExternalDestinationARN)},
			},
			setup: func(m *services.MockCloudWatchLogs, resourceIDs []string) {
				m.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliverySource{env.ownedSource(testReplacedLBARN)}, nil)
				m.EXPECT().DescribeDeliveryDestinationsAsList(gomock.Any(), gomock.Any()).Return(nil, nil)
				m.EXPECT().DescribeDeliveriesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.Delivery{
					{Id: awssdk.String("d2"), DeliverySourceName: awssdk.String(env.accessSource), DeliveryDestinationArn: awssdk.String(testExternalDestinationARN)},
				}, nil)
				gomock.InOrder(
					m.EXPECT().DeleteDeliveryWithContext(gomock.Any(), &cloudwatchlogssdk.DeleteDeliveryInput{Id: awssdk.String("d2")}).Return(&cloudwatchlogssdk.DeleteDeliveryOutput{}, nil),
					m.EXPECT().DeleteDeliverySourceWithContext(gomock.Any(), &cloudwatchlogssdk.DeleteDeliverySourceInput{Name: awssdk.String(env.accessSource)}).Return(&cloudwatchlogssdk.DeleteDeliverySourceOutput{}, nil),
					m.EXPECT().PutDeliverySourceWithContext(gomock.Any(), &cloudwatchlogssdk.PutDeliverySourceInput{
						Name:        awssdk.String(env.accessSource),
						ResourceArn: awssdk.String(testLBARN),
						LogType:     awssdk.String(logdeliverymodel.LogTypeALBAccessLogs),
						Tags:        env.tags(),
					}).Return(&cloudwatchlogssdk.PutDeliverySourceOutput{}, nil),
					m.EXPECT().CreateDeliveryWithContext(gomock.Any(), &cloudwatchlogssdk.CreateDeliveryInput{
						DeliverySourceName:     awssdk.String(env.accessSource),
						DeliveryDestinationArn: awssdk.String(testExternalDestinationARN),
						Tags:                   env.tags(map[string]string{"ingress.k8s.aws/resource": resourceIDs[0]}),
					}).Return(&cloudwatchlogssdk.CreateDeliveryOutput{}, nil),
				)
			},
		},
		{
			name: "moves the delivery to a new destination when the output format changes",
			configs: []logdeliverymodel.Config{
				{LogType: logdeliverymodel.LogTypeALBAccessLogs, DestinationARN: awssdk.String(testLogGroupARN), OutputFormat: awssdk.String("json")},
			},
			setup: func(m *services.MockCloudWatchLogs, resourceIDs []string) {
				m.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliverySource{env.ownedSource(testLBARN)}, nil)
				m.EXPECT().DescribeDeliveryDestinationsAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliveryDestination{
					env.ownedDestination(env.plainDestination, cwltypes.OutputFormatPlain),
				}, nil)
				m.EXPECT().DescribeDeliveriesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.Delivery{
					{Id: awssdk.String("old"), DeliverySourceName: awssdk.String(env.accessSource), DeliveryDestinationArn: awssdk.String(testDeliveryDestinationARN + env.plainDestination)},
				}, nil)
				gomock.InOrder(
					m.EXPECT().DeleteDeliveryWithContext(gomock.Any(), &cloudwatchlogssdk.DeleteDeliveryInput{Id: awssdk.String("old")}).Return(&cloudwatchlogssdk.DeleteDeliveryOutput{}, nil),
					m.EXPECT().PutDeliveryDestinationWithContext(gomock.Any(), gomock.Any()).Return(&cloudwatchlogssdk.PutDeliveryDestinationOutput{
						DeliveryDestination: &cwltypes.DeliveryDestination{Arn: awssdk.String(env.jsonDestinationARN)},
					}, nil),
					m.EXPECT().CreateDeliveryWithContext(gomock.Any(), &cloudwatchlogssdk.CreateDeliveryInput{
						DeliverySourceName:     awssdk.String(env.accessSource),
						DeliveryDestinationArn: awssdk.String(env.jsonDestinationARN),
						Tags:                   env.tags(map[string]string{"ingress.k8s.aws/resource": resourceIDs[0]}),
					}).Return(&cloudwatchlogssdk.CreateDeliveryOutput{}, nil),
					m.EXPECT().DeleteDeliveryDestinationWithContext(gomock.Any(), &cloudwatchlogssdk.DeleteDeliveryDestinationInput{Name: awssdk.String(env.plainDestination)}).Return(&cloudwatchlogssdk.DeleteDeliveryDestinationOutput{}, nil),
				)
			},
		},
		{
			name: "updates the delivery configuration in place",
			configs: []logdeliverymodel.Config{
				{
					LogType:                 logdeliverymodel.LogTypeALBAccessLogs,
					DestinationARN:          awssdk.String(testBucketARN),
					FieldDelimiter:          awssdk.String(","),
					S3DeliveryConfiguration: &logdeliverymodel.S3DeliveryConfiguration{SuffixPath: awssdk.String("{yyyy}/{MM}/{dd}")},
				},
			},
			setup: func(m *services.MockCloudWatchLogs, _ []string) {
				m.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliverySource{env.ownedSource(testLBARN)}, nil)
				m.EXPECT().DescribeDeliveryDestinationsAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliveryDestination{{
					Name: awssdk.String(env.bucketDestination),
					Arn:  awssdk.String(testDeliveryDestinationARN + env.bucketDestination),
					DeliveryDestinationConfiguration: &cwltypes.DeliveryDestinationConfiguration{
						DestinationResourceArn: awssdk.String(testBucketARN),
					},
					OutputFormat: cwltypes.OutputFormatPlain,
				}}, nil)
				m.EXPECT().DescribeDeliveriesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.Delivery{{
					Id:                     awssdk.String("d1"),
					DeliverySourceName:     awssdk.String(env.accessSource),
					DeliveryDestinationArn: awssdk.String(testDeliveryDestinationARN + env.bucketDestination),
					FieldDelimiter:         awssdk.String(""),
					S3DeliveryConfiguration: &cwltypes.S3DeliveryConfiguration{
						SuffixPath:               awssdk.String("{region}/{yyyy}/{MM}/{dd}/"),
						EnableHiveCompatiblePath: awssdk.Bool(false),
					},
				}}, nil)
				m.EXPECT().UpdateDeliveryConfigurationWithContext(gomock.Any(), &cloudwatchlogssdk.UpdateDeliveryConfigurationInput{
					Id:             awssdk.String("d1"),
					FieldDelimiter: awssdk.String(","),
					S3DeliveryConfiguration: &cwltypes.S3DeliveryConfiguration{
						SuffixPath:               awssdk.String("{yyyy}/{MM}/{dd}"),
						EnableHiveCompatiblePath: awssdk.Bool(false),
					},
				}).Return(&cloudwatchlogssdk.UpdateDeliveryConfigurationOutput{}, nil)
			},
		},
		{
			name: "explains a conflicting delivery source the controller doesn't manage",
			configs: []logdeliverymodel.Config{
				{LogType: logdeliverymodel.LogTypeALBAccessLogs, DeliveryDestinationARN: awssdk.String(testExternalDestinationARN)},
			},
			setup: func(m *services.MockCloudWatchLogs, _ []string) {
				m.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return(nil, nil)
				m.EXPECT().DescribeDeliveryDestinationsAsList(gomock.Any(), gomock.Any()).Return(nil, nil)
				m.EXPECT().DescribeDeliveriesAsList(gomock.Any(), gomock.Any()).Return(nil, nil)
				m.EXPECT().PutDeliverySourceWithContext(gomock.Any(), gomock.Any()).Return(nil, &cwltypes.ConflictException{Message: awssdk.String("conflict")})
			},
			wantErr: "the load balancer may already have a delivery source for ALB_ACCESS_LOGS that the controller doesn't manage",
		},
		{
			name: "returns list errors",
			configs: []logdeliverymodel.Config{
				{LogType: logdeliverymodel.LogTypeALBAccessLogs, DeliveryDestinationARN: awssdk.String(testExternalDestinationARN)},
			},
			setup: func(m *services.MockCloudWatchLogs, _ []string) {
				m.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))
			},
			wantErr: "failed to list delivery sources: access denied",
		},
		{
			name: "returns delete errors other than not found",
			setup: func(m *services.MockCloudWatchLogs, _ []string) {
				m.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliverySource{env.ownedSource(testLBARN)}, nil)
				m.EXPECT().DescribeDeliveryDestinationsAsList(gomock.Any(), gomock.Any()).Return(nil, nil)
				m.EXPECT().DescribeDeliveriesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.Delivery{
					{Id: awssdk.String("d1"), DeliverySourceName: awssdk.String(env.accessSource), DeliveryDestinationArn: awssdk.String(testExternalDestinationARN)},
				}, nil)
				m.EXPECT().DeleteDeliveryWithContext(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))
			},
			wantErr: "failed to delete delivery d1: throttled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			stack := core.NewDefaultStack(env.stackID)
			resDeliveries, err := logdeliverymodel.BuildLogDeliveries(stack, elbv2model.LoadBalancerTypeApplication, core.LiteralStringToken(testLBARN), tt.configs, env.userTags)
			assert.NoError(t, err)
			var resourceIDs []string
			for _, resDelivery := range resDeliveries {
				resourceIDs = append(resourceIDs, resDelivery.ID())
			}

			cwlClient := services.NewMockCloudWatchLogs(ctrl)
			tt.setup(cwlClient, resourceIDs)

			trackingProvider := tracking.NewDefaultProvider("ingress.k8s.aws", testClusterName)
			synthesizer := NewLogDeliverySynthesizer(cwlClient, trackingProvider, testClusterName, logr.Discard(), stack)
			err = synthesizer.Synthesize(context.Background())
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.NoError(t, synthesizer.PostSynthesize(context.Background()))
		})
	}
}

func Test_logDeliverySynthesizer_preservesOtherControllersResources(t *testing.T) {
	controllers := []string{"ingress.k8s.aws", "service.k8s.aws", "gateway.k8s.aws.alb", "gateway.k8s.aws.nlb"}
	for _, owner := range controllers {
		for _, other := range controllers {
			if owner == other {
				continue
			}
			for _, tagged := range []bool{false, true} {
				name := owner + "/preserves/" + other
				if tagged {
					name += "/tagged"
				}
				t.Run(name, func(t *testing.T) {
					ctrl := gomock.NewController(t)
					defer ctrl.Finish()
					stack := core.NewDefaultStack(core.StackID{Namespace: "awesome-ns", Name: "awesome-name"})
					provider := tracking.NewDefaultProvider(owner, testClusterName)
					otherProvider := tracking.NewDefaultProvider(other, testClusterName)
					prefix := namePrefix(testClusterName, provider.ResourceIDTagKey(), stack.StackID())
					otherPrefix := namePrefix(testClusterName, otherProvider.ResourceIDTagKey(), stack.StackID())
					assert.NotEqual(t, prefix, otherPrefix)
					sourceName := deliverySourceName(prefix, logdeliverymodel.LogTypeALBAccessLogs)
					otherSourceName := deliverySourceName(otherPrefix, logdeliverymodel.LogTypeALBAccessLogs)
					destinationName := deliveryDestinationName(prefix, logdeliverymodel.DestinationTypeS3, testBucketARN, "")
					otherDestinationName := deliveryDestinationName(otherPrefix, logdeliverymodel.DestinationTypeS3, testBucketARN, "")
					source := cwltypes.DeliverySource{Name: awssdk.String(sourceName)}
					otherSource := cwltypes.DeliverySource{Name: awssdk.String(otherSourceName)}
					destination := cwltypes.DeliveryDestination{Name: awssdk.String(destinationName)}
					otherDestination := cwltypes.DeliveryDestination{Name: awssdk.String(otherDestinationName)}
					if tagged {
						source.Tags, destination.Tags = provider.StackTags(stack), provider.StackTags(stack)
						otherSource.Tags, otherDestination.Tags = otherProvider.StackTags(stack), otherProvider.StackTags(stack)
					}
					cwlClient := services.NewMockCloudWatchLogs(ctrl)
					cwlClient.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliverySource{source, otherSource}, nil)
					cwlClient.EXPECT().DescribeDeliveryDestinationsAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliveryDestination{destination, otherDestination}, nil)
					cwlClient.EXPECT().DescribeDeliveriesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.Delivery{
						{Id: awssdk.String("owned"), DeliverySourceName: awssdk.String(sourceName), DeliveryDestinationArn: awssdk.String(testDeliveryDestinationARN + destinationName)},
						{Id: awssdk.String("other"), DeliverySourceName: awssdk.String(otherSourceName), DeliveryDestinationArn: awssdk.String(testDeliveryDestinationARN + otherDestinationName)},
					}, nil)
					gomock.InOrder(
						cwlClient.EXPECT().DeleteDeliveryWithContext(gomock.Any(), &cloudwatchlogssdk.DeleteDeliveryInput{Id: awssdk.String("owned")}).Return(&cloudwatchlogssdk.DeleteDeliveryOutput{}, nil),
						cwlClient.EXPECT().DeleteDeliveryDestinationWithContext(gomock.Any(), &cloudwatchlogssdk.DeleteDeliveryDestinationInput{Name: awssdk.String(destinationName)}).Return(&cloudwatchlogssdk.DeleteDeliveryDestinationOutput{}, nil),
						cwlClient.EXPECT().DeleteDeliverySourceWithContext(gomock.Any(), &cloudwatchlogssdk.DeleteDeliverySourceInput{Name: awssdk.String(sourceName)}).Return(&cloudwatchlogssdk.DeleteDeliverySourceOutput{}, nil),
					)
					synthesizer := NewLogDeliverySynthesizer(cwlClient, provider, testClusterName, logr.Discard(), stack)
					assert.NoError(t, synthesizer.Synthesize(context.Background()))
				})
			}
		}
	}
}

func Test_buildDeliveryConfigurationUpdate(t *testing.T) {
	current := cwltypes.Delivery{
		Id:             awssdk.String("d1"),
		FieldDelimiter: awssdk.String(""),
		S3DeliveryConfiguration: &cwltypes.S3DeliveryConfiguration{
			SuffixPath:               awssdk.String("{region}/{yyyy}/{MM}/{dd}/"),
			EnableHiveCompatiblePath: awssdk.Bool(false),
		},
	}
	tests := []struct {
		name       string
		delivery   cwltypes.Delivery
		wanted     desiredDelivery
		wantUpdate bool
		wantInput  *cloudwatchlogssdk.UpdateDeliveryConfigurationInput
	}{
		{
			name:     "defaults filled in by CloudWatch Logs don't trigger an update",
			delivery: current,
			wanted:   desiredDelivery{},
		},
		{
			name:     "explicit settings that already match don't trigger an update",
			delivery: current,
			wanted: desiredDelivery{
				fieldDelimiter: awssdk.String(""),
				s3Config:       &logdeliverymodel.S3DeliveryConfiguration{EnableHiveCompatiblePath: awssdk.Bool(false)},
			},
		},
		{
			name:       "enabling Hive-compatible paths keeps the current suffix",
			delivery:   current,
			wanted:     desiredDelivery{s3Config: &logdeliverymodel.S3DeliveryConfiguration{EnableHiveCompatiblePath: awssdk.Bool(true)}},
			wantUpdate: true,
			wantInput: &cloudwatchlogssdk.UpdateDeliveryConfigurationInput{
				Id: awssdk.String("d1"),
				S3DeliveryConfiguration: &cwltypes.S3DeliveryConfiguration{
					SuffixPath:               awssdk.String("{region}/{yyyy}/{MM}/{dd}/"),
					EnableHiveCompatiblePath: awssdk.Bool(true),
				},
			},
		},
		{
			name:       "S3 settings on a delivery that has none yet",
			delivery:   cwltypes.Delivery{Id: awssdk.String("d1")},
			wanted:     desiredDelivery{s3Config: &logdeliverymodel.S3DeliveryConfiguration{SuffixPath: awssdk.String("logs/{yyyy}")}},
			wantUpdate: true,
			wantInput: &cloudwatchlogssdk.UpdateDeliveryConfigurationInput{
				Id:                      awssdk.String("d1"),
				S3DeliveryConfiguration: &cwltypes.S3DeliveryConfiguration{SuffixPath: awssdk.String("logs/{yyyy}")},
			},
		},
		{
			name:       "field delimiter change",
			delivery:   current,
			wanted:     desiredDelivery{fieldDelimiter: awssdk.String("\t")},
			wantUpdate: true,
			wantInput:  &cloudwatchlogssdk.UpdateDeliveryConfigurationInput{Id: awssdk.String("d1"), FieldDelimiter: awssdk.String("\t")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, needsUpdate := buildDeliveryConfigurationUpdate(tt.delivery, tt.wanted)
			assert.Equal(t, tt.wantUpdate, needsUpdate)
			if tt.wantUpdate {
				assert.Equal(t, tt.wantInput, input)
			}
		})
	}
}
