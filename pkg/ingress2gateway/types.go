package ingress2gateway

import (
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
)

// InputResources holds all Kubernetes resources parsed from input (files or cluster).
type InputResources struct {
	Ingresses          []networking.Ingress
	Services           []corev1.Service
	IngressClasses     []networking.IngressClass
	IngressClassParams []elbv2api.IngressClassParams
}

// OutputResources holds all Gateway API resources produced by the translation step.
type OutputResources struct {
	GatewayClass               gwv1.GatewayClass
	Gateways                   []gwv1.Gateway
	HTTPRoutes                 []gwv1.HTTPRoute
	LoadBalancerConfigurations []gatewayv1beta1.LoadBalancerConfiguration
	TargetGroupConfigurations  []gatewayv1beta1.TargetGroupConfiguration
	ListenerRuleConfigurations []gatewayv1beta1.ListenerRuleConfiguration
}

// MigrateOptions holds the resolved configuration for a migration run.
type MigrateOptions struct {
	// Input mode
	Files       []string
	InputDir    string
	FromCluster bool

	// Cluster options
	Namespace     string
	AllNamespaces bool
	Kubeconfig    string

	// Output options
	OutputDir    string
	OutputFormat string

	// Dry-run: when true, the generated Gateway manifests include the
	// gateway.k8s.aws/dry-run annotation so LBC builds the model without deploying.
	DryRun bool
}

// NormalizeNamespaces sets empty namespace fields to "default" on all input
// resources. This mirrors the K8s API server behavior (which defaults namespace
// during admission) for offline/file-based input where no admission runs.
// After this call, downstream code can assume Namespace is always non-empty.
func (r *InputResources) NormalizeNamespaces() {
	for i := range r.Ingresses {
		if r.Ingresses[i].Namespace == "" {
			r.Ingresses[i].Namespace = "default"
		}
	}
	for i := range r.Services {
		if r.Services[i].Namespace == "" {
			r.Services[i].Namespace = "default"
		}
	}
}
