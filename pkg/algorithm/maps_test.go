package algorithm

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMapFindFirst(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		maps      []map[string]string
		wantValue string
		wantFound bool
	}{
		{
			name: "no occurrence",
			key:  "m00nf1sh",
			maps: []map[string]string{
				{
					"foo": "foo_value",
				},
				{
					"bar": "bar_value",
				},
			},
			wantValue: "",
			wantFound: false,
		},
		{
			name: "occurrence in first map",
			key:  "m00nf1sh",
			maps: []map[string]string{
				{
					"m00nf1sh": "hello",
				},
				{
					"bar": "bar_value",
				},
			},
			wantValue: "hello",
			wantFound: true,
		},
		{
			name: "occurrence in second map",
			key:  "m00nf1sh",
			maps: []map[string]string{
				{
					"foo": "foo_value",
				},
				{
					"m00nf1sh": "world",
				},
			},
			wantValue: "world",
			wantFound: true,
		},
		{
			name: "occurrence in both map",
			key:  "m00nf1sh",
			maps: []map[string]string{
				{
					"m00nf1sh": "hello",
				},
				{
					"m00nf1sh": "world",
				},
			},
			wantValue: "hello",
			wantFound: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := MapFindFirst(tt.key, tt.maps...)
			assert.Equal(t, tt.wantValue, got)
			assert.Equal(t, tt.wantFound, found)
		})
	}
}

func TestMergeStringMap(t *testing.T) {
	type args struct {
		maps []map[string]string
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "merge two maps without duplicates",
			args: args{
				maps: []map[string]string{
					{
						"a": "1",
						"b": "2",
					},
					{
						"c": "3",
						"d": "4",
					},
				},
			},
			want: map[string]string{
				"a": "1",
				"b": "2",
				"c": "3",
				"d": "4",
			},
		},
		{
			name: "merge two maps with duplicates",
			args: args{
				maps: []map[string]string{
					{
						"a": "1",
						"b": "2",
					},
					{
						"a": "3",
						"d": "4",
					},
				},
			},
			want: map[string]string{
				"a": "1",
				"b": "2",
				"d": "4",
			},
		},
		{
			name: "merge two maps when first map is nil",
			args: args{
				maps: []map[string]string{
					nil,
					{
						"c": "3",
						"d": "4",
					},
				},
			},
			want: map[string]string{
				"c": "3",
				"d": "4",
			},
		},
		{
			name: "merge two maps when second map is nil",
			args: args{
				maps: []map[string]string{
					{
						"a": "1",
						"b": "2",
					},
					nil,
				},
			},
			want: map[string]string{
				"a": "1",
				"b": "2",
			},
		},
		{
			name: "merge two maps when both map is nil",
			args: args{
				maps: []map[string]string{
					nil,
					nil,
				},
			},
			want: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeStringMap(tt.args.maps...)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDiffStringMap(t *testing.T) {
	type args struct {
		desired map[string]string
		current map[string]string
	}
	tests := []struct {
		name       string
		args       args
		wantUpdate map[string]string
		wantRemove map[string]string
	}{
		{
			name: "standard case",
			args: args{
				desired: map[string]string{
					"a": "a1",
					"b": "b1",
					"c": "c1",
				},
				current: map[string]string{
					"a": "a1",
					"b": "b2",
					"d": "d1",
				},
			},
			wantUpdate: map[string]string{
				"b": "b1",
				"c": "c1",
			},
			wantRemove: map[string]string{
				"d": "d1",
			},
		},
		{
			name: "only need to update",
			args: args{
				desired: map[string]string{
					"a": "a1",
					"b": "b1",
					"c": "c1",
				},
				current: map[string]string{
					"a": "a1",
					"b": "b1",
				},
			},
			wantUpdate: map[string]string{
				"c": "c1",
			},
			wantRemove: map[string]string{},
		},
		{
			name: "only need to remove",
			args: args{
				desired: map[string]string{
					"a": "a1",
					"b": "b1",
				},
				current: map[string]string{
					"a": "a1",
					"b": "b1",
					"c": "c1",
				},
			},
			wantUpdate: map[string]string{},
			wantRemove: map[string]string{
				"c": "c1",
			},
		},
		{
			name: "both map are equal",
			args: args{
				desired: map[string]string{
					"a": "a1",
					"b": "b1",
					"c": "c1",
				},
				current: map[string]string{
					"a": "a1",
					"b": "b1",
					"c": "c1",
				},
			},
			wantUpdate: map[string]string{},
			wantRemove: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUpdate, gotRemove := DiffStringMap(tt.args.desired, tt.args.current)
			assert.Equal(t, tt.wantUpdate, gotUpdate)
			assert.Equal(t, tt.wantRemove, gotRemove)
		})
	}
}
