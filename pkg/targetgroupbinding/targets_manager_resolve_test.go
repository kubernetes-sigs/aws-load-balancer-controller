package targetgroupbinding

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

func Test_cachedTargetsManager_resolveELBV2(t *testing.T) {
	tests := []struct {
		name           string
		defaultRegion  string
		tgARN          string
		hasProvider    bool
		providerRegion string
		providerErr    error
		wantDefault    bool
		wantErr        string
	}{
		{
			name:          "same region ARN returns default client",
			defaultRegion: "us-east-1",
			tgARN:         "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/tg/abc",
			hasProvider:   true,
			wantDefault:   true,
		},
		{
			name:           "different region ARN returns provider client",
			defaultRegion:  "us-east-1",
			tgARN:          "arn:aws:elasticloadbalancing:ap-northeast-1:025054649006:targetgroup/tg/abc",
			hasProvider:    true,
			providerRegion: "ap-northeast-1",
			wantDefault:    false,
		},
		{
			name:          "empty ARN returns default client",
			defaultRegion: "us-east-1",
			tgARN:         "",
			hasProvider:   true,
			wantDefault:   true,
		},
		{
			name:          "nil provider returns default client for cross-region ARN",
			defaultRegion: "us-east-1",
			tgARN:         "arn:aws:elasticloadbalancing:ap-northeast-1:025054649006:targetgroup/tg/abc",
			hasProvider:   false,
			wantDefault:   true,
		},
		{
			name:           "provider error is propagated",
			defaultRegion:  "us-east-1",
			tgARN:          "arn:aws:elasticloadbalancing:eu-west-1:111111111111:targetgroup/tg/abc",
			hasProvider:    true,
			providerRegion: "eu-west-1",
			providerErr:    errors.New("region not supported"),
			wantErr:        "region not supported",
		},
		{
			name:          "malformed ARN returns default client",
			defaultRegion: "us-east-1",
			tgARN:         "not-an-arn",
			hasProvider:   true,
			wantDefault:   true,
		},
		{
			name:          "ARN with empty region field returns default client",
			defaultRegion: "us-east-1",
			tgARN:         "arn:aws:elasticloadbalancing::123456789012:targetgroup/tg/abc",
			hasProvider:   true,
			wantDefault:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			defaultClient := services.NewMockELBV2(ctrl)
			providerClient := services.NewMockELBV2(ctrl)

			var provider ELBV2ClientProvider
			if tt.hasProvider {
				provider = func(region string) (services.ELBV2, error) {
					if tt.providerRegion != "" {
						assert.Equal(t, tt.providerRegion, region)
					}
					if tt.providerErr != nil {
						return nil, tt.providerErr
					}
					return providerClient, nil
				}
			}

			m := &cachedTargetsManager{
				elbv2Client:   defaultClient,
				defaultRegion: tt.defaultRegion,
				elbv2Provider: provider,
			}

			got, err := m.resolveELBV2(tt.tgARN)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				if tt.wantDefault {
					assert.Equal(t, defaultClient, got)
				} else {
					assert.Equal(t, providerClient, got)
				}
			}
		})
	}
}
