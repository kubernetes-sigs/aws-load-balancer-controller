package tagging

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMultiValueTagFilter_Matches(t *testing.T) {
	type args struct {
		tags map[string]string
	}
	tests := []struct {
		name string
		f    MultiValueTagFilter
		args args
		want bool
	}{
		{
			name: "empty tagFilter should match everything",
			f:    MultiValueTagFilter{},
			args: args{
				tags: map[string]string{
					"tagA": "valueA",
				},
			},
			want: true,
		},
		{
			name: "tagFilter with key only should match if key exists",
			f: MultiValueTagFilter{
				"tagA": {},
			},
			args: args{
				tags: map[string]string{
					"tagA": "valueA1",
				},
			},
			want: true,
		},
		{
			name: "tagFilter with key and single value should match if value matches",
			f: MultiValueTagFilter{
				"tagA": {"valueA1"},
			},
			args: args{
				tags: map[string]string{
					"tagA": "valueA1",
				},
			},
			want: true,
		},
		{
			name: "tagFilter with key and single value should mismatch if value mismatches",
			f: MultiValueTagFilter{
				"tagA": {"valueA2"},
			},
			args: args{
				tags: map[string]string{
					"tagA": "valueA1",
				},
			},
			want: false,
		},
		{
			name: "tagFilter with key and multiple values should match if any value matches",
			f: MultiValueTagFilter{
				"tagA": {"valueA1", "valueA2"},
			},
			args: args{
				tags: map[string]string{
					"tagA": "valueA2",
				},
			},
			want: true,
		},
		{
			name: "tagFilter with key and multiple values should mismatch if no value matches",
			f: MultiValueTagFilter{
				"tagA": {"valueA1", "valueA2"},
			},
			args: args{
				tags: map[string]string{
					"tagA": "valueA3",
				},
			},
			want: false,
		},
		{
			name: "multiple tagFilter matches if all of them matches",
			f: MultiValueTagFilter{
				"tagA": {},
				"tagB": {"valueB1"},
				"tagC": {"valueC1", "valueC2"},
			},
			args: args{
				tags: map[string]string{
					"tagA": "valueA1",
					"tagB": "valueB1",
					"tagC": "valueC2",
				},
			},
			want: true,
		},
		{
			name: "multiple tagFilter mismatches if any of them mismatches",
			f: MultiValueTagFilter{
				"tagA": {},
				"tagB": {"valueB1"},
				"tagC": {"valueC1", "valueC2"},
			},
			args: args{
				tags: map[string]string{
					"tagA": "valueA1",
					"tagB": "valueB1",
					"tagC": "valueC3",
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.f.Matches(tt.args.tags)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTagsAsMultiValueTagFilter(t *testing.T) {
	type args struct {
		tags map[string]string
	}
	tests := []struct {
		name string
		args args
		want MultiValueTagFilter
	}{
		{
			name: "single k-v pair",
			args: args{
				tags: map[string]string{
					"key": "value",
				},
			},
			want: MultiValueTagFilter{
				"key": {"value"},
			},
		},
		{
			name: "multiple k-v pair",
			args: args{
				tags: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			want: MultiValueTagFilter{
				"key1": {"value1"},
				"key2": {"value2"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TagsAsMultiValueTagFilter(tt.args.tags)
			assert.Equal(t, tt.want, got)
		})
	}
}
