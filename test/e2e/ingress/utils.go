package ingress

import (
	"strings"

	networking "k8s.io/api/networking/v1"
)

func FindIngressDNSName(ing *networking.Ingress) string {
	for _, ingSTS := range ing.Status.LoadBalancer.Ingress {
		if ingSTS.Hostname != "" {
			return ingSTS.Hostname
		}
	}
	return ""
}

func FindIngressTwoDNSName(ing *networking.Ingress) (albDNS string, nlbDNS string) {
	for _, ingSTS := range ing.Status.LoadBalancer.Ingress {
		if ingSTS.Hostname != "" {
			if strings.Contains(ingSTS.Hostname, "elb.amazonaws.com") {
				albDNS = ingSTS.Hostname
			} else {
				nlbDNS = ingSTS.Hostname
			}
		}
	}
	return albDNS, nlbDNS
}
