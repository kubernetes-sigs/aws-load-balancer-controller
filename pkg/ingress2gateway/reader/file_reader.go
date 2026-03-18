package reader

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/yaml"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
)

var scheme = runtime.NewScheme()
var codecs serializer.CodecFactory

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = elbv2api.AddToScheme(scheme)
	codecs = serializer.NewCodecFactory(scheme)
}

// ReadFromFiles reads Kubernetes resources from the given YAML/JSON file paths.
func ReadFromFiles(files []string) (*ingress2gateway.InputResources, error) {
	resources := &ingress2gateway.InputResources{}
	for _, file := range files {
		if err := readFile(file, resources); err != nil {
			return nil, fmt.Errorf("error reading file %s: %w", file, err)
		}
	}
	return resources, nil
}

// ReadFromDir reads all .yaml/.yml/.json files from the given directory (non-recursive).
func ReadFromDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("error reading directory %s: %w", dir, err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".yaml" || ext == ".yml" || ext == ".json" {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	return files, nil
}

// readFile reads a single manifest file (YAML or JSON, potentially multi-document) and appends
// recognized resources to the InputResources struct.
func readFile(path string, resources *ingress2gateway.InputResources) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return decodeResources(data, resources)
}

// decodeResources splits multi-document input and decodes each document into
// the appropriate typed resource.
func decodeResources(data []byte, resources *ingress2gateway.InputResources) error {
	decoder := codecs.UniversalDeserializer()
	reader := yaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))

	for {
		doc, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading document: %w", err)
		}

		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 {
			continue
		}

		// Skip documents that are only comments (no actual YAML content)
		if isCommentOnly(doc) {
			continue
		}

		obj, gvk, err := decoder.Decode(doc, nil, nil)
		if err != nil {
			// Log unrecognized resources to stderr — they may be other CRDs
			// the user has in their manifests directory.
			fmt.Fprintf(os.Stderr, "Skipping unrecognized resource in input: %v\n", err)
			continue
		}

		switch gvk.Group {
		case shared_constants.IngressAPIGroup:
			switch gvk.Kind {
			case shared_constants.IngressKind:
				if ing, ok := obj.(*networking.Ingress); ok {
					resources.Ingresses = append(resources.Ingresses, *ing)
				}
			case shared_constants.IngressClassKind:
				if ic, ok := obj.(*networking.IngressClass); ok {
					resources.IngressClasses = append(resources.IngressClasses, *ic)
				}
			}
		case shared_constants.CoreAPIGroup:
			switch gvk.Kind {
			case shared_constants.ServiceKind:
				if svc, ok := obj.(*corev1.Service); ok {
					resources.Services = append(resources.Services, *svc)
				}
			}
		case elbv2api.GroupVersion.Group:
			switch gvk.Kind {
			case shared_constants.IngressClassParamsKind:
				if icp, ok := obj.(*elbv2api.IngressClassParams); ok {
					resources.IngressClassParams = append(resources.IngressClassParams, *icp)
				}
			}
		}
	}
	return nil
}

// isCommentOnly returns true if the YAML document contains only comments and whitespace.
func isCommentOnly(doc []byte) bool {
	for _, line := range bytes.Split(doc, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		if trimmed[0] != '#' {
			return false
		}
	}
	return true
}
