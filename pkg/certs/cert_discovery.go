package certs

import (
	"context"
	"strings"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmTypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

const (
	certARNsCacheKey = "certARNs"
	// the certARNs in AWS account will be cached for 1 minute.
	defaultCertARNsCacheTTL = 1 * time.Minute
	// the domain names for imported certificates will be cached for 5 minute.
	defaultImportedCertDomainsCacheTTL = 5 * time.Minute
	// the domain names for private certificates won't change, cache for a longer time.
	defaultPrivateCertDomainsCacheTTL = 10 * time.Hour
)

// CertDiscovery is responsible for auto-discover TLS certificates for tls hosts.
type CertDiscovery interface {
	// Discover will try to find valid certificateARNs for each tlsHost.
	Discover(ctx context.Context, tlsHosts []string, tags map[string]string) ([]string, error)
}

// NewACMCertDiscovery constructs new acmCertDiscovery
func NewACMCertDiscovery(acmClient services.ACM, allowedCAARNs []string, enableCertificateManagement bool, logger logr.Logger) *acmCertDiscovery {
	return &acmCertDiscovery{
		acmClient: acmClient,
		logger:    logger,

		loadDomainsByCertARNMutex:   sync.Mutex{},
		certARNsCache:               cache.NewExpiring(),
		certARNsCacheTTL:            defaultCertARNsCacheTTL,
		certDomainsCache:            cache.NewExpiring(),
		importedCertDomainsCacheTTL: defaultImportedCertDomainsCacheTTL,
		privateCertDomainsCacheTTL:  defaultPrivateCertDomainsCacheTTL,
		allowedCAARNs:               allowedCAARNs,
		enableCertificateManagement: enableCertificateManagement,
	}
}

var _ CertDiscovery = &acmCertDiscovery{}

// CertDiscovery implementation for ACM certificates.
type acmCertDiscovery struct {
	acmClient services.ACM
	logger    logr.Logger

	// mutex to serialize the call to loadDomainsForAllCertificates
	loadDomainsByCertARNMutex   sync.Mutex
	certARNsCache               *cache.Expiring
	certARNsCacheTTL            time.Duration
	certDomainsCache            *cache.Expiring
	allowedCAARNs               []string
	enableCertificateManagement bool
	importedCertDomainsCacheTTL time.Duration
	privateCertDomainsCacheTTL  time.Duration
}

func (d *acmCertDiscovery) Discover(ctx context.Context, tlsHosts []string, ownerTags map[string]string) ([]string, error) {
	domainsByCertARN, err := d.loadDomainsForAllCertificates(ctx, ownerTags)
	if err != nil {
		return nil, err
	}
	certARNs := sets.NewString()
	for _, host := range tlsHosts {
		var certARNsForHost []string
		for certARN, domains := range domainsByCertARN {
			for domain := range domains {
				if d.domainMatchesHost(domain, host) {
					certARNsForHost = append(certARNsForHost, certARN)
					break
				}
			}
		}

		if len(certARNsForHost) == 0 {
			return nil, errors.Errorf("no certificate found for host: %s", host)
		}
		certARNs.Insert(certARNsForHost...)
	}
	return certARNs.List(), nil
}

func (d *acmCertDiscovery) loadDomainsForAllCertificates(ctx context.Context, ownerTags map[string]string) (map[string]sets.String, error) {
	d.loadDomainsByCertARNMutex.Lock()
	defer d.loadDomainsByCertARNMutex.Unlock()

	certARNs, err := d.loadAllCertificateARNs(ctx, ownerTags)
	if err != nil {
		return nil, err
	}
	domainsByCertARN := make(map[string]sets.String, len(certARNs))
	for _, certARN := range certARNs {
		certDomains, err := d.loadDomainsForCertificate(ctx, certARN)
		if err != nil {
			return nil, err
		}
		if len(certDomains) > 0 {
			domainsByCertARN[certARN] = certDomains
		}

	}
	return domainsByCertARN, nil
}

func (d *acmCertDiscovery) loadAllCertificateARNs(ctx context.Context, ownerTags map[string]string) ([]string, error) {
	if rawCacheItem, ok := d.certARNsCache.Get(certARNsCacheKey); ok {
		return rawCacheItem.([]string), nil
	}
	req := &acm.ListCertificatesInput{
		CertificateStatuses: []acmTypes.CertificateStatus{acmTypes.CertificateStatusIssued},
		Includes: &acmTypes.Filters{
			KeyTypes: acmTypes.KeyAlgorithm.Values(""),
		},
	}
	certSummaries, err := d.acmClient.ListCertificatesAsList(ctx, req)
	if err != nil {
		return nil, err
	}

	var certARNs []string
	for _, certSummary := range certSummaries {
		certARN := awssdk.ToString(certSummary.CertificateArn)
		// if certificateManagement is enabled we filter all certificates owned by us
		if d.enableCertificateManagement {
			input := &acm.ListTagsForCertificateInput{
				CertificateArn: certSummary.CertificateArn,
			}
			resp, err := d.acmClient.ListTagsForCertificate(ctx, input)
			if err != nil {
				return nil, err
			}
			certTags := convertTagsFromSDKTags(resp.Tags)
			if !algorithm.ContainsSubMap(certTags, ownerTags) { // only add cert if there's no match
				certARNs = append(certARNs, certARN)
			}
		} else {
			certARNs = append(certARNs, certARN)
		}
	}
	d.certARNsCache.Set(certARNsCacheKey, certARNs, d.certARNsCacheTTL)
	return certARNs, nil
}

func (d *acmCertDiscovery) loadDomainsForCertificate(ctx context.Context, certARN string) (sets.String, error) {
	if rawCacheItem, ok := d.certDomainsCache.Get(certARN); ok {
		return rawCacheItem.(sets.String), nil
	}
	req := &acm.DescribeCertificateInput{
		CertificateArn: awssdk.String(certARN),
	}
	resp, err := d.acmClient.DescribeCertificateWithContext(ctx, req)
	if err != nil {
		return nil, err
	}
	certDetail := resp.Certificate

	// check if cert is issued from an allowed CA
	// otherwise empty-out the list of domains
	domains := sets.String{}
	if len(d.allowedCAARNs) == 0 || slices.Contains(d.allowedCAARNs, awssdk.ToString(certDetail.CertificateAuthorityArn)) {
		domains = sets.NewString(certDetail.SubjectAlternativeNames...)
	}
	switch certDetail.Type {
	case acmTypes.CertificateTypeImported:
		d.certDomainsCache.Set(certARN, domains, d.importedCertDomainsCacheTTL)
	case acmTypes.CertificateTypeAmazonIssued, acmTypes.CertificateTypePrivate:
		d.certDomainsCache.Set(certARN, domains, d.privateCertDomainsCacheTTL)
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

		return cmp.Equal(ds[1:], hs[1:], cmpopts.EquateEmpty())
	}

	return domainName == tlsHost
}

func convertTagsFromSDKTags(sdkTags []acmTypes.Tag) map[string]string {
	if len(sdkTags) == 0 {
		return nil
	}
	tags := make(map[string]string, len(sdkTags))

	for _, tag := range sdkTags {
		tags[awssdk.ToString(tag.Key)] = awssdk.ToString(tag.Value)
	}
	return tags
}
