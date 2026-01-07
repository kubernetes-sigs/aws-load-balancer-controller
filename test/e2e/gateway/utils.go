package gateway

import (
	"fmt"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

// Helper function to generate random OIDC credentials
func GenerateOIDCCredentials() (clientID string, clientSecret string) {
	clientID = fmt.Sprintf("test-client-%s", utils.RandomDNS1123Label(12))
	clientSecret = fmt.Sprintf("%s%s", utils.RandomDNS1123Label(16), utils.RandomDNS1123Label(16))
	return clientID, clientSecret
}

func getNamespaceLabels(podReadinessEnabled bool) map[string]string {
	namespaceLabels := map[string]string{}
	if podReadinessEnabled {
		namespaceLabels["elbv2.k8s.aws/pod-readiness-gate-inject"] = "true"
	}
	return namespaceLabels
}
