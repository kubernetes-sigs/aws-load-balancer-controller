package equality

import (
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestIgnoreStringSliceOrder(t *testing.T) {
	type testStruct struct {
		SliceField []string
	}

	tests := []struct {
		name       string
		argLeft    testStruct
		argRight   testStruct
		wantEquals bool
	}{
		{
			name: "two string slice equals",
			argLeft: testStruct{
				SliceField: []string{"A", "B", "C"},
			},
			argRight: testStruct{
				SliceField: []string{"A", "B", "C"},
			},
			wantEquals: true,
		},
		{
			name: "two string slice equals without order",
			argLeft: testStruct{
				SliceField: []string{"A", "B", "C"},
			},
			argRight: testStruct{
				SliceField: []string{"C", "B", "A"},
			},
			wantEquals: true,
		},
		{
			name: "two string slice not equals",
			argLeft: testStruct{
				SliceField: []string{"A", "B", "C"},
			},
			argRight: testStruct{
				SliceField: []string{"A", "B", "D"},
			},
			wantEquals: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := IgnoreStringSliceOrder()
			gotEquals := cmp.Equal(tt.argLeft, tt.argRight, opts)
			assert.Equal(t, tt.wantEquals, gotEquals)
		})
	}
}
