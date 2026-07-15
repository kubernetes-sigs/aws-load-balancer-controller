package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/ingress2gateway"
)

func TestValidateFlags(t *testing.T) {
	tests := []struct {
		name    string
		opts    *ingress2gateway.MigrateOptions
		wantErr string
	}{
		{
			name:    "no input mode specified",
			opts:    &ingress2gateway.MigrateOptions{},
			wantErr: "must specify at least one of",
		},
		{
			name: "from-cluster with file is invalid",
			opts: &ingress2gateway.MigrateOptions{
				FromCluster: true,
				Files:       []string{"foo.yaml"},
			},
			wantErr: "--from-cluster cannot be used with --file or --input-dir",
		},
		{
			name: "from-cluster with input-dir is invalid",
			opts: &ingress2gateway.MigrateOptions{
				FromCluster: true,
				InputDir:    "./manifests",
			},
			wantErr: "--from-cluster cannot be used with --file or --input-dir",
		},
		{
			name: "namespaces without from-cluster is invalid",
			opts: &ingress2gateway.MigrateOptions{
				Files:      []string{"foo.yaml"},
				Namespaces: []string{"ns"},
			},
			wantErr: "--namespaces can only be used with --from-cluster",
		},
		{
			name: "all-namespaces without from-cluster is invalid",
			opts: &ingress2gateway.MigrateOptions{
				Files:         []string{"foo.yaml"},
				AllNamespaces: true,
			},
			wantErr: "--all-namespaces can only be used with --from-cluster",
		},
		{
			name: "ingress-name without from-cluster is invalid",
			opts: &ingress2gateway.MigrateOptions{
				Files:       []string{"foo.yaml"},
				IngressName: "my-ing",
			},
			wantErr: "--ingress-name can only be used with --from-cluster",
		},
		{
			name: "kubeconfig without from-cluster is invalid",
			opts: &ingress2gateway.MigrateOptions{
				Files:      []string{"foo.yaml"},
				Kubeconfig: "/path/to/config",
			},
			wantErr: "--kubeconfig can only be used with --from-cluster",
		},
		{
			name: "namespaces and all-namespaces are mutually exclusive",
			opts: &ingress2gateway.MigrateOptions{
				FromCluster:   true,
				Namespaces:    []string{"ns"},
				AllNamespaces: true,
			},
			wantErr: "--namespaces and --all-namespaces are mutually exclusive",
		},
		{
			name: "ingress-name with comma is invalid",
			opts: &ingress2gateway.MigrateOptions{
				FromCluster: true,
				Namespaces:  []string{"ns"},
				IngressName: "foo,bar",
			},
			wantErr: "--ingress-name accepts a single Ingress name",
		},
		{
			name: "ingress-name with all-namespaces is invalid",
			opts: &ingress2gateway.MigrateOptions{
				FromCluster:   true,
				AllNamespaces: true,
				IngressName:   "my-ing",
			},
			wantErr: "--ingress-name and --all-namespaces are mutually exclusive",
		},
		{
			name: "ingress-name requires exactly one namespace",
			opts: &ingress2gateway.MigrateOptions{
				FromCluster: true,
				Namespaces:  []string{"ns-a", "ns-b"},
				IngressName: "my-ing",
			},
			wantErr: "--ingress-name requires exactly one --namespaces value",
		},
		{
			name: "ingress-name without namespaces is invalid",
			opts: &ingress2gateway.MigrateOptions{
				FromCluster: true,
				IngressName: "my-ing",
			},
			wantErr: "--ingress-name requires exactly one --namespaces value",
		},
		{
			name: "invalid output format",
			opts: &ingress2gateway.MigrateOptions{
				FromCluster:   true,
				OutputFormat:  "xml",
				AllNamespaces: true,
			},
			wantErr: "--output-format must be yaml or json",
		},
		{
			name: "invalid split mode",
			opts: &ingress2gateway.MigrateOptions{
				FromCluster:   true,
				AllNamespaces: true,
				OutputFormat:  "yaml",
				Split:         "invalid",
			},
			wantErr: "--split must be",
		},
		{
			name: "valid: from-cluster with all-namespaces",
			opts: &ingress2gateway.MigrateOptions{
				FromCluster:   true,
				AllNamespaces: true,
				OutputFormat:  "yaml",
			},
		},
		{
			name: "valid: from-cluster with single namespace",
			opts: &ingress2gateway.MigrateOptions{
				FromCluster:  true,
				Namespaces:   []string{"production"},
				OutputFormat: "yaml",
			},
		},
		{
			name: "valid: from-cluster with ingress-name",
			opts: &ingress2gateway.MigrateOptions{
				FromCluster:  true,
				Namespaces:   []string{"production"},
				IngressName:  "my-ing",
				OutputFormat: "yaml",
			},
		},
		{
			name: "valid: file input",
			opts: &ingress2gateway.MigrateOptions{
				Files:        []string{"foo.yaml"},
				OutputFormat: "yaml",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFlags(tt.opts)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
