package equality

import (
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"testing"
)

type NestedStruct struct {
	StrField string
}

type TestStruct struct {
	StrField       string
	SliceField     []string
	MapField       map[string]string
	StructField    NestedStruct
	StructPtrField *NestedStruct
}

func TestIgnoreLeftHandUnset(t *testing.T) {
	tests := []struct {
		name       string
		argLeft    TestStruct
		argRight   TestStruct
		fields     []string
		wantEquals bool
	}{
		{
			name: "when StrField equals",
			argLeft: TestStruct{
				StrField: "a",
			},
			argRight: TestStruct{
				StrField: "a",
			},
			fields:     []string{"StrField"},
			wantEquals: true,
		},
		{
			name: "when StrField differs",
			argLeft: TestStruct{
				StrField: "a",
			},
			argRight: TestStruct{
				StrField: "b",
			},
			fields:     []string{"StrField"},
			wantEquals: false,
		},
		{
			name: "when SliceField equals",
			argLeft: TestStruct{
				SliceField: []string{"a", "b"},
			},
			argRight: TestStruct{
				SliceField: []string{"a", "b"},
			},
			fields:     []string{"SliceField"},
			wantEquals: true,
		},
		{
			name: "when SliceField differs",
			argLeft: TestStruct{
				SliceField: []string{"a", "b"},
			},
			argRight: TestStruct{
				SliceField: []string{"b", "a"},
			},
			fields:     []string{"SliceField"},
			wantEquals: false,
		},
		{
			name: "when left hand arg have nil SliceField",
			argLeft: TestStruct{
				SliceField: nil,
			},
			argRight: TestStruct{
				SliceField: []string{"b", "a"},
			},
			fields:     []string{"SliceField"},
			wantEquals: true,
		},
		{
			name: "when left hand arg have non-nil but empty SliceField",
			argLeft: TestStruct{
				SliceField: []string{},
			},
			argRight: TestStruct{
				SliceField: []string{"b", "a"},
			},
			fields:     []string{"SliceField"},
			wantEquals: false,
		},
		{
			name: "when MapField equals",
			argLeft: TestStruct{
				MapField: map[string]string{"k": "v"},
			},
			argRight: TestStruct{
				MapField: map[string]string{"k": "v"},
			},
			fields:     []string{"MapField"},
			wantEquals: true,
		},
		{
			name: "when MapField differs by value",
			argLeft: TestStruct{
				MapField: map[string]string{"k": "v1"},
			},
			argRight: TestStruct{
				MapField: map[string]string{"k": "v2"},
			},
			fields:     []string{"MapField"},
			wantEquals: false,
		},
		{
			name: "when MapField differs by key",
			argLeft: TestStruct{
				MapField: map[string]string{"k1": "v"},
			},
			argRight: TestStruct{
				MapField: map[string]string{"k2": "v"},
			},
			fields:     []string{"MapField"},
			wantEquals: false,
		},
		{
			name: "when left hand arg have nil MapField",
			argLeft: TestStruct{
				MapField: nil,
			},
			argRight: TestStruct{
				MapField: map[string]string{"k": "v"},
			},
			fields:     []string{"MapField"},
			wantEquals: true,
		},
		{
			name: "when left hand arg have non-nil but empty MapField",
			argLeft: TestStruct{
				MapField: map[string]string{},
			},
			argRight: TestStruct{
				MapField: map[string]string{"k": "v"},
			},
			fields:     []string{"MapField"},
			wantEquals: false,
		},
		{
			name: "when StructField equals",
			argLeft: TestStruct{
				StructField: NestedStruct{
					StrField: "a",
				},
			},
			argRight: TestStruct{
				StructField: NestedStruct{
					StrField: "a",
				},
			},
			fields:     []string{"StructField"},
			wantEquals: true,
		},
		{
			name: "when StructField differs",
			argLeft: TestStruct{
				StructField: NestedStruct{
					StrField: "a",
				},
			},
			argRight: TestStruct{
				StructField: NestedStruct{
					StrField: "b",
				},
			},
			fields:     []string{"StructField"},
			wantEquals: false,
		},
		{
			name: "when StructPtrField equals",
			argLeft: TestStruct{
				StructPtrField: &NestedStruct{
					StrField: "a",
				},
			},
			argRight: TestStruct{
				StructPtrField: &NestedStruct{
					StrField: "a",
				},
			},
			fields:     []string{"StructPtrField"},
			wantEquals: true,
		},
		{
			name: "when StructPtrField differs",
			argLeft: TestStruct{
				StructPtrField: &NestedStruct{
					StrField: "a",
				},
			},
			argRight: TestStruct{
				StructPtrField: &NestedStruct{
					StrField: "b",
				},
			},
			fields:     []string{"StructPtrField"},
			wantEquals: false,
		},
		{
			name: "when left hand arg have nil StructPtrField",
			argLeft: TestStruct{
				StructPtrField: nil,
			},
			argRight: TestStruct{
				StructPtrField: &NestedStruct{
					StrField: "b",
				},
			},
			fields:     []string{"StructPtrField"},
			wantEquals: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := IgnoreLeftHandUnset(TestStruct{}, tt.fields...)
			gotEquals := cmp.Equal(tt.argLeft, tt.argRight, opts)
			assert.Equal(t, tt.wantEquals, gotEquals)
		})
	}
}
