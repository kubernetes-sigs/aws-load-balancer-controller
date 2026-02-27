package algorithm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_RemoveSliceDuplicates(t *testing.T) {
	type args struct {
		data []string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "empty",
			args: args{
				data: []string{},
			},
			want: []string{},
		},
		{
			name: "no duplicate entries",
			args: args{
				data: []string{"a", "b", "c", "d"},
			},
			want: []string{"a", "b", "c", "d"},
		},
		{
			name: "with duplicates",
			args: args{
				data: []string{"a", "b", "a", "c", "b"},
			},
			want: []string{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RemoveSliceDuplicates(tt.args.data)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsDiffStringSlice(t *testing.T) {
	tests := []struct {
		name   string
		one    []string
		two    []string
		result bool
	}{
		{
			name:   "empty slices",
			one:    nil,
			two:    nil,
			result: true,
		},
		{
			name:   "example with domains",
			one:    []string{"example.com", "one.example.com", "two.example.com"},
			two:    []string{"one.example.com", "two.example.com", "example.com"},
			result: true,
		},
		{
			name:   "exact same order",
			one:    []string{"example.com", "one.example.com", "two.example.com"},
			two:    []string{"example.com", "one.example.com", "two.example.com"},
			result: true,
		},
		{
			name:   "unequal parts",
			one:    []string{"example.com"},
			two:    []string{"otherexample.com"},
			result: false,
		},
		{
			name:   "unequal length",
			one:    []string{"example.com", "otherexample.com"},
			two:    []string{"otherexample.com"},
			result: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := IsDiffStringSlice(tt.one, tt.two)
			assert.Equal(t, tt.result, output)
		})
	}
}
