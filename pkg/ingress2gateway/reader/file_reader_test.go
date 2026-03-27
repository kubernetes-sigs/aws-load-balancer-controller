package reader

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFilesDir = "../utils/test_files"

func TestReadFromFiles_FullResources(t *testing.T) {
	files := []string{filepath.Join(testFilesDir, "full_resources.yaml")}
	resources, err := ReadFromFiles(files)
	require.NoError(t, err)

	assert.Len(t, resources.Ingresses, 1)
	assert.Equal(t, "2048-ingress", resources.Ingresses[0].Name)
	assert.Equal(t, "2048-game", resources.Ingresses[0].Namespace)

	assert.Len(t, resources.Services, 1)
	assert.Equal(t, "service-2048", resources.Services[0].Name)

	assert.Len(t, resources.IngressClasses, 1)
	assert.Equal(t, "alb", resources.IngressClasses[0].Name)

	assert.Len(t, resources.IngressClassParams, 1)
	assert.Equal(t, "alb-params", resources.IngressClassParams[0].Name)
}

func TestReadFromFiles_InvalidInput(t *testing.T) {
	files := []string{filepath.Join(testFilesDir, "invalid.yaml")}
	_, err := ReadFromFiles(files)
	assert.Error(t, err)
}

func TestReadFromFiles_NonexistentFile(t *testing.T) {
	_, err := ReadFromFiles([]string{filepath.Join(testFilesDir, "nope.yaml")})
	assert.Error(t, err)
}

func TestReadFromDir(t *testing.T) {
	files, err := ReadFromDir(testFilesDir)
	require.NoError(t, err)
	assert.Len(t, files, 4) // full_resources.yaml + ingress_missing_service.yaml + invalid.yaml + ingress_basic.json
}

func TestReadFromDir_NonexistentDir(t *testing.T) {
	_, err := ReadFromDir("/nonexistent/path")
	assert.Error(t, err)
}

func TestReadFromFiles_JSONInput(t *testing.T) {
	files := []string{filepath.Join(testFilesDir, "ingress_basic.json")}
	resources, err := ReadFromFiles(files)
	require.NoError(t, err)

	assert.Len(t, resources.Ingresses, 1)
	assert.Equal(t, "json-ingress", resources.Ingresses[0].Name)
	assert.Equal(t, "default", resources.Ingresses[0].Namespace)
	assert.Equal(t, "internet-facing", resources.Ingresses[0].Annotations["alb.ingress.kubernetes.io/scheme"])
}

func TestReadFromDir_IncludesJSON(t *testing.T) {
	files, err := ReadFromDir(testFilesDir)
	require.NoError(t, err)

	hasJSON := false
	for _, f := range files {
		if filepath.Ext(f) == ".json" {
			hasJSON = true
		}
	}
	assert.True(t, hasJSON, "ReadFromDir should include .json files")
}

func TestReadFromFiles_CommentOnlyDocumentsSkipped(t *testing.T) {
	// full_resources.yaml has a comment header and a comment-only document at the end.
	// Verify they don't produce errors or extra resources.
	files := []string{filepath.Join(testFilesDir, "full_resources.yaml")}
	resources, err := ReadFromFiles(files)
	require.NoError(t, err)

	// Should still parse exactly 4 resources — comment-only documents are silently skipped
	assert.Len(t, resources.Ingresses, 1)
	assert.Len(t, resources.Services, 1)
	assert.Len(t, resources.IngressClasses, 1)
	assert.Len(t, resources.IngressClassParams, 1)
}
