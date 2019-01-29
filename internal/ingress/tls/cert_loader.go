package tls

import (
	"bytes"
	"context"
	"encoding/pem"

	"sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// CertLoader loads an Certificates from k8s secret.
type CertLoader interface {
	Load(ctx context.Context, secretKey types.NamespacedName) (RawCertificate, error)
}

func NewCertLoader(cache cache.Cache) CertLoader {
	return &defaultCertLoader{cache: cache}
}

var _ CertLoader = (*defaultCertLoader)(nil)

type defaultCertLoader struct {
	cache cache.Cache
}

func (c *defaultCertLoader) Load(ctx context.Context, secretKey types.NamespacedName) (RawCertificate, error) {
	secret := corev1.Secret{}
	if err := c.cache.Get(ctx, secretKey, &secret); err != nil {
		return RawCertificate{}, errors.Wrapf(err, "failed to load k8s secret: %v", secretKey)
	}
	tlsCrt := secret.Data["tls.crt"]
	tlsKey := secret.Data["tls.key"]

	certs := decodeCertificateChain(tlsCrt)
	if len(certs) == 0 {
		return RawCertificate{}, errors.Errorf("certificate chain cannot be empty!")
	}

	certChain := bytes.Join(certs[1:], []byte("\n"))

	// aws sdk will complain "minimum field size of 1, ImportCertificateInput.CertificateChain" :(
	if len(certChain) == 0 {
		certChain = nil
	}

	return RawCertificate{
		SecretKey:        secretKey,
		Certificate:      certs[0],
		CertificateChain: certChain,
		PrivateKey:       tlsKey,
	}, nil
}

func decodeCertificateChain(payload []byte) [][]byte {
	var certBlocks [][]byte

	var block *pem.Block
	for {
		block, payload = pem.Decode(payload)
		if block == nil {
			break
		}

		// skip non-certificate blocks :D
		if block.Type != "CERTIFICATE" {
			continue
		}
		certBlock := pem.EncodeToMemory(block)
		certBlocks = append(certBlocks, certBlock)
	}
	return certBlocks
}
