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
