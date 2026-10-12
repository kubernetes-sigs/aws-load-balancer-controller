package elbv2

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/smithy-go"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/config"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
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
		{
			name: "target control port nil -> 3000",
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
						TargetType:        elbv2model.TargetTypeIP,
						Port:              awssdk.Int32(8080),
						Protocol:          elbv2model.ProtocolHTTP,
						Name:              "my-tg",
						TargetControlPort: awssdk.Int32(3000),
					},
				},
			},
			want: true,
		},
		{
			name: "target control port 3000 -> nil",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						TargetType:        elbv2types.TargetTypeEnumIp,
						Port:              awssdk.Int32(8080),
						Protocol:          elbv2types.ProtocolEnumHttp,
						TargetGroupName:   awssdk.String("my-tg1"),
						TargetControlPort: awssdk.Int32(3000),
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
			name: "target control port 3000 -> 4000",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						TargetType:        elbv2types.TargetTypeEnumIp,
						Port:              awssdk.Int32(8080),
						Protocol:          elbv2types.ProtocolEnumHttp,
						TargetGroupName:   awssdk.String("my-tg1"),
						TargetControlPort: awssdk.Int32(3000),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						TargetType:        elbv2model.TargetTypeIP,
						Port:              awssdk.Int32(8080),
						Protocol:          elbv2model.ProtocolHTTP,
						Name:              "my-tg",
						TargetControlPort: awssdk.Int32(4000),
					},
				},
			},
			want: true,
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
		{
			name: "ALB TargetGroup healthCheck protocol change does not require replacement",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						Protocol:            elbv2types.ProtocolEnumHttps,
						HealthCheckEnabled:  awssdk.Bool(true),
						HealthCheckPort:     awssdk.String("8080"),
						HealthCheckProtocol: elbv2types.ProtocolEnumHttps,
						HealthCheckPath:     awssdk.String("/readyz"),
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
						Protocol: elbv2model.ProtocolHTTPS,
						HealthCheckConfig: &elbv2model.TargetGroupHealthCheckConfig{
							Port:                    &port8080,
							Protocol:                protocolHTTP,
							Path:                    awssdk.String("/readyz"),
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
			want: false,
		},
		{
			name: "ALB TargetGroup healthCheck interval change does not require replacement",
			args: args{
				sdkTG: TargetGroupWithTags{
					TargetGroup: &elbv2types.TargetGroup{
						Protocol:            elbv2types.ProtocolEnumHttp,
						HealthCheckEnabled:  awssdk.Bool(true),
						HealthCheckPort:     awssdk.String("8080"),
						HealthCheckProtocol: elbv2types.ProtocolEnumHttp,
						HealthCheckPath:     awssdk.String("/"),
						Matcher: &elbv2types.Matcher{
							HttpCode: awssdk.String("200"),
						},
						HealthCheckIntervalSeconds: awssdk.Int32(30),
						HealthCheckTimeoutSeconds:  awssdk.Int32(5),
						HealthyThresholdCount:      awssdk.Int32(3),
						UnhealthyThresholdCount:    awssdk.Int32(2),
					},
				},
				resTG: &elbv2model.TargetGroup{
					Spec: elbv2model.TargetGroupSpec{
						Protocol: elbv2model.ProtocolHTTP,
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

func Test_isL4TargetGroup(t *testing.T) {
	tests := []struct {
		name     string
		protocol elbv2model.Protocol
		want     bool
	}{
		{
			name:     "TCP is L4",
			protocol: elbv2model.ProtocolTCP,
			want:     true,
		},
		{
			name:     "UDP is L4",
			protocol: elbv2model.ProtocolUDP,
			want:     true,
		},
		{
			name:     "TLS is L4",
			protocol: elbv2model.ProtocolTLS,
			want:     true,
		},
		{
			name:     "QUIC is L4",
			protocol: elbv2model.ProtocolQUIC,
			want:     true,
		},
		{
			name:     "TCP_QUIC is L4",
			protocol: elbv2model.ProtocolTCP_QUIC,
			want:     true,
		},
		{
			name:     "TCP_UDP is L4",
			protocol: elbv2model.ProtocolTCP_UDP,
			want:     true,
		},
		{
			name:     "HTTP is not L4",
			protocol: elbv2model.ProtocolHTTP,
			want:     false,
		},
		{
			name:     "HTTPS is not L4",
			protocol: elbv2model.ProtocolHTTPS,
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isL4TargetGroup(tt.protocol)
			assert.Equal(t, tt.want, got)
		})
	}
}

type mockTGOperation int

const (
	createTG mockTGOperation = iota
	deleteTG
	updateTG
)

type mockTGCall struct {
	name string // This will equal Spec.Name for creates and the ARN for deletes/updates
	op   mockTGOperation
}

// mockTargetGroupManager simulates a quota-limited TG manager.
// Create returns TooManyUniqueTargetGroupsPerLoadBalancer when tgCount == maxTGCount.
// deleteErr optionally causes Delete to fail to simulate the ResourceInUse error.
type mockTargetGroupManager struct {
	tgCount    int
	maxTGCount int
	deleteErr  error
	calls      []mockTGCall
}

func (m *mockTargetGroupManager) Create(_ context.Context, resTG *elbv2model.TargetGroup) (elbv2model.TargetGroupStatus, error) {
	if m.tgCount == m.maxTGCount {
		return elbv2model.TargetGroupStatus{}, &smithy.GenericAPIError{
			Code:    "TooManyUniqueTargetGroupsPerLoadBalancer",
			Message: "too many unique target groups per load balancer",
		}
	}
	m.calls = append(m.calls, mockTGCall{name: resTG.Spec.Name, op: createTG})
	m.tgCount++
	return elbv2model.TargetGroupStatus{TargetGroupARN: resTG.Spec.Name}, nil
}

func (m *mockTargetGroupManager) Update(_ context.Context, _ *elbv2model.TargetGroup, sdkTG TargetGroupWithTags) (elbv2model.TargetGroupStatus, error) {
	m.calls = append(m.calls, mockTGCall{name: awssdk.ToString(sdkTG.TargetGroup.TargetGroupArn), op: updateTG})
	return elbv2model.TargetGroupStatus{TargetGroupARN: awssdk.ToString(sdkTG.TargetGroup.TargetGroupArn)}, nil
}

func (m *mockTargetGroupManager) Delete(_ context.Context, sdkTG TargetGroupWithTags) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.calls = append(m.calls, mockTGCall{name: awssdk.ToString(sdkTG.TargetGroup.TargetGroupArn), op: deleteTG})
	m.tgCount--
	return nil
}

// Unlike the listener_rule_synthesizer the one for target groups
// does not expose update and delete functions directly but does it inline.
// Thus we need these helper functions

const testResourceIDTagKey = "ingress.k8s.aws/resource"

type testTrackingProvider struct{}

func (p *testTrackingProvider) ResourceIDTagKey() string { return testResourceIDTagKey }
func (p *testTrackingProvider) StackTags(_ coremodel.Stack) map[string]string {
	return map[string]string{}
}
func (p *testTrackingProvider) StackTagsLegacy(_ coremodel.Stack) map[string]string {
	return map[string]string{}
}
func (p *testTrackingProvider) StackLabels(_ coremodel.Stack) map[string]string {
	return map[string]string{}
}
func (p *testTrackingProvider) ResourceTags(_ coremodel.Stack, _ coremodel.Resource, _ map[string]string) map[string]string {
	return map[string]string{}
}
func (p *testTrackingProvider) LegacyTagKeys() []string { return nil }

// newTestStack creates a stack and adds desired TGs to it (auto-registered via NewTargetGroup).
func newTestStack(desiredNames []string) coremodel.Stack {
	stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "ns", Name: "test"})
	for _, name := range desiredNames {
		elbv2model.NewTargetGroup(stack, name, elbv2model.TargetGroupSpec{Name: name})
	}
	return stack
}

func staleSdkTG(arn, resourceID string) TargetGroupWithTags {
	return TargetGroupWithTags{
		TargetGroup: &elbv2types.TargetGroup{TargetGroupArn: awssdk.String(arn)},
		Tags:        map[string]string{testResourceIDTagKey: resourceID},
	}
}

func newSynthesizerForTest(mock *mockTargetGroupManager, stack coremodel.Stack, sdkTGs []TargetGroupWithTags) *targetGroupSynthesizer {
	return &targetGroupSynthesizer{
		tgManager:        mock,
		trackingProvider: &testTrackingProvider{},
		featureGates:     config.NewFeatureGates(),
		logger:           logr.Discard(),
		stack:            stack,
		findSDKTargetGroups: func() TargetGroupsResult {
			return TargetGroupsResult{TargetGroups: sdkTGs}
		},
	}
}

func Test_isTooManyUniqueTargetGroupsErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "actual error code returns true",
			err:  &smithy.GenericAPIError{Code: "TooManyUniqueTargetGroupsPerLoadBalancer"},
			want: true,
		},
		{
			name: "any API error code returns false",
			err:  &smithy.GenericAPIError{Code: "TooManyRules"},
			want: false,
		},
		{
			name: "plain error returns false",
			err:  errors.New("err"),
			want: false,
		},
		{
			name: "nil returns false",
			err:  nil,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isTooManyUniqueTargetGroupsErr(tt.err))
		})
	}
}

func Test_targetGroupSynthesizer_Synthesize(t *testing.T) {
	tests := []struct {
		name          string
		desiredTGs    []string
		sdkTGs        []TargetGroupWithTags // existing TGs returned by AWS
		tgCount       int                   // current number of TGs counted against quota
		maxTGCount    int
		deleteErr     error
		wantErr       bool
		wantErrSubstr string
		wantCalls     []mockTGCall
	}{
		{
			name:       "no TGs at all",
			tgCount:    0,
			maxTGCount: 100,
		},
		{
			name:       "creates succeed without hitting quota",
			desiredTGs: []string{"tg-1", "tg-2", "tg-3"},
			tgCount:    0,
			maxTGCount: 100,
			wantCalls: []mockTGCall{
				{name: "tg-1", op: createTG},
				{name: "tg-2", op: createTG},
				{name: "tg-3", op: createTG},
			},
		},
		{
			name:       "quota hit once",
			desiredTGs: []string{"tg-new"},
			sdkTGs:     []TargetGroupWithTags{staleSdkTG("stale-arn-1", "stale-1")},
			tgCount:    2,
			maxTGCount: 2,
			wantCalls: []mockTGCall{
				{name: "stale-arn-1", op: deleteTG},
				{name: "tg-new", op: createTG},
			},
		},
		{
			name:       "quota hit multiple times",
			desiredTGs: []string{"tg-new-1", "tg-new-2"},
			sdkTGs: []TargetGroupWithTags{
				staleSdkTG("stale-arn-1", "stale-1"),
				staleSdkTG("stale-arn-2", "stale-2"),
			},
			tgCount:    2,
			maxTGCount: 2,
			wantCalls: []mockTGCall{
				{name: "stale-arn-1", op: deleteTG},
				{name: "tg-new-1", op: createTG},
				{name: "stale-arn-2", op: deleteTG},
				{name: "tg-new-2", op: createTG},
			},
		},
		{
			name:          "quota hit but no stale TGs available",
			desiredTGs:    []string{"tg-new"},
			sdkTGs:        nil,
			tgCount:       2,
			maxTGCount:    2,
			wantErr:       true,
			wantErrSubstr: "no unused target groups available to delete",
		},
		{
			name:          "quota hit but stale TG delete fails (eventual consistency)",
			desiredTGs:    []string{"tg-new"},
			sdkTGs:        []TargetGroupWithTags{staleSdkTG("stale-arn-1", "stale-1")},
			tgCount:       2,
			maxTGCount:    2,
			deleteErr:     errors.New("ResourceInUse: TG still referenced by a listener rule"),
			wantErr:       true,
			wantErrSubstr: "ResourceInUse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := newTestStack(tt.desiredTGs)
			mock := &mockTargetGroupManager{tgCount: tt.tgCount, maxTGCount: tt.maxTGCount, deleteErr: tt.deleteErr}
			s := newSynthesizerForTest(mock, stack, tt.sdkTGs)

			err := s.Synthesize(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrSubstr != "" {
					assert.Contains(t, err.Error(), tt.wantErrSubstr)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCalls, mock.calls)
			}
		})
	}
}

func Test_targetGroupSynthesizer_PostSynthesize(t *testing.T) {
	t.Run("stale TGs already deleted inline by Synthesize are not re-deleted in PostSynthesize", func(t *testing.T) {

		stack := newTestStack([]string{"tg-new-1", "tg-new-2"})
		staleTGs := []TargetGroupWithTags{
			staleSdkTG("stale-arn-1", "stale-1"),
			staleSdkTG("stale-arn-2", "stale-2"),
		}
		mock := &mockTargetGroupManager{tgCount: 2, maxTGCount: 2}
		s := newSynthesizerForTest(mock, stack, staleTGs)

		assert.NoError(t, s.Synthesize(context.Background()))
		callsAfterSynthesize := append([]mockTGCall(nil), mock.calls...)

		assert.NoError(t, s.PostSynthesize(context.Background()))
		assert.Equal(t, callsAfterSynthesize, mock.calls)
	})

	t.Run("stale TGs not consumed inline are deleted in PostSynthesize", func(t *testing.T) {

		stack := newTestStack(nil)
		staleTGs := []TargetGroupWithTags{
			staleSdkTG("stale-arn-1", "stale-1"),
			staleSdkTG("stale-arn-2", "stale-2"),
		}
		mock := &mockTargetGroupManager{tgCount: 0, maxTGCount: 100}
		s := newSynthesizerForTest(mock, stack, staleTGs)

		assert.NoError(t, s.Synthesize(context.Background()))
		assert.Empty(t, mock.calls)

		assert.NoError(t, s.PostSynthesize(context.Background()))
		assert.Equal(t, []mockTGCall{
			{name: "stale-arn-1", op: deleteTG},
			{name: "stale-arn-2", op: deleteTG},
		}, mock.calls)
	})
}
