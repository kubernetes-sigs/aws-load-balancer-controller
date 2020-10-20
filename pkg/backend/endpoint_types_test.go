package backend

import (
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"testing"
)

func TestEndpointResolveOptions_ApplyOptions(t *testing.T) {
	tests := []struct {
		name string
		opts []EndpointResolveOption
		want EndpointResolveOptions
	}{
		{
			name: "set labelSelector",
			opts: []EndpointResolveOption{WithNodeSelector(labels.Everything())},
			want: EndpointResolveOptions{
				NodeSelector: labels.Everything(),
			},
		},
		{
			name: "set two readinessGate",
			opts: []EndpointResolveOption{
				WithPodReadinessGate("target-health.ingress.k8s.aws/some-tgb-1"),
				WithPodReadinessGate("target-health.ingress.k8s.aws/some-tgb-2"),
			},
			want: EndpointResolveOptions{
				NodeSelector: labels.Nothing(),
				PodReadinessGates: []corev1.PodConditionType{
					"target-health.ingress.k8s.aws/some-tgb-1",
					"target-health.ingress.k8s.aws/some-tgb-2",
				},
			},
		},
		{
			name: "set labelSelector & readinessGate",
			opts: []EndpointResolveOption{
				WithNodeSelector(labels.Everything()),
				WithPodReadinessGate("target-health.ingress.k8s.aws/some-tgb"),
			},
			want: EndpointResolveOptions{
				NodeSelector:      labels.Everything(),
				PodReadinessGates: []corev1.PodConditionType{"target-health.ingress.k8s.aws/some-tgb"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultEndpointResolveOptions()
			opts.ApplyOptions(tt.opts)
			assert.Equal(t, tt.want.NodeSelector, opts.NodeSelector)
		})
	}
}
