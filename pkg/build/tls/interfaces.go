package tls

import (
	"context"
	extensions "k8s.io/api/extensions/v1beta1"
)

// CertificateBuilder is responsible for constructing TLSCertificates for Ingress.
type CertificateBuilder interface {
	Build(ctx context.Context, ing *extensions.Ingress) ([]string, error)
}
