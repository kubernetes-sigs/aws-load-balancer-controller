package tls

import "k8s.io/apimachinery/pkg/types"

// raw certificate specified by IngressTLS
type RawCertificate struct {
	SecretKey        types.NamespacedName
	Certificate      []byte
	CertificateChain []byte
	PrivateKey       []byte
}

// tls configuration
type Config struct {
	SSLPolicy       string
	ACMCertificates []string
	RawCertificates []RawCertificate
}
