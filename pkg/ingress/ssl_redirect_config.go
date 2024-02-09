package ingress

// SSLRedirectConfig contains configuration for SSLRedirect feature.
type SSLRedirectConfig struct {
	// The SSLPort to redirect to for all HTTP port
	SSLPort int64
	// The HTTP response code.
	StatusCode string
}
