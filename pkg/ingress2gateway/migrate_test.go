package ingress2gateway_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/ingress2gateway"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/ingress2gateway/reader"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/ingress2gateway/translate"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/ingress2gateway/writer"
)

func TestMigrate_VanillaIngress(t *testing.T) {
	inputFile := filepath.Join("testdata", "vanilla_ingress.yaml")
	expectedFile := filepath.Join("testdata", "expected_vanilla_gateway.yaml")

	outputDir := t.TempDir()

	err := ingress2gateway.Migrate(context.Background(), ingress2gateway.MigrateOptions{
		Files:        []string{inputFile},
		OutputDir:    outputDir,
		OutputFormat: "yaml",
	}, reader.Read, translate.Translate, writer.Write)
	require.NoError(t, err)

	actual, err := os.ReadFile(filepath.Join(outputDir, "gateway-resources.yaml"))
	require.NoError(t, err)

	expected, err := os.ReadFile(expectedFile)
	require.NoError(t, err)

	assert.Equal(t, string(expected), string(actual), "generated gateway manifest does not match expected")
}
