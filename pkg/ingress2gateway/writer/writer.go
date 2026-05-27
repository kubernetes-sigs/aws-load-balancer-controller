package writer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/printers"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/ingress2gateway"
)

const (
	// clusterScopedFileBaseName is the file name (without extension) used for
	// cluster-scoped resources (e.g GatewayClass) when splitting by namespace.
	clusterScopedFileBaseName = "gatewayclass"

	// namespacedFileBaseName is the file name (without extension) used inside each
	// namespace directory when splitting by namespace, and for the single-file mode.
	namespacedFileBaseName = "gateway-resources"
)

// Write writes the OutputResources to the output directory in the specified format.
//
// By default, every resource is written to
// a single manifest file named gateway-resources.<ext> in outputDir, separated by "---".
//
// When opts.Split == ingress2gateway.SplitModeNamespace, resources are grouped by
// metadata.namespace. Each namespace gets <outputDir>/<ns>/gateway-resources.<ext> and
// cluster-scoped resources (GatewayClass) go to <outputDir>/gatewayclass.<ext>.
func Write(resources *ingress2gateway.OutputResources, outputDir string, opts ingress2gateway.WriteOptions) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
	}

	printer, err := newPrinter(opts.Format)
	if err != nil {
		return err
	}

	clusterObjects, namespacedByNS := partitionObjects(resources)

	switch opts.Split {
	case ingress2gateway.SplitModeNone:
		return writeSingleFile(outputDir, opts.Format, printer, clusterObjects, namespacedByNS)
	case ingress2gateway.SplitModeNamespace:
		return writeSplitByNamespace(outputDir, opts.Format, printer, clusterObjects, namespacedByNS)
	default:
		return fmt.Errorf("unsupported split mode %q, must be %q or %q",
			opts.Split, ingress2gateway.SplitModeNone, ingress2gateway.SplitModeNamespace)
	}
}

func partitionObjects(resources *ingress2gateway.OutputResources) ([]runtime.Object, map[string][]runtime.Object) {
	var clusterObjects []runtime.Object
	namespacedByNS := make(map[string][]runtime.Object)

	appendNamespaced := func(ns string, obj runtime.Object) {
		namespacedByNS[ns] = append(namespacedByNS[ns], obj)
	}

	gc := resources.GatewayClass.DeepCopy()
	cleanObjectMeta(&gc.ObjectMeta)
	clusterObjects = append(clusterObjects, gc)

	for i := range resources.LoadBalancerConfigurations {
		lbc := resources.LoadBalancerConfigurations[i].DeepCopy()
		cleanObjectMeta(&lbc.ObjectMeta)
		appendNamespaced(lbc.Namespace, lbc)
	}

	for i := range resources.Gateways {
		gw := resources.Gateways[i].DeepCopy()
		cleanObjectMeta(&gw.ObjectMeta)
		appendNamespaced(gw.Namespace, gw)
	}

	for i := range resources.HTTPRoutes {
		route := resources.HTTPRoutes[i].DeepCopy()
		cleanObjectMeta(&route.ObjectMeta)
		appendNamespaced(route.Namespace, route)
	}

	for i := range resources.TargetGroupConfigurations {
		tgc := resources.TargetGroupConfigurations[i].DeepCopy()
		cleanObjectMeta(&tgc.ObjectMeta)
		appendNamespaced(tgc.Namespace, tgc)
	}

	for i := range resources.ListenerRuleConfigurations {
		lrc := resources.ListenerRuleConfigurations[i].DeepCopy()
		cleanObjectMeta(&lrc.ObjectMeta)
		appendNamespaced(lrc.Namespace, lrc)
	}

	return clusterObjects, namespacedByNS
}

// writeSingleFile concatenates all objects (cluster + every namespace) into a single file.
func writeSingleFile(outputDir, ext string, printer printers.ResourcePrinter, clusterObjects []runtime.Object, namespacedByNS map[string][]runtime.Object) error {
	allObjects := append([]runtime.Object(nil), clusterObjects...)
	for _, ns := range sortedKeys(namespacedByNS) {
		allObjects = append(allObjects, namespacedByNS[ns]...)
	}

	outputFile := filepath.Join(outputDir, fmt.Sprintf("%s.%s", namespacedFileBaseName, ext))
	if err := writeMultiObject(allObjects, outputFile, printer); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Generated %d resource(s) in %s\n", len(allObjects), outputFile)
	return nil
}

// writeSplitByNamespace writes cluster-scoped resources to <outputDir>/gatewayclass.<ext>
// and each namespace's resources to <outputDir>/<ns>/gateway-resources.<ext>.
func writeSplitByNamespace(outputDir, ext string, printer printers.ResourcePrinter, clusterObjects []runtime.Object, namespacedByNS map[string][]runtime.Object) error {
	if len(clusterObjects) > 0 {
		clusterFile := filepath.Join(outputDir, fmt.Sprintf("%s.%s", clusterScopedFileBaseName, ext))
		if err := writeMultiObject(clusterObjects, clusterFile, printer); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Generated %d resource(s) in %s\n", len(clusterObjects), clusterFile)
	}

	for _, ns := range sortedKeys(namespacedByNS) {
		objs := namespacedByNS[ns]
		if len(objs) == 0 {
			continue
		}
		nsDir := filepath.Join(outputDir, ns)
		if err := os.MkdirAll(nsDir, 0755); err != nil {
			return fmt.Errorf("failed to create namespace directory %s: %w", nsDir, err)
		}
		nsFile := filepath.Join(nsDir, fmt.Sprintf("%s.%s", namespacedFileBaseName, ext))
		if err := writeMultiObject(objs, nsFile, printer); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Generated %d resource(s) in %s\n", len(objs), nsFile)
	}

	return nil
}

// sortedKeys returns the keys of m in sorted order, for deterministic file/log output.
func sortedKeys(m map[string][]runtime.Object) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// newPrinter returns a ResourcePrinter for the requested format.
// The format string ("yaml" or "json") is also used directly as the output file extension.
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

	for _, obj := range objects {
		// Convert to unstructured to remove zero-value "status" field
		data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return fmt.Errorf("failed to convert to unstructured: %w", err)
		}
		delete(data, "status")

		u := &unstructured.Unstructured{Object: data}

		if err := printer.PrintObj(u, f); err != nil {
			return fmt.Errorf("failed to write object: %w", err)
		}
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
