package aga

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	agatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/stretchr/testify/assert"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
)

func TestSortModelPortRanges(t *testing.T) {
	tests := []struct {
		name       string
		portRanges []agamodel.PortRange
		want       []agamodel.PortRange
	}{
		{
			name: "already sorted by FromPort",
			portRanges: []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
				{FromPort: 443, ToPort: 443},
			},
			want: []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
				{FromPort: 443, ToPort: 443},
			},
		},
		{
			name: "unsorted by FromPort",
			portRanges: []agamodel.PortRange{
				{FromPort: 443, ToPort: 443},
				{FromPort: 80, ToPort: 80},
			},
			want: []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
				{FromPort: 443, ToPort: 443},
			},
		},
		{
			name: "same FromPort, different ToPort",
			portRanges: []agamodel.PortRange{
				{FromPort: 80, ToPort: 100},
				{FromPort: 80, ToPort: 90},
			},
			want: []agamodel.PortRange{
				{FromPort: 80, ToPort: 90},
				{FromPort: 80, ToPort: 100},
			},
		},
		{
			name:       "empty slice",
			portRanges: []agamodel.PortRange{},
			want:       []agamodel.PortRange{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SortModelPortRanges(tt.portRanges)
			assert.Equal(t, tt.want, tt.portRanges)
		})
	}
}

func TestSortSDKPortRanges(t *testing.T) {
	tests := []struct {
		name       string
		portRanges []agatypes.PortRange
		want       []agatypes.PortRange
	}{
		{
			name: "already sorted by FromPort",
			portRanges: []agatypes.PortRange{
				{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
				{FromPort: aws.Int32(443), ToPort: aws.Int32(443)},
			},
			want: []agatypes.PortRange{
				{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
				{FromPort: aws.Int32(443), ToPort: aws.Int32(443)},
			},
		},
		{
			name: "unsorted by FromPort",
			portRanges: []agatypes.PortRange{
				{FromPort: aws.Int32(443), ToPort: aws.Int32(443)},
				{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
			},
			want: []agatypes.PortRange{
				{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
				{FromPort: aws.Int32(443), ToPort: aws.Int32(443)},
			},
		},
		{
			name: "same FromPort, different ToPort",
			portRanges: []agatypes.PortRange{
				{FromPort: aws.Int32(80), ToPort: aws.Int32(100)},
				{FromPort: aws.Int32(80), ToPort: aws.Int32(90)},
			},
			want: []agatypes.PortRange{
				{FromPort: aws.Int32(80), ToPort: aws.Int32(90)},
				{FromPort: aws.Int32(80), ToPort: aws.Int32(100)},
			},
		},
		{
			name:       "empty slice",
			portRanges: []agatypes.PortRange{},
			want:       []agatypes.PortRange{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SortSDKPortRanges(tt.portRanges)
			assert.Equal(t, tt.want, tt.portRanges)
		})
	}
}

func TestPortRangeCompare(t *testing.T) {
	tests := []struct {
		name      string
		fromPort1 int32
		toPort1   int32
		fromPort2 int32
		toPort2   int32
		want      int
	}{
		{
			name:      "first range starts before second",
			fromPort1: 80,
			toPort1:   100,
			fromPort2: 90,
			toPort2:   110,
			want:      -1,
		},
		{
			name:      "first range starts after second",
			fromPort1: 90,
			toPort1:   110,
			fromPort2: 80,
			toPort2:   100,
			want:      1,
		},
		{
			name:      "same start, first end before second",
			fromPort1: 80,
			toPort1:   100,
			fromPort2: 80,
			toPort2:   110,
			want:      -1,
		},
		{
			name:      "same start, first end after second",
			fromPort1: 80,
			toPort1:   110,
			fromPort2: 80,
			toPort2:   100,
			want:      1,
		},
		{
			name:      "identical port ranges",
			fromPort1: 80,
			toPort1:   100,
			fromPort2: 80,
			toPort2:   100,
			want:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PortRangeCompare(tt.fromPort1, tt.toPort1, tt.fromPort2, tt.toPort2)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestPortRangesToSet(t *testing.T) {
	tests := []struct {
		name     string
		fromPort int32
		toPort   int32
		want     map[int32]bool
	}{
		{
			name:     "single port",
			fromPort: 80,
			toPort:   80,
			want:     map[int32]bool{80: true},
		},
		{
			name:     "port range",
			fromPort: 80,
			toPort:   82,
			want:     map[int32]bool{80: true, 81: true, 82: true},
		},
		{
			name:     "fromPort > toPort (invalid but shouldn't crash)",
			fromPort: 82,
			toPort:   80,
			want:     map[int32]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			portSet := make(map[int32]bool)
			PortRangesToSet(tt.fromPort, tt.toPort, portSet)
			assert.Equal(t, tt.want, portSet)
		})
	}
}

func TestResPortRangesToSet(t *testing.T) {
	tests := []struct {
		name       string
		portRanges []agamodel.PortRange
		want       map[int32]bool
	}{
		{
			name: "single port range",
			portRanges: []agamodel.PortRange{
				{FromPort: 80, ToPort: 82},
			},
			want: map[int32]bool{80: true, 81: true, 82: true},
		},
		{
			name: "multiple port ranges",
			portRanges: []agamodel.PortRange{
				{FromPort: 80, ToPort: 81},
				{FromPort: 443, ToPort: 444},
			},
			want: map[int32]bool{80: true, 81: true, 443: true, 444: true},
		},
		{
			name:       "empty slice",
			portRanges: []agamodel.PortRange{},
			want:       map[int32]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			portSet := make(map[int32]bool)
			ResPortRangesToSet(tt.portRanges, portSet)
			assert.Equal(t, tt.want, portSet)
		})
	}
}

func TestSDKPortRangesToSet(t *testing.T) {
	tests := []struct {
		name       string
		portRanges []agatypes.PortRange
		want       map[int32]bool
	}{
		{
			name: "single port range",
			portRanges: []agatypes.PortRange{
				{FromPort: aws.Int32(80), ToPort: aws.Int32(82)},
			},
			want: map[int32]bool{80: true, 81: true, 82: true},
		},
		{
			name: "multiple port ranges",
			portRanges: []agatypes.PortRange{
				{FromPort: aws.Int32(80), ToPort: aws.Int32(81)},
				{FromPort: aws.Int32(443), ToPort: aws.Int32(444)},
			},
			want: map[int32]bool{80: true, 81: true, 443: true, 444: true},
		},
		{
			name:       "empty slice",
			portRanges: []agatypes.PortRange{},
			want:       map[int32]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			portSet := make(map[int32]bool)
			SDKPortRangesToSet(tt.portRanges, portSet)
			assert.Equal(t, tt.want, portSet)
		})
	}
}

func TestResPortRangesToString(t *testing.T) {
	tests := []struct {
		name       string
		portRanges []agamodel.PortRange
		want       string
	}{
		{
			name: "single port range",
			portRanges: []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
			},
			want: "80-80",
		},
		{
			name: "multiple port ranges",
			portRanges: []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
				{FromPort: 443, ToPort: 443},
			},
			want: "80-80,443-443",
		},
		{
			name:       "empty slice",
			portRanges: []agamodel.PortRange{},
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResPortRangesToString(tt.portRanges)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestSDKPortRangesToString(t *testing.T) {
	tests := []struct {
		name       string
		portRanges []agatypes.PortRange
		want       string
	}{
		{
			name: "single port range",
			portRanges: []agatypes.PortRange{
				{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
			},
			want: "80-80",
		},
		{
			name: "multiple port ranges",
			portRanges: []agatypes.PortRange{
				{FromPort: aws.Int32(80), ToPort: aws.Int32(80)},
				{FromPort: aws.Int32(443), ToPort: aws.Int32(443)},
			},
			want: "80-80,443-443",
		},
		{
			name:       "empty slice",
			portRanges: []agatypes.PortRange{},
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SDKPortRangesToString(tt.portRanges)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestFormatPortRangeToString(t *testing.T) {
	tests := []struct {
		name     string
		fromPort int32
		toPort   int32
		want     string
	}{
		{
			name:     "single port",
			fromPort: 80,
			toPort:   80,
			want:     "80-80",
		},
		{
			name:     "port range",
			fromPort: 80,
			toPort:   100,
			want:     "80-100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatPortRangeToString(tt.fromPort, tt.toPort)
			assert.Equal(t, tt.want, result)
		})
	}
}
