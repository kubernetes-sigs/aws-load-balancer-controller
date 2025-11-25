package aga

import (
	"context"
	"errors"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	agatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/go-logr/logr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
)

// EndpointGroupManager is responsible for managing AWS Global Accelerator endpoint groups.
type EndpointGroupManager interface {
	// Create creates an endpoint group.
	Create(ctx context.Context, resEndpointGroup *agamodel.EndpointGroup) (agamodel.EndpointGroupStatus, error)

	// Update updates an endpoint group.
	Update(ctx context.Context, resEndpointGroup *agamodel.EndpointGroup, sdkEndpointGroup *agatypes.EndpointGroup) (agamodel.EndpointGroupStatus, error)

	// Delete deletes an endpoint group.
	Delete(ctx context.Context, endpointGroupARN string) error
}

// NewDefaultEndpointGroupManager constructs new defaultEndpointGroupManager.
func NewDefaultEndpointGroupManager(gaService services.GlobalAccelerator, logger logr.Logger) *defaultEndpointGroupManager {
	return &defaultEndpointGroupManager{
		gaService: gaService,
		logger:    logger,
	}
}

var _ EndpointGroupManager = &defaultEndpointGroupManager{}

// defaultEndpointGroupManager is the default implementation for EndpointGroupManager.
type defaultEndpointGroupManager struct {
	gaService services.GlobalAccelerator
	logger    logr.Logger
}

// buildSDKPortOverrides converts model port overrides to SDK port overrides
func (m *defaultEndpointGroupManager) buildSDKPortOverrides(modelPortOverrides []agamodel.PortOverride) []agatypes.PortOverride {
	if len(modelPortOverrides) == 0 {
		return nil
	}

	portOverrides := make([]agatypes.PortOverride, 0, len(modelPortOverrides))
	for _, po := range modelPortOverrides {
		portOverrides = append(portOverrides, agatypes.PortOverride{
			ListenerPort: awssdk.Int32(po.ListenerPort),
			EndpointPort: awssdk.Int32(po.EndpointPort),
		})
	}
	return portOverrides
}

func (m *defaultEndpointGroupManager) buildSDKCreateEndpointGroupInput(_ context.Context, resEndpointGroup *agamodel.EndpointGroup) (*globalaccelerator.CreateEndpointGroupInput, error) {
	// Resolve listener ARN
	listenerARN, err := resEndpointGroup.Spec.ListenerARN.Resolve(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to resolve listener ARN: %w", err)
	}

	// Build create input
	createInput := &globalaccelerator.CreateEndpointGroupInput{
		ListenerArn:         awssdk.String(listenerARN),
		EndpointGroupRegion: awssdk.String(resEndpointGroup.Spec.Region),
	}

	// Convert TrafficDialPercentage from int32 to float32 if provided
	if resEndpointGroup.Spec.TrafficDialPercentage != nil {
		createInput.TrafficDialPercentage = awssdk.Float32(float32(*resEndpointGroup.Spec.TrafficDialPercentage))
	}

	// Add port overrides if specified
	if len(resEndpointGroup.Spec.PortOverrides) > 0 {
		createInput.PortOverrides = m.buildSDKPortOverrides(resEndpointGroup.Spec.PortOverrides)
	}

	return createInput, nil
}

func (m *defaultEndpointGroupManager) Create(ctx context.Context, resEndpointGroup *agamodel.EndpointGroup) (agamodel.EndpointGroupStatus, error) {
	// Build create input
	createInput, err := m.buildSDKCreateEndpointGroupInput(ctx, resEndpointGroup)
	if err != nil {
		return agamodel.EndpointGroupStatus{}, err
	}

	// Create endpoint group
	m.logger.V(1).Info("Creating endpoint group",
		"stackID", resEndpointGroup.Stack().StackID(),
		"resourceID", resEndpointGroup.ID())

	createOutput, err := m.gaService.CreateEndpointGroupWithContext(ctx, createInput)
	if err != nil {
		return agamodel.EndpointGroupStatus{}, fmt.Errorf("failed to create endpoint group: %w", err)
	}

	endpointGroup := createOutput.EndpointGroup
	m.logger.Info("Successfully created endpoint group",
		"stackID", resEndpointGroup.Stack().StackID(),
		"resourceID", resEndpointGroup.ID(),
		"endpointGroupARN", *endpointGroup.EndpointGroupArn)

	return agamodel.EndpointGroupStatus{
		EndpointGroupARN: *endpointGroup.EndpointGroupArn,
	}, nil
}

func (m *defaultEndpointGroupManager) buildSDKUpdateEndpointGroupInput(_ context.Context, resEndpointGroup *agamodel.EndpointGroup, sdkEndpointGroup *agatypes.EndpointGroup) (*globalaccelerator.UpdateEndpointGroupInput, error) {
	// Build update input
	updateInput := &globalaccelerator.UpdateEndpointGroupInput{
		EndpointGroupArn: sdkEndpointGroup.EndpointGroupArn,
	}

	// Convert TrafficDialPercentage from int32 to float32 if provided
	if resEndpointGroup.Spec.TrafficDialPercentage != nil {
		updateInput.TrafficDialPercentage = awssdk.Float32(float32(*resEndpointGroup.Spec.TrafficDialPercentage))
	}

	// Add port overrides if specified
	if len(resEndpointGroup.Spec.PortOverrides) > 0 {
		updateInput.PortOverrides = m.buildSDKPortOverrides(resEndpointGroup.Spec.PortOverrides)
	}

	return updateInput, nil
}

func (m *defaultEndpointGroupManager) Update(ctx context.Context, resEndpointGroup *agamodel.EndpointGroup, sdkEndpointGroup *agatypes.EndpointGroup) (agamodel.EndpointGroupStatus, error) {
	// Check if the endpoint group actually needs an update
	if !m.isSDKEndpointGroupSettingsDrifted(resEndpointGroup, sdkEndpointGroup) {
		m.logger.Info("No drift detected in endpoint group settings, skipping update",
			"stackID", resEndpointGroup.Stack().StackID(),
			"resourceID", resEndpointGroup.ID(),
			"endpointGroupARN", *sdkEndpointGroup.EndpointGroupArn)

		return agamodel.EndpointGroupStatus{
			EndpointGroupARN: *sdkEndpointGroup.EndpointGroupArn,
		}, nil
	}

	m.logger.Info("Drift detected in endpoint group settings, updating",
		"stackID", resEndpointGroup.Stack().StackID(),
		"resourceID", resEndpointGroup.ID(),
		"endpointGroupARN", *sdkEndpointGroup.EndpointGroupArn)

	// Build update input
	updateInput, err := m.buildSDKUpdateEndpointGroupInput(ctx, resEndpointGroup, sdkEndpointGroup)
	if err != nil {
		return agamodel.EndpointGroupStatus{}, err
	}

	// Update endpoint group
	updateOutput, err := m.gaService.UpdateEndpointGroupWithContext(ctx, updateInput)
	if err != nil {
		return agamodel.EndpointGroupStatus{}, fmt.Errorf("failed to update endpoint group: %w", err)
	}

	updatedEndpointGroup := updateOutput.EndpointGroup
	m.logger.Info("Successfully updated endpoint group",
		"stackID", resEndpointGroup.Stack().StackID(),
		"resourceID", resEndpointGroup.ID(),
		"endpointGroupARN", *updatedEndpointGroup.EndpointGroupArn)

	return agamodel.EndpointGroupStatus{
		EndpointGroupARN: *updatedEndpointGroup.EndpointGroupArn,
	}, nil
}

func (m *defaultEndpointGroupManager) Delete(ctx context.Context, endpointGroupARN string) error {
	m.logger.Info("Deleting endpoint group", "endpointGroupARN", endpointGroupARN)

	deleteInput := &globalaccelerator.DeleteEndpointGroupInput{
		EndpointGroupArn: awssdk.String(endpointGroupARN),
	}

	if _, err := m.gaService.DeleteEndpointGroupWithContext(ctx, deleteInput); err != nil {
		// Check if it's a not found error - the endpoint group might have been already deleted
		var apiErr *agatypes.EndpointGroupNotFoundException
		if errors.As(err, &apiErr) {
			m.logger.Info("Endpoint group already deleted", "endpointGroupARN", endpointGroupARN)
			return nil
		}
		return fmt.Errorf("failed to delete endpoint group: %w", err)
	}

	m.logger.Info("Successfully deleted endpoint group", "endpointGroupARN", endpointGroupARN)
	return nil
}

// isSDKEndpointGroupSettingsDrifted checks if the endpoint group configuration has drifted from the desired state
func (m *defaultEndpointGroupManager) isSDKEndpointGroupSettingsDrifted(resEndpointGroup *agamodel.EndpointGroup, sdkEndpointGroup *agatypes.EndpointGroup) bool {
	// Cannot change region after creation, so we don't check for region drift

	// Check traffic dial percentage
	if resEndpointGroup.Spec.TrafficDialPercentage != nil {
		resTrafficDialPercentage := float32(*resEndpointGroup.Spec.TrafficDialPercentage)
		sdkTrafficDialPercentage := awssdk.ToFloat32(sdkEndpointGroup.TrafficDialPercentage)
		// Use a small epsilon for float comparison to avoid precision issues
		const epsilon = 0.001
		if resTrafficDialPercentage < sdkTrafficDialPercentage-epsilon || resTrafficDialPercentage > sdkTrafficDialPercentage+epsilon {
			return true
		}
	} else if sdkEndpointGroup.TrafficDialPercentage != nil {
		// Resource has no traffic dial percentage but SDK does
		return true
	}

	// Check port overrides
	if !m.arePortOverridesEqual(resEndpointGroup.Spec.PortOverrides, sdkEndpointGroup.PortOverrides) {
		return true
	}

	return false
}

// arePortOverridesEqual compares port overrides from the resource model and SDK
func (m *defaultEndpointGroupManager) arePortOverridesEqual(modelPortOverrides []agamodel.PortOverride, sdkPortOverrides []agatypes.PortOverride) bool {
	if len(modelPortOverrides) != len(sdkPortOverrides) {
		return false
	}

	// Convert to maps for easier comparison
	modelMap := make(map[int32]int32)
	for _, po := range modelPortOverrides {
		modelMap[po.ListenerPort] = po.EndpointPort
	}

	// Check if all SDK port overrides match the model
	for _, po := range sdkPortOverrides {
		if modelEndpointPort, exists := modelMap[awssdk.ToInt32(po.ListenerPort)]; !exists || modelEndpointPort != awssdk.ToInt32(po.EndpointPort) {
			return false
		}
	}

	return true
}
