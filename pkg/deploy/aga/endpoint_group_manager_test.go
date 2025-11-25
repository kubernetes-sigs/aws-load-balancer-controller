package aga

import (
	"context"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	agatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
)

func Test_defaultEndpointGroupManager_buildSDKPortOverrides(t *testing.T) {
	tests := []struct {
		name               string
		modelPortOverrides []agamodel.PortOverride
		want               []agatypes.PortOverride
	}{
		{
			name:               "nil model port overrides",
			modelPortOverrides: nil,
			want:               nil,
		},
		{
			name:               "empty model port overrides",
			modelPortOverrides: []agamodel.PortOverride{},
			want:               nil,
		},
		{
			name: "single port override",
			modelPortOverrides: []agamodel.PortOverride{
				{
					ListenerPort: 80,
					EndpointPort: 8080,
				},
			},
			want: []agatypes.PortOverride{
				{
					ListenerPort: aws.Int32(80),
					EndpointPort: aws.Int32(8080),
				},
			},
		},
		{
			name: "multiple port overrides",
			modelPortOverrides: []agamodel.PortOverride{
				{
					ListenerPort: 80,
					EndpointPort: 8080,
				},
				{
					ListenerPort: 443,
					EndpointPort: 8443,
				},
			},
			want: []agatypes.PortOverride{
				{
					ListenerPort: aws.Int32(80),
					EndpointPort: aws.Int32(8080),
				},
				{
					ListenerPort: aws.Int32(443),
					EndpointPort: aws.Int32(8443),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logr.Discard()
			m := &defaultEndpointGroupManager{
				logger: logger,
			}
			got := m.buildSDKPortOverrides(tt.modelPortOverrides)

			// Compare nil vs nil
			if tt.want == nil && got == nil {
				// Both are nil, this is correct
				return
			}

			// Compare lengths
			assert.Equal(t, len(tt.want), len(got))

			// Compare individual elements
			for i, wantPO := range tt.want {
				assert.Equal(t, awssdk.ToInt32(wantPO.ListenerPort), awssdk.ToInt32(got[i].ListenerPort))
				assert.Equal(t, awssdk.ToInt32(wantPO.EndpointPort), awssdk.ToInt32(got[i].EndpointPort))
			}
		})
	}
}

func Test_defaultEndpointGroupManager_arePortOverridesEqual(t *testing.T) {
	tests := []struct {
		name               string
		modelPortOverrides []agamodel.PortOverride
		sdkPortOverrides   []agatypes.PortOverride
		want               bool
	}{
		{
			name:               "both nil - equal",
			modelPortOverrides: nil,
			sdkPortOverrides:   nil,
			want:               true,
		},
		{
			name:               "one empty one nil - equal",
			modelPortOverrides: nil,
			sdkPortOverrides:   []agatypes.PortOverride{},
			want:               true,
		},
		{
			name:               "both empty - equal",
			modelPortOverrides: []agamodel.PortOverride{},
			sdkPortOverrides:   []agatypes.PortOverride{},
			want:               true,
		},
		{
			name:               "model empty, sdk not empty - not equal",
			modelPortOverrides: []agamodel.PortOverride{},
			sdkPortOverrides: []agatypes.PortOverride{
				{
					ListenerPort: aws.Int32(80),
					EndpointPort: aws.Int32(8080),
				},
			},
			want: false,
		},
		{
			name: "model not empty, sdk empty - not equal",
			modelPortOverrides: []agamodel.PortOverride{
				{
					ListenerPort: 80,
					EndpointPort: 8080,
				},
			},
			sdkPortOverrides: []agatypes.PortOverride{},
			want:             false,
		},
		{
			name: "different lengths - not equal",
			modelPortOverrides: []agamodel.PortOverride{
				{
					ListenerPort: 80,
					EndpointPort: 8080,
				},
			},
			sdkPortOverrides: []agatypes.PortOverride{
				{
					ListenerPort: aws.Int32(80),
					EndpointPort: aws.Int32(8080),
				},
				{
					ListenerPort: aws.Int32(443),
					EndpointPort: aws.Int32(8443),
				},
			},
			want: false,
		},
		{
			name: "same length but different values - not equal",
			modelPortOverrides: []agamodel.PortOverride{
				{
					ListenerPort: 80,
					EndpointPort: 8080,
				},
			},
			sdkPortOverrides: []agatypes.PortOverride{
				{
					ListenerPort: aws.Int32(80),
					EndpointPort: aws.Int32(9090), // Different endpoint port
				},
			},
			want: false,
		},
		{
			name: "same values, same order - equal",
			modelPortOverrides: []agamodel.PortOverride{
				{
					ListenerPort: 80,
					EndpointPort: 8080,
				},
				{
					ListenerPort: 443,
					EndpointPort: 8443,
				},
			},
			sdkPortOverrides: []agatypes.PortOverride{
				{
					ListenerPort: aws.Int32(80),
					EndpointPort: aws.Int32(8080),
				},
				{
					ListenerPort: aws.Int32(443),
					EndpointPort: aws.Int32(8443),
				},
			},
			want: true,
		},
		{
			name: "same values, different order - equal (order doesn't matter)",
			modelPortOverrides: []agamodel.PortOverride{
				{
					ListenerPort: 443,
					EndpointPort: 8443,
				},
				{
					ListenerPort: 80,
					EndpointPort: 8080,
				},
			},
			sdkPortOverrides: []agatypes.PortOverride{
				{
					ListenerPort: aws.Int32(80),
					EndpointPort: aws.Int32(8080),
				},
				{
					ListenerPort: aws.Int32(443),
					EndpointPort: aws.Int32(8443),
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logr.Discard()
			m := &defaultEndpointGroupManager{
				logger: logger,
			}
			got := m.arePortOverridesEqual(tt.modelPortOverrides, tt.sdkPortOverrides)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultEndpointGroupManager_isSDKEndpointGroupSettingsDrifted(t *testing.T) {
	tests := []struct {
		name             string
		resEndpointGroup *agamodel.EndpointGroup
		sdkEndpointGroup *agatypes.EndpointGroup
		want             bool
	}{
		{
			name: "no drift - all values match",
			resEndpointGroup: &agamodel.EndpointGroup{
				Spec: agamodel.EndpointGroupSpec{
					Region:                "us-west-2",
					TrafficDialPercentage: aws.Int32(100),
					PortOverrides: []agamodel.PortOverride{
						{
							ListenerPort: 80,
							EndpointPort: 8080,
						},
					},
				},
			},
			sdkEndpointGroup: &agatypes.EndpointGroup{
				EndpointGroupRegion:   aws.String("us-west-2"),
				TrafficDialPercentage: aws.Float32(100.0),
				PortOverrides: []agatypes.PortOverride{
					{
						ListenerPort: aws.Int32(80),
						EndpointPort: aws.Int32(8080),
					},
				},
			},
			want: false,
		},
		{
			name: "traffic dial percentage differs",
			resEndpointGroup: &agamodel.EndpointGroup{
				Spec: agamodel.EndpointGroupSpec{
					Region:                "us-west-2",
					TrafficDialPercentage: aws.Int32(50),
					PortOverrides: []agamodel.PortOverride{
						{
							ListenerPort: 80,
							EndpointPort: 8080,
						},
					},
				},
			},
			sdkEndpointGroup: &agatypes.EndpointGroup{
				EndpointGroupRegion:   aws.String("us-west-2"),
				TrafficDialPercentage: aws.Float32(100.0),
				PortOverrides: []agatypes.PortOverride{
					{
						ListenerPort: aws.Int32(80),
						EndpointPort: aws.Int32(8080),
					},
				},
			},
			want: true,
		},
		{
			name: "traffic dial percentage small difference within epsilon - no drift",
			resEndpointGroup: &agamodel.EndpointGroup{
				Spec: agamodel.EndpointGroupSpec{
					Region:                "us-west-2",
					TrafficDialPercentage: aws.Int32(100),
					PortOverrides: []agamodel.PortOverride{
						{
							ListenerPort: 80,
							EndpointPort: 8080,
						},
					},
				},
			},
			sdkEndpointGroup: &agatypes.EndpointGroup{
				EndpointGroupRegion:   aws.String("us-west-2"),
				TrafficDialPercentage: aws.Float32(100.0005), // Small difference within epsilon
				PortOverrides: []agatypes.PortOverride{
					{
						ListenerPort: aws.Int32(80),
						EndpointPort: aws.Int32(8080),
					},
				},
			},
			want: false,
		},
		{
			name: "port overrides differ",
			resEndpointGroup: &agamodel.EndpointGroup{
				Spec: agamodel.EndpointGroupSpec{
					Region:                "us-west-2",
					TrafficDialPercentage: aws.Int32(100),
					PortOverrides: []agamodel.PortOverride{
						{
							ListenerPort: 80,
							EndpointPort: 8080,
						},
					},
				},
			},
			sdkEndpointGroup: &agatypes.EndpointGroup{
				EndpointGroupRegion:   aws.String("us-west-2"),
				TrafficDialPercentage: aws.Float32(100.0),
				PortOverrides: []agatypes.PortOverride{
					{
						ListenerPort: aws.Int32(80),
						EndpointPort: aws.Int32(9090), // Different endpoint port
					},
				},
			},
			want: true,
		},
		{
			name: "model has no traffic dial percentage, sdk does - drift",
			resEndpointGroup: &agamodel.EndpointGroup{
				Spec: agamodel.EndpointGroupSpec{
					Region: "us-west-2",
					PortOverrides: []agamodel.PortOverride{
						{
							ListenerPort: 80,
							EndpointPort: 8080,
						},
					},
				},
			},
			sdkEndpointGroup: &agatypes.EndpointGroup{
				EndpointGroupRegion:   aws.String("us-west-2"),
				TrafficDialPercentage: aws.Float32(100.0),
				PortOverrides: []agatypes.PortOverride{
					{
						ListenerPort: aws.Int32(80),
						EndpointPort: aws.Int32(8080),
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logr.Discard()
			m := &defaultEndpointGroupManager{
				logger: logger,
			}
			got := m.isSDKEndpointGroupSettingsDrifted(tt.resEndpointGroup, tt.sdkEndpointGroup)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultEndpointGroupManager_buildSDKCreateEndpointGroupInput(t *testing.T) {
	testListenerARN := "arn:aws:globalaccelerator::123456789012:listener/1234abcd-abcd-1234-abcd-1234abcdefgh/abcdefghi"
	mockStack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	tests := []struct {
		name             string
		resEndpointGroup *agamodel.EndpointGroup
		want             *globalaccelerator.CreateEndpointGroupInput
		wantErr          bool
	}{
		{
			name: "Standard endpoint group with all fields",
			resEndpointGroup: &agamodel.EndpointGroup{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::EndpointGroup", "endpoint-group-1"),
				Spec: agamodel.EndpointGroupSpec{
					ListenerARN:           core.LiteralStringToken(testListenerARN),
					Region:                "us-west-2",
					TrafficDialPercentage: aws.Int32(85),
					PortOverrides: []agamodel.PortOverride{
						{
							ListenerPort: 80,
							EndpointPort: 8080,
						},
						{
							ListenerPort: 443,
							EndpointPort: 8443,
						},
					},
				},
			},
			want: &globalaccelerator.CreateEndpointGroupInput{
				ListenerArn:           aws.String(testListenerARN),
				EndpointGroupRegion:   aws.String("us-west-2"),
				TrafficDialPercentage: aws.Float32(85.0),
				PortOverrides: []agatypes.PortOverride{
					{
						ListenerPort: aws.Int32(80),
						EndpointPort: aws.Int32(8080),
					},
					{
						ListenerPort: aws.Int32(443),
						EndpointPort: aws.Int32(8443),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Minimal endpoint group",
			resEndpointGroup: &agamodel.EndpointGroup{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::EndpointGroup", "endpoint-group-2"),
				Spec: agamodel.EndpointGroupSpec{
					ListenerARN: core.LiteralStringToken(testListenerARN),
					Region:      "us-west-2",
					// No TrafficDialPercentage or PortOverrides
				},
			},
			want: &globalaccelerator.CreateEndpointGroupInput{
				ListenerArn:         aws.String(testListenerARN),
				EndpointGroupRegion: aws.String("us-west-2"),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create endpoint group manager
			m := &defaultEndpointGroupManager{
				gaService: nil, // Not needed for this test
				logger:    logr.Discard(),
			}

			// Call the method being tested
			got, err := m.buildSDKCreateEndpointGroupInput(context.Background(), tt.resEndpointGroup)

			// Check if error status matches expected
			if (err != nil) != tt.wantErr {
				t.Errorf("buildSDKCreateEndpointGroupInput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check if the result matches expected
			assert.Equal(t, tt.want, got)
		})
	}
}
