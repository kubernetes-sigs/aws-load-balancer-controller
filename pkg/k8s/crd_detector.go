package k8s

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// DiscoveryClient is the interface for querying available API resources.
// Satisfied by kubernetes.Clientset (via its Discovery().ServerResourcesForGroupVersion method).
type DiscoveryClient interface {
	ServerResourcesForGroupVersion(s string) (*metav1.APIResourceList, error)
}

// CRDGroupResult holds the detection result for a single API group version.
type CRDGroupResult struct {
	// GroupVersion is the API group version that was queried (e.g. "gateway.networking.k8s.io/v1").
	GroupVersion string
	// PresentKinds a set of found CRD kinds.
	PresentKinds sets.Set[string]
}

// DetectCRDs queries the Kubernetes API server for the specified resource kinds
// in the given group version.
func DetectCRDs(client DiscoveryClient, apiVersionsOfInterest sets.Set[string]) (map[string]sets.Set[string], error) {

	result := make(map[string]sets.Set[string])
	for _, apiVersion := range apiVersionsOfInterest.UnsortedList() {
		result[apiVersion] = sets.Set[string]{}
		resList, err := client.ServerResourcesForGroupVersion(apiVersion)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		if resList == nil {
			continue
		}
		for _, res := range resList.APIResources {
			result[apiVersion].Insert(res.Kind)
		}
	}

	return result, nil
}
