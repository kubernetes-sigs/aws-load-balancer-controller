package ingress2gateway

import (
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
)

// InputResources holds all Kubernetes resources parsed from input (files or cluster).
type InputResources struct {
	Ingresses          []networking.Ingress
	Services           []corev1.Service
	IngressClasses     []networking.IngressClass
	IngressClassParams []elbv2api.IngressClassParams
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
}
