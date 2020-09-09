package backend

import (
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"testing"
)

func TestWithNodeSelector(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := EndpointResolveOptions{
				NodeSelector: labels.Nothing(),
			}
			opts.ApplyOptions(tt.opts)
			assert.Equal(t, tt.want.NodeSelector, opts.NodeSelector)
		})
	}
}

func TestWithUnreadyPodInclusionCriterion(t *testing.T) {
	podPredicateTrue := func(pod *corev1.Pod) bool { return true }
	podPredicateFalse := func(pod *corev1.Pod) bool { return false }
	tests := []struct {
		name string
		opts []EndpointResolveOption
		want EndpointResolveOptions
	}{
		{
			name: "zero pod predict",
			opts: []EndpointResolveOption{},
			want: EndpointResolveOptions{
				NodeSelector:                labels.Nothing(),
				UnreadyPodInclusionCriteria: nil,
			},
		},
		{
			name: "one pod predict",
			opts: []EndpointResolveOption{WithUnreadyPodInclusionCriterion(podPredicateTrue)},
			want: EndpointResolveOptions{
				NodeSelector:                labels.Nothing(),
				UnreadyPodInclusionCriteria: []PodPredicate{podPredicateTrue},
			},
		},
		{
			name: "two pod predict",
			opts: []EndpointResolveOption{WithUnreadyPodInclusionCriterion(podPredicateTrue), WithUnreadyPodInclusionCriterion(podPredicateFalse)},
			want: EndpointResolveOptions{
				NodeSelector:                labels.Nothing(),
				UnreadyPodInclusionCriteria: []PodPredicate{podPredicateTrue, podPredicateFalse},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := EndpointResolveOptions{
				NodeSelector: labels.Nothing(),
			}
			opts.ApplyOptions(tt.opts)
			assert.Equal(t, len(tt.want.UnreadyPodInclusionCriteria), len(opts.UnreadyPodInclusionCriteria))
		})
	}
}
