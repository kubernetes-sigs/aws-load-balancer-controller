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

func TestDiffStringSlice(t *testing.T) {
	a := "a"
	b := "b"
	c := "c"

	type args struct {
		first  []string
		second []string
	}
	type want struct {
		matchFirst  []*string
		matchBoth   []*string
		matchSecond []*string
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "when only first has values",
			args: args{
				first:  []string{"a", "b"},
				second: []string{},
			},
			want: want{
				matchFirst:  []*string{&a, &b},
				matchBoth:   []*string{},
				matchSecond: []*string{},
			},
		},
		{
			name: "when only second has values",
			args: args{
				first:  []string{},
				second: []string{"a", "b"},
			},
			want: want{
				matchFirst:  []*string{},
				matchBoth:   []*string{},
				matchSecond: []*string{&a, &b},
			},
		},
		{
			name: "when first and second are identical",
			args: args{
				first:  []string{"a", "b"},
				second: []string{"a", "b"},
			},
			want: want{
				matchFirst:  []*string{},
				matchBoth:   []*string{&a, &b},
				matchSecond: []*string{},
			},
		},
		{
			name: "when no values are used",
			args: args{
				first:  []string{},
				second: []string{},
			},
			want: want{
				matchFirst:  []*string{},
				matchBoth:   []*string{},
				matchSecond: []*string{},
			},
		},
		{
			name: "when all return values are required",
			args: args{
				first:  []string{"a", "b"},
				second: []string{"b", "c"},
			},
			want: want{
				matchFirst:  []*string{&a},
				matchBoth:   []*string{&b},
				matchSecond: []*string{&c},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// t.Parallel()
			matchFirst, matchBoth, matchSecond := DiffStringSlice(tt.args.first, tt.args.second)
			assert.Equal(t, tt.want.matchFirst, matchFirst)
			assert.Equal(t, tt.want.matchBoth, matchBoth)
			assert.Equal(t, tt.want.matchSecond, matchSecond)
		})
	}
}
