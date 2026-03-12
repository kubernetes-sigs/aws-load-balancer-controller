package reader

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterReaderOptions holds options for reading from a live cluster.
type ClusterReaderOptions struct {
	Kubeconfig    string
	Namespace     string
	AllNamespaces bool
}

// ReadFromCluster reads Ingress, Service, IngressClass, and IngressClassParams
// from a live Kubernetes cluster.
func ReadFromCluster(ctx context.Context, opts ClusterReaderOptions) (*ingress2gateway.InputResources, error) {
	restConfig, err := buildRestConfig(opts.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return readFromClient(ctx, k8sClient, opts)
}

// readFromClient reads resources using the provided client. Separated for testability.
func readFromClient(ctx context.Context, k8sClient client.Client, opts ClusterReaderOptions) (*ingress2gateway.InputResources, error) {
	resources := &ingress2gateway.InputResources{}

	namespace := opts.Namespace
	if opts.AllNamespaces {
		namespace = ""
	}

	listOpts := []client.ListOption{}
	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}

	// Read Ingresses
	ingressList := &networking.IngressList{}
	if err := k8sClient.List(ctx, ingressList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list Ingresses: %w", err)
	}
	resources.Ingresses = ingressList.Items

	// Read Services
	serviceList := &corev1.ServiceList{}
	if err := k8sClient.List(ctx, serviceList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list Services: %w", err)
	}
	resources.Services = serviceList.Items

	// Read IngressClasses (cluster-scoped, no namespace filter)
	ingressClassList := &networking.IngressClassList{}
	if err := k8sClient.List(ctx, ingressClassList); err != nil {
		return nil, fmt.Errorf("failed to list IngressClasses: %w", err)
	}
	resources.IngressClasses = ingressClassList.Items

	// Read IngressClassParams (cluster-scoped, no namespace filter)
	ingressClassParamsList := &elbv2api.IngressClassParamsList{}
	if err := k8sClient.List(ctx, ingressClassParamsList); err != nil {
		// IngressClassParams CRD may not be installed; warn but don't fail
		fmt.Printf("WARNING: Could not list IngressClassParams (CRD may not be installed): %v\n", err)
	} else {
		resources.IngressClassParams = ingressClassParamsList.Items
	}

	return resources, nil
}

// buildRestConfig builds a rest.Config from the given kubeconfig path.
// If kubeconfig is empty, it falls back to standard resolution
// ($KUBECONFIG env var, then ~/.kube/config).
func buildRestConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	return kubeConfig.ClientConfig()
}
