package aga

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	agatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/aws/smithy-go"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

// NewAcceleratorSynthesizer constructs acceleratorSynthesizer
func NewAcceleratorSynthesizer(gaClient services.GlobalAccelerator, trackingProvider tracking.Provider, taggingManager TaggingManager,
	acceleratorManager AcceleratorManager, logger logr.Logger, featureGates config.FeatureGates, stack core.Stack) *acceleratorSynthesizer {
	return &acceleratorSynthesizer{
		gaClient:                 gaClient,
		trackingProvider:         trackingProvider,
		taggingManager:           taggingManager,
		acceleratorManager:       acceleratorManager,
		logger:                   logger,
		stack:                    stack,
		featureGates:             featureGates,
		unmatchedSDKAccelerators: nil,
	}
}

// acceleratorSynthesizer is responsible for synthesize Accelerator resources types for certain stack.
type acceleratorSynthesizer struct {
	gaClient           services.GlobalAccelerator
	trackingProvider   tracking.Provider
	taggingManager     TaggingManager
	acceleratorManager AcceleratorManager
	logger             logr.Logger
	stack              core.Stack
	featureGates       config.FeatureGates

	// Store unmatched accelerators for deletion in PostSynthesize
	unmatchedSDKAccelerators []AcceleratorWithTags
}

func (s *acceleratorSynthesizer) Synthesize(ctx context.Context) error {
	// Get the accelerator resource from the stack
	resAccelerator, err := s.getAcceleratorResource()
	if err != nil {
		return err
	}

	// Check if accelerator exists in AWS by ARN
	arn := s.getAcceleratorARNFromCRD(resAccelerator)
	if arn == "" {
		// No ARN in status - create new accelerator
		return s.handleCreateAccelerator(ctx, resAccelerator)
	}

	// ARN exists, try to describe the accelerator
	sdkAccelerator, err := s.describeAcceleratorByARN(ctx, arn)
	if err != nil {
		// Handle the case where accelerator doesn't exist in AWS
		if s.isAcceleratorNotFound(err) {
			s.logger.Info("Accelerator ARN found in CRD status but not in AWS, recreating",
				"arn", arn, "resourceID", resAccelerator.ID())
			return s.handleCreateAccelerator(ctx, resAccelerator)
		}
		return err
	}

	// Accelerator exists, determine if it needs replacement or update
	if isSDKAcceleratorRequiresReplacement(sdkAccelerator, resAccelerator) {
		// Store for deletion in PostSynthesize, then recreate
		// TODO: We will test this for BYOIP feature
		s.unmatchedSDKAccelerators = []AcceleratorWithTags{sdkAccelerator}
		return s.handleCreateAccelerator(ctx, resAccelerator)
	} else {
		return s.handleUpdateAccelerator(ctx, resAccelerator, sdkAccelerator)
	}
}

// getAcceleratorResource retrieves the accelerator resource from the stack
func (s *acceleratorSynthesizer) getAcceleratorResource() (*agamodel.Accelerator, error) {
	var resAccelerators []*agamodel.Accelerator
	if err := s.stack.ListResources(&resAccelerators); err != nil {
		return nil, err
	}

	// Stack contains one accelerator
	if len(resAccelerators) == 0 {
		return nil, errors.New("no accelerator resource found in stack")
	}
	return resAccelerators[0], nil
}

// handleCreateAccelerator creates a new accelerator and updates its status
func (s *acceleratorSynthesizer) handleCreateAccelerator(ctx context.Context, resAccelerator *agamodel.Accelerator) error {
	acceleratorStatus, err := s.acceleratorManager.Create(ctx, resAccelerator)
	if err != nil {
		return err
	}
	resAccelerator.SetStatus(acceleratorStatus)
	return nil
}

// handleUpdateAccelerator updates an existing accelerator
func (s *acceleratorSynthesizer) handleUpdateAccelerator(ctx context.Context, resAccelerator *agamodel.Accelerator, sdkAccelerator AcceleratorWithTags) error {
	acceleratorStatus, err := s.acceleratorManager.Update(ctx, resAccelerator, sdkAccelerator)
	if err != nil {
		return err
	}
	resAccelerator.SetStatus(acceleratorStatus)
	return nil
}

func (s *acceleratorSynthesizer) PostSynthesize(ctx context.Context) error {
	// Delete unmatched accelerators after all dependent resources have been cleaned up
	// This is called after all other synthesizers have completed their PostSynthesize
	for _, sdkAccelerator := range s.unmatchedSDKAccelerators {
		if err := s.acceleratorManager.Delete(ctx, sdkAccelerator); err != nil {
			return err
		}
	}
	return nil
}

// getAcceleratorARNFromCRD extracts the ARN from the CRD status if available.
func (s *acceleratorSynthesizer) getAcceleratorARNFromCRD(resAccelerator *agamodel.Accelerator) string {
	return resAccelerator.GetARNFromCRDStatus()
}

// describeAcceleratorByARN describes an accelerator by ARN and returns it with tags.
func (s *acceleratorSynthesizer) describeAcceleratorByARN(ctx context.Context, arn string) (AcceleratorWithTags, error) {
	// Describe the accelerator
	describeInput := &globalaccelerator.DescribeAcceleratorInput{
		AcceleratorArn: awssdk.String(arn),
	}

	describeOutput, err := s.gaClient.DescribeAcceleratorWithContext(ctx, describeInput)
	if err != nil {
		return AcceleratorWithTags{}, err
	}

	// Get tags for the accelerator
	tagsInput := &globalaccelerator.ListTagsForResourceInput{
		ResourceArn: awssdk.String(arn),
	}

	tagsOutput, err := s.gaClient.ListTagsForResourceWithContext(ctx, tagsInput)
	if err != nil {
		return AcceleratorWithTags{}, err
	}

	// Convert tags to map
	tags := make(map[string]string)
	for _, tag := range tagsOutput.Tags {
		if tag.Key != nil && tag.Value != nil {
			tags[*tag.Key] = *tag.Value
		}
	}

	return AcceleratorWithTags{
		Accelerator: describeOutput.Accelerator,
		Tags:        tags,
	}, nil
}

// isAcceleratorNotFound checks if the error indicates the accelerator was not found.
func (s *acceleratorSynthesizer) isAcceleratorNotFound(err error) bool {
	var awsErr *agatypes.AcceleratorNotFoundException
	if errors.As(err, &awsErr) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "AcceleratorNotFoundException"
	}
	return false
}

// isSDKAcceleratorRequiresReplacement checks whether a sdk Accelerator requires replacement to fulfill an Accelerator resource.
func isSDKAcceleratorRequiresReplacement(sdkAccelerator AcceleratorWithTags, resAccelerator *agamodel.Accelerator) bool {
	// The accelerator will only need replacement in BYOIP scenarios. I will implement this later as a separate PR
	// TODO : BYOIP feature
	return false
}
