package ec2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	ec2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/ec2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
	"time"
)

const (
	defaultWaitEIPDeletionPollInterval = 2 * time.Second
	defaultWaitEIPDeletionTimeout      = 2 * time.Minute
)

// ElasticIPAddressManager is responsible for create/update/delete Elastic IP resources.
type ElasticIPAddressManager interface {
	Create(ctx context.Context, resEIP *ec2model.ElasticIPAddress) (ec2model.ElasticIPAddressStatus, error)

	Update(ctx context.Context, resEIP *ec2model.ElasticIPAddress, sdkEIP networking.ElasticIPAddressInfo) (ec2model.ElasticIPAddressStatus, error)

	Delete(ctx context.Context, sdkEIP networking.ElasticIPAddressInfo) error
}

// NewDefaultElasticIPAddressManager constructs new defaultElasticIPAddressManager.
func NewDefaultElasticIPAddressManager(ec2Client services.EC2, trackingProvider tracking.Provider, taggingManager TaggingManager,
	vpcID string, externalManagedTags []string, logger logr.Logger) *defaultElasticIPAddressManager {
	return &defaultElasticIPAddressManager{
		ec2Client:           ec2Client,
		trackingProvider:    trackingProvider,
		taggingManager:      taggingManager,
		vpcID:               vpcID,
		externalManagedTags: externalManagedTags,
		logger:              logger,

		waitEIPDeletionPollInterval: defaultWaitEIPDeletionPollInterval,
		waitEIPDeletionTimeout:      defaultWaitEIPDeletionTimeout,
	}
}

// default implementation for ElasticIPAddressManager.
type defaultElasticIPAddressManager struct {
	ec2Client           services.EC2
	trackingProvider    tracking.Provider
	taggingManager      TaggingManager
	vpcID               string
	externalManagedTags []string
	logger              logr.Logger

	waitEIPDeletionPollInterval time.Duration
	waitEIPDeletionTimeout      time.Duration
}

func (m *defaultElasticIPAddressManager) Create(ctx context.Context, resEIP *ec2model.ElasticIPAddress) (ec2model.ElasticIPAddressStatus, error) {
	sgTags := m.trackingProvider.ResourceTags(resEIP.Stack(), resEIP, resEIP.Spec.Tags)
	sdkTags := convertTagsToSDKTags(sgTags)

	req := &ec2sdk.AllocateAddressInput{
		PublicIpv4Pool: awssdk.String(resEIP.Spec.PublicIPv4PoolID),
		TagSpecifications: []*ec2sdk.TagSpecification{
			{
				ResourceType: awssdk.String("elastic-ip"),
				Tags:         sdkTags,
			},
		},
	}
	m.logger.Info("creating elastic IP",
		"resourceID", resEIP.ID())
	resp, err := m.ec2Client.AllocateAddressWithContext(ctx, req)
	if err != nil {
		return ec2model.ElasticIPAddressStatus{}, errors.Wrap(err, "failed to create elastic IP")
	}
	eipID := awssdk.StringValue(resp.AllocationId)
	m.logger.Info("created elastic IP",
		"resourceID", resEIP.ID(),
		"allocationID", eipID)

	return ec2model.ElasticIPAddressStatus{
		AllocationID: eipID,
	}, nil
}

func (m *defaultElasticIPAddressManager) Update(ctx context.Context, resEIP *ec2model.ElasticIPAddress, sdkEIP networking.ElasticIPAddressInfo) (ec2model.ElasticIPAddressStatus, error) {
	if err := m.updateSDKElasticIPAddressWithTags(ctx, resEIP, sdkEIP); err != nil {
		return ec2model.ElasticIPAddressStatus{}, err
	}
	return ec2model.ElasticIPAddressStatus{
		AllocationID: sdkEIP.AllocationID,
	}, nil
}

func (m *defaultElasticIPAddressManager) updateSDKElasticIPAddressWithTags(ctx context.Context, resEIP *ec2model.ElasticIPAddress, sdkEIP networking.ElasticIPAddressInfo) error {
	desiredEIPTags := m.trackingProvider.ResourceTags(resEIP.Stack(), resEIP, resEIP.Spec.Tags)
	return m.taggingManager.ReconcileTags(ctx, sdkEIP.AllocationID, desiredEIPTags,
		WithCurrentTags(sdkEIP.Tags),
		WithIgnoredTagKeys(m.trackingProvider.LegacyTagKeys()),
		WithIgnoredTagKeys(m.externalManagedTags))
}

func (m *defaultElasticIPAddressManager) Delete(ctx context.Context, sdkEIP networking.ElasticIPAddressInfo) error {
	req := &ec2sdk.ReleaseAddressInput{
		AllocationId: awssdk.String(sdkEIP.AllocationID),
	}

	m.logger.Info("deleting elastic IP",
		"allocationID", sdkEIP.AllocationID)
	if err := runtime.RetryImmediateOnError(m.waitEIPDeletionPollInterval, m.waitEIPDeletionTimeout, isAddressInUseError, func() error {
		_, err := m.ec2Client.ReleaseAddressWithContext(ctx, req)
		return err
	}); err != nil {
		return errors.Wrap(err, "failed to delete elastic IP")
	}
	m.logger.Info("deleted elastic IP",
		"allocationID", sdkEIP.AllocationID)

	return nil
}

func isAddressInUseError(err error) bool {
	var awsErr awserr.Error
	if errors.As(err, &awsErr) {
		return awsErr.Code() == "InvalidIPAddress.InUse"
	}
	return false
}
