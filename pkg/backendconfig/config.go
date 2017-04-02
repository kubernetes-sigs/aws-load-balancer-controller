package backendconfig

import "k8s.io/ingress/core/pkg/ingress/defaults"

// Configuration represents the configmap data. In core its only used
// to render configuration files, doesn't help us
type Configuration struct {
	defaults.Backend `json:",squash"`
}

// NewDefault returns the default configuration
func NewDefault() Configuration {
	cfg := Configuration{}
	return cfg
}
