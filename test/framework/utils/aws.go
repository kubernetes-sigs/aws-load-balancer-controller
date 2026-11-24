package utils

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

// autoModeNodeClassGVK identifies the EKS Auto Mode NodeClass resource.
var autoModeNodeClassGVK = schema.GroupVersionKind{Group: "eks.amazonaws.com", Version: "v1", Kind: "NodeClass"}

// GetClusterZones returns the distinct availability zones of the cluster.
// If nodeClassName is empty, zones are taken from the topology.kubernetes.io/zone label of the
// current nodes. Otherwise, zones are taken from the subnets resolved in the status of the named
// EKS Auto Mode NodeClass, which covers every zone Auto Mode may launch nodes in even if no node
// is currently running there.
func GetClusterZones(ctx context.Context, k8sClient client.Client, nodeClassName string) ([]string, error) {
	if nodeClassName != "" {
		return getAutoModeNodeClassZones(ctx, k8sClient, nodeClassName)
	}
	return getNodeZones(ctx, k8sClient)
}

func getNodeZones(ctx context.Context, k8sClient client.Client) ([]string, error) {
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

func getAutoModeNodeClassZones(ctx context.Context, k8sClient client.Client, nodeClassName string) ([]string, error) {
	nodeClass := &unstructured.Unstructured{}
	nodeClass.SetGroupVersionKind(autoModeNodeClassGVK)
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: nodeClassName}, nodeClass); err != nil {
		return nil, fmt.Errorf("failed to get EKS Auto Mode NodeClass %s: %w", nodeClassName, err)
	}

	subnets, _, err := unstructured.NestedSlice(nodeClass.Object, "status", "subnets")
	if err != nil {
		return nil, fmt.Errorf("failed to read status.subnets of NodeClass %s: %w", nodeClassName, err)
	}

	result := sets.New[string]()
	for _, subnet := range subnets {
		subnetMap, ok := subnet.(map[string]interface{})
		if !ok {
			continue
		}
		zone, _, _ := unstructured.NestedString(subnetMap, "zone")
		if zone != "" {
			result.Insert(zone)
		}
	}

	if result.Len() == 0 {
		return nil, fmt.Errorf("no zones found in status.subnets of EKS Auto Mode NodeClass %s", nodeClassName)
	}
	return result.UnsortedList(), nil
}
