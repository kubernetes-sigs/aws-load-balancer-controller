package ingress

import (
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func FindIngressDNSName(ing *networking.Ingress) string {
	for _, ingSTS := range ing.Status.LoadBalancer.Ingress {
		if ingSTS.Hostname != "" {
			return ingSTS.Hostname
		}
	}
	return ""
}

func FindIngressHostnames(ing *networking.Ingress) []string {
	hosts := sets.NewString()
	for _, r := range ing.Spec.Rules {
		if len(r.Host) != 0 {
			hosts.Insert(r.Host)
		}
	}
	for _, t := range ing.Spec.TLS {
		hosts.Insert(t.Hosts...)
	}

	return hosts.List()
}
