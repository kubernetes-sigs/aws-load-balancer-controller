package ingress2gateway

import (
	"context"
	"fmt"
	"os"
)

// Migrate is the top-level orchestrator that reads input resources,
// checks for missing references, and writes output.
// The translate step (annotation mapping) will be added later.
func Migrate(ctx context.Context, opts MigrateOptions, readFunc ReadFunc, writeFunc WriteFunc) error {
	resources, err := readFunc(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to read input resources: %w", err)
	}

	if len(resources.Ingresses) == 0 {
		fmt.Fprintln(os.Stderr, "No Ingress resources found in input.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Found %d Ingress(es), %d Service(s), %d IngressClass(es), %d IngressClassParams\n",
		len(resources.Ingresses), len(resources.Services),
		len(resources.IngressClasses), len(resources.IngressClassParams))

	// TODO: translate InputResources → OutputResources (Gateway API manifests)
	// For now, we pass through the input resources as-is.

	if err := writeFunc(resources, opts.OutputDir, opts.OutputFormat); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}

// ReadFunc is the signature for reading input resources.
type ReadFunc func(ctx context.Context, opts MigrateOptions) (*InputResources, error)

// WriteFunc is the signature for writing output resources.
type WriteFunc func(resources *InputResources, outputDir string, format string) error
