package config

import "k8s.io/ingress/core/pkg/ingress/defaults"

// Configuration represents the content of nginx.conf file
type Configuration struct {
	defaults.Backend `json:",squash"`
}

// NewDefault returns the default nginx configuration
func NewDefault() Configuration {
	cfg := Configuration{}

	return cfg
}
