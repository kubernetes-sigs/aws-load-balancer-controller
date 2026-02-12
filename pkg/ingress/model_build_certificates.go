package ingress

import (
	"context"
	"fmt"
	"strings"

	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	acmModel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/acm"
)

func (t *defaultModelBuildTask) buildACMCertificates(ctx context.Context, ing *ClassifiedIngress) (*acmModel.Certificate, error) {
	// explicitly set certificates take precedence over creating new ones
	var explicitCertARNs []string
	_ = t.annotationParser.ParseStringSliceAnnotation(annotations.IngressSuffixCertificateARN, &explicitCertARNs, ing.Ing.Annotations)
	if len(explicitCertARNs) > 0 {
		return nil, nil
	}

	certSpec, err := t.buildCertificateSpec(ctx, ing)
	if err != nil {
		return nil, err
	}

	if certSpec == nil {
		return nil, nil
	}

	certID := t.buildCertificateResourceID(certSpec)

	cert := acmModel.NewCertificate(t.stack, certID, *certSpec)
	return cert, nil
}

func (t *defaultModelBuildTask) buildCertificateResourceID(spec *acmModel.CertificateSpec) string {
	if spec.CertificateAuthorityARN == "" {
		return fmt.Sprintf("%s/%s", strings.ToLower(string(spec.Type)), spec.DomainName)
	} else {
		return fmt.Sprintf("%s/%s-%s", strings.ToLower(string(spec.Type)), spec.DomainName, spec.CertificateAuthorityARN)
	}
}

func (t *defaultModelBuildTask) buildCertificateSpec(ctx context.Context, ing *ClassifiedIngress) (*acmModel.CertificateSpec, error) {
	caArn := t.buildCertificateCAArn(ctx, ing)

	hosts := t.buildCertificateHosts(ctx, ing)

	if len(hosts) == 0 {
		// no hosts is valid for an ingress object (catch-all ing), but we can't request any certificate
		return nil, nil
	}

	tags, err := t.buildCertificateTags(ctx, ing)
	if err != nil {
		return nil, err
	}

	// if we have no reference to a CA it will be a public certificate
	certType := acmtypes.CertificateTypePrivate
	if caArn == "" {
		certType = acmtypes.CertificateTypeAmazonIssued
	}

	return &acmModel.CertificateSpec{
		Type:                    certType,
		CertificateAuthorityARN: caArn,
		DomainName:              hosts[0],
		SubjectAlternativeNames: hosts,
		ValidationMethod:        acmtypes.ValidationMethodDns, // currently we only support DNS based validation for AMAZON_ISSUED certificates
		Tags:                    tags,
	}, nil
}

func (t *defaultModelBuildTask) buildCertificateHosts(_ context.Context, ing *ClassifiedIngress) []string {
	// grab all hosts from the ingress
	hosts := sets.NewString()
	for _, r := range ing.Ing.Spec.Rules {
		if len(r.Host) != 0 {
			hosts.Insert(r.Host)
		}
	}
	for _, t := range ing.Ing.Spec.TLS {
		hosts.Insert(t.Hosts...)
	}

	return hosts.List()
}

func (t *defaultModelBuildTask) buildCertificateCAArn(_ context.Context, ing *ClassifiedIngress) string {
	var certArn string
	_ = t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixACMCaARN, &certArn, ing.Ing.Annotations)

	// PCA ARN on the cert takes precedence
	if certArn != "" {
		return certArn
	}

	// otherwise it's the default ARN set on the controller
	if t.defaultCAArn != "" {
		return t.defaultCAArn
	}

	// or no ARN, implying amazon issued certificates
	return ""
}

func (t *defaultModelBuildTask) buildCertificateTags(_ context.Context, ing *ClassifiedIngress) (map[string]string, error) {
	ingTags, err := t.buildIngressResourceTags(*ing)
	if err != nil {
		return nil, err
	}
	return algorithm.MergeStringMap(t.defaultTags, ingTags), nil
}
