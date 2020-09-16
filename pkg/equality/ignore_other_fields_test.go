package equality

import (
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestIgnoreOtherFields(t *testing.T) {
	type testStruct struct {
		StrFieldA string
		StrFieldB string
		StrFieldC string
	}

	type args struct {
		typ    interface{}
		fields []string
	}
	tests := []struct {
		name       string
		args       args
		argLeft    testStruct
		argRight   testStruct
		wantEquals bool
	}{
		{
			name: "only consider strFieldA - all fields equals",
			args: args{
				typ:    testStruct{},
				fields: []string{"StrFieldA"},
			},
			argLeft: testStruct{
				StrFieldA: "A0",
				StrFieldB: "B0",
				StrFieldC: "C0",
			},
			argRight: testStruct{
				StrFieldA: "A0",
				StrFieldB: "B0",
				StrFieldC: "C0",
			},
			wantEquals: true,
		},
		{
			name: "only consider strFieldA - equals if StrFieldA equals",
			args: args{
				typ:    testStruct{},
				fields: []string{"StrFieldA"},
			},
			argLeft: testStruct{
				StrFieldA: "A0",
				StrFieldB: "B0",
				StrFieldC: "C0",
			},
			argRight: testStruct{
				StrFieldA: "A0",
				StrFieldB: "B1",
				StrFieldC: "C1",
			},
			wantEquals: true,
		},
		{
			name: "only consider strFieldA - not equals if StrFieldA not equals",
			args: args{
				typ:    testStruct{},
				fields: []string{"StrFieldA"},
			},
			argLeft: testStruct{
				StrFieldA: "A0",
				StrFieldB: "B0",
				StrFieldC: "C0",
			},
			argRight: testStruct{
				StrFieldA: "A1",
				StrFieldB: "B0",
				StrFieldC: "C0",
			},
			wantEquals: false,
		},
		{
			name: "only consider strFieldA and strFieldC - equals if StrFieldA and strFieldC equals",
			args: args{
				typ:    testStruct{},
				fields: []string{"StrFieldA", "StrFieldC"},
			},
			argLeft: testStruct{
				StrFieldA: "A0",
				StrFieldB: "B0",
				StrFieldC: "C0",
			},
			argRight: testStruct{
				StrFieldA: "A0",
				StrFieldB: "B1",
				StrFieldC: "C0",
			},
			wantEquals: true,
		},
		{
			name: "only consider strFieldA and strFieldC - not equals if StrFieldA or strFieldC not equals",
			args: args{
				typ:    testStruct{},
				fields: []string{"StrFieldA", "StrFieldC"},
			},
			argLeft: testStruct{
				StrFieldA: "A0",
				StrFieldB: "B0",
				StrFieldC: "C0",
			},
			argRight: testStruct{
				StrFieldA: "A0",
				StrFieldB: "B0",
				StrFieldC: "C1",
			},
			wantEquals: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := IgnoreOtherFields(tt.args.typ, tt.args.fields...)
			gotEquals := cmp.Equal(tt.argLeft, tt.argRight, opts)
			assert.Equal(t, tt.wantEquals, gotEquals)
		})
	}
}
