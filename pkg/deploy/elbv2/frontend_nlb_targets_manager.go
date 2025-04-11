package elbv2

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

// FrontendNlbTargetsManager is an abstraction around ELBV2's targets API.
type FrontendNlbTargetsManager interface {
	// Register Targets into TargetGroup.
	RegisterTargets(ctx context.Context, tgArn string, albTarget elbv2types.TargetDescription) error

	// Deregister Targets from TargetGroup.
	DeregisterTargets(ctx context.Context, tgArn string, albTarget elbv2types.TargetDescription) error

	// List Targets from TargetGroup.
	ListTargets(ctx context.Context, tgArn string) ([]elbv2types.TargetHealthDescription, error)
}

// NewFrontendNlbTargetsManager constructs new frontendNlbTargetsManager
func NewFrontendNlbTargetsManager(elbv2Client services.ELBV2, logger logr.Logger) *frontendNlbTargetsManager {
	return &frontendNlbTargetsManager{
		elbv2Client: elbv2Client,
		logger:      logger,
	}
}

var _ FrontendNlbTargetsManager = &frontendNlbTargetsManager{}

type frontendNlbTargetsManager struct {
	elbv2Client services.ELBV2
	logger      logr.Logger
}

func (m *frontendNlbTargetsManager) RegisterTargets(ctx context.Context, tgARN string, albTarget elbv2types.TargetDescription) error {
	targets := []elbv2types.TargetDescription{albTarget}
	req := &elbv2sdk.RegisterTargetsInput{
		TargetGroupArn: aws.String(tgARN),
		Targets:        targets,
	}
	m.logger.Info("registering targets",
		"arn", tgARN,
		"targets", albTarget)

	_, err := m.elbv2Client.RegisterTargetsWithContext(ctx, req)

	if err != nil {
		return errors.Wrap(err, "failed to register targets")
	}

	m.logger.Info("registered targets",
		"arn", tgARN)
	return nil
}

func (m *frontendNlbTargetsManager) DeregisterTargets(ctx context.Context, tgARN string, albTarget elbv2types.TargetDescription) error {
	targets := []elbv2types.TargetDescription{albTarget}
	m.logger.Info("deRegistering targets",
		"arn", tgARN,
		"targets", targets)
	req := &elbv2sdk.DeregisterTargetsInput{
		TargetGroupArn: aws.String(tgARN),
		Targets:        targets,
	}
	_, err := m.elbv2Client.DeregisterTargetsWithContext(ctx, req)
	if err != nil {
		return errors.Wrap(err, "failed to deregister targets")
	}
	m.logger.Info("deregistered targets",
		"arn", tgARN)

	return nil
}

func (m *frontendNlbTargetsManager) ListTargets(ctx context.Context, tgARN string) ([]elbv2types.TargetHealthDescription, error) {
	m.logger.Info("Listing targets",
		"arn", tgARN)

	resp, err := m.elbv2Client.DescribeTargetHealthWithContext(ctx, &elbv2sdk.DescribeTargetHealthInput{
		TargetGroupArn: awssdk.String(tgARN),
	})

	if err != nil {
		return make([]elbv2types.TargetHealthDescription, 0), err
	}

	if len(resp.TargetHealthDescriptions) != 0 {
		return resp.TargetHealthDescriptions, nil
	}

	return make([]elbv2types.TargetHealthDescription, 0), nil
}
