package certs

import (
	"testing"

	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/stretchr/testify/assert"
)

func TestBuildCertificateRequestSpec(t *testing.T) {
	hosts := []string{"*.example.com", "example.com"}
	tags := map[string]string{"k": "v"}

	public := BuildCertificateRequestSpec(hosts, "", tags)
	assert.Equal(t, acmtypes.CertificateTypeAmazonIssued, public.Type)
	assert.Equal(t, "", public.CertificateAuthorityARN)
	assert.Equal(t, "*.example.com", public.DomainName)
	assert.Equal(t, hosts, public.SubjectAlternativeNames)
	assert.Equal(t, acmtypes.ValidationMethodDns, public.ValidationMethod)
	assert.Equal(t, tags, public.Tags)

	private := BuildCertificateRequestSpec(hosts, "arn:aws:acm-pca:us-east-1:123456789012:certificate-authority/x", nil)
	assert.Equal(t, acmtypes.CertificateTypePrivate, private.Type)
	assert.Equal(t, "arn:aws:acm-pca:us-east-1:123456789012:certificate-authority/x", private.CertificateAuthorityARN)
}

func TestBuildCertificateResourceID(t *testing.T) {
	spec := BuildCertificateRequestSpec([]string{"*.example.com"}, "", nil)
	id := BuildCertificateResourceID(spec, "ns/name")
	assert.Regexp(t, `^amazon_issued/wildcard\.example\.com-[0-9a-f]{8}$`, id)
	assert.NotContains(t, id, "*")
	assert.Equal(t, id, BuildCertificateResourceID(spec, "ns/name"), "IDs must be stable across reconciles")
	assert.NotEqual(t, id, BuildCertificateResourceID(spec, "ns/other"))
}
