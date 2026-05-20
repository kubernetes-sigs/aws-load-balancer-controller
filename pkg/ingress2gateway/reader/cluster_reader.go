package reader

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterReaderOptions holds options for reading from a live cluster.
type ClusterReaderOptions struct {
	Kubeconfig    string
	Namespaces    []string
	AllNamespaces bool
	IngressName   string
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

	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return readFromClient(ctx, k8sClient, clientSet, opts)
}

// readFromClient reads resources using the provided clients. Separated for testability.
func readFromClient(ctx context.Context, k8sClient client.Client, clientSet kubernetes.Interface, opts ClusterReaderOptions) (*ingress2gateway.InputResources, error) {
	resources := &ingress2gateway.InputResources{}

	namespaces := opts.Namespaces
	if opts.AllNamespaces {
		namespaces = nil
	}

	// When targeting a specific Ingress, fetch it directly instead of listing
	if opts.IngressName != "" {
		if len(namespaces) != 1 {
			return nil, fmt.Errorf("IngressName requires exactly one namespace, got %d", len(namespaces))
		}
		ns := namespaces[0]
		var ing networking.Ingress
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: opts.IngressName}, &ing); err != nil {
			return nil, fmt.Errorf("failed to get Ingress %s/%s: %w", ns, opts.IngressName, err)
		}
		resources.Ingresses = []networking.Ingress{ing}

		// Still list Services in the namespace for backend resolution
		serviceList := &corev1.ServiceList{}
		if err := k8sClient.List(ctx, serviceList, client.InNamespace(ns)); err != nil {
			return nil, fmt.Errorf("failed to list Services in namespace %q: %w", ns, err)
		}
		resources.Services = serviceList.Items
	} else if len(namespaces) == 0 {
		// All-namespaces: single list call with no namespace filter
		if err := listNamespacedResources(ctx, k8sClient, resources, ""); err != nil {
			return nil, err
		}
	} else {
		// Per-namespace list calls
		for _, ns := range namespaces {
			if err := listNamespacedResources(ctx, k8sClient, resources, ns); err != nil {
				return nil, err
			}
		}
	}

	// Read IngressClasses (cluster-scoped, no namespace filter)
	ingressClassList := &networking.IngressClassList{}
	if err := k8sClient.List(ctx, ingressClassList); err != nil {
		return nil, fmt.Errorf("failed to list IngressClasses: %w", err)
	}
	resources.IngressClasses = ingressClassList.Items

	// Read IngressClassParams only if the CRD is installed on the cluster
	if clientSet != nil {
		resList, err := clientSet.Discovery().ServerResourcesForGroupVersion(elbv2api.GroupVersion.String())
		if err == nil && k8s.IsResourceKindAvailable(resList, shared_constants.IngressClassParamsKind) {
			ingressClassParamsList := &elbv2api.IngressClassParamsList{}
			if err := k8sClient.List(ctx, ingressClassParamsList); err != nil {
				return nil, fmt.Errorf("failed to list IngressClassParams: %w", err)
			}
			resources.IngressClassParams = ingressClassParamsList.Items
		}
	}

	return resources, nil
}

// listNamespacedResources lists Ingresses and Services for a single namespace
// (or all namespaces if ns is empty) and appends them to resources.
func listNamespacedResources(ctx context.Context, k8sClient client.Client, resources *ingress2gateway.InputResources, ns string) error {
	listOpts := []client.ListOption{}
	if ns != "" {
		listOpts = append(listOpts, client.InNamespace(ns))
	}

	ingressList := &networking.IngressList{}
	if err := k8sClient.List(ctx, ingressList, listOpts...); err != nil {
		return fmt.Errorf("failed to list Ingresses in namespace %q: %w", ns, err)
	}
	resources.Ingresses = append(resources.Ingresses, ingressList.Items...)

	serviceList := &corev1.ServiceList{}
	if err := k8sClient.List(ctx, serviceList, listOpts...); err != nil {
		return fmt.Errorf("failed to list Services in namespace %q: %w", ns, err)
	}
	resources.Services = append(resources.Services, serviceList.Items...)

	return nil
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
