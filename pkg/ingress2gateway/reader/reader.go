package reader

import (
	"context"
	"fmt"

	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
)

// Read reads InputResources based on the provided MigrateOptions.
// It picks the right input mode (files vs cluster).
func Read(ctx context.Context, opts ingress2gateway.MigrateOptions) (*ingress2gateway.InputResources, error) {
	if opts.FromCluster {
		return ReadFromCluster(ctx, ClusterReaderOptions{
			Kubeconfig:    opts.Kubeconfig,
			Namespace:     opts.Namespace,
			AllNamespaces: opts.AllNamespaces,
		})
	}

	var allFiles []string

	if opts.InputDir != "" {
		dirFiles, err := ReadFromDir(opts.InputDir)
		if err != nil {
			return nil, err
		}
		allFiles = append(allFiles, dirFiles...)
	}

	allFiles = append(allFiles, opts.Files...)

	if len(allFiles) == 0 {
		return nil, fmt.Errorf("no files found in input")
	}

	return ReadFromFiles(allFiles)
}
