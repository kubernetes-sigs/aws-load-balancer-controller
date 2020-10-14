package ingress

import networking "k8s.io/api/networking/v1beta1"

func FindIngressDNSName(ing *networking.Ingress) string {
	for _, ingSTS := range ing.Status.LoadBalancer.Ingress {
		if ingSTS.Hostname != "" {
			return ingSTS.Hostname
		}
	}
	return ""
}
