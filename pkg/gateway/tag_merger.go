package gateway

import (
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
)

func mergeTags(highPriority *map[string]string, lowPriority *map[string]string) *map[string]string {
	baseTags := make(map[string]string)

	if highPriority != nil {
		baseTags = algorithm.MergeStringMap(baseTags, *highPriority)
	}

	if lowPriority != nil {
		baseTags = algorithm.MergeStringMap(baseTags, *lowPriority)
	}
	return &baseTags
}
