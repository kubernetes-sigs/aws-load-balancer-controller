package utils

import "strings"

// GetDeploymentImage prefixes the registry only for relative refs; fully-qualified refs are returned as-is.
func GetDeploymentImage(registry string, image string) string {
	if isRegistryQualified(image) {
		return image
	}
	return registry + "/" + image
}

// isRegistryQualified reports whether image's first segment is a registry host (contains "." or ":", or is "localhost").
func isRegistryQualified(image string) bool {
	i := strings.IndexByte(image, '/')
	if i < 0 {
		return false
	}
	first := image[:i]
	return first == "localhost" || strings.ContainsAny(first, ".:")
}
