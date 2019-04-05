package ingress

import extensions "k8s.io/api/extensions/v1beta1"

const (
	// annotationKubernetesIngressClass picks a specific "class" for the Ingress.
	annotationKubernetesIngressClass = "kubernetes.io/ingress.class"
	ALBIngressClass                  = "alb"
)

// MatchesIngressClass tests whether specified ingress matches desired ingressClass
func MatchesIngressClass(ingressClass string, ing *extensions.Ingress) bool {
	actualIngressClass := ing.Annotations[annotationKubernetesIngressClass]
	if ingressClass == "" {
		return actualIngressClass == "" || actualIngressClass == ALBIngressClass
	}
	return actualIngressClass == ingressClass
}
