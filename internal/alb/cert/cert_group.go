package cert

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/golang/glog"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"

	"k8s.io/apimachinery/pkg/types"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/tls"
)

const (
	TagKeyCertificateName = "alb.kubernetes.io/certicate-name"
	TagKeyCertificateHash = "alb.kubernetes.io/certicate-hash"
)

// GroupController is responsible for maintaining the managed ACM certificates set for specified ingress.
type GroupController interface {
	// Reconcile will maintaining the managed ACM certificates for specified ingress to match specified certs.
	// e.g. new certs will be imported, out-dated certs will be updated, unused certs will be deleted.
	Reconcile(ctx context.Context, ingKey types.NamespacedName, certs []tls.RawCertificate) (CertGroup, error)
}

func NewGroupController(cloud aws.CloudAPI, tagGen TagGenerator) GroupController {
	return &defaultGroupController{
		cloud:  cloud,
		tagGen: tagGen,
	}
}

var _ GroupController = (*defaultGroupController)(nil)

type defaultGroupController struct {
	cloud  aws.CloudAPI
	tagGen TagGenerator
}

type certInstance struct {
	certArn  string
	certTags map[string]string
}

func (c *defaultGroupController) Reconcile(ctx context.Context, ingKey types.NamespacedName, certs []tls.RawCertificate) (CertGroup, error) {
	certGroupTags := c.tagGen.TagCertGroup(ingKey)
	tagFilters := make(map[string][]string, len(certGroupTags))
	for k, v := range certGroupTags {
		tagFilters[k] = []string{v}
	}

	existingCertArns, err := c.cloud.GetResourcesByFilters(tagFilters, aws.ResourceTypeEnumACMCertificate)
	if err != nil {
		return CertGroup{}, err
	}
	glog.Errorf("certs:%v", existingCertArns)

	existingCertByName := make(map[string]certInstance, len(existingCertArns))
	for _, certArn := range existingCertArns {
		certTags, err := c.cloud.ListTagsForCertificate(ctx, certArn)
		if err != nil {
			return CertGroup{}, err
		}
		certName := certTags[TagKeyCertificateName]
		existingCertByName[certName] = certInstance{
			certArn:  certArn,
			certTags: certTags,
		}
	}

	inUseCertArns := sets.NewString()
	certGroup := CertGroup{}
	for _, cert := range certs {
		certName := cert.SecretKey
		if instance, exists := existingCertByName[certName.String()]; exists {
			if err := c.reconcileCertInstance(ctx, cert, instance); err != nil {
				return CertGroup{}, err
			}
			inUseCertArns.Insert(instance.certArn)
			certGroup[certName] = instance.certArn
		} else {
			certArn, err := c.newCertInstance(ctx, cert, certGroupTags)
			if err != nil {
				return CertGroup{}, err
			}
			inUseCertArns.Insert(certArn)
			certGroup[certName] = certArn
		}
	}

	unusedCertArns := sets.NewString(existingCertArns...).Difference(inUseCertArns)
	for certArn := range unusedCertArns {
		albctx.GetLogger(ctx).Infof("deleting acm certificate: %v", certArn)
		if err := c.cloud.DeleteCertificate(ctx, certArn); err != nil {
			return CertGroup{}, err
		}
	}

	return certGroup, nil
}

func (c *defaultGroupController) newCertInstance(ctx context.Context, cert tls.RawCertificate, certGroupTags map[string]string) (string, error) {
	certTags := make(map[string]string, len(certGroupTags)+2)
	for k, v := range certGroupTags {
		certTags[k] = v
	}
	certTags[TagKeyCertificateName] = cert.SecretKey.String()
	certTags[TagKeyCertificateHash] = buildCertHash(cert)

	albctx.GetLogger(ctx).Infof("importing acm certificate: %v", cert.SecretKey.String())
	certArn, err := c.cloud.ImportCertificate(ctx, &acm.ImportCertificateInput{
		Certificate:      cert.Certificate,
		CertificateChain: cert.CertificateChain,
		PrivateKey:       cert.PrivateKey,
	})
	if err != nil {
		return "", err
	}
	// TODO, there is an gap between creating and tagging. We should figure out what to do then tagging fails. The worse case is the aws resource will be leaked =.=
	albctx.GetLogger(ctx).Infof("updating acm certificate tags: %v, %v", certArn, certTags)
	if err := c.cloud.AddTagsToCertificate(ctx, certArn, certTags); err != nil {
		return "", err
	}
	return certArn, nil
}

func (c *defaultGroupController) reconcileCertInstance(ctx context.Context, cert tls.RawCertificate, instance certInstance) error {
	desiredCertHash := buildCertHash(cert)
	actualCertHash := instance.certTags[TagKeyCertificateHash]
	if actualCertHash == desiredCertHash {
		return nil
	}

	albctx.GetLogger(ctx).Infof("updating acm certificate: %v", instance.certArn)
	if _, err := c.cloud.ImportCertificate(ctx, &acm.ImportCertificateInput{
		CertificateArn:   aws.String(instance.certArn),
		Certificate:      cert.Certificate,
		CertificateChain: cert.CertificateChain,
		PrivateKey:       cert.PrivateKey,
	}); err != nil {
		return err
	}

	certHashTag := map[string]string{TagKeyCertificateHash: desiredCertHash}
	albctx.GetLogger(ctx).Infof("updating acm certificate tags: %v, %v", instance.certArn, certHashTag)
	if err := c.cloud.AddTagsToCertificate(ctx, instance.certArn, certHashTag); err != nil {
		return err
	}
	return nil
}

func buildCertHash(cert tls.RawCertificate) string {
	hash := sha256.New()
	hash.Write(cert.Certificate)
	hash.Write(cert.CertificateChain)
	hash.Write(cert.PrivateKey)
	return hex.EncodeToString(hash.Sum(nil))
}
