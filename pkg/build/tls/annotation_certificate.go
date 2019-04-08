package tls

import (
	"context"
	extensions "k8s.io/api/extensions/v1beta1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
)

func NewAnnotationCertificateBuilder(annotationParser k8s.AnnotationParser) CertificateBuilder {
	return &annotationCertificateBuilder{
		annotationParser: annotationParser,
	}
}

type annotationCertificateBuilder struct {
	annotationParser k8s.AnnotationParser
}

func (b *annotationCertificateBuilder) Build(ctx context.Context, ing *extensions.Ingress) ([]string, error) {
	var rawTLSCerts []string
	_ = b.annotationParser.ParseStringSliceAnnotation(k8s.AnnotationSuffixCertificateARN, &rawTLSCerts, ing.Annotations)

	return rawTLSCerts, nil
}
