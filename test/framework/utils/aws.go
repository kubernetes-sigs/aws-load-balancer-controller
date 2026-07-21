package utils

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsCommercialPartition returns true if the region is in the commercial AWS partition
func IsCommercialPartition(region string) bool {
	unsupportedPrefixes := []string{"cn-", "us-gov-", "us-iso", "eu-isoe-"}
	for _, prefix := range unsupportedPrefixes {
		if strings.HasPrefix(strings.ToLower(region), prefix) {
			return false
		}
	}
	return true
}

func GetClusterZones(ctx context.Context, k8sClient client.Client) ([]string, error) {
	nodes := &corev1.NodeList{}
	err := k8sClient.List(ctx, nodes)
	if err != nil {
		return nil, err
	}

	result := sets.New[string]()

	for _, node := range nodes.Items {
		if node.Labels == nil {
			continue
		}
		v, ok := node.Labels["topology.kubernetes.io/zone"]
		if !ok {
			continue
		}
		result.Insert(v)
	}

	return result.UnsortedList(), nil
}
