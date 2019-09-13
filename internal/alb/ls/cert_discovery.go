package ls

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/errors"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/utils"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// the domain names for imported certificate will be cached for 1 minute.(cache invalidation is hard problem right? :D)
	importedCertDomainsCacheDuration = 1 * time.Minute
)

type CertDiscovery interface {
	// Discover will try to find valid certificates for each tlsHost.
	Discover(ctx context.Context, tlsHosts sets.String) ([]string, error)
}

func NewACMCertDiscovery(cloud aws.CloudAPI) CertDiscovery {
	return &acmCertDiscovery{
		cloud:            cloud,
		certDomainsCache: utils.NewCache(),
	}
}

type acmCertDiscovery struct {
	cloud            aws.CloudAPI
	certDomainsCache utils.Cache
}

func (d *acmCertDiscovery) Discover(ctx context.Context, tlsHosts sets.String) ([]string, error) {
	domainsByCertArn, err := d.loadDomainsForCertificates(ctx)
	if err != nil {
		return nil, err
	}
	certArns := sets.NewString()
	for host := range tlsHosts {
		certArnsForHost := sets.NewString()
		for certArn, domains := range domainsByCertArn {
			for domain := range domains {
				if d.domainMatchesHost(domain, host) {
					certArnsForHost.Insert(certArn)
					break
				}
			}
		}
		if len(certArnsForHost) > 1 {
			return nil, errors.Errorf("multiple certificate found for host: %s, certARNs: %v", host, certArnsForHost.List())
		}
		if len(certArnsForHost) == 0 {
			return nil, errors.Errorf("none certificate found for host: %s", host)
		}
		certArns = certArns.Union(certArnsForHost)
	}
	return certArns.List(), nil
}

func (d *acmCertDiscovery) loadDomainsForCertificates(ctx context.Context) (map[string]sets.String, error) {
	certSummaries, err := d.cloud.ListCertificates(ctx, &acm.ListCertificatesInput{
		CertificateStatuses: aws.StringSlice([]string{acm.CertificateStatusIssued}),
	})
	if err != nil {
		return nil, err
	}
	domainsByCertArn := make(map[string]sets.String, len(certSummaries))
	for _, certSummary := range certSummaries {
		certArn := aws.StringValue(certSummary.CertificateArn)
		certDomains, err := d.loadDomainsForCertificate(ctx, certArn)
		if err != nil {
			return nil, err
		}
		domainsByCertArn[certArn] = certDomains
	}
	d.certDomainsCache.Shrink(sets.StringKeySet(domainsByCertArn))
	return domainsByCertArn, nil
}

func (d *acmCertDiscovery) loadDomainsForCertificate(ctx context.Context, certArn string) (sets.String, error) {
	if domains, ok := d.certDomainsCache.Get(certArn); ok {
		return domains.(sets.String), nil
	}
	certDetail, err := d.cloud.DescribeCertificate(ctx, certArn)
	if err != nil {
		return nil, err
	}
	domains := sets.NewString(aws.StringValueSlice(certDetail.SubjectAlternativeNames)...)
	switch aws.StringValue(certDetail.Type) {
	case acm.CertificateTypeAmazonIssued, acm.CertificateTypePrivate:
		d.certDomainsCache.Set(certArn, domains, utils.CacheNoExpiration)
	case acm.CertificateTypeImported:
		d.certDomainsCache.Set(certArn, domains, importedCertDomainsCacheDuration)
	}
	return domains, nil
}

func (d *acmCertDiscovery) domainMatchesHost(domainName string, tlsHost string) bool {
	if strings.HasPrefix(domainName, "*.") {
		ds := strings.Split(domainName, ".")
		hs := strings.Split(tlsHost, ".")

		if len(ds) != len(hs) {
			return false
		}

		return reflect.DeepEqual(ds[1:], hs[1:])
	}

	return domainName == tlsHost
}
