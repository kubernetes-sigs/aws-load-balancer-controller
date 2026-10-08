package shared_utils

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/services"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
)

func Test_validateMutualAuthenticationConfig(t *testing.T) {
	tests := []struct {
		name                 string
		protocol             elbv2model.Protocol
		port                 int32
		mode                 string
		trustStoreARN        string
		ignoreClientCert     *bool
		advertiseCANames     *string
		expectedErrorMessage *string
	}{
		{
			name:     "happy path no validation error off mode",
			protocol: elbv2model.ProtocolHTTPS,
			port:     800,
			mode:     string(elbv2model.MutualAuthenticationOffMode),
		},
		{
			name:     "happy path no validation error pass through mode",
			protocol: elbv2model.ProtocolHTTPS,
			port:     800,
			mode:     string(elbv2model.MutualAuthenticationPassthroughMode),
		},
		{
			name:          "happy path no validation error verify mode",
			protocol:      elbv2model.ProtocolHTTPS,
			port:          800,
			mode:          string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN: "truststore",
		},
		{
			name:             "happy path no validation error verify mode, with ignore client cert expiry",
			protocol:         elbv2model.ProtocolHTTPS,
			port:             800,
			mode:             string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN:    "truststore",
			ignoreClientCert: awssdk.Bool(true),
		},
		{
			name:             "happy path no validation error verify mode, with ignore client cert expiry false",
			protocol:         elbv2model.ProtocolHTTPS,
			port:             800,
			mode:             string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN:    "truststore",
			ignoreClientCert: awssdk.Bool(false),
		},
		{
			name:             "happy path no validation error verify mode, with advertise ca on",
			protocol:         elbv2model.ProtocolHTTPS,
			port:             800,
			mode:             string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN:    "truststore",
			advertiseCANames: awssdk.String("on"),
		},
		{
			name:             "happy path no validation error verify mode, with advertise ca off",
			protocol:         elbv2model.ProtocolHTTPS,
			port:             800,
			mode:             string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN:    "truststore",
			advertiseCANames: awssdk.String("off"),
		},
		{
			name:                 "no mode",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			expectedErrorMessage: awssdk.String("mutualAuthentication mode cannot be empty for port 800"),
		},
		{
			name:     "unknown mode",
			protocol: elbv2model.ProtocolHTTPS,

			port:                 800,
			mode:                 "foo",
			expectedErrorMessage: awssdk.String("mutualAuthentication mode value must be among"),
		},
		{
			name:                 "port invalid",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800000,
			mode:                 string(elbv2model.MutualAuthenticationOffMode),
			expectedErrorMessage: awssdk.String("listen port must be within [1, 65535]: 800000"),
		},
		{
			name:                 "missing truststore arn for verify",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationVerifyMode),
			expectedErrorMessage: awssdk.String("trustStore is required when mutualAuthentication mode is verify for port 800"),
		},
		{
			name:                 "truststore arn set but mode not verify",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationOffMode),
			trustStoreARN:        "truststore",
			expectedErrorMessage: awssdk.String("Mutual Authentication mode off does not support trustStore for port 800"),
		},
		{
			name:                 "ignore client cert expiry set for off mode",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationOffMode),
			ignoreClientCert:     awssdk.Bool(true),
			expectedErrorMessage: awssdk.String("Mutual Authentication mode off does not support ignoring client certificate expiry for port 800"),
		},
		{
			name:                 "ignore client cert expiry set for passthrough mode",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationPassthroughMode),
			ignoreClientCert:     awssdk.Bool(true),
			expectedErrorMessage: awssdk.String("Mutual Authentication mode passthrough does not support ignoring client certificate expiry for port 800"),
		},
		{
			name:                 "advertise ca set for off mode",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationOffMode),
			advertiseCANames:     awssdk.String("on"),
			expectedErrorMessage: awssdk.String("Authentication mode off does not support advertiseTrustStoreCaNames for port 800"),
		},
		{
			name:                 "advertise ca set for passthrough mode",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationPassthroughMode),
			advertiseCANames:     awssdk.String("on"),
			expectedErrorMessage: awssdk.String("Authentication mode passthrough does not support advertiseTrustStoreCaNames for port 800"),
		},
		{
			name:                 "advertise ca set with invalid value",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN:        "truststore",
			advertiseCANames:     awssdk.String("foo"),
			expectedErrorMessage: awssdk.String("advertiseTrustStoreCaNames only supports the values \"on\" and \"off\" got value foo for port 800"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := validateMutualAuthenticationConfig(tt.protocol, tt.port, tt.mode, tt.trustStoreARN, tt.ignoreClientCert, tt.advertiseCANames)

			if tt.expectedErrorMessage == nil {
				assert.Nil(t, res)
			} else {
				assert.Contains(t, res.Error(), *tt.expectedErrorMessage)
			}
		})
	}
}

func Test_ParseMtlsConfigEntries(t *testing.T) {
	// Helper to reset the package-level trust store ARN cache between cases,
	// since GetTrustStoreArnFromName caches resolved names.
	resetTrustStoreARNCache := func() {
		trustStoreARNCacheMutex.Lock()
		defer trustStoreARNCacheMutex.Unlock()
		trustStoreARNCache = cache.NewExpiring()
	}

	const resolvedArn = "arn:aws:elasticloadbalancing:us-east-1:123456789:truststore/my-trust-store/a00e9da691864a58"

	tests := []struct {
		name       string
		entries    []MutualAuthenticationConfig
		protocol   elbv2model.Protocol
		setupMocks func(elbv2Client *services.MockELBV2)
		want       map[int32]*elbv2model.MutualAuthenticationAttributes
		wantErr    bool
		errorMsg   string
	}{
		{
			name:     "off mode entry, no trust store resolution",
			protocol: elbv2model.ProtocolHTTPS,
			entries: []MutualAuthenticationConfig{
				{
					Port: 800,
					Mode: string(elbv2model.MutualAuthenticationOffMode),
				},
			},
			setupMocks: func(elbv2Client *services.MockELBV2) {},
			want: map[int32]*elbv2model.MutualAuthenticationAttributes{
				800: {
					Mode:          string(elbv2model.MutualAuthenticationOffMode),
					TrustStoreArn: awssdk.String(""),
				},
			},
			wantErr: false,
		},
		{
			name:     "passthrough mode entry, no trust store resolution",
			protocol: elbv2model.ProtocolHTTPS,
			entries: []MutualAuthenticationConfig{
				{
					Port: 801,
					Mode: string(elbv2model.MutualAuthenticationPassthroughMode),
				},
			},
			setupMocks: func(elbv2Client *services.MockELBV2) {},
			want: map[int32]*elbv2model.MutualAuthenticationAttributes{
				801: {
					Mode:          string(elbv2model.MutualAuthenticationPassthroughMode),
					TrustStoreArn: awssdk.String(""),
				},
			},
			wantErr: false,
		},
		{
			name:     "verify mode with trust store ARN, no resolution needed",
			protocol: elbv2model.ProtocolHTTPS,
			entries: []MutualAuthenticationConfig{
				{
					Port:       802,
					Mode:       string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStore: awssdk.String(resolvedArn),
				},
			},
			setupMocks: func(elbv2Client *services.MockELBV2) {},
			want: map[int32]*elbv2model.MutualAuthenticationAttributes{
				802: {
					Mode:                          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStoreArn:                 awssdk.String(resolvedArn),
					IgnoreClientCertificateExpiry: awssdk.Bool(false),
				},
			},
			wantErr: false,
		},
		{
			name:     "verify mode with trust store name, resolves to ARN",
			protocol: elbv2model.ProtocolHTTPS,
			entries: []MutualAuthenticationConfig{
				{
					Port:       803,
					Mode:       string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStore: awssdk.String("my-trust-store"),
				},
			},
			setupMocks: func(elbv2Client *services.MockELBV2) {
				elbv2Client.EXPECT().DescribeTrustStoresWithContext(
					context.Background(),
					&elbv2sdk.DescribeTrustStoresInput{
						Names: []string{"my-trust-store"},
					},
				).Return(&elbv2sdk.DescribeTrustStoresOutput{
					TrustStores: []elbv2types.TrustStore{
						{
							Name:          awssdk.String("my-trust-store"),
							TrustStoreArn: awssdk.String(resolvedArn),
						},
					},
				}, nil)
			},
			want: map[int32]*elbv2model.MutualAuthenticationAttributes{
				803: {
					Mode:                          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStoreArn:                 awssdk.String(resolvedArn),
					IgnoreClientCertificateExpiry: awssdk.Bool(false),
				},
			},
			wantErr: false,
		},
		{
			name:     "verify mode with explicit ignore client cert and advertise ca names preserved",
			protocol: elbv2model.ProtocolHTTPS,
			entries: []MutualAuthenticationConfig{
				{
					Port:                          804,
					Mode:                          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStore:                    awssdk.String(resolvedArn),
					IgnoreClientCertificateExpiry: awssdk.Bool(true),
					AdvertiseTrustStoreCaNames:    awssdk.String("on"),
				},
			},
			setupMocks: func(elbv2Client *services.MockELBV2) {},
			want: map[int32]*elbv2model.MutualAuthenticationAttributes{
				804: {
					Mode:                          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStoreArn:                 awssdk.String(resolvedArn),
					IgnoreClientCertificateExpiry: awssdk.Bool(true),
					AdvertiseTrustStoreCaNames:    awssdk.String("on"),
				},
			},
			wantErr: false,
		},
		{
			name:       "empty entries returns empty map",
			protocol:   elbv2model.ProtocolHTTPS,
			entries:    []MutualAuthenticationConfig{},
			setupMocks: func(elbv2Client *services.MockELBV2) {},
			want:       map[int32]*elbv2model.MutualAuthenticationAttributes{},
			wantErr:    false,
		},
		{
			name:     "validation error is propagated",
			protocol: elbv2model.ProtocolHTTPS,
			entries: []MutualAuthenticationConfig{
				{
					Port: 805,
					Mode: string(elbv2model.MutualAuthenticationVerifyMode),
				},
			},
			setupMocks: func(elbv2Client *services.MockELBV2) {},
			want:       nil,
			wantErr:    true,
			errorMsg:   "trustStore is required when mutualAuthentication mode is verify for port 805",
		},
		{
			name:     "trust store resolution error is propagated",
			protocol: elbv2model.ProtocolHTTPS,
			entries: []MutualAuthenticationConfig{
				{
					Port:       806,
					Mode:       string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStore: awssdk.String("missing-store"),
				},
			},
			setupMocks: func(elbv2Client *services.MockELBV2) {
				elbv2Client.EXPECT().DescribeTrustStoresWithContext(
					context.Background(),
					&elbv2sdk.DescribeTrustStoresInput{
						Names: []string{"missing-store"},
					},
				).Return(nil, errors.New("API error"))
			},
			want:     nil,
			wantErr:  true,
			errorMsg: "API error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			resetTrustStoreARNCache()

			elbv2Client := services.NewMockELBV2(ctrl)
			tt.setupMocks(elbv2Client)

			got, err := ParseMtlsConfigEntries(context.Background(), elbv2Client, tt.entries, tt.protocol)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_parseMtlsAttributesForTrustStoreNames(t *testing.T) {
	resetTrustStoreARNCache := func() {
		trustStoreARNCacheMutex.Lock()
		defer trustStoreARNCacheMutex.Unlock()
		trustStoreARNCache = cache.NewExpiring()
	}

	const storeArn = "arn:aws:elasticloadbalancing:us-east-1:123456789:truststore/store1/a00e9da691864a58"
	const storeArn2 = "arn:aws:elasticloadbalancing:us-east-1:123456789:truststore/store2/b11e9da691864a58"

	tests := []struct {
		name       string
		input      map[int32]*elbv2model.MutualAuthenticationAttributes
		setupMocks func(elbv2Client *services.MockELBV2)
		want       map[int32]*elbv2model.MutualAuthenticationAttributes
		wantErr    bool
		errorMsg   string
	}{
		{
			name: "verify mode with name resolves to ARN",
			input: map[int32]*elbv2model.MutualAuthenticationAttributes{
				800: {
					Mode:          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStoreArn: awssdk.String("store1"),
				},
			},
			setupMocks: func(elbv2Client *services.MockELBV2) {
				elbv2Client.EXPECT().DescribeTrustStoresWithContext(
					context.Background(),
					&elbv2sdk.DescribeTrustStoresInput{
						Names: []string{"store1"},
					},
				).Return(&elbv2sdk.DescribeTrustStoresOutput{
					TrustStores: []elbv2types.TrustStore{
						{
							Name:          awssdk.String("store1"),
							TrustStoreArn: awssdk.String(storeArn),
						},
					},
				}, nil)
			},
			want: map[int32]*elbv2model.MutualAuthenticationAttributes{
				800: {
					Mode:          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStoreArn: awssdk.String(storeArn),
				},
			},
			wantErr: false,
		},
		{
			name: "verify mode already an ARN is left untouched",
			input: map[int32]*elbv2model.MutualAuthenticationAttributes{
				800: {
					Mode:          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStoreArn: awssdk.String(storeArn),
				},
			},
			setupMocks: func(elbv2Client *services.MockELBV2) {},
			want: map[int32]*elbv2model.MutualAuthenticationAttributes{
				800: {
					Mode:          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStoreArn: awssdk.String(storeArn),
				},
			},
			wantErr: false,
		},
		{
			name: "off mode is not resolved",
			input: map[int32]*elbv2model.MutualAuthenticationAttributes{
				800: {
					Mode:          string(elbv2model.MutualAuthenticationOffMode),
					TrustStoreArn: awssdk.String(""),
				},
			},
			setupMocks: func(elbv2Client *services.MockELBV2) {},
			want: map[int32]*elbv2model.MutualAuthenticationAttributes{
				800: {
					Mode:          string(elbv2model.MutualAuthenticationOffMode),
					TrustStoreArn: awssdk.String(""),
				},
			},
			wantErr: false,
		},
		{
			name: "multiple ports with distinct names resolve independently",
			input: map[int32]*elbv2model.MutualAuthenticationAttributes{
				800: {
					Mode:          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStoreArn: awssdk.String("store1"),
				},
				801: {
					Mode:          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStoreArn: awssdk.String("store2"),
				},
			},
			setupMocks: func(elbv2Client *services.MockELBV2) {
				elbv2Client.EXPECT().DescribeTrustStoresWithContext(
					context.Background(),
					gomock.Any(),
				).DoAndReturn(func(_ context.Context, input *elbv2sdk.DescribeTrustStoresInput) (*elbv2sdk.DescribeTrustStoresOutput, error) {
					trustStores := make([]elbv2types.TrustStore, 0, len(input.Names))
					for _, name := range input.Names {
						switch name {
						case "store1":
							trustStores = append(trustStores, elbv2types.TrustStore{
								Name:          awssdk.String("store1"),
								TrustStoreArn: awssdk.String(storeArn),
							})
						case "store2":
							trustStores = append(trustStores, elbv2types.TrustStore{
								Name:          awssdk.String("store2"),
								TrustStoreArn: awssdk.String(storeArn2),
							})
						}
					}
					return &elbv2sdk.DescribeTrustStoresOutput{TrustStores: trustStores}, nil
				}).AnyTimes()
			},
			want: map[int32]*elbv2model.MutualAuthenticationAttributes{
				800: {
					Mode:          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStoreArn: awssdk.String(storeArn),
				},
				801: {
					Mode:          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStoreArn: awssdk.String(storeArn2),
				},
			},
			wantErr: false,
		},
		{
			name:       "empty map returns empty map",
			input:      map[int32]*elbv2model.MutualAuthenticationAttributes{},
			setupMocks: func(elbv2Client *services.MockELBV2) {},
			want:       map[int32]*elbv2model.MutualAuthenticationAttributes{},
			wantErr:    false,
		},
		{
			name: "resolution error is propagated",
			input: map[int32]*elbv2model.MutualAuthenticationAttributes{
				800: {
					Mode:          string(elbv2model.MutualAuthenticationVerifyMode),
					TrustStoreArn: awssdk.String("missing-store"),
				},
			},
			setupMocks: func(elbv2Client *services.MockELBV2) {
				elbv2Client.EXPECT().DescribeTrustStoresWithContext(
					context.Background(),
					&elbv2sdk.DescribeTrustStoresInput{
						Names: []string{"missing-store"},
					},
				).Return(nil, errors.New("API error"))
			},
			want:     nil,
			wantErr:  true,
			errorMsg: "API error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			resetTrustStoreARNCache()

			elbv2Client := services.NewMockELBV2(ctrl)
			tt.setupMocks(elbv2Client)

			got, err := parseMtlsAttributesForTrustStoreNames(context.Background(), elbv2Client, tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
