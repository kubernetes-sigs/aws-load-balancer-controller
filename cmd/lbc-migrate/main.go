package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/reader"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/translate"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/warnings"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/writer"
)

const defaultOutputDir = "./gateway-output"

func main() {
	rootCmd := newRootCommand()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	opts := &ingress2gateway.MigrateOptions{}

	cmd := &cobra.Command{
		Use:   "lbc-migrate",
		Short: "Migrate AWS Load Balancer Controller Ingress resources to Gateway API",
		Long: `lbc-migrate translates Ingress, Service, IngressClass, and IngressClassParams
resources into Gateway API equivalents (Gateway, HTTPRoute, LoadBalancerConfiguration,
TargetGroupConfiguration, ListenerRuleConfiguration etc).

Input can come from YAML/JSON files, a directory of manifest files, or a live Kubernetes cluster.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFlags(opts); err != nil {
				return err
			}
			return runMigrate(cmd.Context(), opts)
		},
	}

	// Input flags
	cmd.Flags().StringSliceVarP(&opts.Files, "file", "f", nil,
		"Comma-separated input YAML/JSON file paths (e.g. -f file1.yaml,file2.yaml)")
	cmd.Flags().StringVar(&opts.InputDir, "input-dir", "",
		"Directory containing YAML/JSON files to read")
	cmd.Flags().BoolVar(&opts.FromCluster, "from-cluster", false,
		"Read resources from a live Kubernetes cluster")

	// Cluster flags
	cmd.Flags().StringVar(&opts.Namespace, "namespace", "",
		"Namespace to read from (only with --from-cluster)")
	cmd.Flags().BoolVar(&opts.AllNamespaces, "all-namespaces", false,
		"Read from all namespaces (only with --from-cluster)")
	cmd.Flags().StringVar(&opts.Kubeconfig, "kubeconfig", "",
		"Path to kubeconfig file (only with --from-cluster, defaults to $KUBECONFIG or ~/.kube/config)")

	// Output flags
	cmd.Flags().StringVar(&opts.OutputDir, "output-dir", defaultOutputDir,
		"Directory to write output manifests")
	cmd.Flags().StringVar(&opts.OutputFormat, "output-format", "yaml",
		"Output format: yaml or json")

	return cmd
}

func validateFlags(opts *ingress2gateway.MigrateOptions) error {
	hasFiles := len(opts.Files) > 0
	hasInputDir := opts.InputDir != ""
	hasFileInput := hasFiles || hasInputDir

	// Must provide at least one input mode
	if !opts.FromCluster && !hasFileInput {
		return fmt.Errorf("must specify at least one of: --file (-f), --input-dir, or --from-cluster")
	}

	// --from-cluster is mutually exclusive with file-based input
	if opts.FromCluster && hasFileInput {
		return fmt.Errorf("--from-cluster cannot be used with --file or --input-dir")
	}

	// Cluster-only flags
	if !opts.FromCluster {
		if opts.Namespace != "" {
			return fmt.Errorf("--namespace can only be used with --from-cluster")
		}
		if opts.AllNamespaces {
			return fmt.Errorf("--all-namespaces can only be used with --from-cluster")
		}
		if opts.Kubeconfig != "" {
			return fmt.Errorf("--kubeconfig can only be used with --from-cluster")
		}
	}

	// --namespace and --all-namespaces are mutually exclusive
	if opts.Namespace != "" && opts.AllNamespaces {
		return fmt.Errorf("--namespace and --all-namespaces are mutually exclusive")
	}

	// Validate output format
	if opts.OutputFormat != "yaml" && opts.OutputFormat != "json" {
		return fmt.Errorf("--output-format must be yaml or json, got %q", opts.OutputFormat)
	}

	return nil
}

func runMigrate(ctx context.Context, opts *ingress2gateway.MigrateOptions) error {
	readFunc := func(ctx context.Context, o ingress2gateway.MigrateOptions) (*ingress2gateway.InputResources, error) {
		resources, err := reader.Read(ctx, o)
		if err != nil {
			return nil, err
		}
		if !o.FromCluster {
			warnings.CheckMissingResources(resources, os.Stderr)
		}
		return resources, nil
	}

	translateFunc := translate.Translate
	writeFunc := writer.Write

	return ingress2gateway.Migrate(ctx, *opts, readFunc, translateFunc, writeFunc)
}
