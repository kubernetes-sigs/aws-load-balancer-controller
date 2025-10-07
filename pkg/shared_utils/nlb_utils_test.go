package shared_utils

import (
	"reflect"
	"testing"

	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

func TestMakeAttributesSliceFromMap(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]string
		want  []elbv2model.LoadBalancerAttribute
	}{
		{
			name:  "empty map",
			input: map[string]string{},
			want:  []elbv2model.LoadBalancerAttribute{},
		},
		{
			name: "single attribute",
			input: map[string]string{
				"key1": "value1",
			},
			want: []elbv2model.LoadBalancerAttribute{
				{Key: "key1", Value: "value1"},
			},
		},
		{
			name: "multiple attributes, unordered input",
			input: map[string]string{
				"zeta":  "last",
				"alpha": "first",
				"mid":   "middle",
			},
			want: []elbv2model.LoadBalancerAttribute{
				{Key: "alpha", Value: "first"},
				{Key: "mid", Value: "middle"},
				{Key: "zeta", Value: "last"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MakeAttributesSliceFromMap(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MakeAttributesSliceFromMap() = %v, want %v", got, tt.want)
			}
		})
	}
}
