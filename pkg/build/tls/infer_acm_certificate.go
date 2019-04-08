package tls

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/pkg/errors"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/sets"
	"reflect"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"strings"
	"sync"
)

func NewInferACMCertificateBuilder(cloud cloud.Cloud) CertificateBuilder {
	return &inferACMCertificateBuilder{
		cloud: cloud,
	}
}

// inferACMCertificateBuilder
type inferACMCertificateBuilder struct {
	cloud cloud.Cloud

	domainNamesByCertMutex sync.Mutex
	domainNamesByCert      map[string][]string
}

// If TLS Certificate can be found for some or all hosts in Ingress, these certificate will be returned.
// If multiple TLS Certificate are found for same host, an error will be returned.
func (b *inferACMCertificateBuilder) Build(ctx context.Context, ing *extensions.Ingress) ([]string, error) {
	var tlsHosts = extractIngressTLSHosts(ing)
	if len(tlsHosts) == 0 {
		return nil, nil
	}

	if err := b.loadDomainNamesByCert(ctx); err != nil {
		return nil, err
	}

	certARNsForIng := sets.NewString()
	for host := range tlsHosts {
		certARNsForHost := sets.NewString()
		for certArn, domainNames := range b.domainNamesByCert {
			for _, domain := range domainNames {
				if isACMCertDomainMatchesTLSHost(domain, host) {
					certARNsForHost.Insert(certArn)
					break
				}
			}
		}
		ingKey := k8s.NamespacedName(ing).String()
		if len(certARNsForHost) > 1 {
			return nil, errors.Errorf("multiple certificate found for host: %s, ingress: %s", host, ingKey)
		}
		if len(certARNsForHost) == 0 {
			return nil, errors.Errorf("none certificate found for host: %s, ingress: %s", host, ingKey)
		}
		certARNsForIng = certARNsForIng.Union(certARNsForHost)
	}
	return certARNsForIng.List(), nil
}

func (b *inferACMCertificateBuilder) loadDomainNamesByCert(ctx context.Context) error {
	if b.domainNamesByCert != nil {
		return nil
	}
	b.domainNamesByCertMutex.Lock()
	defer b.domainNamesByCertMutex.Unlock()
	if b.domainNamesByCert != nil {
		return nil
	}

	certs, err := b.cloud.ACM().ListCertificatesAsList(ctx, &acm.ListCertificatesInput{
		CertificateStatuses: aws.StringSlice([]string{acm.CertificateStatusIssued}),
	})
	if err != nil {
		return err
	}

	domainNamesByCert := make(map[string][]string, len(certs))
	for _, cert := range certs {
		certArn := aws.StringValue(cert.CertificateArn)
		domainNames, err := b.lookupCertificateDomainNames(ctx, certArn)
		if err != nil {
			return err
		}
		domainNamesByCert[certArn] = domainNames
	}
	b.domainNamesByCert = domainNamesByCert
	return nil
}

// lookupCertificateDomainNames lookup the domainNames for certificate from aws ACM API.
func (b *inferACMCertificateBuilder) lookupCertificateDomainNames(ctx context.Context, certArn string) ([]string, error) {
	resp, err := b.cloud.ACM().DescribeCertificateWithContext(ctx, &acm.DescribeCertificateInput{
		CertificateArn: aws.String(certArn),
	})
	if err != nil {
		return nil, err
	}
	return aws.StringValueSlice(resp.Certificate.SubjectAlternativeNames), nil
}

// extractIngressTLSHosts extracts TLS HostNames from ingress.
func extractIngressTLSHosts(ing *extensions.Ingress) sets.String {
	hosts := sets.NewString()

	for _, r := range ing.Spec.Rules {
		if len(r.Host) != 0 {
			hosts.Insert(r.Host)
		}
	}

	for _, t := range ing.Spec.TLS {
		hosts.Insert(t.Hosts...)
	}

	return hosts
}

func isACMCertDomainMatchesTLSHost(certDomain string, tlsHost string) bool {
	if strings.HasPrefix(certDomain, "*.") {
		ds := strings.Split(certDomain, ".")
		hs := strings.Split(tlsHost, ".")

		if len(ds) != len(hs) {
			return false
		}

		return reflect.DeepEqual(ds[1:], hs[1:])
	}

	return certDomain == tlsHost
}
