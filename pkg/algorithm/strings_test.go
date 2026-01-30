package algorithm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChunkStrings(t *testing.T) {
	type args struct {
		targets   []string
		chunkSize int
	}
	tests := []struct {
		name string
		args args
		want [][]string
	}{
		{
			name: "can be evenly chunked",
			args: args{
				targets:   []string{"a", "b", "c", "d"},
				chunkSize: 2,
			},
			want: [][]string{
				{"a", "b"},
				{"c", "d"},
			},
		},
		{
			name: "cannot be evenly chunked",
			args: args{
				targets:   []string{"a", "b", "c", "d"},
				chunkSize: 3,
			},
			want: [][]string{
				{"a", "b", "c"},
				{"d"},
			},
		},
		{
			name: "chunkSize equal to total count",
			args: args{
				targets:   []string{"a", "b", "c", "d"},
				chunkSize: 4,
			},
			want: [][]string{
				{"a", "b", "c", "d"},
			},
		},
		{
			name: "chunkSize greater than total count",
			args: args{
				targets:   []string{"a", "b", "c", "d"},
				chunkSize: 5,
			},
			want: [][]string{
				{"a", "b", "c", "d"},
			},
		},
		{
			name: "chunk nil slice",
			args: args{
				targets:   nil,
				chunkSize: 2,
			},
			want: nil,
		},
		{
			name: "chunk empty slice",
			args: args{
				targets:   []string{},
				chunkSize: 2,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChunkStrings(tt.args.targets, tt.args.chunkSize)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected int
	}{
		{
			name:     "identical strings",
			a:        "hello",
			b:        "hello",
			expected: 0,
		},
		{
			name:     "one insertion",
			a:        "hello",
			b:        "helloo",
			expected: 1,
		},
		{
			name:     "one deletion",
			a:        "hello",
			b:        "helo",
			expected: 1,
		},
		{
			name:     "one substitution",
			a:        "hello",
			b:        "hallo",
			expected: 1,
		},
		{
			name:     "empty first string",
			a:        "",
			b:        "hello",
			expected: 5,
		},
		{
			name:     "empty second string",
			a:        "hello",
			b:        "",
			expected: 5,
		},
		{
			name:     "both empty",
			a:        "",
			b:        "",
			expected: 0,
		},
		{
			name:     "completely different",
			a:        "abc",
			b:        "xyz",
			expected: 3,
		},
		{
			name:     "kitten to sitting",
			a:        "kitten",
			b:        "sitting",
			expected: 3,
		},
		{
			name:     "longer strings with UUID difference",
			a:        "RequestID: f25747d6-52b6-4db6-bde2-a38aed0fa036",
			b:        "RequestID: a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			expected: 29,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := LevenshteinDistance(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStringsSimilar(t *testing.T) {
	tests := []struct {
		name      string
		a         string
		b         string
		threshold float64
		expected  bool
	}{
		{
			name:      "identical strings",
			a:         "hello world",
			b:         "hello world",
			threshold: 0.85,
			expected:  true,
		},
		{
			name:      "both empty strings",
			a:         "",
			b:         "",
			threshold: 0.85,
			expected:  true,
		},
		{
			name:      "one empty string",
			a:         "",
			b:         "hello",
			threshold: 0.85,
			expected:  false,
		},
		{
			name:      "other empty string",
			a:         "hello",
			b:         "",
			threshold: 0.85,
			expected:  false,
		},
		{
			name:      "completely different strings",
			a:         "hello",
			b:         "world",
			threshold: 0.85,
			expected:  false,
		},
		{
			name:      "AWS error messages with different request IDs",
			a:         "operation error Elastic Load Balancingv2: CreateRule, https response error StatusCode: 400, RequestID: f25747d6-52b6-4db6-bde2-a38aed0fa036, InvalidLoadBalancerAction: The redirect configuration is not valid",
			b:         "operation error Elastic Load Balancingv2: CreateRule, https response error StatusCode: 400, RequestID: a1b2c3d4-e5f6-7890-abcd-ef1234567890, InvalidLoadBalancerAction: The redirect configuration is not valid",
			threshold: 0.85,
			expected:  true,
		},
		{
			name:      "similar messages with timestamp difference",
			a:         "operation failed at 2024-01-15T10:30:00Z: connection timeout",
			b:         "operation failed at 2024-01-15T10:31:00Z: connection timeout",
			threshold: 0.85,
			expected:  true,
		},
		{
			name:      "high threshold rejects similar messages",
			a:         "error with ID: abc123",
			b:         "error with ID: xyz789",
			threshold: 0.95,
			expected:  false,
		},
		{
			name:      "low threshold accepts different messages",
			a:         "error A",
			b:         "error B",
			threshold: 0.5,
			expected:  true,
		},
		{
			name:      "zero threshold accepts everything",
			a:         "completely",
			b:         "different",
			threshold: 0.0,
			expected:  true,
		},
		{
			name:      "threshold of 1.0 requires exact match",
			a:         "hello",
			b:         "hello!",
			threshold: 1.0,
			expected:  false,
		},
		{
			name:      "threshold of 1.0 with exact match",
			a:         "hello",
			b:         "hello",
			threshold: 1.0,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringsSimilar(tt.a, tt.b, tt.threshold)
			assert.Equal(t, tt.expected, result)
		})
	}
}
