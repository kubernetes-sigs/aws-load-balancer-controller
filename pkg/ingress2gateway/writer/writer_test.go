package writer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
)

func TestWrite_YAML(t *testing.T) {
	tmpDir := t.TempDir()
	resources := &ingress2gateway.InputResources{
		Ingresses: []networking.Ingress{
			{
				TypeMeta:   metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "Ingress"},
				ObjectMeta: metav1.ObjectMeta{Name: "test-ing", Namespace: "default"},
			},
		},
	}

	err := Write(resources, tmpDir, "yaml")
	require.NoError(t, err)

	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Len(t, files, 1)
	assert.Equal(t, "default-test-ing-ingress.yaml", files[0].Name())
}

func TestWrite_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	resources := &ingress2gateway.InputResources{
		Ingresses: []networking.Ingress{
			{
				TypeMeta:   metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "Ingress"},
				ObjectMeta: metav1.ObjectMeta{Name: "test-ing", Namespace: "ns1"},
			},
		},
	}

	err := Write(resources, tmpDir, "json")
	require.NoError(t, err)

	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Len(t, files, 1)
	assert.Equal(t, "ns1-test-ing-ingress.json", files[0].Name())

	content, err := os.ReadFile(filepath.Join(tmpDir, files[0].Name()))
	require.NoError(t, err)
	assert.Contains(t, string(content), `"kind": "Ingress"`)
}

func TestWrite_InvalidFormat(t *testing.T) {
	tmpDir := t.TempDir()
	resources := &ingress2gateway.InputResources{}
	err := Write(resources, tmpDir, "xml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported output format")
}

func TestWrite_CreatesOutputDir(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "nested", "output")
	resources := &ingress2gateway.InputResources{}

	err := Write(resources, outputDir, "yaml")
	require.NoError(t, err)

	info, err := os.Stat(outputDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestResourceFileName(t *testing.T) {
	tests := []struct {
		namespace string
		name      string
		kind      string
		format    string
		expected  string
	}{
		{"default", "my-ing", "ingress", "yaml", "default-my-ing-ingress.yaml"},
		{"", "alb", "ingressclass", "yaml", "alb-ingressclass.yaml"},
		{"prod", "api", "service", "json", "prod-api-service.json"},
	}

	for _, tt := range tests {
		result := resourceFileName(tt.namespace, tt.name, tt.kind, tt.format)
		assert.Equal(t, tt.expected, result)
	}
}
