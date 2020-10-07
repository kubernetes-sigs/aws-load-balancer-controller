package deploy

import (
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"testing"
)

func Test_defaultDeployer_Deploy(t *testing.T) {
	tests := []struct {
		name           string
		modelBuildFunc func() core.Stack
		want           string
		wantErr        error
	}{
		{
			name: "single resource",
			modelBuildFunc: func() core.Stack {
				stack := core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "name"})
				_ = core.NewFakeResource(stack, "typeX", "resA", core.FakeResourceSpec{
					FieldA: []core.StringToken{core.LiteralStringToken("valueA")},
				}, nil)
				return stack
			},
			want: `{"id":"namespace/name","resources":{"typeX":{"resA":{"spec":{"fieldA":["valueA"]}}}}}`,
		},
		{
			name: "multiple resources",
			modelBuildFunc: func() core.Stack {
				stack := core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "name"})
				resA := core.NewFakeResource(stack, "typeX", "resA", core.FakeResourceSpec{
					FieldA: []core.StringToken{core.LiteralStringToken("valueA")},
				}, nil)
				resB := core.NewFakeResource(stack, "typeX", "resB", core.FakeResourceSpec{
					FieldA: []core.StringToken{resA.FieldB()},
				}, nil)
				_ = core.NewFakeResource(stack, "typeY", "resC", core.FakeResourceSpec{
					FieldA: []core.StringToken{core.LiteralStringToken("valueA"), resB.FieldB()},
				}, nil)
				return stack
			},
			want: `{"id":"namespace/name","resources":{"typeX":{"resA":{"spec":{"fieldA":["valueA"]}},"resB":{"spec":{"fieldA":[{"$ref":"#/resources/typeX/resA/status/fieldB"}]}}},"typeY":{"resC":{"spec":{"fieldA":["valueA",{"$ref":"#/resources/typeX/resB/status/fieldB"}]}}}}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDefaultStackMarshaller()
			stack := tt.modelBuildFunc()
			got, err := d.Marshal(stack)
			assert.Equal(t, tt.want, got)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
		})
	}
}
