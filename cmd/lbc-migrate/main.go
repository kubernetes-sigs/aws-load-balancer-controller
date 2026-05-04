package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/console"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/reader"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/translate"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/warnings"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/writer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
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
	consoleOpts := &ConsoleOptions{Port: 8080}
	var consoleMode bool

	cmd := &cobra.Command{
		Use:   "lbc-migrate",
		Short: "Migrate AWS Load Balancer Controller Ingress resources to Gateway API",
		Long: `lbc-migrate translates Ingress, Service, IngressClass, and IngressClassParams
resources into Gateway API equivalents (Gateway, HTTPRoute, LoadBalancerConfiguration,
TargetGroupConfiguration, ListenerRuleConfiguration etc).

Input can come from YAML/JSON files, a directory of manifest files, or a live Kubernetes cluster.

Use --console to launch a local web UI that compares ingress and gateway dry-run models side by side.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if consoleMode {
				consoleOpts.Namespace = opts.Namespace
				if consoleOpts.Namespace == "" {
					return fmt.Errorf("--namespace is required with --console")
				}
				return runConsole(cmd.Context(), consoleOpts)
			}
			if err := validateFlags(opts); err != nil {
				return err
			}
			return runMigrate(cmd.Context(), opts)
		},
	}

	// Console flags
	cmd.Flags().BoolVar(&consoleMode, "console", false,
		"Launch the migration console web UI to compare ingress and gateway dry-run models")
	cmd.Flags().IntVar(&consoleOpts.Port, "port", 8080,
		"Local port for the console web server (only with --console)")

	// Input flags
	cmd.Flags().StringSliceVarP(&opts.Files, "file", "f", nil,
		"Comma-separated input YAML/JSON file paths (e.g. -f file1.yaml,file2.yaml)")
	cmd.Flags().StringVar(&opts.InputDir, "input-dir", "",
		"Directory containing YAML/JSON files to read")
	cmd.Flags().BoolVar(&opts.FromCluster, "from-cluster", false,
		"Read resources from a live Kubernetes cluster")

	// Cluster flags
	cmd.Flags().StringSliceVar(&opts.Namespaces, "namespaces", nil,
		"Comma-separated namespaces to read from (e.g. --namespaces ns-a,ns-b; only with --from-cluster)")
	cmd.Flags().BoolVar(&opts.AllNamespaces, "all-namespaces", false,
		"Read from all namespaces (only with --from-cluster)")
	cmd.Flags().StringVar(&opts.IngressName, "ingress-name", "",
		"Name of a specific Ingress to migrate (requires exactly one --namespaces value; only with --from-cluster)")
	cmd.Flags().StringVar(&opts.Kubeconfig, "kubeconfig", "",
		"Path to kubeconfig file (only with --from-cluster, defaults to $KUBECONFIG or ~/.kube/config)")

	// Output flags
	cmd.Flags().StringVar(&opts.OutputDir, "output-dir", defaultOutputDir,
		"Directory to write output manifests")
	cmd.Flags().StringVar(&opts.OutputFormat, "output-format", "yaml",
		"Output format: yaml or json")

	cmd.Flags().StringVar(&opts.Split, "split", ingress2gateway.SplitModeNone,
		"Split output layout. Empty (default) writes one combined file; 'namespace' writes one file per namespace plus a gatewayclass file for cluster-scoped resources")

	// Dry-run flag
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false,
		"Add gateway.k8s.aws/dry-run annotation to generated Gateway manifests so LBC previews the generated AWS resources without creating them")

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
		if len(opts.Namespaces) > 0 {
			return fmt.Errorf("--namespaces can only be used with --from-cluster")
		}
		if opts.AllNamespaces {
			return fmt.Errorf("--all-namespaces can only be used with --from-cluster")
		}
		if opts.IngressName != "" {
			return fmt.Errorf("--ingress-name can only be used with --from-cluster")
		}
		if opts.Kubeconfig != "" {
			return fmt.Errorf("--kubeconfig can only be used with --from-cluster")
		}
	}

	// --namespaces and --all-namespaces are mutually exclusive
	if len(opts.Namespaces) > 0 && opts.AllNamespaces {
		return fmt.Errorf("--namespaces and --all-namespaces are mutually exclusive")
	}

	// --ingress-name requires exactly one namespace and conflicts with --all-namespaces
	if opts.IngressName != "" {
		if strings.Contains(opts.IngressName, ",") {
			return fmt.Errorf("--ingress-name accepts a single Ingress name, got %q", opts.IngressName)
		}
		if opts.AllNamespaces {
			return fmt.Errorf("--ingress-name and --all-namespaces are mutually exclusive")
		}
		if len(opts.Namespaces) != 1 {
			return fmt.Errorf("--ingress-name requires exactly one --namespaces value")
		}
	}

	// Validate output format
	if opts.OutputFormat != "yaml" && opts.OutputFormat != "json" {
		return fmt.Errorf("--output-format must be yaml or json, got %q", opts.OutputFormat)
	}

	// Validate split mode
	if opts.Split != ingress2gateway.SplitModeNone && opts.Split != ingress2gateway.SplitModeNamespace {
		return fmt.Errorf("--split must be %q or %q, got %q",
			ingress2gateway.SplitModeNone, ingress2gateway.SplitModeNamespace, opts.Split)
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
			warnings.CheckInputResources(resources, os.Stderr)
		}
		return resources, nil
	}

	writeFunc := writer.Write

	return ingress2gateway.Migrate(ctx, *opts, readFunc, translate.Translate, writeFunc)
}

// ConsoleOptions holds the flags for --console mode.
type ConsoleOptions struct {
	Namespace string
	Port      int
}

func runConsole(ctx context.Context, opts *ConsoleOptions) error {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = gwv1.Install(scheme)
	_ = networking.AddToScheme(scheme)

	restConfig, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	server := console.NewConsoleServer(k8sClient, opts.Namespace)
	addr := fmt.Sprintf("localhost:%d", opts.Port)

	httpServer := &http.Server{
		Addr:    addr,
		Handler: server.Handler(),
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nShutting down...")
		httpServer.Shutdown(context.Background())
	}()

	fmt.Fprintf(os.Stderr, "Console running at http://%s\nPress Ctrl+C to stop.\n", addr)

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}
