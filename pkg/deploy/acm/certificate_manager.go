package acm

import (
	"context"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"

	acmModel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/acm"

	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmsdk "github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	route53sdk "github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
)

const (
	errNoValidationRecordsFound       = "no validation records found"
	validationRecordTTL               = 60
	retryIntervallDescribeCertificate = 5 * time.Second
	retryTimeoutDescribeCertificate   = 30 * time.Second
)

// abstraction around certificate operations for ACM
type CertificateManager interface {
	Create(ctx context.Context, certModel *acmModel.Certificate) (*acmModel.CertificateStatus, error)
	CreateWithValidationRecords(ctx context.Context, certModel *acmModel.Certificate) (*acmModel.CertificateStatus, error)

	Delete(ctx context.Context, arn string) error
	DeleteWithValidationRecords(ctx context.Context, arn string) error

	WaitForCertificateIssuedWithContext(ctx context.Context, arn string, waitTime time.Duration) error
}

// NewDefaultTaggingManager constructs new defaultTaggingManager.
func NewDefaultCertificateManager(acmClient services.ACM, route53Client services.Route53, defaultCaArn string, trackingProvider tracking.Provider, logger logr.Logger) *defaultCertificateManager {
	return &defaultCertificateManager{
		acmClient:        acmClient,
		route53Client:    route53Client,
		defaultCaArn:     defaultCaArn,
		logger:           logger,
		trackingProvider: trackingProvider,
	}
}

var _ CertificateManager = &defaultCertificateManager{}

// default implementation for CertificteManager
type defaultCertificateManager struct {
	acmClient        services.ACM
	route53Client    services.Route53
	defaultCaArn     string
	logger           logr.Logger
	trackingProvider tracking.Provider
}

func (c *defaultCertificateManager) Create(ctx context.Context, certModel *acmModel.Certificate) (*acmModel.CertificateStatus, error) {
	resp, err := c.create(ctx, certModel)
	if err != nil {
		return nil, err
	}

	return &acmModel.CertificateStatus{
		CertificateARN: awssdk.ToString(resp.CertificateArn),
	}, nil
}

func (c *defaultCertificateManager) CreateWithValidationRecords(ctx context.Context, certModel *acmModel.Certificate) (*acmModel.CertificateStatus, error) {
	resp, err := c.create(ctx, certModel)
	if err != nil {
		return &acmModel.CertificateStatus{}, err
	}

	var desc *acm.DescribeCertificateOutput
	if err := runtime.RetryImmediateOnError(retryIntervallDescribeCertificate, retryTimeoutDescribeCertificate, isValidationRecordsNotFoundError, func() error {
		reqDesc := &acm.DescribeCertificateInput{
			CertificateArn: resp.CertificateArn,
		}
		desc, err = c.acmClient.DescribeCertificateWithContext(ctx, reqDesc)
		if err != nil {
			return err
		}

		// look for missing record infos
		for _, opts := range desc.Certificate.DomainValidationOptions {
			if opts.ValidationMethod == acmtypes.ValidationMethodDns {
				if opts.ResourceRecord == nil {
					return fmt.Errorf(errNoValidationRecordsFound)
				}
			}
		}

		return nil
	}); err != nil {
		return &acmModel.CertificateStatus{}, errors.Wrap(err, "failed to find validation records for AMAZON_ISSUED certificate")
	}

	for _, opts := range desc.Certificate.DomainValidationOptions {
		if opts.ValidationMethod == acmtypes.ValidationMethodDns {
			c.logger.Info("creating validation record", "certificateARN", resp.CertificateArn, "record", opts.ResourceRecord)
			id, err := c.route53Client.GetHostedZoneID(ctx, awssdk.ToString(opts.DomainName))
			if err != nil {
				return nil, err
			}
			if opts.ResourceRecord == nil {
				// this should no longer happen since we retry the describe above until the records have been populated by AWS
				return nil, fmt.Errorf("resource record to create was nil but validation method was DNS")
			}
			input := &route53sdk.ChangeResourceRecordSetsInput{
				HostedZoneId: id,
				ChangeBatch: &route53types.ChangeBatch{
					Changes: []route53types.Change{
						{
							Action: "CREATE",
							ResourceRecordSet: &route53types.ResourceRecordSet{
								Name: opts.ResourceRecord.Name,
								Type: route53types.RRType(opts.ResourceRecord.Type),
								TTL:  awssdk.Int64(validationRecordTTL),
								ResourceRecords: []route53types.ResourceRecord{
									{
										Value: opts.ResourceRecord.Value,
									},
								},
							},
						},
					},
				},
			}
			_, err = c.route53Client.ChangeRecordsWithContext(ctx, input)
			if err != nil && !strings.Contains(err.Error(), "exist") { // ignore other cert having the same validation records
				return nil, err
			}
			c.logger.Info("created validation record", "certificateARN", resp.CertificateArn, "record", opts.ResourceRecord)
		}
	}

	return &acmModel.CertificateStatus{
		CertificateARN: awssdk.ToString(resp.CertificateArn),
	}, nil
}

func (c *defaultCertificateManager) create(ctx context.Context, certModel *acmModel.Certificate) (*acm.RequestCertificateOutput, error) {
	certTags := c.trackingProvider.ResourceTags(certModel.Stack(), certModel, certModel.Spec.Tags)
	sdkTags := convertTagsToSDKTags(certTags)

	req := &acmsdk.RequestCertificateInput{
		DomainName: awssdk.String(certModel.Spec.DomainName),
		Tags:       sdkTags,
	}
	if certModel.Spec.ValidationMethod != "" {
		req.ValidationMethod = certModel.Spec.ValidationMethod
	}
	if certModel.Spec.Type == acmtypes.CertificateTypePrivate {
		req.CertificateAuthorityArn = awssdk.String(certModel.Spec.CertificateAuthorityARN)
	}
	if len(certModel.Spec.SubjectAlternativeNames) > 0 {
		req.SubjectAlternativeNames = certModel.Spec.SubjectAlternativeNames
	}
	if certModel.Spec.KeyAlgorithm != "" {
		req.KeyAlgorithm = certModel.Spec.KeyAlgorithm
	}

	c.logger.Info("requesting certificate", "resourceID", certModel.ID())
	resp, err := c.acmClient.RequestCertificateWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	c.logger.Info("requested certificate", "resourceID", certModel.ID(), "certificateARN", resp.CertificateArn)

	return resp, nil
}

func (c *defaultCertificateManager) WaitForCertificateIssuedWithContext(ctx context.Context, arn string, waitTime time.Duration) error {
	c.logger.Info("waiting for certificate to be issued", "certificateARN", arn)
	err := c.acmClient.WaitForCertificateIssuedWithContext(ctx, arn, waitTime)
	if err != nil {
		return err
	}

	c.logger.Info("certificate was successfully validated", "certificateARN", arn)
	return nil
}

func (c *defaultCertificateManager) DeleteWithValidationRecords(ctx context.Context, arn string) error {
	reqDesc := &acmsdk.DescribeCertificateInput{
		CertificateArn: awssdk.String(arn),
	}

	desc, err := c.acmClient.DescribeCertificateWithContext(ctx, reqDesc)
	if err != nil {
		return err
	}

	for _, opts := range desc.Certificate.DomainValidationOptions {
		if opts.ValidationMethod == acmtypes.ValidationMethodDns {
			c.logger.Info("deleting validation records for certificate", "certificateARN", arn)
			id, err := c.route53Client.GetHostedZoneID(ctx, awssdk.ToString(opts.DomainName))
			if err != nil {
				return err
			}
			input := &route53sdk.ChangeResourceRecordSetsInput{
				HostedZoneId: id,
				ChangeBatch: &route53types.ChangeBatch{
					Changes: []route53types.Change{
						{
							Action: "DELETE",
							ResourceRecordSet: &route53types.ResourceRecordSet{
								Name: opts.ResourceRecord.Name,
								Type: route53types.RRType(opts.ResourceRecord.Type),
								TTL:  awssdk.Int64(validationRecordTTL),
								ResourceRecords: []route53types.ResourceRecord{
									{
										Value: opts.ResourceRecord.Value,
									},
								},
							},
						},
					},
				},
			}
			_, err = c.route53Client.ChangeRecordsWithContext(ctx, input)
			if err != nil && strings.Contains(err.Error(), "not found") {
				c.logger.Info("validation records no longer found, ignoring", "name", opts.ResourceRecord.Name, "value", opts.ResourceRecord.Value, "type", opts.ResourceRecord.Type)
				continue
			}
			if err != nil && strings.Contains(err.Error(), "do not match the current values") {
				c.logger.Info("validation records have been reused for another certificate, ignoring", "name", opts.ResourceRecord.Name, "value", opts.ResourceRecord.Value, "type", opts.ResourceRecord.Type)
				continue
			}
			if err != nil {
				return err
			}
		}
	}

	err = c.delete(ctx, arn)
	if err != nil {
		return err
	}

	return nil
}

func (c *defaultCertificateManager) Delete(ctx context.Context, arn string) error {
	err := c.delete(ctx, arn)
	if err != nil {
		return err
	}

	return nil
}

// internal wrapper function for Delete
func (c *defaultCertificateManager) delete(ctx context.Context, arn string) error {
	req := &acmsdk.DeleteCertificateInput{
		CertificateArn: awssdk.String(arn),
	}

	c.logger.Info("deleting certificate", "certificateARN", arn)
	_, err := c.acmClient.DeleteCertificateWithContext(ctx, req)
	if err != nil {
		return err
	}

	c.logger.Info("deleted certificate", "certificateARN", arn)

	return nil
}

func isValidationRecordsNotFoundError(err error) bool {
	if strings.Contains(err.Error(), errNoValidationRecordsFound) {
		return true
	}

	return false
}
