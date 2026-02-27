package acm

import (
	"context"

	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"

	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
)

var _ core.Resource = &Certificate{}

// Certificate represents an ACM Certificate.
type Certificate struct {
	core.ResourceMeta `json:"-"`

	//  desired state of Certificate
	Spec CertificateSpec `json:"spec"`

	// observed state of Certificate
	// +optional
	Status *CertificateStatus `json:"status,omitempty"`
}

// NewExistingCertificate is a dummy constructor of Certificate that only holds the ARN of an existing certificate
func NewExistingCertificate(arn string) *Certificate {
	cert := &Certificate{
		Status: &CertificateStatus{
			CertificateARN: arn,
		},
	}

	return cert
}

// NewCertificate constructs new Certificate resource.
func NewCertificate(stack core.Stack, id string, spec CertificateSpec) *Certificate {
	cert := &Certificate{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::ACM::Certificate", id),
		Spec:         spec,
		Status:       nil,
	}
	stack.AddResource(cert)
	return cert
}

// CertificateARN returns The Amazon Resource Name (ARN) of the certificate.
func (cert *Certificate) CertificateARN() core.StringToken {
	return core.NewResourceFieldStringToken(cert, "status/certificateARN",
		func(ctx context.Context, res core.Resource, fieldPath string) (s string, err error) {
			cert := res.(*Certificate)
			if cert.Status == nil {
				return "", errors.Errorf("Certificate is not provisioned yet: %v", cert.ID())
			}
			return cert.Status.CertificateARN, nil
		},
	)
}

// SetStatus sets the status of this object
func (cert *Certificate) SetStatus(status *CertificateStatus) {
	cert.Status = status
}

// CertificateSpec defines the desired state of Certificate
type CertificateSpec struct {
	// Whether this is a private or public certificate
	Type acmtypes.CertificateType `json:"type"`

	// ARN of the CA to request the certificate from
	CertificateAuthorityARN string `json:"certificateAuthorityARN"`

	// DomainName to issue the cert for
	DomainName string `json:"domainName"`

	// Additional FQDNs to include in the Subject Alternative Name extension of the certificate
	SubjectAlternativeNames []string `json:"subjectAlternativeNames"`

	// Can either be DNS or Mail for public certificates
	// Not needed for private certificates
	ValidationMethod acmtypes.ValidationMethod `json:"validationMethod"`

	// Key algorithm for the key pair
	// currently not used nor set in the model
	// +optional
	KeyAlgorithm acmtypes.KeyAlgorithm `json:"keyAlgorithm"`

	// Tags to associate with this certificate
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// CertificateStatus defines the observed state of Certificate
type CertificateStatus struct {
	// the ARN of the certificate
	CertificateARN string `json:"certificateARN"`
}
