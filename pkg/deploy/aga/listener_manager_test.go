package aga

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	agatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

// ListenerResource is already defined in types.go, no need to redefine it here

func Test_defaultListenerManager_buildSDKCreateListenerInput(t *testing.T) {
	testAcceleratorARN := "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh"
	mockStack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	tests := []struct {
		name        string
		resListener *agamodel.Listener
		want        *globalaccelerator.CreateListenerInput
		wantErr     bool
	}{
		{
			name: "Standard TCP listener",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					AcceleratorARN: core.LiteralStringToken(testAcceleratorARN),
					Protocol:       agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
						{FromPort: 443, ToPort: 443},
					},
				},
			},
			want: &globalaccelerator.CreateListenerInput{
				AcceleratorArn: aws.String(testAcceleratorARN),
				Protocol:       agatypes.ProtocolTcp,
				PortRanges: []agatypes.PortRange{
					{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
					{FromPort: aws.Int32(443), ToPort: aws.Int32(443)},
				},
			},
			wantErr: false,
		},
		{
			name: "UDP listener with client affinity",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-2"),
				Spec: agamodel.ListenerSpec{
					AcceleratorARN: core.LiteralStringToken(testAcceleratorARN),
					Protocol:       agamodel.ProtocolUDP,
					ClientAffinity: agamodel.ClientAffinitySourceIP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 10000, ToPort: 20000},
					},
				},
			},
			want: &globalaccelerator.CreateListenerInput{
				AcceleratorArn: aws.String(testAcceleratorARN),
				Protocol:       agatypes.ProtocolUdp,
				ClientAffinity: agatypes.ClientAffinitySourceIp,
				PortRanges: []agatypes.PortRange{
					{FromPort: aws.Int32(10000), ToPort: aws.Int32(20000)},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create listener manager
			m := &defaultListenerManager{
				gaService: nil, // Not needed for this test
				logger:    logr.Discard(),
			}

			// Call the method being tested
			got, err := m.buildSDKCreateListenerInput(context.Background(), tt.resListener)

			// Check if error status matches expected
			if (err != nil) != tt.wantErr {
				t.Errorf("buildSDKCreateListenerInput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check if the result matches expected
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultListenerManager_buildSDKUpdateListenerInput(t *testing.T) {
	testListenerARN := "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh/listener/abcdef1234"
	testAcceleratorARN := "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh"
	mockStack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	tests := []struct {
		name        string
		resListener *agamodel.Listener
		sdkListener *ListenerResource
		want        *globalaccelerator.UpdateListenerInput
		wantErr     bool
	}{
		{
			name: "Standard TCP listener update",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					AcceleratorARN: core.LiteralStringToken(testAcceleratorARN),
					Protocol:       agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
						{FromPort: 443, ToPort: 443},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					ListenerArn: aws.String(testListenerARN),
				},
			},
			want: &globalaccelerator.UpdateListenerInput{
				ListenerArn: aws.String(testListenerARN),
				Protocol:    agatypes.ProtocolTcp,
				PortRanges: []agatypes.PortRange{
					{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
					{FromPort: aws.Int32(443), ToPort: aws.Int32(443)},
				},
			},
			wantErr: false,
		},
		{
			name: "UDP listener update with client affinity",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-2"),
				Spec: agamodel.ListenerSpec{
					AcceleratorARN: core.LiteralStringToken(testAcceleratorARN),
					Protocol:       agamodel.ProtocolUDP,
					ClientAffinity: agamodel.ClientAffinitySourceIP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 10000, ToPort: 20000},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					ListenerArn: aws.String(testListenerARN),
				},
			},
			want: &globalaccelerator.UpdateListenerInput{
				ListenerArn:    aws.String(testListenerARN),
				Protocol:       agatypes.ProtocolUdp,
				ClientAffinity: agatypes.ClientAffinitySourceIp,
				PortRanges: []agatypes.PortRange{
					{FromPort: aws.Int32(10000), ToPort: aws.Int32(20000)},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create listener manager
			m := &defaultListenerManager{
				gaService: nil, // Not needed for this test
				logger:    logr.Discard(),
			}

			// Call the method being tested
			got, err := m.buildSDKUpdateListenerInput(context.Background(), tt.resListener, tt.sdkListener)

			// Check if error status matches expected
			if (err != nil) != tt.wantErr {
				t.Errorf("buildSDKUpdateListenerInput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check if the result matches expected
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultListenerManager_isSDKListenerSettingsDrifted(t *testing.T) {
	tests := []struct {
		name        string
		resListener *agamodel.Listener
		sdkListener *ListenerResource
		want        bool
	}{
		{
			name: "No drift - exact match",
			resListener: &agamodel.Listener{
				Spec: agamodel.ListenerSpec{
					Protocol:       agamodel.ProtocolTCP,
					ClientAffinity: agamodel.ClientAffinityNone,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
						{FromPort: 443, ToPort: 443},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol:       agatypes.ProtocolTcp,
					ClientAffinity: agatypes.ClientAffinityNone,
					PortRanges: []agatypes.PortRange{
						{FromPort: aws.Int32(443), ToPort: aws.Int32(443)},
						{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
					},
				},
			},
			want: false, // No drift
		},
		{
			name: "Drift - different protocol",
			resListener: &agamodel.Listener{
				Spec: agamodel.ListenerSpec{
					Protocol:       agamodel.ProtocolTCP,
					ClientAffinity: agamodel.ClientAffinityNone,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol:       agatypes.ProtocolUdp, // Different protocol
					ClientAffinity: agatypes.ClientAffinityNone,
					PortRanges: []agatypes.PortRange{
						{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
					},
				},
			},
			want: true, // Drift detected
		},
		{
			name: "Drift - different client affinity",
			resListener: &agamodel.Listener{
				Spec: agamodel.ListenerSpec{
					Protocol:       agamodel.ProtocolTCP,
					ClientAffinity: agamodel.ClientAffinitySourceIP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol:       agatypes.ProtocolTcp,
					ClientAffinity: agatypes.ClientAffinityNone, // Different client affinity
					PortRanges: []agatypes.PortRange{
						{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
					},
				},
			},
			want: true, // Drift detected
		},
		{
			name: "Drift - different port ranges",
			resListener: &agamodel.Listener{
				Spec: agamodel.ListenerSpec{
					Protocol:       agamodel.ProtocolTCP,
					ClientAffinity: agamodel.ClientAffinityNone,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
						{FromPort: 443, ToPort: 443},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol:       agatypes.ProtocolTcp,
					ClientAffinity: agatypes.ClientAffinityNone,
					PortRanges: []agatypes.PortRange{
						{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
						// Missing 443 port
					},
				},
			},
			want: true, // Drift detected
		},
		{
			name: "No drift - same ports in different order",
			resListener: &agamodel.Listener{
				Spec: agamodel.ListenerSpec{
					Protocol:       agamodel.ProtocolTCP,
					ClientAffinity: agamodel.ClientAffinityNone,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
						{FromPort: 443, ToPort: 443},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol:       agatypes.ProtocolTcp,
					ClientAffinity: agatypes.ClientAffinityNone,
					PortRanges: []agatypes.PortRange{
						{FromPort: aws.Int32(443), ToPort: aws.Int32(443)},
						{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
					},
				},
			},
			want: false, // No drift - port orders don't matter
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create mock GlobalAccelerator service
			mockGAService := services.NewMockGlobalAccelerator(ctrl)

			// Set up the mock behavior
			mockGAService.EXPECT().
				ListEndpointGroupsAsList(gomock.Any(), gomock.Any()).
				Return([]agatypes.EndpointGroup{}, nil).
				AnyTimes()

			// Create manager with mock service
			m := &defaultListenerManager{
				gaService: mockGAService,
				logger:    logr.Discard(),
			}

			got := m.isSDKListenerSettingsDrifted(tt.resListener, tt.sdkListener)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultListenerManager_arePortRangesEqual(t *testing.T) {
	tests := []struct {
		name            string
		modelPortRanges []agamodel.PortRange
		sdkPortRanges   []agatypes.PortRange
		want            bool
	}{
		{
			name: "Equal - exact match",
			modelPortRanges: []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
				{FromPort: 443, ToPort: 443},
			},
			sdkPortRanges: []agatypes.PortRange{
				{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
				{FromPort: aws.Int32(443), ToPort: aws.Int32(443)},
			},
			want: true,
		},
		{
			name: "Equal - different order",
			modelPortRanges: []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
				{FromPort: 443, ToPort: 443},
			},
			sdkPortRanges: []agatypes.PortRange{
				{FromPort: aws.Int32(443), ToPort: aws.Int32(443)},
				{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
			},
			want: true,
		},
		{
			name: "Not equal - different count",
			modelPortRanges: []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
				{FromPort: 443, ToPort: 443},
			},
			sdkPortRanges: []agatypes.PortRange{
				{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
			},
			want: false,
		},
		{
			name: "Not equal - different range",
			modelPortRanges: []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
				{FromPort: 443, ToPort: 443},
			},
			sdkPortRanges: []agatypes.PortRange{
				{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
				{FromPort: aws.Int32(8443), ToPort: aws.Int32(8443)},
			},
			want: false,
		},
		{
			name:            "Equal - empty slices",
			modelPortRanges: []agamodel.PortRange{},
			sdkPortRanges:   []agatypes.PortRange{},
			want:            true,
		},
		{
			name: "Not equal - one empty, one not",
			modelPortRanges: []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
			},
			sdkPortRanges: []agatypes.PortRange{},
			want:          false,
		},
		{
			name: "Equal - port ranges with ranges",
			modelPortRanges: []agamodel.PortRange{
				{FromPort: 80, ToPort: 100},
				{FromPort: 443, ToPort: 450},
			},
			sdkPortRanges: []agatypes.PortRange{
				{FromPort: aws.Int32(80), ToPort: aws.Int32(100)},
				{FromPort: aws.Int32(443), ToPort: aws.Int32(450)},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &defaultListenerManager{
				gaService: nil,
				logger:    logr.Discard(),
			}

			got := m.arePortRangesEqual(tt.modelPortRanges, tt.sdkPortRanges)
			assert.Equal(t, tt.want, got)
		})
	}
}
