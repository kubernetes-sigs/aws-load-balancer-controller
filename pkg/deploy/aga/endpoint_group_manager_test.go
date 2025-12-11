package aga

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sort"
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

func Test_defaultEndpointGroupManager_isEndpointConfigurationDrifted(t *testing.T) {
	tests := []struct {
		name             string
		desiredConfig    agamodel.EndpointConfiguration
		existingEndpoint agatypes.EndpointDescription
		want             bool
	}{
		{
			name: "no drift - both weight and client IP preservation nil",
			desiredConfig: agamodel.EndpointConfiguration{
				EndpointID:                  "endpoint-1",
				Weight:                      nil,
				ClientIPPreservationEnabled: nil,
			},
			existingEndpoint: agatypes.EndpointDescription{
				EndpointId:                  aws.String("endpoint-1"),
				Weight:                      nil,
				ClientIPPreservationEnabled: nil,
			},
			want: false,
		},
		{
			name: "weight drift - desired nil, existing not nil",
			desiredConfig: agamodel.EndpointConfiguration{
				EndpointID:                  "endpoint-1",
				Weight:                      nil,
				ClientIPPreservationEnabled: nil,
			},
			existingEndpoint: agatypes.EndpointDescription{
				EndpointId:                  aws.String("endpoint-1"),
				Weight:                      aws.Int32(100),
				ClientIPPreservationEnabled: nil,
			},
			want: true,
		},
		{
			name: "weight drift - desired not nil, existing nil",
			desiredConfig: agamodel.EndpointConfiguration{
				EndpointID:                  "endpoint-1",
				Weight:                      aws.Int32(100),
				ClientIPPreservationEnabled: nil,
			},
			existingEndpoint: agatypes.EndpointDescription{
				EndpointId:                  aws.String("endpoint-1"),
				Weight:                      nil,
				ClientIPPreservationEnabled: nil,
			},
			want: true,
		},
		{
			name: "weight drift - both not nil but different values",
			desiredConfig: agamodel.EndpointConfiguration{
				EndpointID:                  "endpoint-1",
				Weight:                      aws.Int32(80),
				ClientIPPreservationEnabled: nil,
			},
			existingEndpoint: agatypes.EndpointDescription{
				EndpointId:                  aws.String("endpoint-1"),
				Weight:                      aws.Int32(100),
				ClientIPPreservationEnabled: nil,
			},
			want: true,
		},
		{
			name: "no weight drift - both not nil with same values",
			desiredConfig: agamodel.EndpointConfiguration{
				EndpointID:                  "endpoint-1",
				Weight:                      aws.Int32(100),
				ClientIPPreservationEnabled: nil,
			},
			existingEndpoint: agatypes.EndpointDescription{
				EndpointId:                  aws.String("endpoint-1"),
				Weight:                      aws.Int32(100),
				ClientIPPreservationEnabled: nil,
			},
			want: false,
		},
		{
			name: "client IP preservation drift - desired nil, existing not nil",
			desiredConfig: agamodel.EndpointConfiguration{
				EndpointID:                  "endpoint-1",
				Weight:                      nil,
				ClientIPPreservationEnabled: nil,
			},
			existingEndpoint: agatypes.EndpointDescription{
				EndpointId:                  aws.String("endpoint-1"),
				Weight:                      nil,
				ClientIPPreservationEnabled: aws.Bool(true),
			},
			want: true,
		},
		{
			name: "client IP preservation drift - desired not nil, existing nil",
			desiredConfig: agamodel.EndpointConfiguration{
				EndpointID:                  "endpoint-1",
				Weight:                      nil,
				ClientIPPreservationEnabled: aws.Bool(true),
			},
			existingEndpoint: agatypes.EndpointDescription{
				EndpointId:                  aws.String("endpoint-1"),
				Weight:                      nil,
				ClientIPPreservationEnabled: nil,
			},
			want: true,
		},
		{
			name: "client IP preservation drift - both not nil but different values (true vs false)",
			desiredConfig: agamodel.EndpointConfiguration{
				EndpointID:                  "endpoint-1",
				Weight:                      nil,
				ClientIPPreservationEnabled: aws.Bool(true),
			},
			existingEndpoint: agatypes.EndpointDescription{
				EndpointId:                  aws.String("endpoint-1"),
				Weight:                      nil,
				ClientIPPreservationEnabled: aws.Bool(false),
			},
			want: true,
		},
		{
			name: "client IP preservation drift - both not nil but different values (false vs true)",
			desiredConfig: agamodel.EndpointConfiguration{
				EndpointID:                  "endpoint-1",
				Weight:                      nil,
				ClientIPPreservationEnabled: aws.Bool(false),
			},
			existingEndpoint: agatypes.EndpointDescription{
				EndpointId:                  aws.String("endpoint-1"),
				Weight:                      nil,
				ClientIPPreservationEnabled: aws.Bool(true),
			},
			want: true,
		},
		{
			name: "no client IP preservation drift - both not nil with same values (both true)",
			desiredConfig: agamodel.EndpointConfiguration{
				EndpointID:                  "endpoint-1",
				Weight:                      nil,
				ClientIPPreservationEnabled: aws.Bool(true),
			},
			existingEndpoint: agatypes.EndpointDescription{
				EndpointId:                  aws.String("endpoint-1"),
				Weight:                      nil,
				ClientIPPreservationEnabled: aws.Bool(true),
			},
			want: false,
		},
		{
			name: "no client IP preservation drift - both not nil with same values (both false)",
			desiredConfig: agamodel.EndpointConfiguration{
				EndpointID:                  "endpoint-1",
				Weight:                      nil,
				ClientIPPreservationEnabled: aws.Bool(false),
			},
			existingEndpoint: agatypes.EndpointDescription{
				EndpointId:                  aws.String("endpoint-1"),
				Weight:                      nil,
				ClientIPPreservationEnabled: aws.Bool(false),
			},
			want: false,
		},
		{
			name: "drift in both weight and client IP preservation",
			desiredConfig: agamodel.EndpointConfiguration{
				EndpointID:                  "endpoint-1",
				Weight:                      aws.Int32(80),
				ClientIPPreservationEnabled: aws.Bool(true),
			},
			existingEndpoint: agatypes.EndpointDescription{
				EndpointId:                  aws.String("endpoint-1"),
				Weight:                      aws.Int32(100),
				ClientIPPreservationEnabled: aws.Bool(false),
			},
			want: true,
		},
		{
			name: "no drift - both weight and client IP preservation have same non-nil values",
			desiredConfig: agamodel.EndpointConfiguration{
				EndpointID:                  "endpoint-1",
				Weight:                      aws.Int32(100),
				ClientIPPreservationEnabled: aws.Bool(true),
			},
			existingEndpoint: agatypes.EndpointDescription{
				EndpointId:                  aws.String("endpoint-1"),
				Weight:                      aws.Int32(100),
				ClientIPPreservationEnabled: aws.Bool(true),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logr.Discard()
			m := &defaultEndpointGroupManager{
				logger: logger,
			}
			got := m.isEndpointConfigurationDrifted(tt.desiredConfig, tt.existingEndpoint)
			assert.Equal(t, tt.want, got)
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

func Test_defaultEndpointGroupManager_detectEndpointDrift(t *testing.T) {
	tests := []struct {
		name                  string
		existingEndpoints     []agatypes.EndpointDescription
		desiredConfigs        []agamodel.EndpointConfiguration
		wantConfigsToAdd      []agamodel.EndpointConfiguration
		wantConfigsToUpdate   []agamodel.EndpointConfiguration
		wantEndpointsToRemove []string
		wantIsUpdateRequired  bool
	}{
		{
			name:                  "no endpoints - empty lists",
			existingEndpoints:     []agatypes.EndpointDescription{},
			desiredConfigs:        []agamodel.EndpointConfiguration{},
			wantConfigsToAdd:      []agamodel.EndpointConfiguration{},
			wantConfigsToUpdate:   []agamodel.EndpointConfiguration{},
			wantEndpointsToRemove: []string{},
			wantIsUpdateRequired:  false,
		},
		{
			name:              "add new endpoint - no existing endpoints",
			existingEndpoints: []agatypes.EndpointDescription{},
			desiredConfigs: []agamodel.EndpointConfiguration{
				{
					EndpointID: "endpoint-1",
					Weight:     aws.Int32(100),
				},
			},
			wantConfigsToAdd: []agamodel.EndpointConfiguration{
				{
					EndpointID: "endpoint-1",
					Weight:     aws.Int32(100),
				},
			},
			wantConfigsToUpdate:   []agamodel.EndpointConfiguration{},
			wantEndpointsToRemove: []string{},
			wantIsUpdateRequired:  false,
		},
		{
			name: "remove endpoint - no desired endpoints",
			existingEndpoints: []agatypes.EndpointDescription{
				{
					EndpointId: aws.String("endpoint-1"),
				},
			},
			desiredConfigs:        []agamodel.EndpointConfiguration{},
			wantConfigsToAdd:      []agamodel.EndpointConfiguration{},
			wantConfigsToUpdate:   []agamodel.EndpointConfiguration{},
			wantEndpointsToRemove: []string{"endpoint-1"},
			wantIsUpdateRequired:  false,
		},
		{
			name: "no change - same endpoints with same configuration",
			existingEndpoints: []agatypes.EndpointDescription{
				{
					EndpointId:                  aws.String("endpoint-1"),
					Weight:                      aws.Int32(100),
					ClientIPPreservationEnabled: aws.Bool(true),
				},
			},
			desiredConfigs: []agamodel.EndpointConfiguration{
				{
					EndpointID:                  "endpoint-1",
					Weight:                      aws.Int32(100),
					ClientIPPreservationEnabled: aws.Bool(true),
				},
			},
			wantConfigsToAdd: []agamodel.EndpointConfiguration{},
			wantConfigsToUpdate: []agamodel.EndpointConfiguration{
				{
					EndpointID:                  "endpoint-1",
					Weight:                      aws.Int32(100),
					ClientIPPreservationEnabled: aws.Bool(true),
				},
			},
			wantEndpointsToRemove: []string{},
			wantIsUpdateRequired:  false,
		},
		{
			name: "update endpoint - weight drift",
			existingEndpoints: []agatypes.EndpointDescription{
				{
					EndpointId:                  aws.String("endpoint-1"),
					Weight:                      aws.Int32(80),
					ClientIPPreservationEnabled: aws.Bool(true),
				},
			},
			desiredConfigs: []agamodel.EndpointConfiguration{
				{
					EndpointID:                  "endpoint-1",
					Weight:                      aws.Int32(100),
					ClientIPPreservationEnabled: aws.Bool(true),
				},
			},
			wantConfigsToAdd: []agamodel.EndpointConfiguration{},
			wantConfigsToUpdate: []agamodel.EndpointConfiguration{
				{
					EndpointID:                  "endpoint-1",
					Weight:                      aws.Int32(100),
					ClientIPPreservationEnabled: aws.Bool(true),
				},
			},
			wantEndpointsToRemove: []string{},
			wantIsUpdateRequired:  true,
		},
		{
			name: "update endpoint - client IP preservation drift",
			existingEndpoints: []agatypes.EndpointDescription{
				{
					EndpointId:                  aws.String("endpoint-1"),
					Weight:                      aws.Int32(100),
					ClientIPPreservationEnabled: aws.Bool(false),
				},
			},
			desiredConfigs: []agamodel.EndpointConfiguration{
				{
					EndpointID:                  "endpoint-1",
					Weight:                      aws.Int32(100),
					ClientIPPreservationEnabled: aws.Bool(true),
				},
			},
			wantConfigsToAdd: []agamodel.EndpointConfiguration{},
			wantConfigsToUpdate: []agamodel.EndpointConfiguration{
				{
					EndpointID:                  "endpoint-1",
					Weight:                      aws.Int32(100),
					ClientIPPreservationEnabled: aws.Bool(true),
				},
			},
			wantEndpointsToRemove: []string{},
			wantIsUpdateRequired:  true,
		},
		{
			name: "multiple actions - add, update, remove endpoints",
			existingEndpoints: []agatypes.EndpointDescription{
				{
					EndpointId: aws.String("endpoint-1"),
					Weight:     aws.Int32(80),
				},
				{
					EndpointId:                  aws.String("endpoint-2"),
					Weight:                      aws.Int32(100),
					ClientIPPreservationEnabled: aws.Bool(false),
				},
				{
					EndpointId: aws.String("endpoint-to-remove"),
					Weight:     aws.Int32(50),
				},
			},
			desiredConfigs: []agamodel.EndpointConfiguration{
				{
					EndpointID: "endpoint-1",
					Weight:     aws.Int32(100), // Changed weight
				},
				{
					EndpointID:                  "endpoint-2",
					Weight:                      aws.Int32(100), // No change
					ClientIPPreservationEnabled: aws.Bool(false),
				},
				{
					EndpointID: "endpoint-new", // New endpoint
					Weight:     aws.Int32(100),
				},
			},
			wantConfigsToAdd: []agamodel.EndpointConfiguration{
				{
					EndpointID: "endpoint-new",
					Weight:     aws.Int32(100),
				},
			},
			wantConfigsToUpdate: []agamodel.EndpointConfiguration{
				{
					EndpointID: "endpoint-1",
					Weight:     aws.Int32(100),
				},
				{
					EndpointID:                  "endpoint-2",
					Weight:                      aws.Int32(100),
					ClientIPPreservationEnabled: aws.Bool(false),
				},
			},
			wantEndpointsToRemove: []string{"endpoint-to-remove"},
			wantIsUpdateRequired:  true, // Because endpoint-1 weight changed
		},
		{
			name: "nil endpoint ID in existing endpoints",
			existingEndpoints: []agatypes.EndpointDescription{
				{
					EndpointId: nil, // Should be skipped
					Weight:     aws.Int32(80),
				},
				{
					EndpointId: aws.String("endpoint-2"),
					Weight:     aws.Int32(100),
				},
			},
			desiredConfigs: []agamodel.EndpointConfiguration{
				{
					EndpointID: "endpoint-2",
					Weight:     aws.Int32(100),
				},
			},
			wantConfigsToAdd: []agamodel.EndpointConfiguration{},
			wantConfigsToUpdate: []agamodel.EndpointConfiguration{
				{
					EndpointID: "endpoint-2",
					Weight:     aws.Int32(100),
				},
			},
			wantEndpointsToRemove: []string{},
			wantIsUpdateRequired:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logr.Discard()
			m := &defaultEndpointGroupManager{
				logger: logger,
			}
			gotConfigsToAdd, gotConfigsToUpdate, gotEndpointsToRemove, gotIsUpdateRequired := m.detectEndpointDrift(tt.existingEndpoints, tt.desiredConfigs)

			// Sort slices for deterministic comparison
			sort.Slice(gotConfigsToAdd, func(i, j int) bool {
				return gotConfigsToAdd[i].EndpointID < gotConfigsToAdd[j].EndpointID
			})
			sort.Slice(tt.wantConfigsToAdd, func(i, j int) bool {
				return tt.wantConfigsToAdd[i].EndpointID < tt.wantConfigsToAdd[j].EndpointID
			})
			sort.Slice(gotConfigsToUpdate, func(i, j int) bool {
				return gotConfigsToUpdate[i].EndpointID < gotConfigsToUpdate[j].EndpointID
			})
			sort.Slice(tt.wantConfigsToUpdate, func(i, j int) bool {
				return tt.wantConfigsToUpdate[i].EndpointID < tt.wantConfigsToUpdate[j].EndpointID
			})
			sort.Strings(gotEndpointsToRemove)
			sort.Strings(tt.wantEndpointsToRemove)

			// Check if configsToAdd matches expected
			assert.Equal(t, len(tt.wantConfigsToAdd), len(gotConfigsToAdd), "configsToAdd length mismatch")
			for i, config := range tt.wantConfigsToAdd {
				if i < len(gotConfigsToAdd) {
					assert.Equal(t, config.EndpointID, gotConfigsToAdd[i].EndpointID, "EndpointID mismatch")

					// Check Weight
					if config.Weight == nil {
						assert.Nil(t, gotConfigsToAdd[i].Weight, "Weight should be nil")
					} else if gotConfigsToAdd[i].Weight != nil {
						assert.Equal(t, *config.Weight, *gotConfigsToAdd[i].Weight, "Weight value mismatch")
					}

					// Check ClientIPPreservationEnabled
					if config.ClientIPPreservationEnabled == nil {
						assert.Nil(t, gotConfigsToAdd[i].ClientIPPreservationEnabled, "ClientIPPreservationEnabled should be nil")
					} else if gotConfigsToAdd[i].ClientIPPreservationEnabled != nil {
						assert.Equal(t, *config.ClientIPPreservationEnabled, *gotConfigsToAdd[i].ClientIPPreservationEnabled, "ClientIPPreservationEnabled value mismatch")
					}
				}
			}

			// Check if configsToUpdate matches expected
			assert.Equal(t, len(tt.wantConfigsToUpdate), len(gotConfigsToUpdate), "configsToUpdate length mismatch")
			for i, config := range tt.wantConfigsToUpdate {
				if i < len(gotConfigsToUpdate) {
					assert.Equal(t, config.EndpointID, gotConfigsToUpdate[i].EndpointID, "EndpointID mismatch")

					// Check Weight
					if config.Weight == nil {
						assert.Nil(t, gotConfigsToUpdate[i].Weight, "Weight should be nil")
					} else if gotConfigsToUpdate[i].Weight != nil {
						assert.Equal(t, *config.Weight, *gotConfigsToUpdate[i].Weight, "Weight value mismatch")
					}

					// Check ClientIPPreservationEnabled
					if config.ClientIPPreservationEnabled == nil {
						assert.Nil(t, gotConfigsToUpdate[i].ClientIPPreservationEnabled, "ClientIPPreservationEnabled should be nil")
					} else if gotConfigsToUpdate[i].ClientIPPreservationEnabled != nil {
						assert.Equal(t, *config.ClientIPPreservationEnabled, *gotConfigsToUpdate[i].ClientIPPreservationEnabled, "ClientIPPreservationEnabled value mismatch")
					}
				}
			}

			// Check if endpointsToRemove matches expected
			assert.Equal(t, tt.wantEndpointsToRemove, gotEndpointsToRemove, "endpointsToRemove mismatch")

			// Check if isUpdateRequired matches expected
			assert.Equal(t, tt.wantIsUpdateRequired, gotIsUpdateRequired, "isUpdateRequired mismatch")
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

func Test_ManageEndpoints(t *testing.T) {
	testCases := []struct {
		name                      string
		endpointGroupARN          string
		currentEndpoints          []agatypes.EndpointDescription
		loadedEndpoints           []*aga.LoadedEndpoint
		expectError               bool
		describeEndpointErr       error
		addEndpointsErr           error
		addEndpointsErrOnFirstTry bool // For testing limit exceeded with flip-flop pattern
		removeEndpointsErr        error
		expectAddCall             bool
		expectRemoveCall          bool
		expectUpdateCall          bool // Whether to expect update-endpoint-group API call due to property drift
		expectFlipFlopPattern     bool // Whether to expect flip-flop delete-create pattern
	}{
		{
			name:             "limit exceeded - flip-flop pattern",
			endpointGroupARN: "arn:aws:globalaccelerator::123456789012:accelerator/abcd/listener/l-1234/endpoint-group/eg-1234",
			currentEndpoints: []agatypes.EndpointDescription{
				{
					EndpointId: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/existing-lb1/1111111111"),
				},
				{
					EndpointId: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/existing-lb2/2222222222"),
				},
			},
			loadedEndpoints: []*aga.LoadedEndpoint{
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeService,
					Name:      "new-service-1",
					Namespace: "default",
					Weight:    100,
					ARN:       "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/new-lb1/3333333333",
					Status:    aga.EndpointStatusLoaded,
				},
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeService,
					Name:      "new-service-2",
					Namespace: "default",
					Weight:    100,
					ARN:       "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/new-lb2/4444444444",
					Status:    aga.EndpointStatusLoaded,
				},
			},
			addEndpointsErrOnFirstTry: true, // First AddEndpoints call fails with LimitExceededException
			expectError:               false,
			expectAddCall:             true,
			expectRemoveCall:          true,
			expectUpdateCall:          false,
			expectFlipFlopPattern:     true,
		},
		{
			name:             "endpoint property drift - update endpoint-group API call",
			endpointGroupARN: "arn:aws:globalaccelerator::123456789012:accelerator/abcd/listener/l-1234/endpoint-group/eg-1234",
			currentEndpoints: []agatypes.EndpointDescription{
				{
					EndpointId:                  awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/existing-lb/1111111111"),
					Weight:                      awssdk.Int32(80), // Different weight
					ClientIPPreservationEnabled: awssdk.Bool(false),
				},
			},
			loadedEndpoints: []*aga.LoadedEndpoint{
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeService,
					Name:      "existing-service",
					Namespace: "default",
					Weight:    100, // Changed weight
					ARN:       "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/existing-lb/1111111111",
					Status:    aga.EndpointStatusLoaded,
				},
			},
			expectError:      false,
			expectAddCall:    false, // Should not call AddEndpoints
			expectRemoveCall: false, // Should not call RemoveEndpoints
			expectUpdateCall: true,  // Should call UpdateEndpointGroup
		},
		{
			name:             "endpoints to remove only",
			endpointGroupARN: "arn:aws:globalaccelerator::123456789012:accelerator/abcd/listener/l-1234/endpoint-group/eg-1234",
			currentEndpoints: []agatypes.EndpointDescription{
				{
					EndpointId: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-lb/1234567890"),
				},
				{
					EndpointId: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-lb2/0987654321"),
				},
			},
			loadedEndpoints:  []*aga.LoadedEndpoint{},
			expectError:      false,
			expectAddCall:    false,
			expectRemoveCall: true,
		},
		{
			name:             "both add and remove endpoints",
			endpointGroupARN: "arn:aws:globalaccelerator::123456789012:accelerator/abcd/listener/l-1234/endpoint-group/eg-1234",
			currentEndpoints: []agatypes.EndpointDescription{
				{
					EndpointId: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-lb/1234567890"),
				},
			},
			loadedEndpoints: []*aga.LoadedEndpoint{
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeService,
					Name:      "test-service-2",
					Namespace: "default",
					Weight:    100,
					ARN:       "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-lb2/0987654321",
					Status:    aga.EndpointStatusLoaded,
				},
			},
			expectError:      false,
			expectAddCall:    true,
			expectRemoveCall: true,
		},
		{
			name:             "add endpoints error",
			endpointGroupARN: "arn:aws:globalaccelerator::123456789012:accelerator/abcd/listener/l-1234/endpoint-group/eg-1234",
			currentEndpoints: []agatypes.EndpointDescription{},
			loadedEndpoints: []*aga.LoadedEndpoint{
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeService,
					Name:      "test-service-1",
					Namespace: "default",
					Weight:    100,
					ARN:       "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-lb/1234567890",
					Status:    aga.EndpointStatusLoaded,
				},
			},
			addEndpointsErr:  errors.New("add error"),
			expectError:      true,
			expectAddCall:    true,
			expectRemoveCall: false,
		},
		{
			name:             "remove endpoints error",
			endpointGroupARN: "arn:aws:globalaccelerator::123456789012:accelerator/abcd/listener/l-1234/endpoint-group/eg-1234",
			currentEndpoints: []agatypes.EndpointDescription{
				{
					EndpointId: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-lb/1234567890"),
				},
			},
			loadedEndpoints:    []*aga.LoadedEndpoint{},
			removeEndpointsErr: errors.New("remove error"),
			expectError:        true,
			expectAddCall:      false,
			expectRemoveCall:   true,
		},
		{
			name:             "add and remove with remove error",
			endpointGroupARN: "arn:aws:globalaccelerator::123456789012:accelerator/abcd/listener/l-1234/endpoint-group/eg-1234",
			currentEndpoints: []agatypes.EndpointDescription{
				{
					EndpointId: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-lb/1234567890"),
				},
			},
			loadedEndpoints: []*aga.LoadedEndpoint{
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeService,
					Name:      "test-service-2",
					Namespace: "default",
					Weight:    100,
					ARN:       "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-lb2/0987654321",
					Status:    aga.EndpointStatusLoaded,
				},
			},
			removeEndpointsErr: errors.New("remove error"),
			expectError:        true,
			expectAddCall:      true,
			expectRemoveCall:   true,
		},
		{
			name:             "endpoint with failed status",
			endpointGroupARN: "arn:aws:globalaccelerator::123456789012:accelerator/abcd/listener/l-1234/endpoint-group/eg-1234",
			currentEndpoints: []agatypes.EndpointDescription{},
			loadedEndpoints: []*aga.LoadedEndpoint{
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeService,
					Name:      "test-service-1",
					Namespace: "default",
					Weight:    100,
					ARN:       "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-lb/1234567890",
					Status:    aga.EndpointStatusFatal,
				},
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeService,
					Name:      "test-service-2",
					Namespace: "default",
					Weight:    100,
					ARN:       "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-lb2/0987654321",
					Status:    aga.EndpointStatusLoaded,
				},
			},
			expectError:      false,
			expectAddCall:    true,  // Should call add for the loaded endpoint
			expectRemoveCall: false, // No endpoints to remove
		},
		{
			name:             "limit exceeded - flip-flop pattern",
			endpointGroupARN: "arn:aws:globalaccelerator::123456789012:accelerator/abcd/listener/l-1234/endpoint-group/eg-1234",
			currentEndpoints: []agatypes.EndpointDescription{
				{
					EndpointId: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/existing-lb1/1111111111"),
				},
				{
					EndpointId: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/existing-lb2/2222222222"),
				},
			},
			loadedEndpoints: []*aga.LoadedEndpoint{
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeService,
					Name:      "new-service-1",
					Namespace: "default",
					Weight:    100,
					ARN:       "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/new-lb1/3333333333",
					Status:    aga.EndpointStatusLoaded,
				},
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeService,
					Name:      "new-service-2",
					Namespace: "default",
					Weight:    100,
					ARN:       "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/new-lb2/4444444444",
					Status:    aga.EndpointStatusLoaded,
				},
			},
			addEndpointsErrOnFirstTry: true, // First AddEndpoints call fails with LimitExceededException
			expectError:               false,
			expectAddCall:             true,
			expectRemoveCall:          true,
			expectFlipFlopPattern:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockGaService := services.NewMockGlobalAccelerator(ctrl)

			// Setup expectations for DescribeEndpointGroup
			describeOutput := &globalaccelerator.DescribeEndpointGroupOutput{
				EndpointGroup: &agatypes.EndpointGroup{
					EndpointGroupArn:     awssdk.String(tc.endpointGroupARN),
					EndpointDescriptions: tc.currentEndpoints,
				},
			}
			mockGaService.EXPECT().
				DescribeEndpointGroupWithContext(gomock.Any(), gomock.Any()).
				Return(describeOutput, tc.describeEndpointErr).
				AnyTimes()

			// Setup expectations for UpdateEndpointGroup if applicable (for drift in properties)
			if tc.expectUpdateCall {
				mockGaService.EXPECT().
					UpdateEndpointGroupWithContext(gomock.Any(), gomock.Any()).
					Return(&globalaccelerator.UpdateEndpointGroupOutput{}, nil).
					Times(1)
			} else if tc.expectAddCall { // Don't expect any other API calls if using UpdateEndpointGroup
				if tc.expectFlipFlopPattern {
					// For flip-flop pattern, we first expect one AddEndpoints call that returns LimitExceededException
					limitExceededErr := &agatypes.LimitExceededException{
						Message: awssdk.String("Endpoint limit exceeded"),
					}
					firstAddCall := mockGaService.EXPECT().
						AddEndpointsWithContext(gomock.Any(), gomock.Any()).
						Return(nil, limitExceededErr).
						Times(1)

					// Then we expect individual AddEndpoints calls for each endpoint (after removal)
					// These calls should succeed
					mockGaService.EXPECT().
						AddEndpointsWithContext(gomock.Any(), gomock.Any()).
						Return(&globalaccelerator.AddEndpointsOutput{}, nil).
						After(firstAddCall).
						AnyTimes()
				} else if tc.addEndpointsErrOnFirstTry {
					// Set up a sequence for error on first try only
					limitExceededErr := &agatypes.LimitExceededException{
						Message: awssdk.String("Endpoint limit exceeded"),
					}
					mockGaService.EXPECT().
						AddEndpointsWithContext(gomock.Any(), gomock.Any()).
						Return(nil, limitExceededErr).
						Times(1)

					mockGaService.EXPECT().
						AddEndpointsWithContext(gomock.Any(), gomock.Any()).
						Return(&globalaccelerator.AddEndpointsOutput{}, nil).
						AnyTimes()
				} else {
					// Standard expectation for normal cases
					mockGaService.EXPECT().
						AddEndpointsWithContext(gomock.Any(), gomock.Any()).
						Return(&globalaccelerator.AddEndpointsOutput{}, tc.addEndpointsErr).
						AnyTimes()
				}
			}

			// Setup expectations for RemoveEndpoints if applicable
			if tc.expectRemoveCall {
				mockGaService.EXPECT().
					RemoveEndpointsWithContext(gomock.Any(), gomock.Any()).
					Return(&globalaccelerator.RemoveEndpointsOutput{}, tc.removeEndpointsErr).
					AnyTimes()
			}

			manager := &defaultEndpointGroupManager{
				gaService: mockGaService,
				logger:    logr.Discard(),
			}

			// Convert LoadedEndpoints to EndpointConfigurations
			endpointConfigs := []agamodel.EndpointConfiguration{}
			for _, loadedEndpoint := range tc.loadedEndpoints {
				if loadedEndpoint.Status == aga.EndpointStatusLoaded {
					endpointConfig := agamodel.EndpointConfiguration{
						EndpointID: loadedEndpoint.ARN,
					}
					if loadedEndpoint.Weight > 0 {
						weight := int32(loadedEndpoint.Weight)
						endpointConfig.Weight = &weight
					}
					endpointConfigs = append(endpointConfigs, endpointConfig)
				}
			}

			// Add more test-specific handling if needed here
			err := manager.ManageEndpoints(context.Background(), tc.endpointGroupARN, endpointConfigs, tc.currentEndpoints)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
