package shield

import (
	"context"
	"testing"
	"time"

	shieldtypes "github.com/aws/aws-sdk-go-v2/service/shield/types"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/controller-runtime/pkg/log"
	shieldsdk "github.com/aws/aws-sdk-go-v2/service/shield"
)

func Test_defaultProtectionManager_IsSubscribed(t *testing.T) {
	type getSubscriptionStateCall struct {
		req  *shieldsdk.GetSubscriptionStateInput
		resp *shieldsdk.GetSubscriptionStateOutput
		err  error
	}
	type fields struct {
		subscriptionStateCacheTTL time.Duration
		getSubscriptionStateCalls []getSubscriptionStateCall
	}
	type isSubscribedCall struct {
		want    bool
		wantErr error
	}
	tests := []struct {
		name              string
		fields            fields
		isSubscribedCalls []isSubscribedCall
	}{
		{
			name: "invoke isSubscribed once without cache - subscriptionState == ACTIVE",
			fields: fields{
				subscriptionStateCacheTTL: 2 * time.Hour,
				getSubscriptionStateCalls: []getSubscriptionStateCall{
					{
						req: &shieldsdk.GetSubscriptionStateInput{},
						resp: &shieldsdk.GetSubscriptionStateOutput{
							SubscriptionState: shieldtypes.SubscriptionStateActive,
						},
					},
				},
			},
			isSubscribedCalls: []isSubscribedCall{
				{
					want: true,
				},
			},
		},
		{
			name: "invoke isSubscribed once without cache - subscriptionState == INACTIVE",
			fields: fields{
				subscriptionStateCacheTTL: 2 * time.Hour,
				getSubscriptionStateCalls: []getSubscriptionStateCall{
					{
						req: &shieldsdk.GetSubscriptionStateInput{},
						resp: &shieldsdk.GetSubscriptionStateOutput{
							SubscriptionState: shieldtypes.SubscriptionStateInactive,
						},
					},
				},
			},
			isSubscribedCalls: []isSubscribedCall{
				{
					want: false,
				},
			},
		},
		{
			name: "invoke isSubscribed once without cache - AWS API error",
			fields: fields{
				subscriptionStateCacheTTL: 2 * time.Hour,
				getSubscriptionStateCalls: []getSubscriptionStateCall{
					{
						req: &shieldsdk.GetSubscriptionStateInput{},
						err: errors.New("some aws api error"),
					},
				},
			},
			isSubscribedCalls: []isSubscribedCall{
				{
					wantErr: errors.New("some aws api error"),
				},
			},
		},
		{
			name: "invoke isSubscribed twice with cache - two call within cacheTTL",
			fields: fields{
				subscriptionStateCacheTTL: 2 * time.Hour,
				getSubscriptionStateCalls: []getSubscriptionStateCall{
					{
						req: &shieldsdk.GetSubscriptionStateInput{},
						resp: &shieldsdk.GetSubscriptionStateOutput{
							SubscriptionState: shieldtypes.SubscriptionStateInactive,
						},
					},
				},
			},
			isSubscribedCalls: []isSubscribedCall{
				{
					want: false,
				},
				{
					want: false,
				},
			},
		},
		{
			name: "invoke isSubscribed twice with cache - two call beyond cacheTTL",
			fields: fields{
				subscriptionStateCacheTTL: 0,
				getSubscriptionStateCalls: []getSubscriptionStateCall{
					{
						req: &shieldsdk.GetSubscriptionStateInput{},
						resp: &shieldsdk.GetSubscriptionStateOutput{
							SubscriptionState: shieldtypes.SubscriptionStateInactive,
						},
					},
					{
						req: &shieldsdk.GetSubscriptionStateInput{},
						resp: &shieldsdk.GetSubscriptionStateOutput{
							SubscriptionState: shieldtypes.SubscriptionStateActive,
						},
					},
				},
			},
			isSubscribedCalls: []isSubscribedCall{
				{
					want: false,
				},
				{
					want: true,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			shieldClient := services.NewMockShield(ctrl)
			for _, call := range tt.fields.getSubscriptionStateCalls {
				shieldClient.EXPECT().GetSubscriptionStateWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			m := &defaultProtectionManager{
				shieldClient:              shieldClient,
				logger:                    logr.New(&log.NullLogSink{}),
				subscriptionStateCache:    cache.NewExpiring(),
				subscriptionStateCacheTTL: tt.fields.subscriptionStateCacheTTL,
			}
			for _, call := range tt.isSubscribedCalls {
				got, err := m.IsSubscribed(context.Background())
				if call.wantErr != nil {
					assert.EqualError(t, err, call.wantErr.Error())
				} else {
					assert.NoError(t, err)
					assert.Equal(t, call.want, got)
				}
			}
		})
	}
}

func Test_defaultProtectionManager_DeleteProtection(t *testing.T) {
	type deleteProtectionCall struct {
		req  *shieldsdk.DeleteProtectionInput
		resp *shieldsdk.DeleteProtectionOutput
		err  error
	}
	type describeProtectionCall struct {
		req  *shieldsdk.DescribeProtectionInput
		resp *shieldsdk.DescribeProtectionOutput
		err  error
	}
	type fields struct {
		deleteProtectionCalls      []deleteProtectionCall
		describeProtectionCalls     []describeProtectionCall
		protectionInfoByResourceARNCacheTTL time.Duration
	}
	type testCase struct {
		resourceARN string
		protectionID string
		wantErr      error
	}
	tests := []struct {
		name                string
		fields              fields
		testCases           []testCase
	}{
		{
			name: "delete protection successfully",
			fields: fields{
				deleteProtectionCalls: []deleteProtectionCall{
					{
						req:  &shieldsdk.DeleteProtectionInput{ProtectionId: aws.String("protection-id")},
						resp: &shieldsdk.DeleteProtectionOutput{},
					},
				},
				describeProtectionCalls: []describeProtectionCall{
					{
						req:  &shieldsdk.DescribeProtectionInput{ProtectionId: aws.String("protection-id")},
						err:  &shieldtypes.ResourceNotFoundException{},
					},
				},
				protectionInfoByResourceARNCacheTTL: 10 * time.Minute,
			},
			testCases: []testCase{
				{
					resourceARN: "resource-arn",
					protectionID: "protection-id",
				},
			},
		},
		{
			name: "delete protection fails",
			fields: fields{
				deleteProtectionCalls: []deleteProtectionCall{
					{
						req: &shieldsdk.DeleteProtectionInput{ProtectionId: aws.String("protection-id")},
						err: errors.New("some aws api error"),
					},
				},
				protectionInfoByResourceARNCacheTTL: 10 * time.Minute,
			},
			testCases: []testCase{
				{
					resourceARN: "resource-arn",
					protectionID: "protection-id",
					wantErr:      errors.New("some aws api error"),
				},
			},
		},
		{
			name: "protection still exists after deletion",
			fields: fields{
				deleteProtectionCalls: []deleteProtectionCall{
					{
						req:  &shieldsdk.DeleteProtectionInput{ProtectionId: aws.String("protection-id")},
						resp: &shieldsdk.DeleteProtectionOutput{},
					},
				},
				describeProtectionCalls: []describeProtectionCall{
					{
						req:  &shieldsdk.DescribeProtectionInput{ProtectionId: aws.String("protection-id")},
						resp: &shieldsdk.DescribeProtectionOutput{Protection: &shieldtypes.Protection{}},
					},
				},
				protectionInfoByResourceARNCacheTTL: 10 * time.Minute,
			},
			testCases: []testCase{
				{
					resourceARN: "resource-arn",
					protectionID: "protection-id",
					wantErr:      errors.New("protection resource still exists"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			shieldClient := services.NewMockShield(ctrl)
			for _, call := range tt.fields.deleteProtectionCalls {
				shieldClient.EXPECT().DeleteProtectionWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			for _, call := range tt.fields.describeProtectionCalls {
				shieldClient.EXPECT().DescribeProtectionWithContext(gomock.Any(), call.req).Return(call.resp, call.err)
			}

			m := &defaultProtectionManager{
				shieldClient:                    shieldClient,
				logger:                          logr.New(&log.NullLogSink{}),
				protectionInfoByResourceARNCache: cache.NewExpiring(),
				protectionInfoByResourceARNCacheTTL: tt.fields.protectionInfoByResourceARNCacheTTL,
			}
			for _, testCase := range tt.testCases {
				err := m.DeleteProtection(context.Background(), testCase.resourceARN, testCase.protectionID)
				if testCase.wantErr != nil {
					assert.EqualError(t, err, testCase.wantErr.Error())
				} else {
					assert.NoError(t, err)
				}
			}
		})
	}
}
