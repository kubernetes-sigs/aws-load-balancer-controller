package shared_utils

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"testing"
)

func Test_GetTrustStoreArnFromName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	elbv2Client := services.NewMockELBV2(ctrl)

	// Helper function to reset cache between tests
	resetTrustStoreARNCache := func() {
		trustStoreARNCacheMutex.Lock()
		defer trustStoreARNCacheMutex.Unlock()
		trustStoreARNCache = cache.NewExpiring()
	}

	// Reset cache before running tests
	resetTrustStoreARNCache()

	tests := []struct {
		name           string
		trustStoreName []string
		setupMocks     func()
		want           map[string]*string
		wantErr        bool
		errorMsg       string
	}{
		{
			name:           "Successfully get trust store ARN",
			trustStoreName: []string{"my-trust-store"},
			setupMocks: func() {
				elbv2Client.EXPECT().DescribeTrustStoresWithContext(
					context.Background(),
					&elbv2sdk.DescribeTrustStoresInput{
						Names: []string{"my-trust-store"},
					},
				).Return(&elbv2sdk.DescribeTrustStoresOutput{
					TrustStores: []elbv2types.TrustStore{
						{
							Name:          aws.String("my-trust-store"),
							TrustStoreArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789:truststore/my-trust-store/a00e9da691864a58"),
						},
					},
				}, nil)
			},
			want: map[string]*string{
				"my-trust-store": aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789:truststore/my-trust-store/a00e9da691864a58"),
			},
			wantErr: false,
		},
		{
			name:           "Trust store not found",
			trustStoreName: []string{"non-existent-store"},
			setupMocks: func() {
				elbv2Client.EXPECT().DescribeTrustStoresWithContext(
					context.Background(),
					&elbv2sdk.DescribeTrustStoresInput{
						Names: []string{"non-existent-store"},
					},
				).Return(&elbv2sdk.DescribeTrustStoresOutput{
					TrustStores: []elbv2types.TrustStore{},
				}, nil)
			},
			want:     map[string]*string{},
			wantErr:  true,
			errorMsg: "couldn't find TrustStores with names [non-existent-store]",
		},
		{
			name:           "API error",
			trustStoreName: []string{"my-trust-store"},
			setupMocks: func() {
				elbv2Client.EXPECT().DescribeTrustStoresWithContext(
					context.Background(),
					&elbv2sdk.DescribeTrustStoresInput{
						Names: []string{"my-trust-store"},
					},
				).Return(nil, errors.New("API error"))
			},
			want:     map[string]*string{},
			wantErr:  true,
			errorMsg: "API error",
		},
		{
			name:           "Multiple trust stores",
			trustStoreName: []string{"store1", "store2"},
			setupMocks: func() {
				elbv2Client.EXPECT().DescribeTrustStoresWithContext(
					context.Background(),
					&elbv2sdk.DescribeTrustStoresInput{
						Names: []string{"store1", "store2"},
					},
				).Return(&elbv2sdk.DescribeTrustStoresOutput{
					TrustStores: []elbv2types.TrustStore{
						{
							Name:          aws.String("store1"),
							TrustStoreArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789:truststore/store1/a00e9da691864a58"),
						},
						{
							Name:          aws.String("store2"),
							TrustStoreArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789:truststore/store2/b11e9da691864a58"),
						},
					},
				}, nil)
			},
			want: map[string]*string{
				"store1": aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789:truststore/store1/a00e9da691864a58"),
				"store2": aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789:truststore/store2/b11e9da691864a58"),
			},
			wantErr: false,
		},
		{
			name:           "Cached result is returned",
			trustStoreName: []string{"cached-store"},
			setupMocks: func() {
				// First call to cache the result
				elbv2Client.EXPECT().DescribeTrustStoresWithContext(
					context.Background(),
					&elbv2sdk.DescribeTrustStoresInput{
						Names: []string{"cached-store"},
					},
				).Return(&elbv2sdk.DescribeTrustStoresOutput{
					TrustStores: []elbv2types.TrustStore{
						{
							Name:          aws.String("cached-store"),
							TrustStoreArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789:truststore/cached-store/c00e9da691864a58"),
						},
					},
				}, nil).Times(1)

				// Prime the cache
				result, err := GetTrustStoreArnFromName(context.Background(), elbv2Client, []string{"cached-store"})
				assert.NoError(t, err)
				assert.NotNil(t, result["cached-store"])

				// No second API call expected - will use cached value
			},
			want: map[string]*string{
				"cached-store": aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789:truststore/cached-store/c00e9da691864a58"),
			},
			wantErr: false,
		},
		{
			name:           "Partial cache hit",
			trustStoreName: []string{"cached-store", "new-store"},
			setupMocks: func() {
				// First cache "cached-store"
				elbv2Client.EXPECT().DescribeTrustStoresWithContext(
					context.Background(),
					&elbv2sdk.DescribeTrustStoresInput{
						Names: []string{"cached-store"},
					},
				).Return(&elbv2sdk.DescribeTrustStoresOutput{
					TrustStores: []elbv2types.TrustStore{
						{
							Name:          aws.String("cached-store"),
							TrustStoreArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789:truststore/cached-store/c00e9da691864a58"),
						},
					},
				}, nil).Times(1)

				// Prime the cache
				_, _ = GetTrustStoreArnFromName(context.Background(), elbv2Client, []string{"cached-store"})

				// Then expect API call only for the new store
				elbv2Client.EXPECT().DescribeTrustStoresWithContext(
					context.Background(),
					&elbv2sdk.DescribeTrustStoresInput{
						Names: []string{"new-store"},
					},
				).Return(&elbv2sdk.DescribeTrustStoresOutput{
					TrustStores: []elbv2types.TrustStore{
						{
							Name:          aws.String("new-store"),
							TrustStoreArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789:truststore/new-store/d00e9da691864a58"),
						},
					},
				}, nil).Times(1)
			},
			want: map[string]*string{
				"cached-store": aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789:truststore/cached-store/c00e9da691864a58"),
				"new-store":    aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789:truststore/new-store/d00e9da691864a58"),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset cache for each test case
			resetTrustStoreARNCache()

			tt.setupMocks()

			got, err := GetTrustStoreArnFromName(context.Background(), elbv2Client, tt.trustStoreName)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errorMsg, err.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
