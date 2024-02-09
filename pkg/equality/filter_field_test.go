package equality

import (
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFilterField(t *testing.T) {
	type testStruct struct {
		SliceField0 []string
		SliceField1 []string
	}
	type args struct {
		field string
		opt   cmp.Option
	}
	tests := []struct {
		name       string
		args       args
		argLeft    testStruct
		argRight   testStruct
		wantEquals bool
	}{
		{
			name: "both sliceField equals",
			args: args{
				field: "SliceField0",
				opt:   IgnoreStringSliceOrder(),
			},
			argLeft: testStruct{
				SliceField0: []string{"A", "B", "C"},
				SliceField1: []string{"1", "2", "3"},
			},
			argRight: testStruct{
				SliceField0: []string{"A", "B", "C"},
				SliceField1: []string{"1", "2", "3"},
			},
			wantEquals: true,
		},
		{
			name: "SliceField0 compares without order - equals",
			args: args{
				field: "SliceField0",
				opt:   IgnoreStringSliceOrder(),
			},
			argLeft: testStruct{
				SliceField0: []string{"A", "B", "C"},
				SliceField1: []string{"1", "2", "3"},
			},
			argRight: testStruct{
				SliceField0: []string{"C", "B", "A"},
				SliceField1: []string{"1", "2", "3"},
			},
			wantEquals: true,
		},
		{
			name: "SliceField0 compares without order - not equals",
			args: args{
				field: "SliceField0",
				opt:   IgnoreStringSliceOrder(),
			},
			argLeft: testStruct{
				SliceField0: []string{"A", "B", "C"},
				SliceField1: []string{"1", "2", "3"},
			},
			argRight: testStruct{
				SliceField0: []string{"A", "B", "D"},
				SliceField1: []string{"1", "2", "3"},
			},
			wantEquals: false,
		},
		{
			name: "SliceField0 compares without order - slice1 not equals",
			args: args{
				field: "SliceField0",
				opt:   IgnoreStringSliceOrder(),
			},
			argLeft: testStruct{
				SliceField0: []string{"A", "B", "C"},
				SliceField1: []string{"1", "2", "3"},
			},
			argRight: testStruct{
				SliceField0: []string{"A", "B", "C"},
				SliceField1: []string{"3", "2", "1"},
			},
			wantEquals: false,
		},
		{
			name: "SliceField1 compares without order - equals",
			args: args{
				field: "SliceField1",
				opt:   IgnoreStringSliceOrder(),
			},
			argLeft: testStruct{
				SliceField0: []string{"A", "B", "C"},
				SliceField1: []string{"1", "2", "3"},
			},
			argRight: testStruct{
				SliceField0: []string{"A", "B", "C"},
				SliceField1: []string{"3", "2", "1"},
			},
			wantEquals: true,
		},
		{
			name: "SliceField1 compares without order - not equals",
			args: args{
				field: "SliceField1",
				opt:   IgnoreStringSliceOrder(),
			},
			argLeft: testStruct{
				SliceField0: []string{"A", "B", "C"},
				SliceField1: []string{"1", "2", "3"},
			},
			argRight: testStruct{
				SliceField0: []string{"A", "B", "C"},
				SliceField1: []string{"1", "2", "4"},
			},
			wantEquals: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := FilterField(testStruct{}, tt.args.field, tt.args.opt)
			gotEquals := cmp.Equal(tt.argLeft, tt.argRight, opts)
			assert.Equal(t, tt.wantEquals, gotEquals)
		})
	}
}
