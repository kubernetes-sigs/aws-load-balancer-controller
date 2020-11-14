package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTags_Matches(t *testing.T) {
	tests := []struct {
		name        string
		desiredTags map[string][]string
		checkedTags map[string]string
		want        bool
	}{
		{
			name:        "empty tagFilter should match everything",
			desiredTags: map[string][]string{},
			checkedTags: map[string]string{
				"tagA": "valueA",
			},
			want: true,
		},
		{
			name: "tagFilter with key only should match if key exists",
			desiredTags: map[string][]string{
				"tagA": {},
			},
			checkedTags: map[string]string{
				"tagA": "valueA1",
			},
			want: true,
		},
		{
			name: "tagFilter with key and single value should match if value matches",
			desiredTags: map[string][]string{
				"tagA": {"valueA1"},
			},
			checkedTags: map[string]string{
				"tagA": "valueA1",
			},
			want: true,
		},
		{
			name: "tagFilter with key and single value should mismatch if value mismatches",
			desiredTags: map[string][]string{
				"tagA": {"valueA2"},
			},
			checkedTags: map[string]string{
				"tagA": "valueA1",
			},
			want: false,
		},
		{
			name: "tagFilter with key and multiple values should match if any value matches",
			desiredTags: map[string][]string{
				"tagA": {"valueA1", "valueA2"},
			},
			checkedTags: map[string]string{
				"tagA": "valueA2",
			},
			want: true,
		},
		{
			name: "tagFilter with key and multiple values should mismatch if no value matches",
			desiredTags: map[string][]string{
				"tagA": {"valueA1", "valueA2"},
			},
			checkedTags: map[string]string{
				"tagA": "valueA3",
			},
			want: false,
		},
		{
			name: "multiple tagFilter matches if all of them matches",
			desiredTags: map[string][]string{
				"tagA": {},
				"tagB": {"valueB1"},
				"tagC": {"valueC1", "valueC2"},
			},
			checkedTags: map[string]string{
				"tagA": "valueA1",
				"tagB": "valueB1",
				"tagC": "valueC2",
			},
			want: true,
		},
		{
			name: "multiple tagFilter mismatches if any of them mismatches",
			desiredTags: map[string][]string{
				"tagA": {},
				"tagB": {"valueB1"},
				"tagC": {"valueC1", "valueC2"},
			},
			checkedTags: map[string]string{
				"tagA": "valueA1",
				"tagB": "valueB1",
				"tagC": "valueC3",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Matches(tt.checkedTags, tt.desiredTags)
			assert.Equal(t, tt.want, got)
		})
	}
}
