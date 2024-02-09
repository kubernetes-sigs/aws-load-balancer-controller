package deploy

import (
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"testing"
)

func Test_stackSchemaBuilder_Visit(t *testing.T) {
	stack := core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "name"})
	resA := core.NewFakeResource(stack, "typeX", "resA", core.FakeResourceSpec{
		FieldA: []core.StringToken{core.LiteralStringToken("valueA")},
	}, nil)
	resB := core.NewFakeResource(stack, "typeX", "resB", core.FakeResourceSpec{
		FieldA: []core.StringToken{resA.FieldB()},
	}, nil)
	resC := core.NewFakeResource(stack, "typeY", "resC", core.FakeResourceSpec{
		FieldA: []core.StringToken{core.LiteralStringToken("valueA"), resB.FieldB()},
	}, nil)

	type args struct {
		res core.Resource
	}
	tests := []struct {
		name            string
		args            []args
		wantStackSchema StackSchema
	}{
		{
			name: "single resource",
			args: []args{
				{
					res: resA,
				},
			},
			wantStackSchema: StackSchema{
				ID: "namespace/name",
				Resources: map[string]map[string]interface{}{
					"typeX": {
						"resA": resA,
					},
				}},
		},
		{
			name: "multiple resources",
			args: []args{
				{
					res: resA,
				},
				{
					res: resB,
				},
				{
					res: resC,
				},
			},
			wantStackSchema: StackSchema{
				ID: "namespace/name",
				Resources: map[string]map[string]interface{}{
					"typeX": {
						"resA": resA,
						"resB": resB,
					},
					"typeY": {
						"resC": resC,
					},
				}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewStackSchemaBuilder(stack.StackID())
			for _, arg := range tt.args {
				b.Visit(arg.res)
			}
			gotStackSchema := b.Build()
			assert.Equal(t, tt.wantStackSchema, gotStackSchema)
		})
	}
}
