package writer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestWrite(t *testing.T) {
	baseGatewayClass := gwv1.GatewayClass{
		TypeMeta:   metav1.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1", Kind: "GatewayClass"},
		ObjectMeta: metav1.ObjectMeta{Name: "aws-alb"},
	}

	tests := []struct {
		name         string
		resources    *ingress2gateway.OutputResources
		format       string
		wantErr      bool
		errContains  string
		wantFileName string
		wantContains []string
	}{
		{
			name: "yaml with gateway class and gateway",
			resources: &ingress2gateway.OutputResources{
				GatewayClass: baseGatewayClass,
				Gateways: []gwv1.Gateway{
					{
						TypeMeta:   metav1.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1", Kind: "Gateway"},
						ObjectMeta: metav1.ObjectMeta{Name: "test-gateway", Namespace: "default"},
					},
				},
			},
			format:       "yaml",
			wantFileName: "gateway-resources.yaml",
			wantContains: []string{"kind: GatewayClass", "kind: Gateway"},
		},
		{
			name: "json output",
			resources: &ingress2gateway.OutputResources{
				GatewayClass: baseGatewayClass,
			},
			format:       "json",
			wantFileName: "gateway-resources.json",
			wantContains: []string{`"kind": "GatewayClass"`},
		},
		{
			name:        "invalid format",
			resources:   &ingress2gateway.OutputResources{},
			format:      "xml",
			wantErr:     true,
			errContains: "unsupported output format",
		},
		{
			name: "creates nested output directory",
			resources: &ingress2gateway.OutputResources{
				GatewayClass: baseGatewayClass,
			},
			format:       "yaml",
			wantFileName: "gateway-resources.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			outputDir := tmpDir
			if tt.name == "creates nested output directory" {
				outputDir = filepath.Join(tmpDir, "nested", "output")
			}

			err := Write(tt.resources, outputDir, tt.format)

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
			assert.True(t, info.IsDir())

			if tt.wantFileName != "" {
				files, err := os.ReadDir(outputDir)
				require.NoError(t, err)
				assert.Len(t, files, 1)
				assert.Equal(t, tt.wantFileName, files[0].Name())

				if len(tt.wantContains) > 0 {
					content, err := os.ReadFile(filepath.Join(outputDir, files[0].Name()))
					require.NoError(t, err)
					for _, s := range tt.wantContains {
						assert.Contains(t, string(content), s)
					}
				}
			}
		})
	}
}
