package acm

import (
	"context"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/certs"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	acmModel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/acm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
)

const (
	reissueWaitTime = 5 * time.Minute

	// how long to wait for a certificate to become valid after it has been requested
	validateWaitTime = 1 * time.Minute

	deleteWaitTimeout = 30 * time.Second

	deleteWaitInterval = 5 * time.Second
)

// NewWebACLAssociationSynthesizer constructs new webACLAssociationSynthesizer.
func NewCertificateSynthesizer(certificateManager CertificateManager, certDiscovery certs.CertDiscovery, trackingProvider tracking.Provider, taggingManager TaggingManager, logger logr.Logger, stack core.Stack) *certificateSynthesizer {
	return &certificateSynthesizer{
		certificateManager: certificateManager,
		trackingProvider:   trackingProvider,
		taggingManager:     taggingManager,
		logger:             logger,
		stack:              stack,
		certDiscovery:      certDiscovery,
	}
}

type certificateSynthesizer struct {
	certificateManager CertificateManager
	trackingProvider   tracking.Provider
	taggingManager     TaggingManager
	logger             logr.Logger
	stack              core.Stack
	certDiscovery      certs.CertDiscovery
	toDeleteCerts      []CertificateWithTags
}

func (c *certificateSynthesizer) Synthesize(ctx context.Context) error {
	var resCerts []*acmModel.Certificate
	if err := c.stack.ListResources(&resCerts); err != nil {
		return fmt.Errorf("[should never happen] failed to list reosources: %w", err)
	}

	sdkCerts, err := c.findSDKCertificates(ctx)
	if err != nil {
		return err
	}

	matchedCerts, unmatchedResCerts, unmatchedSDKCerts, err := matchResAndSDKCertificates(resCerts, sdkCerts, c.trackingProvider.ResourceIDTagKey())
	if err != nil {
		return err
	}

	// For Certificates, we deleted unmatched ones during post synthesize given below facts:
	// * unmatched certificates migth still be in use by a listener rule
	c.toDeleteCerts = unmatchedSDKCerts

	// Create certs not found in the SDK
	for _, cert := range unmatchedResCerts {
		var certStatus *acmModel.CertificateStatus
		var err error
		if cert.Spec.Type == acmtypes.CertificateTypeAmazonIssued {
			certStatus, err = c.certificateManager.CreateWithValidationRecords(ctx, cert)
		} else {
			certStatus, err = c.certificateManager.Create(ctx, cert)
		}

		if err != nil {
			return err
		}

		err = c.certificateManager.WaitForCertificateWithContext(ctx, certStatus.CertificateARN, validateWaitTime)
		if err != nil {
			return err
		}

		cert.SetStatus(certStatus)
	}

	// Matched certs are 100% identical but might not be issued yet (but we know they aren't older then reissueTime) based on isSDKCertificateRequiresReplacement
	// we try to wait for them again and then set their ARN so that the model can proceed
	for _, cert := range matchedCerts {
		if cert.sdkCert.Certificate.Status != acmtypes.CertificateStatusIssued {
			c.logger.Info("waiting for existing certificate to become issued", "certificateARN", cert.sdkCert.Certificate.CertificateArn, "status", cert.sdkCert.Certificate.Status, "requested_at", cert.sdkCert.Certificate.CreatedAt)
			err := c.certificateManager.WaitForCertificateWithContext(ctx, *cert.sdkCert.Certificate.CertificateArn, validateWaitTime)
			if err != nil {
				return err
			}
		}
		cert.resCert.SetStatus(&acmModel.CertificateStatus{CertificateARN: *cert.sdkCert.Certificate.CertificateArn})
	}

	return nil
}

// delete certificates that got obsolete due to added hosts or similar
// must happen after newly created certificates are attached to listeners
func (c *certificateSynthesizer) PostSynthesize(ctx context.Context) error {
	for _, cert := range c.toDeleteCerts {
		if err := runtime.RetryImmediateOnError(deleteWaitInterval, deleteWaitTimeout, isInUseError, func() error {
			var err error
			if cert.Certificate.Type == acmtypes.CertificateTypeAmazonIssued {
				err = c.certificateManager.DeleteWithValidationRecords(ctx, awssdk.ToString(cert.Certificate.CertificateArn))
			} else {
				err = c.certificateManager.Delete(ctx, awssdk.ToString(cert.Certificate.CertificateArn))
			}
			if err != nil {
				return err
			}

			return nil
		}); err != nil {
			return errors.Wrap(err, "waited too long for the ALB to release the certificate")
		}
	}

	return nil
}

// find existing certificates that match the trags of the tracking provider
func (c *certificateSynthesizer) findSDKCertificates(ctx context.Context) ([]CertificateWithTags, error) {
	stackTags := c.trackingProvider.StackTags(c.stack)
	stackTagsLegacy := c.trackingProvider.StackTagsLegacy(c.stack)
	certs, err := c.taggingManager.ListCertificates(ctx, tracking.TagsAsTagFilter(stackTagsLegacy), tracking.TagsAsTagFilter(stackTags))
	if err != nil {
		return nil, err
	}

	return certs, nil
}

// helper to return all hosts set on a certificate (DomainName + SANs)
func getAllHostsFromCert(cert *acmModel.Certificate) []string {
	var hosts []string
	hosts = append(hosts, cert.Spec.DomainName)
	for _, host := range cert.Spec.SubjectAlternativeNames {
		hosts = append(hosts, host)
	}

	return hosts
}

type resAndSDKCertificatePair struct {
	resCert *acmModel.Certificate
	sdkCert CertificateWithTags
}

// takes a list of certs to look for and returns which ones already exist and which ones don't
// unmatchedCerts is a list of certs in the model that aren't in existing
// matchedCerts are certs that are identical in the model and sdk
func matchResAndSDKCertificates(resCerts []*acmModel.Certificate, sdkCerts []CertificateWithTags, resourceIDTagKey string) ([]resAndSDKCertificatePair, []*acmModel.Certificate, []CertificateWithTags, error) {
	var matchedCerts []resAndSDKCertificatePair
	var unmatchedResCerts []*acmModel.Certificate
	var unmatchedSDKCerts []CertificateWithTags

	resCertsByID := mapResCertificateByResourceID(resCerts)
	sdkCertsByID, err := mapSDKCertificatesByResourceID(sdkCerts, resourceIDTagKey)
	if err != nil {
		return nil, nil, nil, err
	}

	resCertIDs := sets.StringKeySet(resCertsByID)
	sdkCertIDs := sets.StringKeySet(sdkCertsByID)
	for _, resID := range resCertIDs.Intersection(sdkCertIDs).List() {
		resCert := resCertsByID[resID]
		sdkCerts := sdkCertsByID[resID]
		foundMatch := false
		for _, sdkCert := range sdkCerts {
			if isSDKCertificateRequiresReplacement(sdkCert, resCert) {
				unmatchedSDKCerts = append(unmatchedSDKCerts, sdkCert)
				continue
			}
			matchedCerts = append(matchedCerts, resAndSDKCertificatePair{
				resCert: resCert,
				sdkCert: sdkCert,
			})
			foundMatch = true
			if !foundMatch {
				unmatchedResCerts = append(unmatchedResCerts, resCert)
			}
		}
	}
	for _, resID := range resCertIDs.Difference(sdkCertIDs).List() {
		unmatchedResCerts = append(unmatchedResCerts, resCertsByID[resID])
	}
	for _, resID := range sdkCertIDs.Difference(resCertIDs).List() {
		unmatchedSDKCerts = append(unmatchedSDKCerts, sdkCertsByID[resID]...)
	}

	return matchedCerts, unmatchedResCerts, unmatchedSDKCerts, nil
}

// isSDKCertificateRequiresReplacement checks whether a sdk Certificate requires replacement to fulfill a Certificate resource.
func isSDKCertificateRequiresReplacement(sdkCert CertificateWithTags, resCert *acmModel.Certificate) bool {
	// this method is called when the certificate has the same type, domainName and CA as one we have in the model
	// for them to be interchangable in the context of an ALB it also needs to have the same list of SANs and be issued
	// certificates that aren't yet issued will be reissued
	// TODO: this definition of identical might need extension in the future?
	// e.g the KeyAlgorithm or ValidationMethod might also trigger a replacement?

	if !algorithm.IsDiffStringSlice(sdkCert.Certificate.SubjectAlternativeNameSummaries, resCert.Spec.SubjectAlternativeNames) {
		return true
	}

	// if the cert hasn't been issued within reissueTime, we better try replacing it with a fresh one
	if sdkCert.Certificate.Status != acmtypes.CertificateStatusIssued && sdkCert.Certificate.CreatedAt.Add(reissueWaitTime).Compare(time.Now()) < 0 {
		return true
	}

	return false
}

func mapResCertificateByResourceID(resCerts []*acmModel.Certificate) map[string]*acmModel.Certificate {
	resCertsByID := make(map[string]*acmModel.Certificate, len(resCerts))
	// TODO: figure out how this ID is generated
	for _, resCert := range resCerts {
		resCertsByID[resCert.ID()] = resCert
	}
	return resCertsByID
}

func mapSDKCertificatesByResourceID(sdkCerts []CertificateWithTags, resourceIDTagKey string) (map[string][]CertificateWithTags, error) {
	sdkCertsByID := make(map[string][]CertificateWithTags, len(sdkCerts))
	for _, sdkCert := range sdkCerts {
		resourceID, ok := sdkCert.Tags[resourceIDTagKey]
		if !ok {
			return nil, errors.Errorf("unexpected certificate with no resourceID: %s", awssdk.ToString(sdkCert.Certificate.CertificateArn))
		}
		sdkCertsByID[resourceID] = append(sdkCertsByID[resourceID], sdkCert)
	}

	return sdkCertsByID, nil
}

func isInUseError(err error) bool {
	var inUseErr *acmtypes.ResourceInUseException
	return errors.As(err, &inUseErr)
}
