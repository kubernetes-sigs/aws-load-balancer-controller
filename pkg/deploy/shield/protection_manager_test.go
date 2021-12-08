package shield

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	shieldsdk "github.com/aws/aws-sdk-go/service/shield"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
	"time"
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
							SubscriptionState: awssdk.String(shieldsdk.SubscriptionStateActive),
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
							SubscriptionState: awssdk.String(shieldsdk.SubscriptionStateInactive),
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
							SubscriptionState: awssdk.String(shieldsdk.SubscriptionStateInactive),
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
							SubscriptionState: awssdk.String(shieldsdk.SubscriptionStateInactive),
						},
					},
					{
						req: &shieldsdk.GetSubscriptionStateInput{},
						resp: &shieldsdk.GetSubscriptionStateOutput{
							SubscriptionState: awssdk.String(shieldsdk.SubscriptionStateActive),
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
				logger:                    &log.NullLogger{},
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
