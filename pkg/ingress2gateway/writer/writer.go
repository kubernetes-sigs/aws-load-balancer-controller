package writer

import (
	"fmt"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/printers"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
)

// Write writes the OutputResources to the output directory in the specified format.
// Each Ingress produces a single manifest file containing all related Gateway API resources
// (GatewayClass, Gateway, LoadBalancerConfiguration, HTTPRoute, TargetGroupConfigurations)
// separated by "---".
func Write(resources *ingress2gateway.OutputResources, outputDir string, format string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
	}

	printer, err := newPrinter(format)
	if err != nil {
		return err
	}

	// Collect all resources into a single file per output directory
	var allObjects []runtime.Object

	// GatewayClass (one per run)
	gc := resources.GatewayClass.DeepCopy()
	cleanObjectMeta(&gc.ObjectMeta)
	allObjects = append(allObjects, gc)

	// Gateways + LoadBalancerConfigurations + HTTPRoutes + TargetGroupConfigurations
	for i := range resources.LoadBalancerConfigurations {
		lbc := resources.LoadBalancerConfigurations[i].DeepCopy()
		cleanObjectMeta(&lbc.ObjectMeta)
		allObjects = append(allObjects, lbc)
	}

	for i := range resources.Gateways {
		gw := resources.Gateways[i].DeepCopy()
		cleanObjectMeta(&gw.ObjectMeta)
		allObjects = append(allObjects, gw)
	}

	for i := range resources.HTTPRoutes {
		route := resources.HTTPRoutes[i].DeepCopy()
		cleanObjectMeta(&route.ObjectMeta)
		allObjects = append(allObjects, route)
	}

	for i := range resources.TargetGroupConfigurations {
		tgc := resources.TargetGroupConfigurations[i].DeepCopy()
		cleanObjectMeta(&tgc.ObjectMeta)
		allObjects = append(allObjects, tgc)
	}

	for i := range resources.ListenerRuleConfigurations {
		lrc := resources.ListenerRuleConfigurations[i].DeepCopy()
		cleanObjectMeta(&lrc.ObjectMeta)
		allObjects = append(allObjects, lrc)
	}

	ext := format
	outputFile := filepath.Join(outputDir, fmt.Sprintf("gateway-resources.%s", ext))
	if err := writeMultiObject(allObjects, outputFile, printer); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Generated %d resource(s) in %s\n", len(allObjects), outputFile)
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

func writeMultiObject(objects []runtime.Object, path string, printer printers.ResourcePrinter) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}
	defer f.Close()

	for i, obj := range objects {
		// Convert to unstructured to remove zero-value "status" field
		data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return fmt.Errorf("failed to convert to unstructured: %w", err)
		}
		delete(data, "status")

		u := &unstructured.Unstructured{Object: data}

		if i > 0 && isYAMLPrinter(printer) {
			// YAMLPrinter already adds "---\n" separator between documents
		}

		if err := printer.PrintObj(u, f); err != nil {
			return fmt.Errorf("failed to write object: %w", err)
		}
	}
	return nil
}

func isYAMLPrinter(p printers.ResourcePrinter) bool {
	_, ok := p.(*printers.YAMLPrinter)
	return ok
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
