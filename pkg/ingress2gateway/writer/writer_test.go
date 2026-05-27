package writer

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/v3/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/ingress2gateway"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func baseGatewayClass() gwv1.GatewayClass {
	return gwv1.GatewayClass{
		TypeMeta:   metav1.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1", Kind: "GatewayClass"},
		ObjectMeta: metav1.ObjectMeta{Name: "aws-alb"},
	}
}

func gatewayIn(ns, name string) gwv1.Gateway {
	return gwv1.Gateway{
		TypeMeta:   metav1.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1", Kind: "Gateway"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
}

func httpRouteIn(ns, name string) gwv1.HTTPRoute {
	return gwv1.HTTPRoute{
		TypeMeta:   metav1.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1", Kind: "HTTPRoute"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
}

func tgcIn(ns, name string) gatewayv1beta1.TargetGroupConfiguration {
	return gatewayv1beta1.TargetGroupConfiguration{
		TypeMeta:   metav1.TypeMeta{APIVersion: "gateway.k8s.aws/v1beta1", Kind: "TargetGroupConfiguration"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
}

// fileExpectation describes an expected output file.
type fileExpectation struct {
	// path is relative to outputDir.
	path string
	// contains are substrings that MUST appear in the file content.
	contains []string
	// notContains are substrings that MUST NOT appear in the file content.
	notContains []string
}

func TestWrite(t *testing.T) {
	tests := []struct {
		name string
		// nestedOutputDir exercises auto-creation of intermediate directories.
		nestedOutputDir bool
		resources       *ingress2gateway.OutputResources
		opts            ingress2gateway.WriteOptions
		wantErr         bool
		errContains     string
		// expectedFiles lists files that must exist and their content assertions.
		expectedFiles []fileExpectation
		// topLevelEntries, when non-nil, is the exhaustive set of directory entries
		// expected directly under outputDir (file or directory names, sorted).
		topLevelEntries []string
	}{
		// ----- single-file mode (Split == "") -----
		{
			name: "yaml with gateway class and gateway",
			resources: &ingress2gateway.OutputResources{
				GatewayClass: baseGatewayClass(),
				Gateways:     []gwv1.Gateway{gatewayIn("default", "test-gateway")},
			},
			opts: ingress2gateway.WriteOptions{Format: "yaml"},
			expectedFiles: []fileExpectation{
				{path: "gateway-resources.yaml", contains: []string{"kind: GatewayClass", "kind: Gateway"}},
			},
			topLevelEntries: []string{"gateway-resources.yaml"},
		},
		{
			name:      "json output",
			resources: &ingress2gateway.OutputResources{GatewayClass: baseGatewayClass()},
			opts:      ingress2gateway.WriteOptions{Format: "json"},
			expectedFiles: []fileExpectation{
				{path: "gateway-resources.json", contains: []string{`"kind": "GatewayClass"`}},
			},
			topLevelEntries: []string{"gateway-resources.json"},
		},
		{
			name:        "invalid format",
			resources:   &ingress2gateway.OutputResources{},
			opts:        ingress2gateway.WriteOptions{Format: "xml"},
			wantErr:     true,
			errContains: "unsupported output format",
		},
		{
			name:        "invalid split mode",
			resources:   &ingress2gateway.OutputResources{GatewayClass: baseGatewayClass()},
			opts:        ingress2gateway.WriteOptions{Format: "yaml", Split: "group"},
			wantErr:     true,
			errContains: "unsupported split mode",
		},
		{
			name:            "creates nested output directory",
			nestedOutputDir: true,
			resources:       &ingress2gateway.OutputResources{GatewayClass: baseGatewayClass()},
			opts:            ingress2gateway.WriteOptions{Format: "yaml"},
			expectedFiles: []fileExpectation{
				{path: "gateway-resources.yaml"},
			},
			topLevelEntries: []string{"gateway-resources.yaml"},
		},

		// ----- split-by-namespace mode -----
		{
			name: "split namespace: resources across two namespaces",
			resources: &ingress2gateway.OutputResources{
				GatewayClass: baseGatewayClass(),
				Gateways: []gwv1.Gateway{
					gatewayIn("team-a", "gw-a"),
					gatewayIn("team-b", "gw-b"),
				},
				HTTPRoutes: []gwv1.HTTPRoute{
					httpRouteIn("team-a", "route-a"),
					httpRouteIn("team-b", "route-b"),
				},
				TargetGroupConfigurations: []gatewayv1beta1.TargetGroupConfiguration{
					tgcIn("team-a", "tgc-a"),
				},
			},
			opts: ingress2gateway.WriteOptions{
				Format: "yaml",
				Split:  ingress2gateway.SplitModeNamespace,
			},
			expectedFiles: []fileExpectation{
				{
					path:        "gatewayclass.yaml",
					contains:    []string{"kind: GatewayClass"},
					notContains: []string{"kind: Gateway\n", "kind: HTTPRoute"},
				},
				{
					path:        filepath.Join("team-a", "gateway-resources.yaml"),
					contains:    []string{"name: gw-a", "name: route-a", "name: tgc-a"},
					notContains: []string{"name: gw-b", "name: route-b", "kind: GatewayClass"},
				},
				{
					path:        filepath.Join("team-b", "gateway-resources.yaml"),
					contains:    []string{"name: gw-b", "name: route-b"},
					notContains: []string{"name: gw-a", "name: tgc-a"},
				},
			},
			topLevelEntries: []string{"gatewayclass.yaml", "team-a", "team-b"},
		},
		{
			name: "split namespace: cross-namespace group splits across directories",
			// Simulates a cross-namespace IngressGroup: one Gateway in team-a,
			// HTTPRoutes live in their member namespaces (team-a and team-b).
			resources: &ingress2gateway.OutputResources{
				GatewayClass: baseGatewayClass(),
				Gateways:     []gwv1.Gateway{gatewayIn("team-a", "shared-gw")},
				HTTPRoutes: []gwv1.HTTPRoute{
					httpRouteIn("team-a", "route-a"),
					httpRouteIn("team-b", "route-b"),
				},
			},
			opts: ingress2gateway.WriteOptions{
				Format: "yaml",
				Split:  ingress2gateway.SplitModeNamespace,
			},
			expectedFiles: []fileExpectation{
				{
					path:        filepath.Join("team-a", "gateway-resources.yaml"),
					contains:    []string{"name: shared-gw", "name: route-a"},
					notContains: []string{"name: route-b"},
				},
				{
					path:        filepath.Join("team-b", "gateway-resources.yaml"),
					contains:    []string{"name: route-b"},
					notContains: []string{"name: shared-gw", "name: route-a"},
				},
			},
			topLevelEntries: []string{"gatewayclass.yaml", "team-a", "team-b"},
		},
		{
			name:      "split namespace: only GatewayClass produces only cluster file",
			resources: &ingress2gateway.OutputResources{GatewayClass: baseGatewayClass()},
			opts: ingress2gateway.WriteOptions{
				Format: "yaml",
				Split:  ingress2gateway.SplitModeNamespace,
			},
			expectedFiles: []fileExpectation{
				{path: "gatewayclass.yaml", contains: []string{"kind: GatewayClass"}},
			},
			topLevelEntries: []string{"gatewayclass.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			outputDir := tmpDir
			if tt.nestedOutputDir {
				outputDir = filepath.Join(tmpDir, "nested", "output")
			}

			err := Write(tt.resources, outputDir, tt.opts)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)

			info, err := os.Stat(outputDir)
			require.NoError(t, err)
			assert.True(t, info.IsDir(), "outputDir should be a directory")

			for _, f := range tt.expectedFiles {
				fullPath := filepath.Join(outputDir, f.path)
				content, err := os.ReadFile(fullPath)
				require.NoErrorf(t, err, "expected file %q to exist", f.path)
				for _, s := range f.contains {
					assert.Containsf(t, string(content), s, "file %q should contain %q", f.path, s)
				}
				for _, s := range f.notContains {
					assert.NotContainsf(t, string(content), s, "file %q should NOT contain %q", f.path, s)
				}
			}

			if tt.topLevelEntries != nil {
				entries, err := os.ReadDir(outputDir)
				require.NoError(t, err)
				names := make([]string, 0, len(entries))
				for _, e := range entries {
					names = append(names, e.Name())
				}
				sort.Strings(names)
				assert.Equal(t, tt.topLevelEntries, names, "unexpected top-level entries in outputDir")
			}
		})
	}
}
