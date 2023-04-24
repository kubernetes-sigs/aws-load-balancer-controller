package ingress

import (
	networking "k8s.io/api/networking/v1"
)

// ExtractIngresses returns the list of *networking.Ingress contained in the list of classifiedIngresses
func ExtractIngresses(classifiedIngresses []ClassifiedIngress) []*networking.Ingress {
	result := make([]*networking.Ingress, len(classifiedIngresses))
	for i, v := range classifiedIngresses {
		result[i] = v.Ing
	}
	return result
}
