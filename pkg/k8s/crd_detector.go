package k8s

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// DiscoveryClient is the interface for querying available API resources.
// Satisfied by kubernetes.Clientset (via its Discovery().ServerResourcesForGroupVersion method).
type DiscoveryClient interface {
	ServerGroups() (*metav1.APIGroupList, error)
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
	resList, err := client.ServerGroups()
	if err != nil {
		return nil, err
	}

	result := make(map[string]sets.Set[string])
	for _, res := range resList.Groups {
		fmt.Printf("%#v\n", res)
		if !apiVersionsOfInterest.Has(res.APIVersion) {
			continue
		}
		if _, ok := result[res.APIVersion]; !ok {
			result[res.APIVersion] = sets.Set[string]{}
		}
		result[res.APIVersion].Insert(res.Kind)
	}

	return result, nil
}
