package writer

import (
	"fmt"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/printers"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
)

// Known GVKs for resources we write. When objects come from the cluster via
// controller-runtime client.List, TypeMeta is not populated, so we set it
// explicitly before serialization.
var (
	ingressGVK            = networking.SchemeGroupVersion.WithKind("Ingress")
	ingressClassGVK       = networking.SchemeGroupVersion.WithKind("IngressClass")
	serviceGVK            = corev1.SchemeGroupVersion.WithKind("Service")
	ingressClassParamsGVK = schema.GroupVersionKind{Group: elbv2api.GroupVersion.Group, Version: elbv2api.GroupVersion.Version, Kind: "IngressClassParams"}
)

// Write writes the InputResources to the output directory in the specified format.
// For now this is a pass-through that writes input resources as-is.
// TO DO: Once translation is implemented, this will write the translated Gateway API resources.
func Write(resources *ingress2gateway.InputResources, outputDir string, format string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
	}

	printer, err := newPrinter(format)
	if err != nil {
		return err
	}

	written := 0

	for i := range resources.Ingresses {
		ing := resources.Ingresses[i]
		cleanObjectMeta(&ing.ObjectMeta)
		ing.SetGroupVersionKind(ingressGVK)
		name := resourceFileName(ing.Namespace, ing.Name, "ingress", format)
		if err := writeObject(&ing, filepath.Join(outputDir, name), printer); err != nil {
			return err
		}
		written++
	}

	for i := range resources.Services {
		svc := resources.Services[i]
		cleanObjectMeta(&svc.ObjectMeta)
		svc.SetGroupVersionKind(serviceGVK)
		name := resourceFileName(svc.Namespace, svc.Name, "service", format)
		if err := writeObject(&svc, filepath.Join(outputDir, name), printer); err != nil {
			return err
		}
		written++
	}

	for i := range resources.IngressClasses {
		ic := resources.IngressClasses[i]
		cleanObjectMeta(&ic.ObjectMeta)
		ic.SetGroupVersionKind(ingressClassGVK)
		name := resourceFileName("", ic.Name, "ingressclass", format)
		if err := writeObject(&ic, filepath.Join(outputDir, name), printer); err != nil {
			return err
		}
		written++
	}

	for i := range resources.IngressClassParams {
		icp := resources.IngressClassParams[i]
		cleanObjectMeta(&icp.ObjectMeta)
		icp.SetGroupVersionKind(ingressClassParamsGVK)
		name := resourceFileName("", icp.Name, "ingressclassparams", format)
		if err := writeObject(&icp, filepath.Join(outputDir, name), printer); err != nil {
			return err
		}
		written++
	}

	fmt.Printf("Generated %d files in %s\n", written, outputDir)
	return nil
}

func newPrinter(format string) (printers.ResourcePrinter, error) {
	switch format {
	case "yaml":
		return &printers.YAMLPrinter{}, nil
	case "json":
		return &printers.JSONPrinter{}, nil
	default:
		return nil, fmt.Errorf("unsupported output format %q, must be yaml or json", format)
	}
}

func resourceFileName(namespace, name, kind, format string) string {
	ext := format
	if namespace != "" {
		return fmt.Sprintf("%s-%s-%s.%s", namespace, name, kind, ext)
	}
	return fmt.Sprintf("%s-%s.%s", name, kind, ext)
}

func writeObject(obj runtime.Object, path string, printer printers.ResourcePrinter) error {
	// Convert to unstructured so we can remove the zero-value "status" field
	// that the k8s serializer would otherwise include.
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return fmt.Errorf("failed to convert to unstructured: %w", err)
	}
	delete(data, "status")

	unstructured := &unstructured.Unstructured{Object: data}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}
	defer f.Close()

	if err := printer.PrintObj(unstructured, f); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

// cleanObjectMeta removes cluster-specific metadata fields that shouldn't
// appear in output files (managed fields, resource version, UID, etc.).
func cleanObjectMeta(meta *metav1.ObjectMeta) {
	meta.ManagedFields = nil
	meta.ResourceVersion = ""
	meta.UID = ""
	meta.Generation = 0
	meta.CreationTimestamp = metav1.Time{}
}
