package build

import (
	"context"
	"encoding/json"
	"github.com/pkg/errors"
	extensions "k8s.io/api/extensions/v1beta1"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
)

// build the TLS Cert list for specified Ingress.
func (b *defaultBuilder) buildIngressTLSCerts(ctx context.Context, ing *extensions.Ingress) ([]string, error) {
	var rawTLSCerts []string
	_ = b.annotationParser.ParseStringSliceAnnotation(k8s.AnnotationSuffixCertificateARN, &rawTLSCerts, ing.Annotations)

	// TODO(@M00nF1sh): Cert Discovery
	return rawTLSCerts, nil
}

func (b *defaultBuilder) buildIngressTLSPolicy(ctx context.Context, ing *extensions.Ingress) string {
	var tlsPolicy string
	_ = b.annotationParser.ParseStringAnnotation(k8s.AnnotationSuffixSSLPolicy, &tlsPolicy, ing.Annotations)
	return tlsPolicy
}

// build the Listen ports for specified Ingress.
// defaultTLS indicates whether TLS is enabled by default if no `listen-ports` annotation is specified.
func (b *defaultBuilder) buildIngressListenPorts(ctx context.Context, ing *extensions.Ingress, defaultTLS bool) (map[int64]api.Protocol, error) {
	rawListenPorts := ""
	if exists := b.annotationParser.ParseStringAnnotation(k8s.AnnotationSuffixListenPorts, &rawListenPorts, ing.Annotations); !exists {
		if defaultTLS {
			rawListenPorts = "[{\"HTTPS\": 443}]"
		} else {
			rawListenPorts = "[{\"HTTP\": 80}]"
		}
	}

	var entries []map[string]int64
	if err := json.Unmarshal([]byte(rawListenPorts), &entries); err != nil {
		return nil, errors.Wrapf(err, "failed to parse `%s` into listen-ports configuration", rawListenPorts)
	}

	listenPorts := map[int64]api.Protocol{}
	for _, entry := range entries {
		for protocol, port := range entry {
			// Verify port value is valid for ALB: [1, 65535]
			if port < 1 || port > 65535 {
				return nil, errors.Errorf("invalid port: %d, listen port should be within [1, 65535]", port)
			}
			switch protocol := api.Protocol(protocol); protocol {
			case api.ProtocolHTTP:
				listenPorts[port] = protocol
			case api.ProtocolHTTPS:
				listenPorts[port] = protocol
			default:
				return nil, errors.Errorf("invalid protocol: %v, listen protocol should be HTTP or HTTPS", protocol)
			}
		}
	}
	return listenPorts, nil
}
