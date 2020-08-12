package targetgroupbinding

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/cache"
	"sync"
	"testing"
	"time"
)

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
				targetsCache: targetsCache,
			}
			m.recordSuccessfulRegisterTargetsOperation(tt.args.tgARN, tt.args.targets)
			assert.Equal(t, len(tt.wantTargetsCache), targetsCache.Len())
			for tgARN, targets := range tt.wantTargetsCache {
				rawTargetsCacheItem, exists := targetsCache.Get(tgARN)
				assert.True(t, exists)
				targetsCacheItem := rawTargetsCacheItem.(*targetsCacheItem)
				assert.Equal(t, targets, targetsCacheItem.targets)
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
				targetsCache: targetsCache,
			}
			m.recordSuccessfulDeregisterTargetsOperation(tt.args.tgARN, tt.args.targets)
			assert.Equal(t, len(tt.wantTargetsCache), targetsCache.Len())
			for tgARN, targets := range tt.wantTargetsCache {
				rawTargetsCacheItem, exists := targetsCache.Get(tgARN)
				assert.True(t, exists)
				targetsCacheItem := rawTargetsCacheItem.(*targetsCacheItem)
				assert.Equal(t, targets, targetsCacheItem.targets)
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
