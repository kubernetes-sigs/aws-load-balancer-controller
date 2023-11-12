package targetgroupbinding

import (
	"context"
	"sync"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/eks"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_cachedTargetsManager_RegisterTargets(t *testing.T) {
	type registerTargetsWithContextCall struct {
		req  *elbv2sdk.RegisterTargetsInput
		resp *elbv2sdk.RegisterTargetsOutput
		err  error
	}

	type fields struct {
		registerTargetsWithContextCalls []registerTargetsWithContextCall
		targetsCache                    map[string][]TargetInfo
	}
	type args struct {
		tgARN   string
		targets []elbv2sdk.TargetDescription
	}
	tests := []struct {
		name             string
		fields           fields
		args             args
		wantTargetsCache map[string][]TargetInfo
		wantErr          error
	}{
		{
			name: "register targets and targets for TargetGroup already exists in cache",
			fields: fields{
				registerTargetsWithContextCalls: []registerTargetsWithContextCall{
					{
						req: &elbv2sdk.RegisterTargetsInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets: []*elbv2sdk.TargetDescription{
								{
									Id:   awssdk.String("192.168.1.2"),
									Port: awssdk.Int64(8080),
								},
								{
									Id:   awssdk.String("192.168.1.3"),
									Port: awssdk.Int64(8080),
								},
							},
						},
						resp: &elbv2sdk.RegisterTargetsOutput{},
					},
				},
				targetsCache: map[string][]TargetInfo{
					"my-tg": {
						{
							Target: elbv2sdk.TargetDescription{
								Id:   awssdk.String("192.168.1.1"),
								Port: awssdk.Int64(8080),
							},
							TargetHealth: &elbv2sdk.TargetHealth{
								State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
							},
						},
						{
							Target: elbv2sdk.TargetDescription{
								Id:   awssdk.String("192.168.1.2"),
								Port: awssdk.Int64(8080),
							},
							TargetHealth: &elbv2sdk.TargetHealth{
								Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetTimeout),
								State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnhealthy),
							},
						},
					},
				},
			},
			args: args{
				tgARN: "my-tg",
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			wantTargetsCache: map[string][]TargetInfo{
				"my-tg": {
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
						},
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.2"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: nil,
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.3"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: nil,
					},
				},
			},
		},
		{
			name: "register targets and targets for TargetGroup don't exists in cache",
			fields: fields{
				registerTargetsWithContextCalls: []registerTargetsWithContextCall{
					{
						req: &elbv2sdk.RegisterTargetsInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets: []*elbv2sdk.TargetDescription{
								{
									Id:   awssdk.String("192.168.1.2"),
									Port: awssdk.Int64(8080),
								},
								{
									Id:   awssdk.String("192.168.1.3"),
									Port: awssdk.Int64(8080),
								},
							},
						},
						resp: &elbv2sdk.RegisterTargetsOutput{},
					},
				},
				targetsCache: nil,
			},
			args: args{
				tgARN: "my-tg",
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			wantTargetsCache: nil,
		},
		{
			name: "register multiple targets in batches",
			fields: fields{
				registerTargetsWithContextCalls: []registerTargetsWithContextCall{
					{
						req: &elbv2sdk.RegisterTargetsInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets: []*elbv2sdk.TargetDescription{
								{
									Id:   awssdk.String("192.168.1.1"),
									Port: awssdk.Int64(8080),
								},
								{
									Id:   awssdk.String("192.168.1.2"),
									Port: awssdk.Int64(8080),
								},
							},
						},
						resp: &elbv2sdk.RegisterTargetsOutput{},
					},
					{
						req: &elbv2sdk.RegisterTargetsInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets: []*elbv2sdk.TargetDescription{
								{
									Id:   awssdk.String("192.168.1.3"),
									Port: awssdk.Int64(8080),
								},
								{
									Id:   awssdk.String("192.168.1.4"),
									Port: awssdk.Int64(8080),
								},
							},
						},
						resp: &elbv2sdk.RegisterTargetsOutput{},
					},
				},
				targetsCache: map[string][]TargetInfo{
					"my-tg": nil,
				},
			},
			args: args{
				tgARN: "my-tg",
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.4"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			wantTargetsCache: map[string][]TargetInfo{
				"my-tg": {
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: nil,
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.2"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: nil,
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.3"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: nil,
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.4"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: nil,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.registerTargetsWithContextCalls {
				elbv2Client.EXPECT().RegisterTargetsWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			targetsCache := cache.NewExpiring()
			targetsCacheTTL := 1 * time.Minute
			for tgARN, targets := range tt.fields.targetsCache {
				targetsCache.Set(tgARN, &targetsCacheItem{
					mutex:   sync.RWMutex{},
					targets: targets,
				}, targetsCacheTTL)
			}
			m := cachedTargetsManager{
				elbv2Client:              elbv2Client,
				targetsCache:             targetsCache,
				targetsCacheTTL:          targetsCacheTTL,
				registerTargetsChunkSize: 2,
				logger:                   log.Log,
			}

			ctx := context.Background()
			err := m.RegisterTargets(ctx, tt.args.tgARN, tt.args.targets)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tt.wantTargetsCache), targetsCache.Len())
				for tgARN, targets := range tt.wantTargetsCache {
					rawTargetsCacheItem, exists := targetsCache.Get(tgARN)
					assert.True(t, exists)
					targetsCacheItem := rawTargetsCacheItem.(*targetsCacheItem)
					assert.ElementsMatch(t, targets, targetsCacheItem.targets)
				}
			}
		})
	}
}

func Test_cachedTargetsManager_DeregisterTargets(t *testing.T) {
	type deregisterTargetsWithContextCall struct {
		req  *elbv2sdk.DeregisterTargetsInput
		resp *elbv2sdk.DeregisterTargetsOutput
		err  error
	}

	type fields struct {
		deregisterTargetsWithContextCalls []deregisterTargetsWithContextCall
		targetsCache                      map[string][]TargetInfo
	}
	type args struct {
		tgARN   string
		targets []elbv2sdk.TargetDescription
	}
	tests := []struct {
		name             string
		fields           fields
		args             args
		wantTargetsCache map[string][]TargetInfo
		wantErr          error
	}{
		{
			name: "deregister targets and targets for TargetGroup already exists in cache",
			fields: fields{
				deregisterTargetsWithContextCalls: []deregisterTargetsWithContextCall{
					{
						req: &elbv2sdk.DeregisterTargetsInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets: []*elbv2sdk.TargetDescription{
								{
									Id:   awssdk.String("192.168.1.2"),
									Port: awssdk.Int64(8080),
								},
								{
									Id:   awssdk.String("192.168.1.3"),
									Port: awssdk.Int64(8080),
								},
							},
						},
						resp: &elbv2sdk.DeregisterTargetsOutput{},
					},
				},
				targetsCache: map[string][]TargetInfo{
					"my-tg": {
						{
							Target: elbv2sdk.TargetDescription{
								Id:   awssdk.String("192.168.1.1"),
								Port: awssdk.Int64(8080),
							},
							TargetHealth: &elbv2sdk.TargetHealth{
								State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
							},
						},
						{
							Target: elbv2sdk.TargetDescription{
								Id:   awssdk.String("192.168.1.2"),
								Port: awssdk.Int64(8080),
							},
							TargetHealth: &elbv2sdk.TargetHealth{
								Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetTimeout),
								State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnhealthy),
							},
						},
					},
				},
			},
			args: args{
				tgARN: "my-tg",
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			wantTargetsCache: map[string][]TargetInfo{
				"my-tg": {
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
						},
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.2"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: nil,
					},
				},
			},
		},
		{
			name: "register targets and targets for TargetGroup don't exists in cache",
			fields: fields{
				deregisterTargetsWithContextCalls: []deregisterTargetsWithContextCall{
					{
						req: &elbv2sdk.DeregisterTargetsInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets: []*elbv2sdk.TargetDescription{
								{
									Id:   awssdk.String("192.168.1.2"),
									Port: awssdk.Int64(8080),
								},
								{
									Id:   awssdk.String("192.168.1.3"),
									Port: awssdk.Int64(8080),
								},
							},
						},
						resp: &elbv2sdk.DeregisterTargetsOutput{},
					},
				},
				targetsCache: nil,
			},
			args: args{
				tgARN: "my-tg",
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			wantTargetsCache: nil,
		},
		{
			name: "register multiple targets in batches",
			fields: fields{
				deregisterTargetsWithContextCalls: []deregisterTargetsWithContextCall{
					{
						req: &elbv2sdk.DeregisterTargetsInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets: []*elbv2sdk.TargetDescription{
								{
									Id:   awssdk.String("192.168.1.1"),
									Port: awssdk.Int64(8080),
								},
								{
									Id:   awssdk.String("192.168.1.2"),
									Port: awssdk.Int64(8080),
								},
							},
						},
						resp: &elbv2sdk.DeregisterTargetsOutput{},
					},
					{
						req: &elbv2sdk.DeregisterTargetsInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets: []*elbv2sdk.TargetDescription{
								{
									Id:   awssdk.String("192.168.1.3"),
									Port: awssdk.Int64(8080),
								},
								{
									Id:   awssdk.String("192.168.1.4"),
									Port: awssdk.Int64(8080),
								},
							},
						},
						resp: &elbv2sdk.DeregisterTargetsOutput{},
					},
				},
				targetsCache: nil,
			},
			args: args{
				tgARN: "my-tg",
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.4"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			wantTargetsCache: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.deregisterTargetsWithContextCalls {
				elbv2Client.EXPECT().DeregisterTargetsWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			targetsCache := cache.NewExpiring()
			targetsCacheTTL := 1 * time.Minute
			for tgARN, targets := range tt.fields.targetsCache {
				targetsCache.Set(tgARN, &targetsCacheItem{
					mutex:   sync.RWMutex{},
					targets: targets,
				}, targetsCacheTTL)
			}
			m := cachedTargetsManager{
				elbv2Client:                elbv2Client,
				targetsCache:               targetsCache,
				targetsCacheTTL:            targetsCacheTTL,
				deregisterTargetsChunkSize: 2,
				logger:                     log.Log,
			}

			ctx := context.Background()
			err := m.DeregisterTargets(ctx, tt.args.tgARN, tt.args.targets)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tt.wantTargetsCache), targetsCache.Len())
				for tgARN, targets := range tt.wantTargetsCache {
					rawTargetsCacheItem, exists := targetsCache.Get(tgARN)
					assert.True(t, exists)
					targetsCacheItem := rawTargetsCacheItem.(*targetsCacheItem)
					assert.ElementsMatch(t, targets, targetsCacheItem.targets)
				}
			}
		})
	}
}

func Test_cachedTargetsManager_ListTargets(t *testing.T) {
	type describeTargetHealthWithContextCall struct {
		req  *elbv2sdk.DescribeTargetHealthInput
		resp *elbv2sdk.DescribeTargetHealthOutput
		err  error
	}
	type fields struct {
		describeTargetHealthWithContextCalls []describeTargetHealthWithContextCall
		targetsCache                         map[string][]TargetInfo
	}
	type args struct {
		tgARN string
	}
	tests := []struct {
		name             string
		fields           fields
		args             args
		want             []TargetInfo
		wantTargetsCache map[string][]TargetInfo
		wantErr          error
	}{
		{
			name: "when targets for targetGroup don't exists in cache",
			fields: fields{
				describeTargetHealthWithContextCalls: []describeTargetHealthWithContextCall{
					{
						req: &elbv2sdk.DescribeTargetHealthInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets:        nil,
						},
						resp: &elbv2sdk.DescribeTargetHealthOutput{
							TargetHealthDescriptions: []*elbv2sdk.TargetHealthDescription{
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("192.168.1.1"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
									},
								},
							},
						},
					},
				},
				targetsCache: nil,
			},
			args: args{
				tgARN: "my-tg",
			},
			want: []TargetInfo{
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
					},
				},
			},
			wantTargetsCache: map[string][]TargetInfo{
				"my-tg": {
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
						},
					},
				},
			},
		},
		{
			name: "when targets for targetGroup exists in cache and don't need refresh",
			fields: fields{
				describeTargetHealthWithContextCalls: nil,
				targetsCache: map[string][]TargetInfo{
					"my-tg": {
						{
							Target: elbv2sdk.TargetDescription{
								Id:   awssdk.String("192.168.1.1"),
								Port: awssdk.Int64(8080),
							},
							TargetHealth: &elbv2sdk.TargetHealth{
								State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
							},
						},
					},
				},
			},
			args: args{
				tgARN: "my-tg",
			},
			want: []TargetInfo{
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
					},
				},
			},
			wantTargetsCache: map[string][]TargetInfo{
				"my-tg": {
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
						},
					},
				},
			},
		},
		{
			name: "when targets for targetGroup exists in cache and needs refresh",
			fields: fields{
				describeTargetHealthWithContextCalls: []describeTargetHealthWithContextCall{
					{
						req: &elbv2sdk.DescribeTargetHealthInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets: []*elbv2sdk.TargetDescription{
								{
									Id:   awssdk.String("192.168.1.2"),
									Port: awssdk.Int64(8080),
								},
							},
						},
						resp: &elbv2sdk.DescribeTargetHealthOutput{
							TargetHealthDescriptions: []*elbv2sdk.TargetHealthDescription{
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("192.168.1.2"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
									},
								},
							},
						},
					},
				},
				targetsCache: map[string][]TargetInfo{
					"my-tg": {
						{
							Target: elbv2sdk.TargetDescription{
								Id:   awssdk.String("192.168.1.1"),
								Port: awssdk.Int64(8080),
							},
							TargetHealth: &elbv2sdk.TargetHealth{
								State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
							},
						},
						{
							Target: elbv2sdk.TargetDescription{
								Id:   awssdk.String("192.168.1.2"),
								Port: awssdk.Int64(8080),
							},
							TargetHealth: &elbv2sdk.TargetHealth{
								Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
								State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
							},
						},
					},
				},
			},
			args: args{
				tgARN: "my-tg",
			},
			want: []TargetInfo{
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
					},
				},
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
					},
				},
			},
			wantTargetsCache: map[string][]TargetInfo{
				"my-tg": {
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
						},
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.2"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeTargetHealthWithContextCalls {
				elbv2Client.EXPECT().DescribeTargetHealthWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			targetsCache := cache.NewExpiring()
			targetsCacheTTL := 1 * time.Minute
			for tgARN, targets := range tt.fields.targetsCache {
				targetsCache.Set(tgARN, &targetsCacheItem{
					mutex:   sync.RWMutex{},
					targets: targets,
				}, targetsCacheTTL)
			}
			m := &cachedTargetsManager{
				elbv2Client:       elbv2Client,
				targetsCache:      targetsCache,
				targetsCacheMutex: sync.RWMutex{},
				targetsCacheTTL:   targetsCacheTTL,
			}

			ctx := context.Background()
			got, err := m.ListTargets(ctx, tt.args.tgARN)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
				assert.Equal(t, len(tt.wantTargetsCache), targetsCache.Len())
				for tgARN, targets := range tt.wantTargetsCache {
					rawTargetsCacheItem, exists := targetsCache.Get(tgARN)
					assert.True(t, exists)
					targetsCacheItem := rawTargetsCacheItem.(*targetsCacheItem)
					assert.ElementsMatch(t, targets, targetsCacheItem.targets)
				}
			}
		})
	}
}

func Test_cachedTargetsManager_ListOwnedTargets(t *testing.T) {
	type describeTargetHealthWithContextCall struct {
		req  *elbv2sdk.DescribeTargetHealthInput
		resp *elbv2sdk.DescribeTargetHealthOutput
		err  error
	}
	type describeClusterWithContextCall struct {
		req  *eks.DescribeClusterInput
		resp *eks.DescribeClusterOutput
		err  error
	}
	type describeSubnetsCall struct {
		req  *ec2sdk.DescribeSubnetsInput
		resp *ec2sdk.DescribeSubnetsOutput
		err  error
	}
	type describeInstancesAsListCall struct {
		req  *ec2sdk.DescribeInstancesInput
		resp []*ec2sdk.Instance
		err  error
	}
	type fields struct {
		describeTargetHealthWithContextCalls []describeTargetHealthWithContextCall
		describeClusterWithContextCalls      []describeClusterWithContextCall
		describeSubnetsCalls                 []describeSubnetsCall
		describeInstancesAsListCalls         []describeInstancesAsListCall
		clusterName                          string
	}
	type args struct {
		tgARN string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []TargetInfo
		wantErr error
	}{
		{
			name: "when targets are IPs, one is in the cluster",
			fields: fields{
				describeTargetHealthWithContextCalls: []describeTargetHealthWithContextCall{
					{
						req: &elbv2sdk.DescribeTargetHealthInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets:        nil,
						},
						resp: &elbv2sdk.DescribeTargetHealthOutput{
							TargetHealthDescriptions: []*elbv2sdk.TargetHealthDescription{
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("192.168.1.1"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
									},
								},
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("10.0.0.1"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
									},
								},
							},
						},
					},
				},
				describeClusterWithContextCalls: []describeClusterWithContextCall{
					{
						req: &eks.DescribeClusterInput{
							Name: awssdk.String("cluster_1"),
						},
						resp: &eks.DescribeClusterOutput{
							Cluster: &eks.Cluster{
								ResourcesVpcConfig: &eks.VpcConfigResponse{
									SubnetIds: awssdk.StringSlice([]string{"subnet-01234567890abcdef", "subnet-0bb1c79de3abcdef"}),
								},
							},
						},
					},
				},
				describeSubnetsCalls: []describeSubnetsCall{
					{
						req: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: awssdk.StringSlice([]string{"subnet-01234567890abcdef", "subnet-0bb1c79de3abcdef"}),
						},
						resp: &ec2sdk.DescribeSubnetsOutput{
							Subnets: []*ec2sdk.Subnet{
								{
									CidrBlock: awssdk.String("10.0.0.0/24"),
								},
								{
									CidrBlock: awssdk.String("10.0.1.0/24"),
								},
							},
						},
					},
				},
				clusterName: "cluster_1",
			},
			args: args{
				tgARN: "my-tg",
			},
			want: []TargetInfo{
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("10.0.0.1"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
					},
				},
			},
		},
		{
			name: "when targets are instances, one is in the cluster",
			fields: fields{
				describeTargetHealthWithContextCalls: []describeTargetHealthWithContextCall{
					{
						req: &elbv2sdk.DescribeTargetHealthInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets:        nil,
						},
						resp: &elbv2sdk.DescribeTargetHealthOutput{
							TargetHealthDescriptions: []*elbv2sdk.TargetHealthDescription{
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("i-0fa2d0064e848c69e"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
									},
								},
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("i-gdh270064e848c70e"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
									},
								},
							},
						},
					},
				},
				describeClusterWithContextCalls: []describeClusterWithContextCall{
					{
						req: &eks.DescribeClusterInput{
							Name: awssdk.String("cluster_1"),
						},
						resp: &eks.DescribeClusterOutput{
							Cluster: &eks.Cluster{
								ResourcesVpcConfig: &eks.VpcConfigResponse{
									SubnetIds: awssdk.StringSlice([]string{"subnet-01234567890abcdef", "subnet-0bb1c79de3abcdef"}),
								},
							},
						},
					},
				},
				describeSubnetsCalls: []describeSubnetsCall{
					{
						req: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: awssdk.StringSlice([]string{"subnet-01234567890abcdef", "subnet-0bb1c79de3abcdef"}),
						},
						resp: &ec2sdk.DescribeSubnetsOutput{
							Subnets: []*ec2sdk.Subnet{
								{
									CidrBlock: awssdk.String("10.0.0.0/24"),
								},
								{
									CidrBlock: awssdk.String("10.0.1.0/24"),
								},
							},
						},
					},
				},
				describeInstancesAsListCalls: []describeInstancesAsListCall{
					{
						req: &ec2sdk.DescribeInstancesInput{
							InstanceIds: []*string{
								awssdk.String("i-0fa2d0064e848c69e"),
							},
						},
						resp: []*ec2sdk.Instance{
							{
								Tags: []*ec2sdk.Tag{
									{
										Key:   awssdk.String("kubernetes.io/cluster/cluster_1"),
										Value: awssdk.String("owned"),
									},
								},
							},
						},
					},
					{
						req: &ec2sdk.DescribeInstancesInput{
							InstanceIds: []*string{
								awssdk.String("i-gdh270064e848c70e"),
							},
						},
						resp: []*ec2sdk.Instance{
							{
								Tags: []*ec2sdk.Tag{
									{
										Key:   awssdk.String("terraform"),
										Value: awssdk.String("true"),
									},
								},
							},
						},
					},
				},
				clusterName: "cluster_1",
			},
			args: args{
				tgARN: "my-tg",
			},
			want: []TargetInfo{
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("i-0fa2d0064e848c69e"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
					},
				},
			},
		},
		{
			name: "when targets include lambda ARNs, and the ip is in the cluster",
			fields: fields{
				describeTargetHealthWithContextCalls: []describeTargetHealthWithContextCall{
					{
						req: &elbv2sdk.DescribeTargetHealthInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets:        nil,
						},
						resp: &elbv2sdk.DescribeTargetHealthOutput{
							TargetHealthDescriptions: []*elbv2sdk.TargetHealthDescription{
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("10.0.0.1"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
									},
								},
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("i-gdh270064e848c70e"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
									},
								},
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("arn:aws:lambda:us-west-2:123456789012:function:my-function"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
									},
								},
							},
						},
					},
				},
				describeClusterWithContextCalls: []describeClusterWithContextCall{
					{
						req: &eks.DescribeClusterInput{
							Name: awssdk.String("cluster_1"),
						},
						resp: &eks.DescribeClusterOutput{
							Cluster: &eks.Cluster{
								ResourcesVpcConfig: &eks.VpcConfigResponse{
									SubnetIds: awssdk.StringSlice([]string{"subnet-01234567890abcdef", "subnet-0bb1c79de3abcdef"}),
								},
							},
						},
					},
				},
				describeSubnetsCalls: []describeSubnetsCall{
					{
						req: &ec2sdk.DescribeSubnetsInput{
							SubnetIds: awssdk.StringSlice([]string{"subnet-01234567890abcdef", "subnet-0bb1c79de3abcdef"}),
						},
						resp: &ec2sdk.DescribeSubnetsOutput{
							Subnets: []*ec2sdk.Subnet{
								{
									CidrBlock: awssdk.String("10.0.0.0/24"),
								},
								{
									CidrBlock: awssdk.String("10.0.1.0/24"),
								},
							},
						},
					},
				},
				describeInstancesAsListCalls: []describeInstancesAsListCall{
					{
						req: &ec2sdk.DescribeInstancesInput{
							InstanceIds: []*string{
								awssdk.String("i-gdh270064e848c70e"),
							},
						},
						resp: []*ec2sdk.Instance{
							{
								Tags: []*ec2sdk.Tag{
									{
										Key:   awssdk.String("terraform"),
										Value: awssdk.String("true"),
									},
								},
							},
						},
					},
				},
				clusterName: "cluster_1",
			},
			args: args{
				tgARN: "my-tg",
			},
			want: []TargetInfo{
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("10.0.0.1"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeTargetHealthWithContextCalls {
				elbv2Client.EXPECT().DescribeTargetHealthWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			eksClient := services.NewMockEKS(ctrl)
			for _, call := range tt.fields.describeClusterWithContextCalls {
				eksClient.EXPECT().DescribeClusterWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeSubnetsCalls {
				ec2Client.EXPECT().DescribeSubnets(call.req).Return(call.resp, call.err)
			}
			for _, call := range tt.fields.describeInstancesAsListCalls {
				ec2Client.EXPECT().DescribeInstancesAsList(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			cacheTTL := 1 * time.Minute
			targetsCache := cache.NewExpiring()
			instanceCache := cache.NewExpiring()
			m := &cachedTargetsManager{
				elbv2Client:        elbv2Client,
				targetsCache:       targetsCache,
				targetsCacheTTL:    cacheTTL,
				targetsCacheMutex:  sync.RWMutex{},
				instanceCache:      instanceCache,
				instanceCacheTTL:   cacheTTL,
				instanceCacheMutex: sync.RWMutex{},
			}
			eksInfoResolver := networking.NewDefaultEKSInfoResolver(
				eksClient, ec2Client, tt.fields.clusterName,
			)

			ctx := context.Background()
			got, err := m.ListOwnedTargets(ctx, tt.args.tgARN, eksInfoResolver)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_cachedTargetsManager_isCachedInstanceInCluster(t *testing.T) {
	type describeInstancesAsListCall struct {
		req  *ec2sdk.DescribeInstancesInput
		resp []*ec2sdk.Instance
		err  error
	}
	type args struct {
		instanceTarget TargetInfo
	}
	type fields struct {
		instanceCache map[string]bool
		clusterName   string
		calls         []describeInstancesAsListCall
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		want      bool
		wantCache map[string]bool
		wantErr   error
	}{
		{
			name: "when instance is not in cache",
			fields: fields{
				instanceCache: map[string]bool{
					"i-0fa2d0064e848c69e": true,
					"i-gdh270064e848c70e": false,
				},
				clusterName: "cluster_1",
				calls: []describeInstancesAsListCall{
					{
						req: &ec2sdk.DescribeInstancesInput{
							InstanceIds: []*string{
								awssdk.String("i-sdfh27464e848c70e"),
							},
						},
						resp: []*ec2sdk.Instance{
							{
								Tags: []*ec2sdk.Tag{
									{
										Key:   awssdk.String("kubernetes.io/cluster/cluster_1"),
										Value: awssdk.String("owned"),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				instanceTarget: TargetInfo{
					Target: elbv2sdk.TargetDescription{
						Id: awssdk.String("i-sdfh27464e848c70e"),
					},
				},
			},
			want: true,
			wantCache: map[string]bool{
				"i-0fa2d0064e848c69e": true,
				"i-gdh270064e848c70e": false,
				"i-sdfh27464e848c70e": true,
			},
			wantErr: nil,
		},
		{
			name: "when instance is in cache",
			fields: fields{
				instanceCache: map[string]bool{
					"i-0fa2d0064e848c69e": true,
					"i-gdh270064e848c70e": false,
					"i-sdfh27464e848c70e": true,
				},
				clusterName: "cluster_1",
				calls:       []describeInstancesAsListCall{},
			},
			args: args{
				instanceTarget: TargetInfo{
					Target: elbv2sdk.TargetDescription{
						Id: awssdk.String("i-sdfh27464e848c70e"),
					},
				},
			},
			want: true,
			wantCache: map[string]bool{
				"i-0fa2d0064e848c69e": true,
				"i-gdh270064e848c70e": false,
				"i-sdfh27464e848c70e": true,
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			eksClient := services.NewMockEKS(ctrl)
			elbv2Client := services.NewMockELBV2(ctrl)
			ec2Client := services.NewMockEC2(ctrl)

			for _, call := range tt.fields.calls {
				ec2Client.EXPECT().DescribeInstancesAsList(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			eksInfoResolver := networking.NewDefaultEKSInfoResolver(
				eksClient, ec2Client, tt.fields.clusterName,
			)

			instanceCache := cache.NewExpiring()
			instanceCacheTTL := 5 * time.Minute
			for instanceID, inCluster := range tt.fields.instanceCache {
				instanceCache.Set(instanceID, &instanceCacheItem{
					mutex:   sync.RWMutex{},
					cluster: inCluster,
				}, instanceCacheTTL)
			}

			m := &cachedTargetsManager{
				elbv2Client:        elbv2Client,
				instanceCache:      instanceCache,
				instanceCacheMutex: sync.RWMutex{},
				instanceCacheTTL:   instanceCacheTTL,
			}

			ctx := context.Background()
			got, err := m.isCachedInstanceInCluster(ctx, tt.args.instanceTarget, eksInfoResolver)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
				assert.Equal(t, len(tt.wantCache), instanceCache.Len())
				for instanceID, inCluster := range tt.wantCache {
					rawInstanceCacheItem, exists := instanceCache.Get(instanceID)
					assert.True(t, exists)
					instanceCacheItem := rawInstanceCacheItem.(*instanceCacheItem)
					assert.Equal(t, inCluster, instanceCacheItem.cluster)
				}
			}
		})
	}
}

func Test_cachedTargetsManager_refreshUnhealthyTargets(t *testing.T) {
	type describeTargetHealthWithContextCall struct {
		req  *elbv2sdk.DescribeTargetHealthInput
		resp *elbv2sdk.DescribeTargetHealthOutput
		err  error
	}
	type fields struct {
		describeTargetHealthWithContextCalls []describeTargetHealthWithContextCall
	}
	type args struct {
		tgARN         string
		cachedTargets []TargetInfo
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []TargetInfo
		wantErr error
	}{
		{
			name: "when all targets are already healthy",
			fields: fields{
				describeTargetHealthWithContextCalls: nil,
			},
			args: args{
				tgARN: "my-tg",
				cachedTargets: []TargetInfo{
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
						},
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.2"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
						},
					},
				},
			},
			want: []TargetInfo{
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
					},
				},
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
					},
				},
			},
		},
		{
			name: "when all targets are not healthy",
			fields: fields{
				describeTargetHealthWithContextCalls: []describeTargetHealthWithContextCall{
					{
						req: &elbv2sdk.DescribeTargetHealthInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets: []*elbv2sdk.TargetDescription{
								{
									Id:   awssdk.String("192.168.1.1"),
									Port: awssdk.Int64(8080),
								},
								{
									Id:   awssdk.String("192.168.1.2"),
									Port: awssdk.Int64(8080),
								},
							},
						},
						resp: &elbv2sdk.DescribeTargetHealthOutput{
							TargetHealthDescriptions: []*elbv2sdk.TargetHealthDescription{
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("192.168.1.1"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetTimeout),
										State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnhealthy),
									},
								},
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("192.168.1.2"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				tgARN: "my-tg",
				cachedTargets: []TargetInfo{
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetTimeout),
							State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnhealthy),
						},
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.2"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
							State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
						},
					},
				},
			},
			want: []TargetInfo{
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetTimeout),
						State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnhealthy),
					},
				},
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
					},
				},
			},
		},
		{
			name: "when some targets are healthy, some targets are not healthy",
			fields: fields{
				describeTargetHealthWithContextCalls: []describeTargetHealthWithContextCall{
					{
						req: &elbv2sdk.DescribeTargetHealthInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets: []*elbv2sdk.TargetDescription{
								{
									Id:   awssdk.String("192.168.1.2"),
									Port: awssdk.Int64(8080),
								},
								{
									Id:   awssdk.String("192.168.1.3"),
									Port: awssdk.Int64(8080),
								},
							},
						},
						resp: &elbv2sdk.DescribeTargetHealthOutput{
							TargetHealthDescriptions: []*elbv2sdk.TargetHealthDescription{
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("192.168.1.2"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetTimeout),
										State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnhealthy),
									},
								},
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("192.168.1.3"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				tgARN: "my-tg",
				cachedTargets: []TargetInfo{
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
						},
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.2"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetTimeout),
							State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnhealthy),
						},
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.3"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
							State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
						},
					},
				},
			},
			want: []TargetInfo{
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
					},
				},
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetTimeout),
						State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnhealthy),
					},
				},
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
					},
				},
			},
		},
		{
			name: "when some targets have unknown targetHealth and removed after refresh",
			fields: fields{
				describeTargetHealthWithContextCalls: []describeTargetHealthWithContextCall{
					{
						req: &elbv2sdk.DescribeTargetHealthInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets: []*elbv2sdk.TargetDescription{
								{
									Id:   awssdk.String("192.168.1.2"),
									Port: awssdk.Int64(8080),
								},
								{
									Id:   awssdk.String("192.168.1.3"),
									Port: awssdk.Int64(8080),
								},
							},
						},
						resp: &elbv2sdk.DescribeTargetHealthOutput{
							TargetHealthDescriptions: []*elbv2sdk.TargetHealthDescription{
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("192.168.1.2"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetTimeout),
										State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnhealthy),
									},
								},
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("192.168.1.3"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetNotRegistered),
										State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnused),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				tgARN: "my-tg",
				cachedTargets: []TargetInfo{
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
						},
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.2"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetTimeout),
							State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnhealthy),
						},
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.3"),
							Port: awssdk.Int64(8080),
						},
					},
				},
			},
			want: []TargetInfo{
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
					},
				},
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetTimeout),
						State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnhealthy),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeTargetHealthWithContextCalls {
				elbv2Client.EXPECT().DescribeTargetHealthWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			m := &cachedTargetsManager{
				elbv2Client: elbv2Client,
			}
			ctx := context.Background()
			got, err := m.refreshUnhealthyTargets(ctx, tt.args.tgARN, tt.args.cachedTargets)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_cachedTargetsManager_listTargetsFromAWS(t *testing.T) {
	type describeTargetHealthWithContextCall struct {
		req  *elbv2sdk.DescribeTargetHealthInput
		resp *elbv2sdk.DescribeTargetHealthOutput
		err  error
	}
	type fields struct {
		describeTargetHealthWithContextCalls []describeTargetHealthWithContextCall
	}

	type args struct {
		tgARN   string
		targets []elbv2sdk.TargetDescription
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []TargetInfo
		wantErr error
	}{
		{
			name: "list with non-nil targets",
			fields: fields{
				describeTargetHealthWithContextCalls: []describeTargetHealthWithContextCall{
					{
						req: &elbv2sdk.DescribeTargetHealthInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets: []*elbv2sdk.TargetDescription{
								{
									Id:   awssdk.String("192.168.1.1"),
									Port: awssdk.Int64(8080),
								},
							},
						},
						resp: &elbv2sdk.DescribeTargetHealthOutput{
							TargetHealthDescriptions: []*elbv2sdk.TargetHealthDescription{
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("192.168.1.1"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
										State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				tgARN: "my-tg",
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			want: []TargetInfo{
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
						State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
					},
				},
			},
		},
		{
			name: "list with nil targets",
			fields: fields{
				describeTargetHealthWithContextCalls: []describeTargetHealthWithContextCall{
					{
						req: &elbv2sdk.DescribeTargetHealthInput{
							TargetGroupArn: awssdk.String("my-tg"),
							Targets:        nil,
						},
						resp: &elbv2sdk.DescribeTargetHealthOutput{
							TargetHealthDescriptions: []*elbv2sdk.TargetHealthDescription{
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("192.168.1.1"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
										State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
									},
								},
								{
									Target: &elbv2sdk.TargetDescription{
										Id:   awssdk.String("192.168.1.2"),
										Port: awssdk.Int64(8080),
									},
									TargetHealth: &elbv2sdk.TargetHealth{
										Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
										State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				tgARN:   "my-tg",
				targets: nil,
			},
			want: []TargetInfo{
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
						State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
					},
				},
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
						State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			elbv2Client := services.NewMockELBV2(ctrl)
			for _, call := range tt.fields.describeTargetHealthWithContextCalls {
				elbv2Client.EXPECT().DescribeTargetHealthWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			m := &cachedTargetsManager{
				elbv2Client: elbv2Client,
			}
			ctx := context.Background()
			got, err := m.listTargetsFromAWS(ctx, tt.args.tgARN, tt.args.targets)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_cachedTargetsManager_recordSuccessfulRegisterTargetsOperation(t *testing.T) {
	type fields struct {
		targetsCache map[string][]TargetInfo
	}
	type args struct {
		tgARN   string
		targets []elbv2sdk.TargetDescription
	}
	tests := []struct {
		name             string
		fields           fields
		args             args
		wantTargetsCache map[string][]TargetInfo
	}{
		{
			name: "targets for tgARN don't exists in cache",
			fields: fields{
				targetsCache: nil,
			},
			args: args{
				tgARN: "my-tg",
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			wantTargetsCache: nil,
		},
		{
			name: "targets for tgARN exists in cache, and contain the target",
			fields: fields{
				targetsCache: map[string][]TargetInfo{
					"my-tg": {
						{
							Target: elbv2sdk.TargetDescription{
								Id:   awssdk.String("192.168.1.1"),
								Port: awssdk.Int64(8080),
							},
							TargetHealth: &elbv2sdk.TargetHealth{
								Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
								State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
							},
						},
					},
				},
			},
			args: args{
				tgARN: "my-tg",
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			wantTargetsCache: map[string][]TargetInfo{
				"my-tg": {
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
					},
				},
			},
		},
		{
			name: "targets for tgARN exists in cache, but don't contain the target",
			fields: fields{
				targetsCache: map[string][]TargetInfo{
					"my-tg": {
						{
							Target: elbv2sdk.TargetDescription{
								Id:   awssdk.String("192.168.1.1"),
								Port: awssdk.Int64(8080),
							},
							TargetHealth: &elbv2sdk.TargetHealth{
								Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
								State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
							},
						},
					},
				},
			},
			args: args{
				tgARN: "my-tg",
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			wantTargetsCache: map[string][]TargetInfo{
				"my-tg": {
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
							State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
						},
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.2"),
							Port: awssdk.Int64(8080),
						},
					},
				},
			},
		},
		{
			name: "targets for tgARN exists in cache, and contains multiple targets",
			fields: fields{
				targetsCache: map[string][]TargetInfo{
					"my-tg": {
						{
							Target: elbv2sdk.TargetDescription{
								Id:   awssdk.String("192.168.1.1"),
								Port: awssdk.Int64(8080),
							},
							TargetHealth: &elbv2sdk.TargetHealth{
								Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
								State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
							},
						},
						{
							Target: elbv2sdk.TargetDescription{
								Id:   awssdk.String("192.168.1.2"),
								Port: awssdk.Int64(8080),
							},
							TargetHealth: &elbv2sdk.TargetHealth{
								Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
								State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
							},
						},
					},
				},
			},
			args: args{
				tgARN: "my-tg",
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			wantTargetsCache: map[string][]TargetInfo{
				"my-tg": {
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
							State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
						},
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.2"),
							Port: awssdk.Int64(8080),
						},
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.3"),
							Port: awssdk.Int64(8080),
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetsCache := cache.NewExpiring()
			targetsCacheTTL := 1 * time.Minute
			for tgARN, targets := range tt.fields.targetsCache {
				targetsCache.Set(tgARN, &targetsCacheItem{
					mutex:   sync.RWMutex{},
					targets: targets,
				}, targetsCacheTTL)
			}

			m := &cachedTargetsManager{
				targetsCache:      targetsCache,
				targetsCacheMutex: sync.RWMutex{},
			}
			m.recordSuccessfulRegisterTargetsOperation(tt.args.tgARN, tt.args.targets)
			assert.Equal(t, len(tt.wantTargetsCache), targetsCache.Len())
			for tgARN, targets := range tt.wantTargetsCache {
				rawTargetsCacheItem, exists := targetsCache.Get(tgARN)
				assert.True(t, exists)
				targetsCacheItem := rawTargetsCacheItem.(*targetsCacheItem)
				assert.ElementsMatch(t, targets, targetsCacheItem.targets)
			}
		})
	}
}

func Test_cachedTargetsManager_recordSuccessfulDeregisterTargetsOperation(t *testing.T) {
	type fields struct {
		targetsCache map[string][]TargetInfo
	}
	type args struct {
		tgARN   string
		targets []elbv2sdk.TargetDescription
	}
	tests := []struct {
		name             string
		fields           fields
		args             args
		wantTargetsCache map[string][]TargetInfo
	}{
		{
			name: "targets for tgARN don't exists in cache",
			fields: fields{
				targetsCache: nil,
			},
			args: args{
				tgARN: "my-tg",
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			wantTargetsCache: nil,
		},
		{
			name: "targets for tgARN exists in cache, and contain the target",
			fields: fields{
				targetsCache: map[string][]TargetInfo{
					"my-tg": {
						{
							Target: elbv2sdk.TargetDescription{
								Id:   awssdk.String("192.168.1.1"),
								Port: awssdk.Int64(8080),
							},
							TargetHealth: &elbv2sdk.TargetHealth{
								Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
								State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
							},
						},
					},
				},
			},
			args: args{
				tgARN: "my-tg",
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			wantTargetsCache: map[string][]TargetInfo{
				"my-tg": {
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
					},
				},
			},
		},
		{
			name: "targets for tgARN exists in cache, but don't contain the target",
			fields: fields{
				targetsCache: map[string][]TargetInfo{
					"my-tg": {
						{
							Target: elbv2sdk.TargetDescription{
								Id:   awssdk.String("192.168.1.1"),
								Port: awssdk.Int64(8080),
							},
							TargetHealth: &elbv2sdk.TargetHealth{
								Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
								State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
							},
						},
					},
				},
			},
			args: args{
				tgARN: "my-tg",
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			wantTargetsCache: map[string][]TargetInfo{
				"my-tg": {
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
							State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
						},
					},
				},
			},
		},
		{
			name: "targets for tgARN exists in cache, and contains multiple targets",
			fields: fields{
				targetsCache: map[string][]TargetInfo{
					"my-tg": {
						{
							Target: elbv2sdk.TargetDescription{
								Id:   awssdk.String("192.168.1.1"),
								Port: awssdk.Int64(8080),
							},
							TargetHealth: &elbv2sdk.TargetHealth{
								Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
								State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
							},
						},
						{
							Target: elbv2sdk.TargetDescription{
								Id:   awssdk.String("192.168.1.2"),
								Port: awssdk.Int64(8080),
							},
							TargetHealth: &elbv2sdk.TargetHealth{
								Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
								State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
							},
						},
					},
				},
			},
			args: args{
				tgARN: "my-tg",
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			wantTargetsCache: map[string][]TargetInfo{
				"my-tg": {
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
							State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
						},
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.2"),
							Port: awssdk.Int64(8080),
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetsCache := cache.NewExpiring()
			targetsCacheTTL := 1 * time.Minute
			for tgARN, targets := range tt.fields.targetsCache {
				targetsCache.Set(tgARN, &targetsCacheItem{
					mutex:   sync.RWMutex{},
					targets: targets,
				}, targetsCacheTTL)
			}

			m := &cachedTargetsManager{
				targetsCache:      targetsCache,
				targetsCacheMutex: sync.RWMutex{},
			}
			m.recordSuccessfulDeregisterTargetsOperation(tt.args.tgARN, tt.args.targets)
			assert.Equal(t, len(tt.wantTargetsCache), targetsCache.Len())
			for tgARN, targets := range tt.wantTargetsCache {
				rawTargetsCacheItem, exists := targetsCache.Get(tgARN)
				assert.True(t, exists)
				targetsCacheItem := rawTargetsCacheItem.(*targetsCacheItem)
				assert.ElementsMatch(t, targets, targetsCacheItem.targets)
			}
		})
	}
}

func Test_chunkTargetDescriptions(t *testing.T) {
	type args struct {
		targets   []elbv2sdk.TargetDescription
		chunkSize int
	}
	tests := []struct {
		name string
		args args
		want [][]elbv2sdk.TargetDescription
	}{
		{
			name: "can be evenly chunked",
			args: args{
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.4"),
						Port: awssdk.Int64(8080),
					},
				},
				chunkSize: 2,
			},
			want: [][]elbv2sdk.TargetDescription{
				{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
				},
				{
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.4"),
						Port: awssdk.Int64(8080),
					},
				},
			},
		},
		{
			name: "cannot be evenly chunked",
			args: args{
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.4"),
						Port: awssdk.Int64(8080),
					},
				},
				chunkSize: 3,
			},
			want: [][]elbv2sdk.TargetDescription{
				{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
				},
				{

					{
						Id:   awssdk.String("192.168.1.4"),
						Port: awssdk.Int64(8080),
					},
				},
			},
		},
		{
			name: "chunkSize equal to total count",
			args: args{
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.4"),
						Port: awssdk.Int64(8080),
					},
				},
				chunkSize: 4,
			},
			want: [][]elbv2sdk.TargetDescription{
				{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.4"),
						Port: awssdk.Int64(8080),
					},
				},
			},
		},
		{
			name: "chunkSize greater than total count",
			args: args{
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.4"),
						Port: awssdk.Int64(8080),
					},
				},
				chunkSize: 10,
			},
			want: [][]elbv2sdk.TargetDescription{
				{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.3"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.4"),
						Port: awssdk.Int64(8080),
					},
				},
			},
		},
		{
			name: "chunk nil slice",
			args: args{
				targets:   nil,
				chunkSize: 2,
			},
			want: nil,
		},
		{
			name: "chunk empty slice",
			args: args{
				targets:   []elbv2sdk.TargetDescription{},
				chunkSize: 2,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunkTargetDescriptions(tt.args.targets, tt.args.chunkSize)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_pointerizeTargetDescriptions(t *testing.T) {
	type args struct {
		targets []elbv2sdk.TargetDescription
	}
	tests := []struct {
		name string
		args args
		want []*elbv2sdk.TargetDescription
	}{
		{
			name: "nil targets",
			args: args{
				targets: nil,
			},
			want: nil,
		},
		{
			name: "empty targets",
			args: args{
				targets: []elbv2sdk.TargetDescription{},
			},
			want: nil,
		},
		{
			name: "non-empty targets",
			args: args{
				targets: []elbv2sdk.TargetDescription{
					{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
				},
			},
			want: []*elbv2sdk.TargetDescription{
				{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int64(8080),
				},
				{
					Id:   awssdk.String("192.168.1.2"),
					Port: awssdk.Int64(8080),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pointerizeTargetDescriptions(tt.args.targets)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_cloneTargetInfoSlice(t *testing.T) {
	type args struct {
		targets []TargetInfo
	}
	tests := []struct {
		name string
		args args
		want []TargetInfo
	}{
		{
			name: "nil targets",
			args: args{
				targets: nil,
			},
			want: nil,
		},
		{
			name: "empty targets",
			args: args{
				targets: []TargetInfo{},
			},
			want: []TargetInfo{},
		},
		{
			name: "non-empty targets",
			args: args{
				targets: []TargetInfo{
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.1"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: nil,
					},
					{
						Target: elbv2sdk.TargetDescription{
							Id:   awssdk.String("192.168.1.2"),
							Port: awssdk.Int64(8080),
						},
						TargetHealth: &elbv2sdk.TargetHealth{
							Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
							State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
						},
					},
				},
			},
			want: []TargetInfo{
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.1"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: nil,
				},
				{
					Target: elbv2sdk.TargetDescription{
						Id:   awssdk.String("192.168.1.2"),
						Port: awssdk.Int64(8080),
					},
					TargetHealth: &elbv2sdk.TargetHealth{
						Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
						State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cloneTargetInfoSlice(tt.args.targets)
			assert.Equal(t, tt.want, got)
		})
	}
}
