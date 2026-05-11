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

// Split mode values accepted by MigrateOptions.Split and WriteOptions.Split.
const (
	// SplitModeNone emits a single manifest file containing every generated resource.
	SplitModeNone = ""
	// SplitModeNamespace emits one manifest file per namespace plus a file for
	// cluster-scoped resources. See writer package for the exact layout.
	SplitModeNamespace = "namespace"
)

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

	// Split controls how the writer lays out the output files. Valid values are
	// SplitModeNone (default, single file) and SplitModeNamespace (one file per namespace).
	Split string

	// Dry-run: when true, the generated Gateway manifests include the
	// gateway.k8s.aws/dry-run annotation so LBC builds the model without deploying.
	DryRun bool
}

// WriteOptions holds the options consumed by a WriteFunc implementation.
// Kept separate from MigrateOptions so the writer is not coupled to CLI/input concerns.
type WriteOptions struct {
	// Format is the serialization format. Supported values are "yaml" and "json".
	Format string
	// Split controls the file layout. See the Split mode constants above.
	Split string
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
