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

// PartitionDNSSuffix returns the ELB DNS suffix for the given region's partition.
func PartitionDNSSuffix(region string) string {
	r := strings.ToLower(region)
	switch {
	case strings.HasPrefix(r, "cn-"):
		return "amazonaws.com.cn"
	case strings.HasPrefix(r, "us-isob-"):
		return "sc2s.sgov.gov"
	case strings.HasPrefix(r, "us-isof-"):
		return "csp.hci.ic.gov"
	case strings.HasPrefix(r, "us-iso-"):
		return "c2s.ic.gov"
	case strings.HasPrefix(r, "eu-isoe-"):
		return "cloud.adc-e.uk"
	default:
		return "amazonaws.com"
	}
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
