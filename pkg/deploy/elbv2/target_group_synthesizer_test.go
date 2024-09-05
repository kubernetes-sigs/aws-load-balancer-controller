package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

func Test_matchResAndSDKTargetGroups(t *testing.T) {
	stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})
	type args struct {
		resTGs           []*elbv2model.TargetGroup
		sdkTGs           []TargetGroupWithTags
		resourceIDTagKey string
	}
	tests := []struct {
		name    string
		args    args
		want    []resAndSDKTargetGroupPair
		want1   []*elbv2model.TargetGroup
		want2   []TargetGroupWithTags
		wantErr error
	}{
		{
			name: "all TargetGroup has match",
			args: args{
				resTGs: []*elbv2model.TargetGroup{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-1"),
						Spec: elbv2model.TargetGroupSpec{
							Name: "id-1",
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-2"),
						Spec: elbv2model.TargetGroupSpec{
							Name: "id-2",
						},
					},
				},
				sdkTGs: []TargetGroupWithTags{
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-2"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: []resAndSDKTargetGroupPair{
				{
					resTG: &elbv2model.TargetGroup{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-1"),
						Spec: elbv2model.TargetGroupSpec{
							Name: "id-1",
						},
					},
					sdkTG: TargetGroupWithTags{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				{
					resTG: &elbv2model.TargetGroup{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-2"),
						Spec: elbv2model.TargetGroupSpec{
							Name: "id-2",
						},
					},
					sdkTG: TargetGroupWithTags{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-2"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
			},
		},
		{
			name: "some res TargetGroup don't have match",
			args: args{
				resTGs: []*elbv2model.TargetGroup{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-1"),
						Spec: elbv2model.TargetGroupSpec{
							Name: "id-1",
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-2"),
						Spec: elbv2model.TargetGroupSpec{
							Name: "id-2",
						},
					},
				},
				sdkTGs: []TargetGroupWithTags{
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: []resAndSDKTargetGroupPair{
				{
					resTG: &elbv2model.TargetGroup{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-1"),
						Spec: elbv2model.TargetGroupSpec{
							Name: "id-1",
						},
					},
					sdkTG: TargetGroupWithTags{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
			},
			want1: []*elbv2model.TargetGroup{
				{
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-2"),
					Spec: elbv2model.TargetGroupSpec{
						Name: "id-2",
					},
				},
			},
		},
		{
			name: "some sdk TargetGroup don't have match",
			args: args{
				resTGs: []*elbv2model.TargetGroup{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-1"),
						Spec: elbv2model.TargetGroupSpec{
							Name: "id-1",
						},
					},
				},
				sdkTGs: []TargetGroupWithTags{
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-2"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: []resAndSDKTargetGroupPair{
				{
					resTG: &elbv2model.TargetGroup{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-1"),
						Spec: elbv2model.TargetGroupSpec{
							Name: "id-1",
						},
					},
					sdkTG: TargetGroupWithTags{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
			},
			want2: []TargetGroupWithTags{
				{
					TargetGroup: &elbv2types.TargetGroup{
						TargetGroupArn: awssdk.String("arn-2"),
					},
					Tags: map[string]string{
						"ingress.k8s.aws/resource": "id-2",
					},
				},
			},
		},
		{
			name: "one TargetGroup need to be replaced",
			args: args{
				resTGs: []*elbv2model.TargetGroup{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-1"),
						Spec: elbv2model.TargetGroupSpec{
							Name:       "my-name",
							TargetType: elbv2model.TargetTypeIP,
						},
					},
				},
				sdkTGs: []TargetGroupWithTags{
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-1"),
							TargetType:     elbv2types.TargetTypeEnum("instance"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-2"),
							TargetType:     elbv2types.TargetTypeEnum("ip"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: []resAndSDKTargetGroupPair{
				{
					resTG: &elbv2model.TargetGroup{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-1"),
						Spec: elbv2model.TargetGroupSpec{
							Name:       "my-name",
							TargetType: elbv2model.TargetTypeIP,
						},
					},
					sdkTG: TargetGroupWithTags{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-2"),
							TargetType:     elbv2types.TargetTypeEnum("ip"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
			},
			want2: []TargetGroupWithTags{
				{
					TargetGroup: &elbv2types.TargetGroup{
						TargetGroupArn: awssdk.String("arn-1"),
						TargetType:     elbv2types.TargetTypeEnum("instance"),
					},
					Tags: map[string]string{
						"ingress.k8s.aws/resource": "id-1",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			featureGates := config.NewFeatureGates()
			got, got1, got2, err := matchResAndSDKTargetGroups(tt.args.resTGs, tt.args.sdkTGs, tt.args.resourceIDTagKey, featureGates)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
				assert.Equal(t, tt.want1, got1)
				assert.Equal(t, tt.want2, got2)
			}
		})
	}
}

func Test_mapResTargetGroupByResourceID(t *testing.T) {
	stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})
	type args struct {
		resTGs []*elbv2model.TargetGroup
	}
	tests := []struct {
		name string
		args args
		want map[string]*elbv2model.TargetGroup
	}{
		{
			name: "standard case",
			args: args{
				resTGs: []*elbv2model.TargetGroup{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-1"),
						Spec: elbv2model.TargetGroupSpec{
							Name: "id-1",
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-2"),
						Spec: elbv2model.TargetGroupSpec{
							Name: "id-2",
						},
					},
				},
			},
			want: map[string]*elbv2model.TargetGroup{
				"id-1": {
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-1"),
					Spec: elbv2model.TargetGroupSpec{
						Name: "id-1",
					},
				},
				"id-2": {
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-2"),
					Spec: elbv2model.TargetGroupSpec{
						Name: "id-2",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapResTargetGroupByResourceID(tt.args.resTGs)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_mapSDKTargetGroupByResourceID(t *testing.T) {
	type args struct {
		sdkTGs           []TargetGroupWithTags
		resourceIDTagKey string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string][]TargetGroupWithTags
		wantErr error
	}{
		{
			name: "standard case",
			args: args{
				sdkTGs: []TargetGroupWithTags{
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-2"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: map[string][]TargetGroupWithTags{
				"id-1": {
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				"id-2": {
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-2"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
			},
		},
		{
			name: "multiple targetGroups with same ID",
			args: args{
				sdkTGs: []TargetGroupWithTags{
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-2A"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-2B"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: map[string][]TargetGroupWithTags{
				"id-1": {
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				"id-2": {
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-2A"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-2B"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
			},
		},
		{
			name: "targetGroups without resourceID tag",
			args: args{
				sdkTGs: []TargetGroupWithTags{
					{
						TargetGroup: &elbv2types.TargetGroup{
							TargetGroupArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			wantErr: errors.New("unexpected targetGroup with no resourceID: arn-1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapSDKTargetGroupByResourceID(tt.args.sdkTGs, tt.args.resourceIDTagKey)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_isSDKTargetGroupRequiresReplacement(t *testing.T) {
	port8080 := intstr.FromInt(8080)
	protocolHTTP := elbv2model.ProtocolHTTP
	type args struct {
		sdkTG TargetGroupWithTags
		resTG *elbv2model.TargetGroup
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "targetGroup don't need replacement",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						TargetType:      elbv2types.TargetTypeEnumIp,
						Port:            awssdk.Int32(8080),
						Protocol:        elbv2types.ProtocolEnumHttp,
						TargetGroupName: awssdk.String("my-tg"),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						TargetType: elbv2model.TargetTypeIP,
						Port:       awssdk.Int32(8080),
						Protocol:   elbv2model.ProtocolHTTP,
						Name:       "my-tg",
					},
				},
			},
			want: false,
		},
		{
			name: "name-only change shouldn't need replacement",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						TargetType:      elbv2types.TargetTypeEnumIp,
						Port:            awssdk.Int32(8080),
						Protocol:        elbv2types.ProtocolEnumHttp,
						TargetGroupName: awssdk.String("my-tg1"),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						TargetType: elbv2model.TargetTypeIP,
						Port:       awssdk.Int32(8080),
						Protocol:   elbv2model.ProtocolHTTP,
						Name:       "my-tg",
					},
				},
			},
			want: false,
		},
		{
			name: "port-only change shouldn't need replacement",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						TargetType:      elbv2types.TargetTypeEnumIp,
						Port:            awssdk.Int32(9090),
						Protocol:        elbv2types.ProtocolEnumHttp,
						TargetGroupName: awssdk.String("my-tg"),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						TargetType: elbv2model.TargetTypeIP,
						Port:       awssdk.Int32(8080),
						Protocol:   elbv2model.ProtocolHTTP,
						Name:       "my-tg",
					},
				},
			},
			want: false,
		},
		{
			name: "targetType change need replacement",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						TargetType:      elbv2types.TargetTypeEnumInstance,
						Port:            awssdk.Int32(8080),
						Protocol:        elbv2types.ProtocolEnumHttp,
						TargetGroupName: awssdk.String("my-tg"),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						TargetType: elbv2model.TargetTypeIP,
						Port:       awssdk.Int32(8080),
						Protocol:   elbv2model.ProtocolHTTP,
						Name:       "my-tg",
					},
				},
			},
			want: true,
		},
		{
			name: "protocol change need replacement",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						TargetType:      elbv2types.TargetTypeEnumIp,
						Port:            awssdk.Int32(8080),
						Protocol:        elbv2types.ProtocolEnumTcp,
						TargetGroupName: awssdk.String("my-tg"),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						TargetType: elbv2model.TargetTypeIP,
						Port:       awssdk.Int32(8080),
						Protocol:   elbv2model.ProtocolHTTP,
						Name:       "my-tg",
					},
				},
			},
			want: true,
		},
		{
			name: "healthCheck change needs no replacement for protocol change",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						Protocol:            elbv2types.ProtocolEnumTcp,
						HealthCheckEnabled:  awssdk.Bool(true),
						HealthCheckPort:     awssdk.String("8080"),
						HealthCheckProtocol: elbv2types.ProtocolEnumHttp,
						HealthCheckPath:     awssdk.String("/"),
						Matcher: &elbv2types.Matcher{
							HttpCode: awssdk.String("200"),
						},
						HealthCheckIntervalSeconds: awssdk.Int32(11),
						HealthCheckTimeoutSeconds:  awssdk.Int32(5),
						HealthyThresholdCount:      awssdk.Int32(3),
						UnhealthyThresholdCount:    awssdk.Int32(2),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						Protocol: elbv2model.ProtocolTCP,
						HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
							Port:                    &port8080,
							Protocol:                protocolHTTP,
							Path:                    awssdk.String("/"),
							Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
							IntervalSeconds:         awssdk.Int32(10),
							TimeoutSeconds:          awssdk.Int32(5),
							HealthyThresholdCount:   awssdk.Int32(3),
							UnhealthyThresholdCount: awssdk.Int32(2),
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			featureGates := config.NewFeatureGates()
			got := isSDKTargetGroupRequiresReplacement(tt.args.sdkTG, tt.args.resTG, featureGates)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_isSDKTargetGroupRequiresReplacementDueToNLBHealthCheck(t *testing.T) {
	port8080 := intstr.FromInt(8080)
	protocolHTTP := elbv2model.ProtocolHTTP
	type args struct {
		sdkTG                               TargetGroupWithTags
		resTG                               *elbv2model.TargetGroup
		disableAdvancedNLBHealthCheckConfig bool
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "NLB TargetGroup healthCheck haven't changed",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						Protocol:            elbv2types.ProtocolEnumTcp,
						HealthCheckEnabled:  awssdk.Bool(true),
						HealthCheckPort:     awssdk.String("8080"),
						HealthCheckProtocol: elbv2types.ProtocolEnumHttp,
						HealthCheckPath:     awssdk.String("/"),
						Matcher: &elbv2types.Matcher{
							HttpCode: awssdk.String("200"),
						},
						HealthCheckIntervalSeconds: awssdk.Int32(10),
						HealthCheckTimeoutSeconds:  awssdk.Int32(5),
						HealthyThresholdCount:      awssdk.Int32(3),
						UnhealthyThresholdCount:    awssdk.Int32(2),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						Protocol: elbv2model.ProtocolTCP,
						HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
							Port:                    &port8080,
							Protocol:                protocolHTTP,
							Path:                    awssdk.String("/"),
							Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
							IntervalSeconds:         awssdk.Int32(10),
							TimeoutSeconds:          awssdk.Int32(5),
							HealthyThresholdCount:   awssdk.Int32(3),
							UnhealthyThresholdCount: awssdk.Int32(2),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "NLB TargetGroup healthCheck cannot change protocol without advanced config",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						Protocol:            elbv2types.ProtocolEnumTcp,
						HealthCheckEnabled:  awssdk.Bool(true),
						HealthCheckPort:     awssdk.String("8080"),
						HealthCheckProtocol: elbv2types.ProtocolEnumHttps,
						HealthCheckPath:     awssdk.String("/"),
						Matcher: &elbv2types.Matcher{
							HttpCode: awssdk.String("200"),
						},
						HealthCheckIntervalSeconds: awssdk.Int32(10),
						HealthCheckTimeoutSeconds:  awssdk.Int32(5),
						HealthyThresholdCount:      awssdk.Int32(3),
						UnhealthyThresholdCount:    awssdk.Int32(2),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						Protocol: elbv2model.ProtocolTCP,
						HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
							Port:                    &port8080,
							Protocol:                protocolHTTP,
							Path:                    awssdk.String("/"),
							Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
							IntervalSeconds:         awssdk.Int32(10),
							TimeoutSeconds:          awssdk.Int32(5),
							HealthyThresholdCount:   awssdk.Int32(3),
							UnhealthyThresholdCount: awssdk.Int32(2),
						},
					},
				},
				disableAdvancedNLBHealthCheckConfig: true,
			},
			want: true,
		},
		{
			name: "NLB TargetGroup healthCheck cannot changed matcher",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						Protocol:            elbv2types.ProtocolEnumTcp,
						HealthCheckEnabled:  awssdk.Bool(true),
						HealthCheckPort:     awssdk.String("8080"),
						HealthCheckProtocol: elbv2types.ProtocolEnumHttp,
						HealthCheckPath:     awssdk.String("/"),
						Matcher: &elbv2types.Matcher{
							HttpCode: awssdk.String("300"),
						},
						HealthCheckIntervalSeconds: awssdk.Int32(10),
						HealthCheckTimeoutSeconds:  awssdk.Int32(5),
						HealthyThresholdCount:      awssdk.Int32(3),
						UnhealthyThresholdCount:    awssdk.Int32(2),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						Protocol: elbv2model.ProtocolTCP,
						HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
							Port:                    &port8080,
							Protocol:                protocolHTTP,
							Path:                    awssdk.String("/"),
							Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
							IntervalSeconds:         awssdk.Int32(10),
							TimeoutSeconds:          awssdk.Int32(5),
							HealthyThresholdCount:   awssdk.Int32(3),
							UnhealthyThresholdCount: awssdk.Int32(2),
						},
					},
				},
				disableAdvancedNLBHealthCheckConfig: true,
			},
			want: true,
		},
		{
			name: "NLB TargetGroup healthCheck cannot change intervalSeconds",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						Protocol:            elbv2types.ProtocolEnumTcp,
						HealthCheckEnabled:  awssdk.Bool(true),
						HealthCheckPort:     awssdk.String("8080"),
						HealthCheckProtocol: elbv2types.ProtocolEnumHttp,
						HealthCheckPath:     awssdk.String("/"),
						Matcher: &elbv2types.Matcher{
							HttpCode: awssdk.String("200"),
						},
						HealthCheckIntervalSeconds: awssdk.Int32(11),
						HealthCheckTimeoutSeconds:  awssdk.Int32(5),
						HealthyThresholdCount:      awssdk.Int32(3),
						UnhealthyThresholdCount:    awssdk.Int32(2),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						Protocol: elbv2model.ProtocolTCP,
						HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
							Port:                    &port8080,
							Protocol:                protocolHTTP,
							Path:                    awssdk.String("/"),
							Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
							IntervalSeconds:         awssdk.Int32(10),
							TimeoutSeconds:          awssdk.Int32(5),
							HealthyThresholdCount:   awssdk.Int32(3),
							UnhealthyThresholdCount: awssdk.Int32(2),
						},
					},
				},
				disableAdvancedNLBHealthCheckConfig: true,
			},
			want: true,
		},
		{
			name: "NLB TargetGroup healthCheck cannot change timeoutSecond",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						Protocol:            elbv2types.ProtocolEnumTcp,
						HealthCheckEnabled:  awssdk.Bool(true),
						HealthCheckPort:     awssdk.String("8080"),
						HealthCheckProtocol: elbv2types.ProtocolEnumHttp,
						HealthCheckPath:     awssdk.String("/"),
						Matcher: &elbv2types.Matcher{
							HttpCode: awssdk.String("200"),
						},
						HealthCheckIntervalSeconds: awssdk.Int32(10),
						HealthCheckTimeoutSeconds:  awssdk.Int32(6),
						HealthyThresholdCount:      awssdk.Int32(3),
						UnhealthyThresholdCount:    awssdk.Int32(2),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						Protocol: elbv2model.ProtocolTCP,
						HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
							Port:                    &port8080,
							Protocol:                protocolHTTP,
							Path:                    awssdk.String("/"),
							Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
							IntervalSeconds:         awssdk.Int32(10),
							TimeoutSeconds:          awssdk.Int32(5),
							HealthyThresholdCount:   awssdk.Int32(3),
							UnhealthyThresholdCount: awssdk.Int32(2),
						},
					},
				},
				disableAdvancedNLBHealthCheckConfig: true,
			},
			want: true,
		},
		{
			name: "NLB TargetGroup healthCheck can change port",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						Protocol:            elbv2types.ProtocolEnumTcp,
						HealthCheckEnabled:  awssdk.Bool(true),
						HealthCheckPort:     awssdk.String("9090"),
						HealthCheckProtocol: elbv2types.ProtocolEnumHttp,
						HealthCheckPath:     awssdk.String("/"),
						Matcher: &elbv2types.Matcher{
							HttpCode: awssdk.String("200"),
						},
						HealthCheckIntervalSeconds: awssdk.Int32(10),
						HealthCheckTimeoutSeconds:  awssdk.Int32(5),
						HealthyThresholdCount:      awssdk.Int32(3),
						UnhealthyThresholdCount:    awssdk.Int32(2),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						Protocol: elbv2model.ProtocolTCP,
						HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
							Port:                    &port8080,
							Protocol:                protocolHTTP,
							Path:                    awssdk.String("/"),
							Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
							IntervalSeconds:         awssdk.Int32(10),
							TimeoutSeconds:          awssdk.Int32(5),
							HealthyThresholdCount:   awssdk.Int32(3),
							UnhealthyThresholdCount: awssdk.Int32(2),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "NLB TargetGroup healthCheck can change path",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						Protocol:            elbv2types.ProtocolEnumTcp,
						HealthCheckEnabled:  awssdk.Bool(true),
						HealthCheckPort:     awssdk.String("8080"),
						HealthCheckProtocol: elbv2types.ProtocolEnumHttp,
						HealthCheckPath:     awssdk.String("/some-other"),
						Matcher: &elbv2types.Matcher{
							HttpCode: awssdk.String("200"),
						},
						HealthCheckIntervalSeconds: awssdk.Int32(10),
						HealthCheckTimeoutSeconds:  awssdk.Int32(5),
						HealthyThresholdCount:      awssdk.Int32(3),
						UnhealthyThresholdCount:    awssdk.Int32(2),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						Protocol: elbv2model.ProtocolTCP,
						HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
							Port:                    &port8080,
							Protocol:                protocolHTTP,
							Path:                    awssdk.String("/"),
							Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
							IntervalSeconds:         awssdk.Int32(10),
							TimeoutSeconds:          awssdk.Int32(5),
							HealthyThresholdCount:   awssdk.Int32(3),
							UnhealthyThresholdCount: awssdk.Int32(2),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "NLB TargetGroup healthCheck can change healthyThresholdCount",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						Protocol:            elbv2types.ProtocolEnumTcp,
						HealthCheckEnabled:  awssdk.Bool(true),
						HealthCheckPort:     awssdk.String("8080"),
						HealthCheckProtocol: elbv2types.ProtocolEnumHttp,
						HealthCheckPath:     awssdk.String("/"),
						Matcher: &elbv2types.Matcher{
							HttpCode: awssdk.String("200"),
						},
						HealthCheckIntervalSeconds: awssdk.Int32(10),
						HealthCheckTimeoutSeconds:  awssdk.Int32(5),
						HealthyThresholdCount:      awssdk.Int32(4),
						UnhealthyThresholdCount:    awssdk.Int32(2),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						Protocol: elbv2model.ProtocolTCP,
						HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
							Port:                    &port8080,
							Protocol:                protocolHTTP,
							Path:                    awssdk.String("/"),
							Matcher:                 &elbv2model.HealthCheckMatcher{HTTPCode: awssdk.String("200")},
							IntervalSeconds:         awssdk.Int32(10),
							TimeoutSeconds:          awssdk.Int32(5),
							HealthyThresholdCount:   awssdk.Int32(3),
							UnhealthyThresholdCount: awssdk.Int32(2),
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			featureGates := config.NewFeatureGates()
			if tt.args.disableAdvancedNLBHealthCheckConfig {
				featureGates.Disable(config.NLBHealthCheckAdvancedConfig)
			}
			got := isSDKTargetGroupRequiresReplacementDueToNLBHealthCheck(tt.args.sdkTG, tt.args.resTG, featureGates)
			assert.Equal(t, tt.want, got)
		})
	}
}
