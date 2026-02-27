package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/shield"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_matchResAndSDKLoadBalancers(t *testing.T) {
	stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})
	type args struct {
		resLBs           []*elbv2model.LoadBalancer
		sdkLBs           []LoadBalancerWithTags
		resourceIDTagKey string
	}
	tests := []struct {
		name    string
		args    args
		want    []resAndSDKLoadBalancerPair
		want1   []*elbv2model.LoadBalancer
		want2   []LoadBalancerWithTags
		wantErr error
	}{
		{
			name: "all loadBalancer has match",
			args: args{
				resLBs: []*elbv2model.LoadBalancer{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-1",
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-2"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-2",
						},
					},
				},
				sdkLBs: []LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: []resAndSDKLoadBalancerPair{
				{
					resLB: &elbv2model.LoadBalancer{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-1",
						},
					},
					sdkLB: LoadBalancerWithTags{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				{
					resLB: &elbv2model.LoadBalancer{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-2"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-2",
						},
					},
					sdkLB: LoadBalancerWithTags{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
			},
		},
		{
			name: "some res LoadBalancer don't have match",
			args: args{
				resLBs: []*elbv2model.LoadBalancer{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-1",
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-2"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-2",
						},
					},
				},
				sdkLBs: []LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: []resAndSDKLoadBalancerPair{
				{
					resLB: &elbv2model.LoadBalancer{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-1",
						},
					},
					sdkLB: LoadBalancerWithTags{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
			},
			want1: []*elbv2model.LoadBalancer{
				{
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-2"),
					Spec: elbv2model.LoadBalancerSpec{
						Name: "id-2",
					},
				},
			},
		},
		{
			name: "some sdk LoadBalancer don't have match",
			args: args{
				resLBs: []*elbv2model.LoadBalancer{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-1",
						},
					},
				},
				sdkLBs: []LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: []resAndSDKLoadBalancerPair{
				{
					resLB: &elbv2model.LoadBalancer{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-1",
						},
					},
					sdkLB: LoadBalancerWithTags{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
			},
			want2: []LoadBalancerWithTags{
				{
					LoadBalancer: &elbv2types.LoadBalancer{
						LoadBalancerArn: awssdk.String("arn-2"),
					},
					Tags: map[string]string{
						"ingress.k8s.aws/resource": "id-2",
					},
				},
			},
		},
		{
			name: "one loadBalancer need to be replaced",
			args: args{
				resLBs: []*elbv2model.LoadBalancer{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "my-name",
							Type: elbv2model.LoadBalancerTypeNetwork,
						},
					},
				},
				sdkLBs: []LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
							Type:            elbv2types.LoadBalancerTypeEnum("application"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2"),
							Type:            elbv2types.LoadBalancerTypeEnum("network"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: []resAndSDKLoadBalancerPair{
				{
					resLB: &elbv2model.LoadBalancer{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "my-name",
							Type: elbv2model.LoadBalancerTypeNetwork,
						},
					},
					sdkLB: LoadBalancerWithTags{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2"),
							Type:            elbv2types.LoadBalancerTypeEnum("network"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
			},
			want2: []LoadBalancerWithTags{
				{
					LoadBalancer: &elbv2types.LoadBalancer{
						LoadBalancerArn: awssdk.String("arn-1"),
						Type:            elbv2types.LoadBalancerTypeEnum("application"),
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
			got, got1, got2, err := matchResAndSDKLoadBalancers(tt.args.resLBs, tt.args.sdkLBs, tt.args.resourceIDTagKey)
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

func Test_mapResLoadBalancerByResourceID(t *testing.T) {
	stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})
	type args struct {
		resLBs []*elbv2model.LoadBalancer
	}
	tests := []struct {
		name string
		args args
		want map[string]*elbv2model.LoadBalancer
	}{
		{
			name: "standard case",
			args: args{
				resLBs: []*elbv2model.LoadBalancer{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-1",
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-2"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-2",
						},
					},
				},
			},
			want: map[string]*elbv2model.LoadBalancer{
				"id-1": {
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
					Spec: elbv2model.LoadBalancerSpec{
						Name: "id-1",
					},
				},
				"id-2": {
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-2"),
					Spec: elbv2model.LoadBalancerSpec{
						Name: "id-2",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapResLoadBalancerByResourceID(tt.args.resLBs)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_mapSDKLoadBalancerByResourceID(t *testing.T) {
	type args struct {
		sdkLBs           []LoadBalancerWithTags
		resourceIDTagKey string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string][]LoadBalancerWithTags
		wantErr error
	}{
		{
			name: "standard case",
			args: args{
				sdkLBs: []LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: map[string][]LoadBalancerWithTags{
				"id-1": {
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				"id-2": {
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
			},
		},
		{
			name: "multiple LoadBalancers with same ID",
			args: args{
				sdkLBs: []LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2A"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2B"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: map[string][]LoadBalancerWithTags{
				"id-1": {
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				"id-2": {
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2A"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2B"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
			},
		},
		{
			name: "LoadBalancers without resourceID tag",
			args: args{
				sdkLBs: []LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			wantErr: errors.New("unexpected loadBalancer with no resourceID: arn-1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapSDKLoadBalancerByResourceID(tt.args.sdkLBs, tt.args.resourceIDTagKey)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_isSDKLoadBalancerRequiresReplacement(t *testing.T) {
	schemaInternetFacing := elbv2model.LoadBalancerSchemeInternetFacing
	type args struct {
		sdkLB LoadBalancerWithTags
		resLB *elbv2model.LoadBalancer
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "don't need replacement",
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
						Type:             elbv2types.LoadBalancerTypeEnum("application"),
						Scheme:           elbv2types.LoadBalancerSchemeEnum("internet-facing"),
						LoadBalancerName: awssdk.String("my-lb"),
					},
				},
				resLB: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						Type:   elbv2model.LoadBalancerTypeApplication,
						Scheme: schemaInternetFacing,
						Name:   "my-lb",
					},
				},
			},
			want: false,
		},
		{
			name: "name-only change shouldn't need replacement",
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
						Type:             elbv2types.LoadBalancerTypeEnum("application"),
						Scheme:           elbv2types.LoadBalancerSchemeEnum("internet-facing"),
						LoadBalancerName: awssdk.String("my-lb1"),
					},
				},
				resLB: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						Type:   elbv2model.LoadBalancerTypeApplication,
						Scheme: schemaInternetFacing,
						Name:   "my-lb",
					},
				},
			},
			want: false,
		},
		{
			name: "type change need replacement",
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
						Type:             elbv2types.LoadBalancerTypeEnum("network"),
						Scheme:           elbv2types.LoadBalancerSchemeEnum("internet-facing"),
						LoadBalancerName: awssdk.String("my-lb"),
					},
				},
				resLB: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						Type:   elbv2model.LoadBalancerTypeApplication,
						Scheme: schemaInternetFacing,
						Name:   "my-lb",
					},
				},
			},
			want: true,
		},
		{
			name: "scheme change need replacement",
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
						Type:             elbv2types.LoadBalancerTypeEnumApplication,
						Scheme:           elbv2types.LoadBalancerSchemeEnumInternal,
						LoadBalancerName: awssdk.String("my-lb"),
					},
				},
				resLB: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						Type:   elbv2model.LoadBalancerTypeApplication,
						Scheme: schemaInternetFacing,
						Name:   "my-lb",
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSDKLoadBalancerRequiresReplacement(tt.args.sdkLB, tt.args.resLB)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_isLoadBalancerInProvisioningState(t *testing.T) {
	type describeLoadBalancersAsListCall struct {
		req  *elbv2sdk.DescribeLoadBalancersInput
		resp []elbv2types.LoadBalancer
		err  error
	}
	type fields struct {
		describeLoadBalancersAsListCalls []describeLoadBalancersAsListCall
	}
	type args struct {
		sdkLB LoadBalancerWithTags
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr error
	}{
		{
			name: "load balancer in provisioning state case",
			fields: fields{
				describeLoadBalancersAsListCalls: []describeLoadBalancersAsListCall{
					{
						req: &elbv2sdk.DescribeLoadBalancersInput{
							LoadBalancerArns: []string{"my-arn"},
						},
						resp: []elbv2types.LoadBalancer{
							{
								LoadBalancerArn: awssdk.String("lb-1"),
								State:           &elbv2types.LoadBalancerState{Code: elbv2types.LoadBalancerStateEnumProvisioning},
							},
						},
					},
				},
			},
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
						LoadBalancerArn: awssdk.String("my-arn"),
					},
					Tags: nil,
				},
			},
			want: true,
		},
		{
			name: "load balancer in active state case",
			fields: fields{
				describeLoadBalancersAsListCalls: []describeLoadBalancersAsListCall{
					{
						req: &elbv2sdk.DescribeLoadBalancersInput{
							LoadBalancerArns: []string{"my-arn"},
						},
						resp: []elbv2types.LoadBalancer{
							{
								LoadBalancerArn: awssdk.String("lb-1"),
								State:           &elbv2types.LoadBalancerState{Code: elbv2types.LoadBalancerStateEnumActive},
							},
						},
					},
				},
			},
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
						LoadBalancerArn: awssdk.String("my-arn"),
					},
					Tags: nil,
				},
			},
			want: false,
		},
		{
			name: "error case",
			fields: fields{
				describeLoadBalancersAsListCalls: []describeLoadBalancersAsListCall{
					{
						req: &elbv2sdk.DescribeLoadBalancersInput{
							LoadBalancerArns: []string{"my-arn"},
						},
						err: errors.New("some error"),
					},
				},
			},
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
						LoadBalancerArn: awssdk.String("my-arn"),
					},
					Tags: nil,
				},
			},
			wantErr: errors.New("some error"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeLoadBalancersAsListCalls {
				elbv2Client.EXPECT().DescribeLoadBalancersAsList(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			r := &loadBalancerSynthesizer{
				elbv2Client: elbv2Client,
			}
			got, err := r.isLoadBalancerInProvisioningState(context.Background(), tt.args.sdkLB)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_loadBalancerSynthesizer_Synthesize_cleansUpManagedShieldProtectionForDeletedALB(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taggingManager := NewMockTaggingManager(ctrl)
	shieldProtectionManager := shield.NewMockProtectionManager(ctrl)

	lbARN := "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test/123"
	stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "ns", Name: "ing"})
	trackingProvider := tracking.NewDefaultProvider("ingress.k8s.aws", "cluster")
	sdkLB := LoadBalancerWithTags{
		LoadBalancer: &elbv2types.LoadBalancer{
			LoadBalancerArn: awssdk.String(lbARN),
			Type:            elbv2types.LoadBalancerTypeEnumApplication,
		},
		Tags: map[string]string{
			trackingProvider.ResourceIDTagKey(): "LoadBalancer",
		},
	}
	taggingManager.EXPECT().ListLoadBalancers(gomock.Any(), gomock.Any(), gomock.Any()).Return([]LoadBalancerWithTags{sdkLB}, nil)
	shieldProtectionManager.EXPECT().GetProtection(gomock.Any(), lbARN).Return(&shield.ProtectionInfo{
		Name: shield.ProtectionNameManaged,
		ID:   "protection-id",
	}, nil)
	shieldProtectionManager.EXPECT().DeleteProtection(gomock.Any(), lbARN, "protection-id").Return(nil)

	lbManager := &stubLoadBalancerManager{}
	s := &loadBalancerSynthesizer{
		trackingProvider:               trackingProvider,
		taggingManager:                 taggingManager,
		lbManager:                      lbManager,
		logger:                         logr.New(&log.NullLogSink{}),
		shieldProtectionManager:        shieldProtectionManager,
		shieldProtectionCleanupEnabled: true,
		stack:                          stack,
	}

	err := s.Synthesize(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, []string{lbARN}, lbManager.deletedLbarns)
}

func Test_loadBalancerSynthesizer_deleteManagedShieldProtection(t *testing.T) {
	type fields struct {
		shieldCleanupEnabled bool
		lbType               elbv2types.LoadBalancerTypeEnum
		getProtectionInfo    *shield.ProtectionInfo
		getProtectionErr     error
		deleteProtectionErr  error
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr string
	}{
		{
			name: "cleanup disabled",
			fields: fields{
				shieldCleanupEnabled: false,
				lbType:               elbv2types.LoadBalancerTypeEnumApplication,
			},
		},
		{
			name: "non ALB",
			fields: fields{
				shieldCleanupEnabled: true,
				lbType:               elbv2types.LoadBalancerTypeEnumNetwork,
			},
		},
		{
			name: "unmanaged protection",
			fields: fields{
				shieldCleanupEnabled: true,
				lbType:               elbv2types.LoadBalancerTypeEnumApplication,
				getProtectionInfo: &shield.ProtectionInfo{
					Name: "external manager",
					ID:   "protection-id",
				},
			},
		},
		{
			name: "get protection failed",
			fields: fields{
				shieldCleanupEnabled: true,
				lbType:               elbv2types.LoadBalancerTypeEnumApplication,
				getProtectionErr:     errors.New("boom"),
			},
			wantErr: "failed to get shield protection on LoadBalancer: boom",
		},
		{
			name: "delete protection failed",
			fields: fields{
				shieldCleanupEnabled: true,
				lbType:               elbv2types.LoadBalancerTypeEnumApplication,
				getProtectionInfo: &shield.ProtectionInfo{
					Name: shield.ProtectionNameManaged,
					ID:   "protection-id",
				},
				deleteProtectionErr: errors.New("boom"),
			},
			wantErr: "failed to delete shield protection on LoadBalancer: boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			shieldProtectionManager := shield.NewMockProtectionManager(ctrl)
			lbARN := "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test/123"
			sdkLB := LoadBalancerWithTags{
				LoadBalancer: &elbv2types.LoadBalancer{
					LoadBalancerArn: awssdk.String(lbARN),
					Type:            tt.fields.lbType,
				},
			}
			if tt.fields.shieldCleanupEnabled && tt.fields.lbType == elbv2types.LoadBalancerTypeEnumApplication {
				shieldProtectionManager.EXPECT().GetProtection(gomock.Any(), lbARN).Return(tt.fields.getProtectionInfo, tt.fields.getProtectionErr)
				if tt.fields.getProtectionErr == nil && tt.fields.getProtectionInfo != nil && shield.IsManagedProtectionName(tt.fields.getProtectionInfo.Name) {
					shieldProtectionManager.EXPECT().DeleteProtection(gomock.Any(), lbARN, tt.fields.getProtectionInfo.ID).Return(tt.fields.deleteProtectionErr)
				}
			}

			s := &loadBalancerSynthesizer{
				logger:                         logr.New(&log.NullLogSink{}),
				shieldProtectionManager:        shieldProtectionManager,
				shieldProtectionCleanupEnabled: tt.fields.shieldCleanupEnabled,
			}
			err := s.deleteManagedShieldProtection(context.Background(), sdkLB)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

type stubLoadBalancerManager struct {
	deletedLbarns []string
}

func (m *stubLoadBalancerManager) Create(_ context.Context, _ *elbv2model.LoadBalancer) (elbv2model.LoadBalancerStatus, LoadBalancerWithTags, error) {
	return elbv2model.LoadBalancerStatus{}, LoadBalancerWithTags{}, errors.New("unexpected Create call")
}

func (m *stubLoadBalancerManager) Update(_ context.Context, _ *elbv2model.LoadBalancer, _ LoadBalancerWithTags) (elbv2model.LoadBalancerStatus, error) {
	return elbv2model.LoadBalancerStatus{}, errors.New("unexpected Update call")
}

func (m *stubLoadBalancerManager) Delete(_ context.Context, sdkLB LoadBalancerWithTags) error {
	m.deletedLbarns = append(m.deletedLbarns, awssdk.ToString(sdkLB.LoadBalancer.LoadBalancerArn))
	return nil
}
