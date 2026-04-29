package ingress2gateway

import (
	"context"
	"fmt"
	"os"

	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
)

// Migrate is the top-level orchestrator that reads input resources,
// translates them to Gateway API manifests, and writes output.
func Migrate(ctx context.Context, opts MigrateOptions, readFunc ReadFunc, translateFunc TranslateFunc, writeFunc WriteFunc) error {
	resources, err := readFunc(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to read input resources: %w", err)
	}

	if len(resources.Ingresses) == 0 {
		fmt.Fprintln(os.Stderr, "No Ingress resources found in input.")
		return nil
	}

	// Normalize empty namespaces to "default" once, so all downstream code
	// (warnings, translate, write) can assume Namespace is always non-empty.
	resources.NormalizeNamespaces()

	fmt.Fprintf(os.Stderr, "Found %d Ingress(es), %d Service(s), %d IngressClass(es), %d IngressClassParams\n",
		len(resources.Ingresses), len(resources.Services),
		len(resources.IngressClasses), len(resources.IngressClassParams))

	output, err := translateFunc(resources)
	if err != nil {
		return fmt.Errorf("failed to translate resources: %w", err)
	}

	// When --dry-run is set, inject the dry-run annotation onto every generated Gateway
	// so LBC builds the model without creating AWS resources.
	if opts.DryRun {
		for i := range output.Gateways {
			if output.Gateways[i].Annotations == nil {
				output.Gateways[i].Annotations = map[string]string{}
			}
			output.Gateways[i].Annotations[gateway_constants.AnnotationDryRun] = gateway_constants.AnnotationDryRunEnabledValue
		}
	}

	if err := writeFunc(output, opts.OutputDir, opts.OutputFormat); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}

// ReadFunc is the signature for reading input resources.
type ReadFunc func(ctx context.Context, opts MigrateOptions) (*InputResources, error)

// TranslateFunc is the signature for translating input resources to output resources.
type TranslateFunc func(in *InputResources) (*OutputResources, error)

// WriteFunc is the signature for writing output resources.
type WriteFunc func(resources *OutputResources, outputDir string, format string) error
