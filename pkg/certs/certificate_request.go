package certs

import (
	"crypto/sha256"
	"fmt"
	"strings"

	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	acmModel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/acm"
)

// BuildCertificateRequestSpec builds the spec for a controller-managed ACM certificate covering hosts.
// hosts must be sorted and non-empty; the first host becomes the DomainName and all hosts become SANs.
// A non-empty caARN requests a private certificate from that AWS Private CA, otherwise an Amazon-issued
// certificate validated through DNS is requested.
func BuildCertificateRequestSpec(hosts []string, caARN string, tags map[string]string) acmModel.CertificateSpec {
	certType := acmtypes.CertificateTypePrivate
	if caARN == "" {
		certType = acmtypes.CertificateTypeAmazonIssued
	}
	return acmModel.CertificateSpec{
		Type:                    certType,
		CertificateAuthorityARN: caARN,
		DomainName:              hosts[0],
		SubjectAlternativeNames: hosts,
		ValidationMethod:        acmtypes.ValidationMethodDns, // currently we only support DNS based validation for AMAZON_ISSUED certificates
		Tags:                    tags,
	}
}

// BuildCertificateResourceID builds a unique resource ID for a controller-managed certificate.
// ownerKey identifies the Kubernetes object the certificate belongs to (e.g. an Ingress or a Gateway listener),
// and is hashed so that every owner gets its own certificate.
func BuildCertificateResourceID(spec acmModel.CertificateSpec, ownerKey string) string {
	ownerHash := fmt.Sprintf("%x", sha256.Sum256([]byte(ownerKey)))[:8]
	// The resourceID becomes a tag value, which rejects '*' (ACM pattern [\p{L}\p{Z}\p{N}_.:/=+\-@]*).
	domainName := strings.ReplaceAll(spec.DomainName, "*", "wildcard")
	return fmt.Sprintf("%s/%s-%s", strings.ToLower(string(spec.Type)), domainName, ownerHash)
}
