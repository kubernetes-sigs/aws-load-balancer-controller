package translate

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	annotations "sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
)

// annotationKey returns the full annotation key for a given suffix.
func annotationKey(suffix string) string {
	return fmt.Sprintf("%s/%s", annotations.AnnotationPrefixIngress, suffix)
}

// getString returns the annotation value for the given suffix, or empty string if not present.
func getString(annotations map[string]string, suffix string) string {
	return annotations[annotationKey(suffix)]
}

// getBool parses a boolean annotation. Returns nil if not present.
func getBool(annotations map[string]string, suffix string) *bool {
	annotation := getString(annotations, suffix)
	if annotation == "" {
		return nil
	}
	b, err := strconv.ParseBool(annotation)
	if err != nil {
		return nil
	}
	return &b
}

// getInt32 parses an int32 annotation. Returns nil if not present.
func getInt32(annotations map[string]string, suffix string) *int32 {
	annotation := getString(annotations, suffix)
	if annotation == "" {
		return nil
	}
	i, err := strconv.ParseInt(annotation, 10, 32)
	if err != nil {
		return nil
	}
	val := int32(i)
	return &val
}

// getInt64 parses an int64 annotation. Returns nil if not present.
func getInt64(annotations map[string]string, suffix string) *int64 {
	v := getString(annotations, suffix)
	if v == "" {
		return nil
	}
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil
	}
	return &i
}

// getStringSlice parses a comma-separated string list annotation.
func getStringSlice(annotations map[string]string, suffix string) []string {
	annotation := getString(annotations, suffix)
	if annotation == "" {
		return nil
	}
	parts := strings.Split(annotation, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// getStringMap parses a stringMap annotation (k1=v1,k2=v2).
func getStringMap(annotations map[string]string, suffix string) map[string]string {
	annotation := getString(annotations, suffix)
	if annotation == "" {
		return nil
	}
	result := make(map[string]string)
	for _, pair := range strings.Split(annotation, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			result[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// listenPortEntry represents a single entry in the listen-ports JSON annotation.
// e.g. {"HTTP": 80} or {"HTTPS": 443}
type listenPortEntry struct {
	Protocol string
	Port     int32
}

// parseListenPorts parses the listen-ports JSON annotation.
// Format: '[{"HTTP": 80}, {"HTTPS": 443}]'
func parseListenPorts(annos map[string]string) ([]listenPortEntry, error) {
	annotation := getString(annos, annotations.IngressSuffixListenPorts)
	if annotation == "" {
		return nil, nil
	}
	var raw []map[string]int32
	if err := json.Unmarshal([]byte(annotation), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse listen-ports annotation: %w", err)
	}
	var result []listenPortEntry
	for _, entry := range raw {
		for proto, port := range entry {
			result = append(result, listenPortEntry{
				Protocol: strings.ToUpper(proto),
				Port:     port,
			})
		}
	}
	return result, nil
}
