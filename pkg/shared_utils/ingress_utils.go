package shared_utils

import (
	networking "k8s.io/api/networking/v1"
	"strings"
)

func FindIngressTwoDNSName(ing *networking.Ingress) (albDNS string, nlbDNS string) {
	for _, ingSTS := range ing.Status.LoadBalancer.Ingress {
		if ingSTS.Hostname != "" {
			if strings.Contains(ingSTS.Hostname, "elb.amazonaws") {
				albDNS = ingSTS.Hostname
			} else {
				nlbDNS = ingSTS.Hostname
			}
		}
	}
	return albDNS, nlbDNS
}
