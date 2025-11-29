package aga

import (
	"context"
	"errors"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	agatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
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

	// ManageEndpoints manages endpoints in an endpoint group based on the desired state.
	ManageEndpoints(ctx context.Context, endpointGroupARN string, resEndpointConfigs []agamodel.EndpointConfiguration, sdkEndpoints []agatypes.EndpointDescription) error
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
		return []agatypes.PortOverride{}
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

	// Manage endpoints for newly created endpoint group
	// For new endpoint groups, there are no existing endpoints
	var noEndpoints []agatypes.EndpointDescription
	if err := m.ManageEndpoints(ctx, *endpointGroup.EndpointGroupArn, resEndpointGroup.Spec.EndpointConfigurations, noEndpoints); err != nil {
		m.logger.Error(err, "Failed to manage endpoints for newly created endpoint group",
			"endpointGroupARN", *endpointGroup.EndpointGroupArn,
			"endpointCount", len(resEndpointGroup.Spec.EndpointConfigurations))
		return agamodel.EndpointGroupStatus{}, fmt.Errorf("failed to manage endpoints for endpoint group %s: %w", *endpointGroup.EndpointGroupArn, err)
	}

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
	} else {
		updateInput.TrafficDialPercentage = nil
	}

	// Add port overrides if specified
	updateInput.PortOverrides = m.buildSDKPortOverrides(resEndpointGroup.Spec.PortOverrides)

	return updateInput, nil
}

func (m *defaultEndpointGroupManager) Update(ctx context.Context, resEndpointGroup *agamodel.EndpointGroup, sdkEndpointGroup *agatypes.EndpointGroup) (agamodel.EndpointGroupStatus, error) {
	// Check if the endpoint group actually needs an update
	if !m.isSDKEndpointGroupSettingsDrifted(resEndpointGroup, sdkEndpointGroup) {
		m.logger.Info("No drift detected in endpoint group settings, skipping update",
			"stackID", resEndpointGroup.Stack().StackID(),
			"resourceID", resEndpointGroup.ID(),
			"endpointGroupARN", *sdkEndpointGroup.EndpointGroupArn)

		// Even if the endpoint group itself doesn't need an update, we still need to check endpoints
		if err := m.ManageEndpoints(ctx, *sdkEndpointGroup.EndpointGroupArn, resEndpointGroup.Spec.EndpointConfigurations, sdkEndpointGroup.EndpointDescriptions); err != nil {
			m.logger.Error(err, "Failed to manage endpoints for endpoint group",
				"endpointGroupARN", *sdkEndpointGroup.EndpointGroupArn,
				"desiredEndpointCount", len(resEndpointGroup.Spec.EndpointConfigurations),
				"currentEndpointCount", len(sdkEndpointGroup.EndpointDescriptions))
			return agamodel.EndpointGroupStatus{}, fmt.Errorf("failed to manage endpoints for endpoint group %s: %w", *sdkEndpointGroup.EndpointGroupArn, err)
		}

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

	// After updating the endpoint group, manage endpoints
	if err := m.ManageEndpoints(ctx, *updatedEndpointGroup.EndpointGroupArn, resEndpointGroup.Spec.EndpointConfigurations, updatedEndpointGroup.EndpointDescriptions); err != nil {
		m.logger.Error(err, "Failed to manage endpoints for updated endpoint group",
			"endpointGroupARN", *updatedEndpointGroup.EndpointGroupArn,
			"desiredEndpointCount", len(resEndpointGroup.Spec.EndpointConfigurations),
			"currentEndpointCount", len(updatedEndpointGroup.EndpointDescriptions))
		return agamodel.EndpointGroupStatus{}, fmt.Errorf("failed to manage endpoints for updated endpoint group %s: %w", *updatedEndpointGroup.EndpointGroupArn, err)
	}

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

// isEndpointConfigurationDrifted checks if the endpoint settings have drifted between desired and existing configuration
func (m *defaultEndpointGroupManager) isEndpointConfigurationDrifted(
	desiredConfig agamodel.EndpointConfiguration,
	existingEndpoint agatypes.EndpointDescription) bool {

	// Check weight drift
	if (desiredConfig.Weight == nil) != (existingEndpoint.Weight == nil) {
		return true
	} else if desiredConfig.Weight != nil && awssdk.ToInt32(desiredConfig.Weight) != awssdk.ToInt32(existingEndpoint.Weight) {
		return true
	}

	// Check client IP preservation drift
	if (desiredConfig.ClientIPPreservationEnabled == nil) != (existingEndpoint.ClientIPPreservationEnabled == nil) {
		return true
	} else if desiredConfig.ClientIPPreservationEnabled != nil &&
		awssdk.ToBool(desiredConfig.ClientIPPreservationEnabled) != awssdk.ToBool(existingEndpoint.ClientIPPreservationEnabled) {
		return true
	}

	return false
}

// buildSDKEndpointConfiguration converts a model endpoint configuration to an AWS SDK endpoint configuration
func (m *defaultEndpointGroupManager) buildSDKEndpointConfiguration(config agamodel.EndpointConfiguration) agatypes.EndpointConfiguration {
	endpointConfig := agatypes.EndpointConfiguration{
		EndpointId: awssdk.String(config.EndpointID),
	}

	// Add weight if specified
	if config.Weight != nil {
		endpointConfig.Weight = config.Weight
	}

	// Add client IP preservation if specified
	if config.ClientIPPreservationEnabled != nil {
		endpointConfig.ClientIPPreservationEnabled = config.ClientIPPreservationEnabled
	}

	return endpointConfig
}

// detectEndpointDrift compares existing endpoints with desired endpoint configurations
// It efficiently determines which endpoints need to be added, updated or removed using set operations.
// Returns:
//   - configsToAdd: Endpoint configurations that need to be added (present in desired but not in existing)
//   - configsToUpdate: Endpoint configurations present in both desired and existing
//   - endpointsToRemove: Endpoint IDs that need to be removed (present in existing but not in desired)
//   - isUpdateRequired: Returns true if any existing endpoint needs property updates (weight or clientIPPreservation)
//     This flag is used to determine the optimal API call strategy
func (m *defaultEndpointGroupManager) detectEndpointDrift(
	existingEndpoints []agatypes.EndpointDescription,
	desiredConfigs []agamodel.EndpointConfiguration) (configsToAdd []agamodel.EndpointConfiguration, configsToUpdate []agamodel.EndpointConfiguration, endpointsToRemove []string, isUpdateRequired bool) {

	// Extract all endpoint IDs from existing endpoints
	existingEndpointIDs := sets.NewString()
	existingIDToEndpoint := make(map[string]agatypes.EndpointDescription)
	for _, endpoint := range existingEndpoints {
		if endpoint.EndpointId != nil {
			id := awssdk.ToString(endpoint.EndpointId)
			existingEndpointIDs.Insert(id)
			existingIDToEndpoint[id] = endpoint
		}
	}

	// Extract all endpoint IDs from desired configs and create a lookup map
	desiredEndpointIDs := sets.NewString()
	idToConfig := make(map[string]agamodel.EndpointConfiguration)
	for _, config := range desiredConfigs {
		desiredEndpointIDs.Insert(config.EndpointID)
		idToConfig[config.EndpointID] = config
	}

	// Find endpoints to update (present in both desired and existing)
	endpointsToUpdateIDs := desiredEndpointIDs.Intersection(existingEndpointIDs)
	isUpdateRequired = false
	for id := range endpointsToUpdateIDs {
		resConfig, _ := idToConfig[id]
		sdkConfig, _ := existingIDToEndpoint[id]
		if m.isEndpointConfigurationDrifted(resConfig, sdkConfig) {
			isUpdateRequired = true
		}
		configsToUpdate = append(configsToUpdate, resConfig)

	}

	// Find endpoints to add (in desired but not in existing)
	endpointsToAddIDs := desiredEndpointIDs.Difference(existingEndpointIDs)
	for id := range endpointsToAddIDs {
		config, _ := idToConfig[id]
		configsToAdd = append(configsToAdd, config)
	}

	// Find endpoints to remove (in existing but not in desired)
	endpointsToRemove = existingEndpointIDs.Difference(desiredEndpointIDs).List()

	return configsToAdd, configsToUpdate, endpointsToRemove, isUpdateRequired
}

// ManageEndpoints manages endpoints in an endpoint group based on the desired state.
// It implements drift detection by comparing existing endpoints with desired ones,
// then performs necessary additions, updates, and removals to reconcile the state.
//
// This implementation optimizes API usage based on the type of changes needed:
//  1. For updates to existing endpoints: Uses UpdateEndpointGroup API which can handle both
//     new and updated endpoints in a single call (since AddEndpoints API doesn't support updates)
//  2. For simple additions/removals: Uses more efficient AddEndpoints and RemoveEndpoints APIs
//
// Following AWS Global Accelerator best practices, this implementation:
//  1. Adds endpoints first, then removes later to minimize connection disruption
//  2. Handles LimitExceededException by implementing a flip-flop Delete-Create pattern
//     where some existing endpoints are removed first to make room for new additions
func (m *defaultEndpointGroupManager) ManageEndpoints(
	ctx context.Context,
	endpointGroupARN string,
	resEndpointConfigs []agamodel.EndpointConfiguration,
	sdkEndpoints []agatypes.EndpointDescription) error {

	// Early return if there are no endpoints to manage
	if len(resEndpointConfigs) == 0 && len(sdkEndpoints) == 0 {
		m.logger.V(1).Info("No endpoint configurations found for endpoint group", "endpointGroupARN", endpointGroupARN)
		return nil
	}

	// Determine drift (endpoints to add/update/remove)
	configsToAdd, configsToUpdate, endpointsToRemove, isUpdateRequired := m.detectEndpointDrift(sdkEndpoints, resEndpointConfigs)

	if len(configsToAdd) == 0 && len(endpointsToRemove) == 0 && !isUpdateRequired {
		m.logger.V(1).Info("No drift found for endpoints", "endpointGroupARN", endpointGroupARN)
		return nil
	}

	m.logger.V(1).Info("Managing endpoints for endpoint group",
		"endpointGroupARN", endpointGroupARN,
		"addCount", len(configsToAdd),
		"updateCount", len(configsToUpdate),
		"removeCount", len(endpointsToRemove),
		"updateRequired", isUpdateRequired)

	// add-endpoints API doesn't support updating existing endpoints so we need to use update-endpoint-groups API for updates and add
	if isUpdateRequired {
		// Use UpdateEndpointGroup API to handle both adds and updates
		updatedConfigs := append(configsToAdd, configsToUpdate...)

		endpointConfigs := make([]agatypes.EndpointConfiguration, 0, len(updatedConfigs))
		for _, config := range updatedConfigs {
			endpointConfigs = append(endpointConfigs, m.buildSDKEndpointConfiguration(config))
		}

		// Call UpdateEndpointGroup with all configs
		updateInput := &globalaccelerator.UpdateEndpointGroupInput{
			EndpointGroupArn:       awssdk.String(endpointGroupARN),
			EndpointConfigurations: endpointConfigs,
		}

		if _, err := m.gaService.UpdateEndpointGroupWithContext(ctx, updateInput); err != nil {
			return fmt.Errorf("failed to update endpoint group %s: %w", endpointGroupARN, err)
		}
		return nil
	}

	// This is pure add and remove case. So we can use faster and efficient APIs
	// Try adding endpoints first - this follows AWS best practice to minimize connection disruption
	if len(configsToAdd) > 0 {
		err := m.addEndpoints(ctx, endpointGroupARN, configsToAdd)
		// If we hit a limit exception, we need to use flip-flop Delete-Create pattern
		var apiErr *agatypes.LimitExceededException
		if errors.As(err, &apiErr) {
			m.logger.V(1).Info("Hit endpoint limit, will remove some endpoints first and retry additions",
				"endpointGroupARN", endpointGroupARN)
			// Only proceed with flip-flop if we have endpoints to remove
			if len(endpointsToRemove) > 0 {
				if err := m.flipFlopEndpoints(ctx, endpointGroupARN, configsToAdd, endpointsToRemove); err != nil {
					return err
				}
				// All endpoints processed with flip-flop, so we're done
				return nil
			}
			// If no endpoints to remove but hit limit, just return the original error
			return fmt.Errorf("failed to add endpoints due to limit and no endpoints available to remove for endpoint group %s: %w", endpointGroupARN, err)
		} else if err != nil {
			// For any other error, return it directly
			return fmt.Errorf("failed to add endpoints for endpoint group %s: %w", endpointGroupARN, err)
		}
	}

	// Now remove endpoints that are no longer needed
	// (We do this after successfully adding to minimize connection disruption)
	if len(endpointsToRemove) > 0 {
		if err := m.removeEndpoints(ctx, endpointGroupARN, endpointsToRemove); err != nil {
			return err
		}
	}

	return nil
}

// addEndpoints adds endpoints to the endpoint group
func (m *defaultEndpointGroupManager) addEndpoints(
	ctx context.Context,
	endpointGroupARN string,
	configsToAdd []agamodel.EndpointConfiguration) error {

	// Skip if no endpoints to add
	if len(configsToAdd) == 0 {
		return nil
	}

	// Convert endpoint configurations to SDK format
	endpointConfigs := make([]agatypes.EndpointConfiguration, 0, len(configsToAdd))
	for _, config := range configsToAdd {
		endpointConfigs = append(endpointConfigs, m.buildSDKEndpointConfiguration(config))
	}

	// Prepare and execute the request
	addInput := &globalaccelerator.AddEndpointsInput{
		EndpointGroupArn:       awssdk.String(endpointGroupARN),
		EndpointConfigurations: endpointConfigs,
	}

	if _, err := m.gaService.AddEndpointsWithContext(ctx, addInput); err != nil {
		return fmt.Errorf("failed to add endpoints to endpoint group %s: %w", endpointGroupARN, err)
	}

	m.logger.V(1).Info("Successfully added endpoints",
		"endpointGroupARN", endpointGroupARN,
		"count", len(endpointConfigs))
	return nil
}

// removeEndpoints removes endpoints from the endpoint group
func (m *defaultEndpointGroupManager) removeEndpoints(
	ctx context.Context,
	endpointGroupARN string,
	endpointsToRemove []string) error {

	// Skip if no endpoints to remove
	if len(endpointsToRemove) == 0 {
		return nil
	}

	// Convert string endpoint IDs to EndpointIdentifier objects
	endpointIdentifiers := make([]agatypes.EndpointIdentifier, len(endpointsToRemove))
	for i, endpointID := range endpointsToRemove {
		endpointIdentifiers[i] = agatypes.EndpointIdentifier{
			EndpointId: awssdk.String(endpointID),
		}
	}

	// Create and execute the request
	removeInput := &globalaccelerator.RemoveEndpointsInput{
		EndpointGroupArn:    awssdk.String(endpointGroupARN),
		EndpointIdentifiers: endpointIdentifiers,
	}

	if _, err := m.gaService.RemoveEndpointsWithContext(ctx, removeInput); err != nil {
		return fmt.Errorf("failed to remove endpoints from endpoint group %s: %w", endpointGroupARN, err)
	}

	m.logger.V(1).Info("Successfully removed endpoints",
		"endpointGroupARN", endpointGroupARN,
		"count", len(endpointIdentifiers))
	return nil
}

// flipFlopEndpoints implements a simplified flip-flop Delete-Create pattern:
// 1. Remove all existing endpoints that need to be removed
// 2. Add all new endpoints at once
// This simple approach ensures we have room to add all new endpoints by removing old ones first.
func (m *defaultEndpointGroupManager) flipFlopEndpoints(
	ctx context.Context,
	endpointGroupARN string,
	configsToAdd []agamodel.EndpointConfiguration,
	endpointsToRemove []string) error {

	// First, remove all endpoints that need to be removed
	m.logger.V(1).Info("Flip-flop: Removing all endpoints to make room",
		"endpointGroupARN", endpointGroupARN,
		"removingCount", len(endpointsToRemove))

	if err := m.removeEndpoints(ctx, endpointGroupARN, endpointsToRemove); err != nil {
		return fmt.Errorf("flip-flop: failed to remove endpoints for endpoint group %s: %w", endpointGroupARN, err)
	}

	// Then, add all new endpoints at once
	m.logger.V(1).Info("Flip-flop: Adding all new endpoints",
		"endpointGroupARN", endpointGroupARN,
		"addingCount", len(configsToAdd))

	if err := m.addEndpoints(ctx, endpointGroupARN, configsToAdd); err != nil {
		return fmt.Errorf("flip-flop: failed to add endpoints after removing old ones for endpoint group %s: %w", endpointGroupARN, err)
	}

	return nil
}
