package tagging

import (
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
	"testing"
)

func Test_defaultProvider_ResourceIDTagKey(t *testing.T) {
	tests := []struct {
		name     string
		provider *defaultProvider
		want     string
	}{
		{
			name:     "resourceTagKey for Ingress",
			provider: NewDefaultProvider("ingress.k8s.aws", "cluster-name"),
			want:     "ingress.k8s.aws/resource",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.provider.ResourceIDTagKey()
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultProvider_StackTags(t *testing.T) {
	type args struct {
		stack core.Stack
	}
	tests := []struct {
		name     string
		provider *defaultProvider
		args     args
		want     map[string]string
	}{
		{
			name:     "stackTags for Ingress",
			provider: NewDefaultProvider("ingress.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack("namespace/name")},
			want: map[string]string{
				"ingress.k8s.aws/cluster": "cluster-name",
				"ingress.k8s.aws/stack":   "namespace/name",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.provider.StackTags(tt.args.stack)
			assert.Equal(t, tt.want, got)
		})
	}
}
