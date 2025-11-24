package aga

import (
	"sort"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	agatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

func Test_listenerSynthesizer_hasPortRangeConflict(t *testing.T) {
	mockStack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	tests := []struct {
		name        string
		resListener *agamodel.Listener
		sdkListener *ListenerResource
		want        bool
	}{
		{
			name: "different protocols - no conflict",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolUdp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
					},
				},
			},
			want: false,
		},
		{
			name: "same protocol, non-overlapping ports - no conflict",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(443), ToPort: awssdk.Int32(443)},
					},
				},
			},
			want: false,
		},
		{
			name: "same protocol, same ports - conflict",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
					},
				},
			},
			want: true,
		},
		{
			name: "same protocol, overlapping port ranges - conflict",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 100},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(90), ToPort: awssdk.Int32(110)},
					},
				},
			},
			want: true,
		},
		{
			name: "same protocol, multiple port ranges with one overlap - conflict",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolUDP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
						{FromPort: 443, ToPort: 443},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolUdp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(443), ToPort: awssdk.Int32(443)},
						{FromPort: awssdk.Int32(8080), ToPort: awssdk.Int32(8080)},
					},
				},
			},
			want: true,
		},
		{
			name: "same protocol, adjacent port ranges - no conflict",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 90},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(91), ToPort: awssdk.Int32(100)},
					},
				},
			},
			want: false,
		},
		{
			name: "same protocol, one port at edge of range - conflict",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 90},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(90), ToPort: awssdk.Int32(100)},
					},
				},
			},
			want: true,
		},
		{
			name: "same protocol, complex multiple ranges with overlap - conflict",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
						{FromPort: 443, ToPort: 443},
						{FromPort: 8000, ToPort: 8010},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(22), ToPort: awssdk.Int32(22)},
						{FromPort: awssdk.Int32(5000), ToPort: awssdk.Int32(5010)},
						{FromPort: awssdk.Int32(8005), ToPort: awssdk.Int32(8015)},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &listenerSynthesizer{
				logger: logr.Discard(),
			}
			got := s.hasPortRangeConflict(tt.resListener, tt.sdkListener)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_listenerSynthesizer_generateResListenerKey(t *testing.T) {
	mockStack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	tests := []struct {
		name     string
		listener *agamodel.Listener
		want     string
	}{
		{
			name: "TCP listener with single port range",
			listener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
					},
				},
			},
			want: "TCP:80-80",
		},
		{
			name: "UDP listener with multiple port ranges - ordered",
			listener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolUDP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
						{FromPort: 443, ToPort: 443},
					},
				},
			},
			want: "UDP:80-80,443-443",
		},
		{
			name: "TCP listener with multiple port ranges - unordered",
			listener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 443, ToPort: 443},
						{FromPort: 80, ToPort: 80},
					},
				},
			},
			want: "TCP:80-80,443-443", // Should be sorted
		},
		{
			name: "UDP listener with complex port ranges - unordered",
			listener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolUDP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 8000, ToPort: 8100},
						{FromPort: 443, ToPort: 443},
						{FromPort: 80, ToPort: 80},
					},
				},
			},
			want: "UDP:80-80,443-443,8000-8100", // Should be sorted by FromPort
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &listenerSynthesizer{
				logger: logr.Discard(),
			}
			got := s.generateResListenerKey(tt.listener)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_listenerSynthesizer_calculateSimilarityScore(t *testing.T) {
	mockStack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	tests := []struct {
		name        string
		resListener *agamodel.Listener
		sdkListener *ListenerResource
		want        int
	}{
		{
			name: "exact match - protocol, ports, and client affinity",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
						{FromPort: 443, ToPort: 443},
					},
					ClientAffinity: "SOURCE_IP",
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
						{FromPort: awssdk.Int32(443), ToPort: awssdk.Int32(443)},
					},
					ClientAffinity: agatypes.ClientAffinitySourceIp,
				},
			},
			want: 150, // 40 (protocol) + 100 (full port overlap) + 10 (client affinity)
		},
		{
			name: "protocol match, complete port overlap, no client affinity",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
					},
					ClientAffinity: agatypes.ClientAffinityNone,
				},
			},
			want: 140, // 40 (protocol) + 100 (full port overlap)
		},
		{
			name: "protocol match, no port overlap",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(443), ToPort: awssdk.Int32(443)},
					},
				},
			},
			want: 40, // 40 (protocol) + 0 (no port overlap)
		},
		{
			name: "protocol mismatch, partial port overlap",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
						{FromPort: 443, ToPort: 443},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolUdp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
						{FromPort: awssdk.Int32(8080), ToPort: awssdk.Int32(8080)},
					},
				},
			},
			want: 33, // 0 (protocol mismatch) + 33 (1 common port out of 3 total unique ports)
		},
		{
			name: "protocol match, partial port overlap with ranges",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 90},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(85), ToPort: awssdk.Int32(95)},
					},
				},
			},
			want: 77, // 40 (protocol) + 37 (port overlap)
		},
		{
			name: "protocol mismatch, no port overlap, client affinity match",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
					},
					ClientAffinity: "SOURCE_IP",
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolUdp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(443), ToPort: awssdk.Int32(443)},
					},
					ClientAffinity: agatypes.ClientAffinitySourceIp,
				},
			},
			want: 10, // 0 (protocol mismatch) + 0 (no port overlap) + 10 (client affinity match)
		},
		{
			name: "protocol match, complete port overlap, client affinity mismatch",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
					},
					ClientAffinity: "SOURCE_IP",
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
					},
					ClientAffinity: agatypes.ClientAffinityNone,
				},
			},
			want: 140, // 40 (protocol) + 100 (complete port overlap) + 0 (client affinity mismatch)
		},
		{
			name: "complex case - protocol match, multiple port ranges with partial overlap",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 80},
						{FromPort: 443, ToPort: 443},
						{FromPort: 8000, ToPort: 8010},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(443), ToPort: awssdk.Int32(443)},
						{FromPort: awssdk.Int32(8005), ToPort: awssdk.Int32(8015)},
						{FromPort: awssdk.Int32(9000), ToPort: awssdk.Int32(9010)},
					},
				},
			},
			want: 64, // 40 (protocol) + 24 (partial port overlap)
		},
		{
			name: "empty port ranges",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol:   agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol:   agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{},
				},
			},
			want: 40, // 40 (protocol) + 0 (no ports)
		},
		{
			name: "large port ranges with partial overlap",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolTCP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 1000, ToPort: 2000},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(1500), ToPort: awssdk.Int32(2500)},
					},
				},
			},
			want: 73, // 40 (protocol) + 33 (port overlap)
		},
		{
			name: "nil and empty client affinity - no match bonus",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol:       agamodel.ProtocolTCP,
					PortRanges:     []agamodel.PortRange{{FromPort: 80, ToPort: 80}},
					ClientAffinity: "", // Empty
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol:   agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)}},
					// ClientAffinity is nil or not set
				},
			},
			want: 140, // 40 (protocol) + 100 (full port overlap) + 0 (no client affinity bonus)
		},
		{
			name: "protocol case sensitivity test (should still match)",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol:   agamodel.ProtocolTCP, // Upper case
					PortRanges: []agamodel.PortRange{{FromPort: 80, ToPort: 80}},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol:   agatypes.ProtocolTcp, // Title case
					PortRanges: []agatypes.PortRange{{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)}},
				},
			},
			want: 140, // 40 (protocol) + 100 (full port overlap)
		},
		{
			name: "different port ranges but same total ports",
			resListener: &agamodel.Listener{
				ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
				Spec: agamodel.ListenerSpec{
					Protocol: agamodel.ProtocolUDP,
					PortRanges: []agamodel.PortRange{
						{FromPort: 80, ToPort: 85},
					},
				},
			},
			sdkListener: &ListenerResource{
				Listener: &agatypes.Listener{
					Protocol: agatypes.ProtocolUdp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
						{FromPort: awssdk.Int32(81), ToPort: awssdk.Int32(85)},
					},
				},
			},
			want: 140, // 40 (protocol) + 100 (full port overlap - different ranges but same ports)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &listenerSynthesizer{
				logger: logr.Discard(),
			}
			got := s.calculateSimilarityScore(tt.resListener, tt.sdkListener)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_listenerSynthesizer_findExactMatches(t *testing.T) {
	mockStack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	tests := []struct {
		name             string
		resListeners     []*agamodel.Listener
		sdkListeners     []*ListenerResource
		wantMatchedPairs []struct {
			resID  string
			sdkARN string
		}
		wantUnmatchedResIDs  []string
		wantUnmatchedSDKARNs []string
	}{
		{
			name:         "empty lists",
			resListeners: []*agamodel.Listener{},
			sdkListeners: []*ListenerResource{},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{},
			wantUnmatchedResIDs:  []string{},
			wantUnmatchedSDKARNs: []string{},
		},
		{
			name: "one exact match",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-80"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 80, ToPort: 80},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{
				{
					resID:  "tcp-80",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80",
				},
			},
			wantUnmatchedResIDs:  []string{},
			wantUnmatchedSDKARNs: []string{},
		},
		{
			name: "one exact match among multiple listeners",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-80"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 80, ToPort: 80},
						},
					},
				},
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-443"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 443, ToPort: 443},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
						},
					},
				},
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-udp-53"),
						Protocol:    agatypes.ProtocolUdp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(53), ToPort: awssdk.Int32(53)},
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{
				{
					resID:  "tcp-80",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80",
				},
			},
			wantUnmatchedResIDs:  []string{"tcp-443"},
			wantUnmatchedSDKARNs: []string{"arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-udp-53"},
		},
		{
			name: "multiple exact matches",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-80"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 80, ToPort: 80},
						},
					},
				},
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "udp-53"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolUDP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 53, ToPort: 53},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
						},
					},
				},
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-udp-53"),
						Protocol:    agatypes.ProtocolUdp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(53), ToPort: awssdk.Int32(53)},
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{
				{
					resID:  "tcp-80",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80",
				},
				{
					resID:  "udp-53",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-udp-53",
				},
			},
			wantUnmatchedResIDs:  []string{},
			wantUnmatchedSDKARNs: []string{},
		},
		{
			name: "exact match with different port range ordering",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-multi-port"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 443, ToPort: 443}, // Note the order - 443 first, then 80
							{FromPort: 80, ToPort: 80},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-multi"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)}, // Different order - 80 first, then 443
							{FromPort: awssdk.Int32(443), ToPort: awssdk.Int32(443)},
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{
				{
					resID:  "tcp-multi-port",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-multi",
				},
			},
			wantUnmatchedResIDs:  []string{},
			wantUnmatchedSDKARNs: []string{},
		},
		{
			name: "no matches",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-80"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 80, ToPort: 80},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-udp-80"),
						Protocol:    agatypes.ProtocolUdp, // Different protocol
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{},
			wantUnmatchedResIDs:  []string{"tcp-80"},
			wantUnmatchedSDKARNs: []string{"arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-udp-80"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &listenerSynthesizer{
				logger: logr.Discard(),
			}

			// Run the function
			matchedPairs, unmatchedResListeners, unmatchedSDKListeners := s.findExactMatches(tt.resListeners, tt.sdkListeners)

			// Collect the actual pairs and IDs for verification
			var actualMatchedPairs []struct {
				resID  string
				sdkARN string
			}

			for _, pair := range matchedPairs {
				actualMatchedPairs = append(actualMatchedPairs, struct {
					resID  string
					sdkARN string
				}{
					resID:  pair.resListener.ID(),
					sdkARN: awssdk.ToString(pair.sdkListener.Listener.ListenerArn),
				})
			}

			var actualUnmatchedResIDs []string
			for _, listener := range unmatchedResListeners {
				actualUnmatchedResIDs = append(actualUnmatchedResIDs, listener.ID())
			}

			var actualUnmatchedSDKARNs []string
			for _, listener := range unmatchedSDKListeners {
				actualUnmatchedSDKARNs = append(actualUnmatchedSDKARNs, awssdk.ToString(listener.Listener.ListenerArn))
			}

			// Sort all slices to ensure consistent comparison
			sort.Slice(actualMatchedPairs, func(i, j int) bool {
				if actualMatchedPairs[i].resID != actualMatchedPairs[j].resID {
					return actualMatchedPairs[i].resID < actualMatchedPairs[j].resID
				}
				return actualMatchedPairs[i].sdkARN < actualMatchedPairs[j].sdkARN
			})

			sort.Slice(tt.wantMatchedPairs, func(i, j int) bool {
				if tt.wantMatchedPairs[i].resID != tt.wantMatchedPairs[j].resID {
					return tt.wantMatchedPairs[i].resID < tt.wantMatchedPairs[j].resID
				}
				return tt.wantMatchedPairs[i].sdkARN < tt.wantMatchedPairs[j].sdkARN
			})

			sort.Strings(actualUnmatchedResIDs)
			sort.Strings(tt.wantUnmatchedResIDs)
			sort.Strings(actualUnmatchedSDKARNs)
			sort.Strings(tt.wantUnmatchedSDKARNs)

			// Verify matched pairs
			assert.Equal(t, len(tt.wantMatchedPairs), len(actualMatchedPairs), "matched pairs count")
			for i := range tt.wantMatchedPairs {
				if i < len(actualMatchedPairs) {
					assert.Equal(t, tt.wantMatchedPairs[i].resID, actualMatchedPairs[i].resID, "matched pair resID at index %d", i)
					assert.Equal(t, tt.wantMatchedPairs[i].sdkARN, actualMatchedPairs[i].sdkARN, "matched pair sdkARN at index %d", i)
				}
			}

			// Handle nil vs empty slices
			if len(actualUnmatchedResIDs) == 0 && len(tt.wantUnmatchedResIDs) == 0 {
				// Both empty, no need to compare
			} else {
				// Verify unmatched resource listeners
				assert.ElementsMatch(t, tt.wantUnmatchedResIDs, actualUnmatchedResIDs, "unmatched resource listeners")
			}

			if len(actualUnmatchedSDKARNs) == 0 && len(tt.wantUnmatchedSDKARNs) == 0 {
				// Both empty, no need to compare
			} else {
				// Verify unmatched SDK listeners
				assert.ElementsMatch(t, tt.wantUnmatchedSDKARNs, actualUnmatchedSDKARNs, "unmatched SDK listeners")
			}
		})
	}
}

func Test_listenerSynthesizer_findSimilarityMatches(t *testing.T) {
	mockStack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	tests := []struct {
		name             string
		resListeners     []*agamodel.Listener
		sdkListeners     []*ListenerResource
		wantMatchedPairs []struct {
			resID  string
			sdkARN string
		}
		wantUnmatchedResIDs  []string
		wantUnmatchedSDKARNs []string
	}{
		{
			name:         "empty lists",
			resListeners: []*agamodel.Listener{},
			sdkListeners: []*ListenerResource{},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{},
			wantUnmatchedResIDs:  []string{},
			wantUnmatchedSDKARNs: []string{},
		},
		{
			name:         "empty resource listeners",
			resListeners: []*agamodel.Listener{},
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list123"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{},
			wantUnmatchedResIDs:  []string{},
			wantUnmatchedSDKARNs: []string{"arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list123"},
		},
		{
			name: "empty sdk listeners",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 80, ToPort: 80},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{},
			wantUnmatchedResIDs:  []string{"listener-1"},
			wantUnmatchedSDKARNs: []string{},
		},
		{
			name: "one exact similarity match",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "listener-1"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 80, ToPort: 80},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list123"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{
				{
					resID:  "listener-1",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list123",
				},
			},
			wantUnmatchedResIDs:  []string{},
			wantUnmatchedSDKARNs: []string{},
		},
		{
			name: "multiple listeners with some matches",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-80"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 80, ToPort: 80},
						},
					},
				},
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-443"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 443, ToPort: 443},
						},
					},
				},
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "udp-53"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolUDP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 53, ToPort: 53},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
						},
					},
				},
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-8080"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(8080), ToPort: awssdk.Int32(8080)},
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{
				{
					resID:  "tcp-80",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80",
				},
				{
					resID:  "tcp-443",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-8080",
				},
			},
			wantUnmatchedResIDs:  []string{"udp-53"},
			wantUnmatchedSDKARNs: []string{},
		},
		{
			name: "complex case with partial similarity matches - greedy algorithm test",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-80-100"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 80, ToPort: 100},
						},
					},
				},
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-443"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 443, ToPort: 443},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-90-110"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(90), ToPort: awssdk.Int32(110)},
						},
					},
				},
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-440-450"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(440), ToPort: awssdk.Int32(450)},
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{
				{
					resID:  "tcp-80-100",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-90-110",
				},
				{
					resID:  "tcp-443",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-440-450",
				},
			},
			wantUnmatchedResIDs:  []string{},
			wantUnmatchedSDKARNs: []string{},
			// The higher similarity will be between tcp-80-100 and tcp-90-110 due to more overlapping ports
			// This verifies the greedy algorithm is matching highest scores first
		},
		{
			name: "no matches below threshold",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-80"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 80, ToPort: 80},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-udp-80"),
						Protocol:    agatypes.ProtocolUdp, // Different protocol, similarity will be low
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(8080), ToPort: awssdk.Int32(8080)}, // Different port too
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{},
			wantUnmatchedResIDs:  []string{"tcp-80"},
			wantUnmatchedSDKARNs: []string{"arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-udp-80"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &listenerSynthesizer{
				logger: logr.Discard(),
			}

			// Run the function
			matchedPairs, unmatchedResListeners, unmatchedSDKListeners := s.findSimilarityMatches(tt.resListeners, tt.sdkListeners)

			// Collect the actual pairs and IDs for verification
			var actualMatchedPairs []struct {
				resID  string
				sdkARN string
			}

			for _, pair := range matchedPairs {
				actualMatchedPairs = append(actualMatchedPairs, struct {
					resID  string
					sdkARN string
				}{
					resID:  pair.resListener.ID(),
					sdkARN: awssdk.ToString(pair.sdkListener.Listener.ListenerArn),
				})
			}

			var actualUnmatchedResIDs []string
			for _, listener := range unmatchedResListeners {
				actualUnmatchedResIDs = append(actualUnmatchedResIDs, listener.ID())
			}

			var actualUnmatchedSDKARNs []string
			for _, listener := range unmatchedSDKListeners {
				actualUnmatchedSDKARNs = append(actualUnmatchedSDKARNs, awssdk.ToString(listener.Listener.ListenerArn))
			}

			// Sort all slices to ensure consistent comparison
			sort.Slice(actualMatchedPairs, func(i, j int) bool {
				if actualMatchedPairs[i].resID != actualMatchedPairs[j].resID {
					return actualMatchedPairs[i].resID < actualMatchedPairs[j].resID
				}
				return actualMatchedPairs[i].sdkARN < actualMatchedPairs[j].sdkARN
			})

			sort.Slice(tt.wantMatchedPairs, func(i, j int) bool {
				if tt.wantMatchedPairs[i].resID != tt.wantMatchedPairs[j].resID {
					return tt.wantMatchedPairs[i].resID < tt.wantMatchedPairs[j].resID
				}
				return tt.wantMatchedPairs[i].sdkARN < tt.wantMatchedPairs[j].sdkARN
			})

			sort.Strings(actualUnmatchedResIDs)
			sort.Strings(tt.wantUnmatchedResIDs)
			sort.Strings(actualUnmatchedSDKARNs)
			sort.Strings(tt.wantUnmatchedSDKARNs)

			// Verify matched pairs
			assert.Equal(t, len(tt.wantMatchedPairs), len(actualMatchedPairs), "matched pairs count")
			for i := range tt.wantMatchedPairs {
				if i < len(actualMatchedPairs) {
					assert.Equal(t, tt.wantMatchedPairs[i].resID, actualMatchedPairs[i].resID, "matched pair resID at index %d", i)
					assert.Equal(t, tt.wantMatchedPairs[i].sdkARN, actualMatchedPairs[i].sdkARN, "matched pair sdkARN at index %d", i)
				}
			}

			// Handle nil vs empty slices
			if len(actualUnmatchedResIDs) == 0 && len(tt.wantUnmatchedResIDs) == 0 {
				// Both empty, no need to compare
			} else {
				// Verify unmatched resource listeners
				assert.ElementsMatch(t, tt.wantUnmatchedResIDs, actualUnmatchedResIDs, "unmatched resource listeners")
			}

			if len(actualUnmatchedSDKARNs) == 0 && len(tt.wantUnmatchedSDKARNs) == 0 {
				// Both empty, no need to compare
			} else {
				// Verify unmatched SDK listeners
				assert.ElementsMatch(t, tt.wantUnmatchedSDKARNs, actualUnmatchedSDKARNs, "unmatched SDK listeners")
			}
		})
	}
}

func Test_listenerSynthesizer_matchResAndSDKListeners(t *testing.T) {
	mockStack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	tests := []struct {
		name             string
		resListeners     []*agamodel.Listener
		sdkListeners     []*ListenerResource
		wantMatchedPairs []struct {
			resID  string
			sdkARN string
		}
		wantUnmatchedResIDs  []string
		wantUnmatchedSDKARNs []string
	}{
		{
			name:         "empty lists",
			resListeners: []*agamodel.Listener{},
			sdkListeners: []*ListenerResource{},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{},
			wantUnmatchedResIDs:  []string{},
			wantUnmatchedSDKARNs: []string{},
		},
		{
			name:         "empty resource listeners, multiple SDK listeners",
			resListeners: []*agamodel.Listener{}, // Empty resource listeners
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
						},
					},
				},
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-udp-53"),
						Protocol:    agatypes.ProtocolUdp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(53), ToPort: awssdk.Int32(53)},
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{}, // No matches expected
			wantUnmatchedResIDs: []string{}, // No unmatched resource listeners
			wantUnmatchedSDKARNs: []string{
				"arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80",
				"arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-udp-53",
			}, // All SDK listeners should be unmatched
		},
		{
			name: "multiple resource listeners, empty SDK listeners",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-80"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 80, ToPort: 80},
						},
					},
				},
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "udp-53"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolUDP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 53, ToPort: 53},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{}, // Empty SDK listeners
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{}, // No matches expected
			wantUnmatchedResIDs: []string{
				"tcp-80",
				"udp-53",
			}, // All resource listeners should be unmatched
			wantUnmatchedSDKARNs: []string{}, // No unmatched SDK listeners
		},
		{
			name: "exact match - should be identified in first pass",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-80"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 80, ToPort: 80},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{
				{
					resID:  "tcp-80",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80",
				},
			},
			wantUnmatchedResIDs:  []string{},
			wantUnmatchedSDKARNs: []string{},
		},
		{
			name: "similarity match - should be identified in second pass",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-80-90"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 80, ToPort: 90},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-85-95"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(85), ToPort: awssdk.Int32(95)},
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{
				{
					resID:  "tcp-80-90",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-85-95",
				},
			},
			wantUnmatchedResIDs:  []string{},
			wantUnmatchedSDKARNs: []string{},
		},
		{
			name: "mix of exact and similarity matches",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-80"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 80, ToPort: 80},
						},
					},
				},
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-443"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 443, ToPort: 443},
						},
					},
				},
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-8080-8090"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 8080, ToPort: 8090},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
						},
					},
				},
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-8085-8095"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(8085), ToPort: awssdk.Int32(8095)},
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{
				{
					resID:  "tcp-80",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80",
				},
				{
					resID:  "tcp-8080-8090",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-8085-8095",
				},
			},
			wantUnmatchedResIDs:  []string{"tcp-443"},
			wantUnmatchedSDKARNs: []string{},
		},
		{
			name: "unmatched listeners - no similarities above threshold",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-80"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 80, ToPort: 80},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-udp-53"),
						Protocol:    agatypes.ProtocolUdp, // Different protocol
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(53), ToPort: awssdk.Int32(53)}, // Different port
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{},
			wantUnmatchedResIDs:  []string{"tcp-80"},
			wantUnmatchedSDKARNs: []string{"arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-udp-53"},
		},
		{
			name: "complex case with multiple matches of both types",
			resListeners: []*agamodel.Listener{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-80"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 80, ToPort: 80},
						},
					},
				},
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "udp-53"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolUDP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 53, ToPort: 53},
						},
					},
				},
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-8080-8090"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 8080, ToPort: 8090},
						},
					},
				},
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", "tcp-443"),
					Spec: agamodel.ListenerSpec{
						Protocol: agamodel.ProtocolTCP,
						PortRanges: []agamodel.PortRange{
							{FromPort: 443, ToPort: 443},
						},
					},
				},
			},
			sdkListeners: []*ListenerResource{
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
						},
					},
				},
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-udp-53"),
						Protocol:    agatypes.ProtocolUdp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(53), ToPort: awssdk.Int32(53)},
						},
					},
				},
				{
					Listener: &agatypes.Listener{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-8085-8095"),
						Protocol:    agatypes.ProtocolTcp,
						PortRanges: []agatypes.PortRange{
							{FromPort: awssdk.Int32(8085), ToPort: awssdk.Int32(8095)},
						},
					},
				},
			},
			wantMatchedPairs: []struct {
				resID  string
				sdkARN string
			}{
				{
					resID:  "tcp-80",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-80",
				},
				{
					resID:  "udp-53",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-udp-53",
				},
				{
					resID:  "tcp-8080-8090",
					sdkARN: "arn:aws:globalaccelerator::123456789012:accelerator/acc123/listener/list-tcp-8085-8095",
				},
			},
			wantUnmatchedResIDs:  []string{"tcp-443"},
			wantUnmatchedSDKARNs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &listenerSynthesizer{
				logger: logr.Discard(),
			}

			// Run the function
			matchedPairs, unmatchedResListeners, unmatchedSDKListeners := s.matchResAndSDKListeners(tt.resListeners, tt.sdkListeners)

			// Collect the actual pairs and IDs for verification
			var actualMatchedPairs []struct {
				resID  string
				sdkARN string
			}

			for _, pair := range matchedPairs {
				actualMatchedPairs = append(actualMatchedPairs, struct {
					resID  string
					sdkARN string
				}{
					resID:  pair.resListener.ID(),
					sdkARN: awssdk.ToString(pair.sdkListener.Listener.ListenerArn),
				})
			}

			var actualUnmatchedResIDs []string
			for _, listener := range unmatchedResListeners {
				actualUnmatchedResIDs = append(actualUnmatchedResIDs, listener.ID())
			}

			var actualUnmatchedSDKARNs []string
			for _, listener := range unmatchedSDKListeners {
				actualUnmatchedSDKARNs = append(actualUnmatchedSDKARNs, awssdk.ToString(listener.Listener.ListenerArn))
			}

			// Sort all slices to ensure consistent comparison
			sort.Slice(actualMatchedPairs, func(i, j int) bool {
				if actualMatchedPairs[i].resID != actualMatchedPairs[j].resID {
					return actualMatchedPairs[i].resID < actualMatchedPairs[j].resID
				}
				return actualMatchedPairs[i].sdkARN < actualMatchedPairs[j].sdkARN
			})

			sort.Slice(tt.wantMatchedPairs, func(i, j int) bool {
				if tt.wantMatchedPairs[i].resID != tt.wantMatchedPairs[j].resID {
					return tt.wantMatchedPairs[i].resID < tt.wantMatchedPairs[j].resID
				}
				return tt.wantMatchedPairs[i].sdkARN < tt.wantMatchedPairs[j].sdkARN
			})

			sort.Strings(actualUnmatchedResIDs)
			sort.Strings(tt.wantUnmatchedResIDs)
			sort.Strings(actualUnmatchedSDKARNs)
			sort.Strings(tt.wantUnmatchedSDKARNs)

			// Verify matched pairs
			assert.Equal(t, len(tt.wantMatchedPairs), len(actualMatchedPairs), "matched pairs count")
			for i := range tt.wantMatchedPairs {
				if i < len(actualMatchedPairs) {
					assert.Equal(t, tt.wantMatchedPairs[i].resID, actualMatchedPairs[i].resID, "matched pair resID at index %d", i)
					assert.Equal(t, tt.wantMatchedPairs[i].sdkARN, actualMatchedPairs[i].sdkARN, "matched pair sdkARN at index %d", i)
				}
			}

			// Handle nil vs empty slices
			if len(actualUnmatchedResIDs) == 0 && len(tt.wantUnmatchedResIDs) == 0 {
				// Both empty, no need to compare
			} else {
				// Verify unmatched resource listeners
				assert.ElementsMatch(t, tt.wantUnmatchedResIDs, actualUnmatchedResIDs, "unmatched resource listeners")
			}

			if len(actualUnmatchedSDKARNs) == 0 && len(tt.wantUnmatchedSDKARNs) == 0 {
				// Both empty, no need to compare
			} else {
				// Verify unmatched SDK listeners
				assert.ElementsMatch(t, tt.wantUnmatchedSDKARNs, actualUnmatchedSDKARNs, "unmatched SDK listeners")
			}
		})
	}
}

func Test_listenerSynthesizer_generateSDKListenerKey(t *testing.T) {
	tests := []struct {
		name     string
		listener *ListenerResource
		want     string
	}{
		{
			name: "TCP listener with single port range",
			listener: &ListenerResource{
				Listener: &agatypes.Listener{
					ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh/listener/abcdef1234"),
					Protocol:    agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
					},
				},
			},
			want: "TCP:80-80",
		},
		{
			name: "UDP listener with multiple port ranges - ordered",
			listener: &ListenerResource{
				Listener: &agatypes.Listener{
					ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh/listener/abcdef1234"),
					Protocol:    agatypes.ProtocolUdp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
						{FromPort: awssdk.Int32(443), ToPort: awssdk.Int32(443)},
					},
				},
			},
			want: "UDP:80-80,443-443",
		},
		{
			name: "TCP listener with multiple port ranges - unordered",
			listener: &ListenerResource{
				Listener: &agatypes.Listener{
					ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh/listener/abcdef1234"),
					Protocol:    agatypes.ProtocolTcp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(443), ToPort: awssdk.Int32(443)},
						{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
					},
				},
			},
			want: "TCP:80-80,443-443", // Should be sorted
		},
		{
			name: "UDP listener with complex port ranges - unordered",
			listener: &ListenerResource{
				Listener: &agatypes.Listener{
					ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh/listener/abcdef1234"),
					Protocol:    agatypes.ProtocolUdp,
					PortRanges: []agatypes.PortRange{
						{FromPort: awssdk.Int32(8000), ToPort: awssdk.Int32(8100)},
						{FromPort: awssdk.Int32(443), ToPort: awssdk.Int32(443)},
						{FromPort: awssdk.Int32(80), ToPort: awssdk.Int32(80)},
					},
				},
			},
			want: "UDP:80-80,443-443,8000-8100", // Should be sorted by FromPort
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &listenerSynthesizer{
				logger: logr.Discard(),
			}
			got := s.generateSDKListenerKey(tt.listener)
			assert.Equal(t, tt.want, got)
		})
	}
}
