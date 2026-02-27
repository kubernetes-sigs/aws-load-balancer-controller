package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DiscoveryClient is the interface for querying available API resources.
// Satisfied by kubernetes.Clientset (via its Discovery().ServerResourcesForGroupVersion method).
type DiscoveryClient interface {
	ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error)
}

// CRDGroupResult holds the detection result for a single API group version.
type CRDGroupResult struct {
	// GroupVersion is the API group version that was queried (e.g. "gateway.networking.k8s.io/v1").
	GroupVersion string
	// AllPresent is true when all required kinds were found.
	AllPresent bool
	// MissingKinds lists the resource kinds that were expected but not found.
	MissingKinds []string
}

// DetectCRDs queries the Kubernetes API server for the specified resource kinds
// in the given group version and reports which are present and which are missing.
// On API error, it returns AllPresent: false with all required kinds as missing (fail-closed).
func DetectCRDs(client DiscoveryClient, groupVersion string, requiredKinds []string) CRDGroupResult {
	result := CRDGroupResult{
		GroupVersion: groupVersion,
	}

	resList, err := client.ServerResourcesForGroupVersion(groupVersion)
	if err != nil {
		result.AllPresent = false
		result.MissingKinds = make([]string, len(requiredKinds))
		copy(result.MissingKinds, requiredKinds)
		return result
	}

	presentKinds := make(map[string]bool, len(resList.APIResources))
	for _, res := range resList.APIResources {
		presentKinds[res.Kind] = true
	}

	for _, kind := range requiredKinds {
		if !presentKinds[kind] {
			result.MissingKinds = append(result.MissingKinds, kind)
		}
	}

	result.AllPresent = len(result.MissingKinds) == 0
	return result
}
