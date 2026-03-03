package elbv2

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

func Test_regionFromTGARN(t *testing.T) {
	tests := []struct {
		name string
		arn  string
		want string
	}{
		{
			name: "standard TG ARN",
			arn:  "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/my-tg/abc123",
			want: "us-east-1",
		},
		{
			name: "tokyo region TG ARN",
			arn:  "arn:aws:elasticloadbalancing:ap-northeast-1:025054649006:targetgroup/k8s-kubesyst-traefikt-1cdbaaab9f/964ac6392723fe3a",
			want: "ap-northeast-1",
		},
		{
			name: "eu-west-1 region",
			arn:  "arn:aws:elasticloadbalancing:eu-west-1:111111111111:targetgroup/tg-name/deadbeef",
			want: "eu-west-1",
		},
		{
			name: "empty ARN",
			arn:  "",
			want: "",
		},
		{
			name: "malformed ARN with fewer colons",
			arn:  "arn:aws:elasticloadbalancing",
			want: "",
		},
		{
			name: "ARN with exactly 4 parts",
			arn:  "arn:aws:elasticloadbalancing:us-west-2",
			want: "",
		},
		{
			name: "ARN with 5 parts (minimum for region extraction)",
			arn:  "arn:aws:elasticloadbalancing:us-west-2:123456789012",
			want: "us-west-2",
		},
		{
			name: "china partition ARN",
			arn:  "arn:aws-cn:elasticloadbalancing:cn-north-1:123456789012:targetgroup/tg/abc",
			want: "cn-north-1",
		},
		{
			name: "govcloud ARN",
			arn:  "arn:aws-us-gov:elasticloadbalancing:us-gov-west-1:123456789012:targetgroup/tg/abc",
			want: "us-gov-west-1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := regionFromTGARN(tt.arn)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_resolveELBV2ForTGB(t *testing.T) {
	tests := []struct {
		name            string
		defaultRegion   string
		tgARN           string
		providerRegion  string
		providerReturns bool
		providerErr     error
		wantDefault     bool
		wantErr         string
	}{
		{
			name:          "empty ARN returns default client",
			defaultRegion: "us-east-1",
			tgARN:         "",
			wantDefault:   true,
		},
		{
			name:          "same region returns default client",
			defaultRegion: "us-east-1",
			tgARN:         "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/tg/abc",
			wantDefault:   true,
		},
		{
			name:            "different region returns provider client",
			defaultRegion:   "us-east-1",
			tgARN:           "arn:aws:elasticloadbalancing:ap-northeast-1:123456789012:targetgroup/tg/abc",
			providerRegion:  "ap-northeast-1",
			providerReturns: true,
			wantDefault:     false,
		},
		{
			name:           "provider error is propagated",
			defaultRegion:  "us-east-1",
			tgARN:          "arn:aws:elasticloadbalancing:eu-west-1:123456789012:targetgroup/tg/abc",
			providerRegion: "eu-west-1",
			providerErr:    errors.New("failed to create client"),
			wantErr:        "failed to create client",
		},
		{
			name:          "nil provider returns default client even for different region",
			defaultRegion: "us-east-1",
			tgARN:         "arn:aws:elasticloadbalancing:ap-northeast-1:123456789012:targetgroup/tg/abc",
			wantDefault:   true,
		},
		{
			name:          "malformed ARN returns default client",
			defaultRegion: "us-east-1",
			tgARN:         "not-an-arn",
			wantDefault:   true,
		},
		{
			name:          "ARN with empty region returns default client",
			defaultRegion: "us-east-1",
			tgARN:         "arn:aws:elasticloadbalancing::123456789012:targetgroup/tg/abc",
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
			if tt.providerRegion != "" {
				provider = func(region string) (services.ELBV2, error) {
					assert.Equal(t, tt.providerRegion, region)
					if tt.providerErr != nil {
						return nil, tt.providerErr
					}
					return providerClient, nil
				}
			}

			got, err := resolveELBV2ForTGB(defaultClient, tt.defaultRegion, provider, tt.tgARN)
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
