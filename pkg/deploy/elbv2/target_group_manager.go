package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
	"time"
)

const (
	defaultWaitTGDeletionPollInterval = 2 * time.Second
	defaultWaitTGDeletionTimeout      = 20 * time.Second
)

// TargetGroupManager is responsible for create/update/delete TargetGroup resources.
type TargetGroupManager interface {
	Create(ctx context.Context, resTG *elbv2model.TargetGroup) (elbv2model.TargetGroupStatus, error)

	Update(ctx context.Context, resTG *elbv2model.TargetGroup, sdkTG TargetGroupWithTags) (elbv2model.TargetGroupStatus, error)

	Delete(ctx context.Context, sdkTG TargetGroupWithTags) error
}

// NewDefaultTargetGroupManager constructs new defaultTargetGroupManager.
func NewDefaultTargetGroupManager(elbv2Client services.ELBV2, trackingProvider tracking.Provider,
	taggingManager TaggingManager, vpcID string, externalManagedTags []string, logger logr.Logger) *defaultTargetGroupManager {
	return &defaultTargetGroupManager{
		elbv2Client:          elbv2Client,
		trackingProvider:     trackingProvider,
		taggingManager:       taggingManager,
		attributesReconciler: NewDefaultTargetGroupAttributesReconciler(elbv2Client, logger),
		vpcID:                vpcID,
		externalManagedTags:  externalManagedTags,
		logger:               logger,

		waitTGDeletionPollInterval: defaultWaitTGDeletionPollInterval,
		waitTGDeletionTimeout:      defaultWaitTGDeletionTimeout,
	}
}

var _ TargetGroupManager = &defaultTargetGroupManager{}

// default implementation for TargetGroupManager
type defaultTargetGroupManager struct {
	elbv2Client          services.ELBV2
	trackingProvider     tracking.Provider
	taggingManager       TaggingManager
	attributesReconciler TargetGroupAttributesReconciler
	vpcID                string
	externalManagedTags  []string

	logger logr.Logger

	waitTGDeletionPollInterval time.Duration
	waitTGDeletionTimeout      time.Duration
}

func (m *defaultTargetGroupManager) Create(ctx context.Context, resTG *elbv2model.TargetGroup) (elbv2model.TargetGroupStatus, error) {
	req := buildSDKCreateTargetGroupInput(resTG.Spec)
	req.VpcId = awssdk.String(m.vpcID)
	tgTags := m.trackingProvider.ResourceTags(resTG.Stack(), resTG, resTG.Spec.Tags)
	req.Tags = convertTagsToSDKTags(tgTags)

	m.logger.Info("creating targetGroup",
		"stackID", resTG.Stack().StackID(),
		"resourceID", resTG.ID())
	resp, err := m.elbv2Client.CreateTargetGroupWithContext(ctx, req)
	if err != nil {
		return elbv2model.TargetGroupStatus{}, err
	}
	sdkTG := TargetGroupWithTags{
		TargetGroup: resp.TargetGroups[0],
		Tags:        tgTags,
	}
	m.logger.Info("created targetGroup",
		"stackID", resTG.Stack().StackID(),
		"resourceID", resTG.ID(),
		"arn", awssdk.StringValue(sdkTG.TargetGroup.TargetGroupArn))
	if err := m.attributesReconciler.Reconcile(ctx, resTG, sdkTG); err != nil {
		return elbv2model.TargetGroupStatus{}, err
	}

	return buildResTargetGroupStatus(sdkTG), nil
}

func (m *defaultTargetGroupManager) Update(ctx context.Context, resTG *elbv2model.TargetGroup, sdkTG TargetGroupWithTags) (elbv2model.TargetGroupStatus, error) {
	if err := m.updateSDKTargetGroupWithTags(ctx, resTG, sdkTG); err != nil {
		return elbv2model.TargetGroupStatus{}, err
	}
	if err := m.updateSDKTargetGroupWithHealthCheck(ctx, resTG, sdkTG); err != nil {
		return elbv2model.TargetGroupStatus{}, err
	}
	if err := m.attributesReconciler.Reconcile(ctx, resTG, sdkTG); err != nil {
		return elbv2model.TargetGroupStatus{}, err
	}

	return buildResTargetGroupStatus(sdkTG), nil
}

func (m *defaultTargetGroupManager) Delete(ctx context.Context, sdkTG TargetGroupWithTags) error {
	req := &elbv2sdk.DeleteTargetGroupInput{
		TargetGroupArn: sdkTG.TargetGroup.TargetGroupArn,
	}

	m.logger.Info("deleting targetGroup",
		"arn", awssdk.StringValue(req.TargetGroupArn))
	if err := runtime.RetryImmediateOnError(m.waitTGDeletionPollInterval, m.waitTGDeletionTimeout, isTargetGroupResourceInUseError, func() error {
		_, err := m.elbv2Client.DeleteTargetGroupWithContext(ctx, req)
		return err
	}); err != nil {
		return errors.Wrap(err, "failed to delete targetGroup")
	}
	m.logger.Info("deleted targetGroup",
		"arn", awssdk.StringValue(req.TargetGroupArn))

	return nil
}

func (m *defaultTargetGroupManager) updateSDKTargetGroupWithHealthCheck(ctx context.Context, resTG *elbv2model.TargetGroup, sdkTG TargetGroupWithTags) error {
	if !isSDKTargetGroupHealthCheckDrifted(resTG.Spec, sdkTG) {
		return nil
	}
	req := buildSDKModifyTargetGroupInput(resTG.Spec)
	req.TargetGroupArn = sdkTG.TargetGroup.TargetGroupArn

	m.logger.Info("modifying targetGroup healthCheck",
		"stackID", resTG.Stack().StackID(),
		"resourceID", resTG.ID(),
		"arn", awssdk.StringValue(sdkTG.TargetGroup.TargetGroupArn))
	if _, err := m.elbv2Client.ModifyTargetGroupWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("modified targetGroup healthCheck",
		"stackID", resTG.Stack().StackID(),
		"resourceID", resTG.ID(),
		"arn", awssdk.StringValue(sdkTG.TargetGroup.TargetGroupArn))

	return nil
}

func (m *defaultTargetGroupManager) updateSDKTargetGroupWithTags(ctx context.Context, resTG *elbv2model.TargetGroup, sdkTG TargetGroupWithTags) error {
	desiredTGTags := m.trackingProvider.ResourceTags(resTG.Stack(), resTG, resTG.Spec.Tags)
	return m.taggingManager.ReconcileTags(ctx, awssdk.StringValue(sdkTG.TargetGroup.TargetGroupArn), desiredTGTags,
		WithCurrentTags(sdkTG.Tags),
		WithIgnoredTagKeys(m.trackingProvider.LegacyTagKeys()),
		WithIgnoredTagKeys(m.externalManagedTags))
}

func isSDKTargetGroupHealthCheckDrifted(tgSpec elbv2model.TargetGroupSpec, sdkTG TargetGroupWithTags) bool {
	if tgSpec.HealthCheckConfig == nil {
		return false
	}
	sdkObj := sdkTG.TargetGroup
	hcConfig := *tgSpec.HealthCheckConfig
	if hcConfig.Port != nil && hcConfig.Port.String() != awssdk.StringValue(sdkObj.HealthCheckPort) {
		return true
	}
	if hcConfig.Protocol != nil && string(*hcConfig.Protocol) != awssdk.StringValue(sdkObj.HealthCheckProtocol) {
		return true
	}
	if hcConfig.Path != nil && awssdk.StringValue(hcConfig.Path) != awssdk.StringValue(sdkObj.HealthCheckPath) {
		return true
	}
	if hcConfig.Matcher != nil && (sdkObj.Matcher == nil || awssdk.StringValue(hcConfig.Matcher.GRPCCode) != awssdk.StringValue(sdkObj.Matcher.GrpcCode) || awssdk.StringValue(hcConfig.Matcher.HTTPCode) != awssdk.StringValue(sdkObj.Matcher.HttpCode)) {
		return true
	}
	if hcConfig.IntervalSeconds != nil && awssdk.Int64Value(hcConfig.IntervalSeconds) != awssdk.Int64Value(sdkObj.HealthCheckIntervalSeconds) {
		return true
	}
	if hcConfig.TimeoutSeconds != nil && awssdk.Int64Value(hcConfig.TimeoutSeconds) != awssdk.Int64Value(sdkObj.HealthCheckTimeoutSeconds) {
		return true
	}
	if hcConfig.HealthyThresholdCount != nil && awssdk.Int64Value(hcConfig.HealthyThresholdCount) != awssdk.Int64Value(sdkObj.HealthyThresholdCount) {
		return true
	}
	if hcConfig.UnhealthyThresholdCount != nil && awssdk.Int64Value(hcConfig.UnhealthyThresholdCount) != awssdk.Int64Value(sdkObj.UnhealthyThresholdCount) {
		return true
	}
	return false
}

func buildSDKCreateTargetGroupInput(tgSpec elbv2model.TargetGroupSpec) *elbv2sdk.CreateTargetGroupInput {
	sdkObj := &elbv2sdk.CreateTargetGroupInput{}
	sdkObj.Name = awssdk.String(tgSpec.Name)
	sdkObj.TargetType = awssdk.String(string(tgSpec.TargetType))
	sdkObj.Port = awssdk.Int64(tgSpec.Port)
	sdkObj.Protocol = awssdk.String(string(tgSpec.Protocol))
	if tgSpec.IPAddressType != nil && *tgSpec.IPAddressType != elbv2model.TargetGroupIPAddressTypeIPv4 {
		sdkObj.IpAddressType = (*string)(tgSpec.IPAddressType)
	}
	if tgSpec.ProtocolVersion != nil {
		sdkObj.ProtocolVersion = (*string)(tgSpec.ProtocolVersion)
	}
	if tgSpec.HealthCheckConfig != nil {
		hcConfig := *tgSpec.HealthCheckConfig
		sdkObj.HealthCheckEnabled = awssdk.Bool(true)
		if hcConfig.Port != nil {
			sdkObj.HealthCheckPort = awssdk.String(hcConfig.Port.String())
		}
		sdkObj.HealthCheckProtocol = (*string)(hcConfig.Protocol)
		sdkObj.HealthCheckPath = hcConfig.Path
		if tgSpec.HealthCheckConfig.Matcher != nil {
			sdkObj.Matcher = buildSDKMatcher(*hcConfig.Matcher)
		}
		sdkObj.HealthCheckIntervalSeconds = hcConfig.IntervalSeconds
		sdkObj.HealthCheckTimeoutSeconds = hcConfig.TimeoutSeconds
		sdkObj.HealthyThresholdCount = hcConfig.HealthyThresholdCount
		sdkObj.UnhealthyThresholdCount = hcConfig.UnhealthyThresholdCount
	}

	return sdkObj
}

func buildSDKModifyTargetGroupInput(tgSpec elbv2model.TargetGroupSpec) *elbv2sdk.ModifyTargetGroupInput {
	sdkObj := &elbv2sdk.ModifyTargetGroupInput{}
	if tgSpec.HealthCheckConfig != nil {
		hcConfig := *tgSpec.HealthCheckConfig
		sdkObj.HealthCheckEnabled = awssdk.Bool(true)
		if hcConfig.Port != nil {
			sdkObj.HealthCheckPort = awssdk.String(hcConfig.Port.String())
		}
		sdkObj.HealthCheckProtocol = (*string)(hcConfig.Protocol)
		sdkObj.HealthCheckPath = hcConfig.Path
		if tgSpec.HealthCheckConfig.Matcher != nil {
			sdkObj.Matcher = buildSDKMatcher(*hcConfig.Matcher)
		}
		sdkObj.HealthCheckIntervalSeconds = hcConfig.IntervalSeconds
		sdkObj.HealthCheckTimeoutSeconds = hcConfig.TimeoutSeconds
		sdkObj.HealthyThresholdCount = hcConfig.HealthyThresholdCount
		sdkObj.UnhealthyThresholdCount = hcConfig.UnhealthyThresholdCount
	}
	return sdkObj
}

func buildSDKMatcher(modelMatcher elbv2model.HealthCheckMatcher) *elbv2sdk.Matcher {
	return &elbv2sdk.Matcher{
		GrpcCode: modelMatcher.GRPCCode,
		HttpCode: modelMatcher.HTTPCode,
	}
}

func buildResTargetGroupStatus(sdkTG TargetGroupWithTags) elbv2model.TargetGroupStatus {
	return elbv2model.TargetGroupStatus{
		TargetGroupARN: awssdk.StringValue(sdkTG.TargetGroup.TargetGroupArn),
	}
}

func isTargetGroupResourceInUseError(err error) bool {
	var awsErr awserr.Error
	if errors.As(err, &awsErr) {
		return awsErr.Code() == "ResourceInUse"
	}
	return false
}
