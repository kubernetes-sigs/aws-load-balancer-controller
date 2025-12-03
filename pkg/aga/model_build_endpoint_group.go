package aga

import (
	"context"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

// endpointGroupBuilder builds EndpointGroup model resources
type endpointGroupBuilder interface {
	// Build builds all endpoint groups for all listeners
	Build(ctx context.Context, stack core.Stack, listeners []*agamodel.Listener,
		listenerConfigs []agaapi.GlobalAcceleratorListener, loadedEndpoints []*LoadedEndpoint) ([]*agamodel.EndpointGroup, error)

	// buildEndpointGroupsForListener builds endpoint groups for a specific listener
	buildEndpointGroupsForListener(ctx context.Context, stack core.Stack, listener *agamodel.Listener,
		endpointGroups []agaapi.GlobalAcceleratorEndpointGroup, listenerIndex int,
		loadedEndpoints []*LoadedEndpoint) ([]*agamodel.EndpointGroup, error)
}

// NewEndpointGroupBuilder constructs new endpointGroupBuilder
func NewEndpointGroupBuilder(clusterRegion string, gaNamespace string, logger logr.Logger) endpointGroupBuilder {
	return &defaultEndpointGroupBuilder{
		clusterRegion: clusterRegion,
		gaNamespace:   gaNamespace,
		logger:        logger,
	}
}

var _ endpointGroupBuilder = &defaultEndpointGroupBuilder{}

type defaultEndpointGroupBuilder struct {
	clusterRegion string
	gaNamespace   string
	logger        logr.Logger
}

// Build builds EndpointGroup model resources
func (b *defaultEndpointGroupBuilder) Build(ctx context.Context, stack core.Stack, listeners []*agamodel.Listener,
	listenerConfigs []agaapi.GlobalAcceleratorListener, loadedEndpoints []*LoadedEndpoint) ([]*agamodel.EndpointGroup, error) {
	if listeners == nil || len(listeners) == 0 {
		return nil, nil
	}

	var result []*agamodel.EndpointGroup

	// Create a map of all listener port ranges
	listenerPortRanges := make(map[string][]agamodel.PortRange) // Maps listener ID to its port ranges
	for _, listener := range listeners {
		listenerPortRanges[listener.ID()] = listener.Spec.PortRanges
	}

	for i, listener := range listeners {
		listenerConfig := listenerConfigs[i]
		if listenerConfig.EndpointGroups == nil {
			continue
		}

		listenerEndpointGroups, err := b.buildEndpointGroupsForListener(ctx, stack, listener, *listenerConfig.EndpointGroups, i, loadedEndpoints)
		if err != nil {
			return nil, err
		}
		result = append(result, listenerEndpointGroups...)
	}

	// Validate endpoint ports in all port overrides across all listeners
	if err := b.validateEndpointPortOverridesCrossListeners(result, listenerPortRanges); err != nil {
		return nil, err
	}

	return result, nil
}

// validateEndpointPortOverridesCrossListeners performs validations for endpoint port overrides across all listeners
func (b *defaultEndpointGroupBuilder) validateEndpointPortOverridesCrossListeners(endpointGroups []*agamodel.EndpointGroup, listenerPortRanges map[string][]agamodel.PortRange) error {
	// Track endpoint port usage across all endpoint groups
	endpointPortUsage := make(map[int32]string) // Maps endpoint port to listener ID

	// Check all endpoint groups for port overrides
	for _, endpointGroup := range endpointGroups {
		listenerID := endpointGroup.Listener.ID()

		for _, portOverride := range endpointGroup.Spec.PortOverrides {
			endpointPort := portOverride.EndpointPort

			// Rule 1: Check if endpoint port is within any listener's port range
			if err := b.validateEndpointPortOverridesWithinListener(endpointPort, listenerPortRanges); err != nil {
				return err
			}

			// Rule 2: Check for duplicate endpoint port usage across listeners
			if existingListenerID, exists := endpointPortUsage[endpointPort]; exists && existingListenerID != listenerID {
				return fmt.Errorf("duplicate endpoint port %d: the same endpoint port cannot be used in port overrides from different listeners (used in %s and %s)",
					endpointPort, existingListenerID, listenerID)
			}

			// Register this endpoint port usage
			endpointPortUsage[endpointPort] = listenerID
		}
	}

	return nil
}

// validateEndpointPortOverridesWithinListener checks if an endpoint port is within any listener's port range
func (b *defaultEndpointGroupBuilder) validateEndpointPortOverridesWithinListener(endpointPort int32, listenerPortRanges map[string][]agamodel.PortRange) error {
	for listenerID, portRanges := range listenerPortRanges {
		if IsPortInRanges(endpointPort, portRanges) {
			// Find the specific port range for the error message
			for _, portRange := range portRanges {
				if endpointPort >= portRange.FromPort && endpointPort <= portRange.ToPort {
					return fmt.Errorf("endpoint port %d conflicts with listener %s port range %d-%d: endpoint port cannot be included in any listener port range",
						endpointPort, listenerID, portRange.FromPort, portRange.ToPort)
				}
			}
		}
	}
	return nil
}

// buildEndpointGroupsForListener builds EndpointGroup models for a specific listener
func (b *defaultEndpointGroupBuilder) buildEndpointGroupsForListener(ctx context.Context, stack core.Stack,
	listener *agamodel.Listener, endpointGroups []agaapi.GlobalAcceleratorEndpointGroup,
	listenerIndex int, loadedEndpoints []*LoadedEndpoint) ([]*agamodel.EndpointGroup, error) {
	var result []*agamodel.EndpointGroup

	for i, endpointGroup := range endpointGroups {
		spec, err := b.buildEndpointGroupSpec(ctx, listener, endpointGroup, loadedEndpoints)
		if err != nil {
			return nil, err
		}

		resourceID := fmt.Sprintf("EndpointGroup-%d-%d", listenerIndex, i)
		endpointGroupModel := agamodel.NewEndpointGroup(stack, resourceID, spec, listener)
		result = append(result, endpointGroupModel)
	}

	return result, nil
}

// buildEndpointGroupSpec builds the EndpointGroupSpec for a single EndpointGroup model resource
func (b *defaultEndpointGroupBuilder) buildEndpointGroupSpec(ctx context.Context,
	listener *agamodel.Listener, endpointGroup agaapi.GlobalAcceleratorEndpointGroup,
	loadedEndpoints []*LoadedEndpoint) (agamodel.EndpointGroupSpec, error) {
	region, err := b.determineRegion(endpointGroup)
	if err != nil {
		return agamodel.EndpointGroupSpec{}, err
	}

	// Handle trafficDialPercentage
	trafficDialPercentage := endpointGroup.TrafficDialPercentage

	portOverrides, err := b.buildPortOverrides(ctx, listener, endpointGroup)
	if err != nil {
		return agamodel.EndpointGroupSpec{}, err
	}

	// Build endpoint configurations from both static configurations and loaded endpoints
	endpointConfigurations, err := b.buildEndpointConfigurations(ctx, endpointGroup, loadedEndpoints)
	if err != nil {
		return agamodel.EndpointGroupSpec{}, err
	}

	return agamodel.EndpointGroupSpec{
		ListenerARN:            listener.ListenerARN(),
		Region:                 region,
		TrafficDialPercentage:  trafficDialPercentage,
		PortOverrides:          portOverrides,
		EndpointConfigurations: endpointConfigurations,
	}, nil
}

// generateEndpointKey creates a consistent string key for endpoint lookup
func generateEndpointKey(ep agaapi.GlobalAcceleratorEndpoint, gaNamespace string) string {
	namespace := gaNamespace
	if ep.Namespace != nil {
		namespace = awssdk.ToString(ep.Namespace)
	}
	name := awssdk.ToString(ep.Name)

	if ep.Type == agaapi.GlobalAcceleratorEndpointTypeEndpointID {
		return fmt.Sprintf("%s/%s", ep.Type, awssdk.ToString(ep.EndpointID))
	}
	return fmt.Sprintf("%s/%s/%s", ep.Type, namespace, name)
}

// buildEndpointConfigurations builds endpoint configurations from both static configurations in the API struct
// and from successfully loaded endpoints
func (b *defaultEndpointGroupBuilder) buildEndpointConfigurations(_ context.Context,
	endpointGroup agaapi.GlobalAcceleratorEndpointGroup, loadedEndpoints []*LoadedEndpoint) ([]agamodel.EndpointConfiguration, error) {

	var endpointConfigurations []agamodel.EndpointConfiguration

	// Skip if no endpoints defined in the endpoint group
	if endpointGroup.Endpoints == nil {
		return nil, nil
	}

	// Build a map of loaded endpoints with for quick lookup
	loadedEndpointsMap := make(map[string]*LoadedEndpoint)
	for _, le := range loadedEndpoints {
		key := le.GetKey()
		loadedEndpointsMap[key] = le

	}

	// Process the endpoints defined in the CRD and match with loaded endpoints
	for _, ep := range *endpointGroup.Endpoints {
		// Create key for lookup using the helper function
		lookupKey := generateEndpointKey(ep, b.gaNamespace)

		// Find the loaded endpoint
		if loadedEndpoint, found := loadedEndpointsMap[lookupKey]; found {
			// Add endpoint to model stack only if its in Loaded status and has valid ARN
			if loadedEndpoint.Status == EndpointStatusLoaded {
				// Create a base configuration with the loaded endpoint's ARN
				endpointConfig := agamodel.EndpointConfiguration{
					EndpointID: loadedEndpoint.ARN,
				}
				endpointConfig.Weight = awssdk.Int32(loadedEndpoint.Weight)
				endpointConfig.ClientIPPreservationEnabled = ep.ClientIPPreservationEnabled
				endpointConfigurations = append(endpointConfigurations, endpointConfig)
			} else {
				// Log warning for endpoints which are not loaded successfully during loading and has Warning status
				b.logger.Info("Endpoint not added to endpoint group as no valid ARN was found during loading",
					"endpoint", lookupKey,
					"message", loadedEndpoint.Message,
					"error", loadedEndpoint.Error)
			}
		} else {
			b.logger.Info("Endpoint not found in loaded endpoints",
				"endpoint", lookupKey)
		}
	}

	return endpointConfigurations, nil
}

// Note: The TargetsEndpointGroup method is no longer needed since we match endpoints based on
// the explicit references in the GlobalAcceleratorEndpoint resources under each endpoint group

// validateListenerPortOverrideWithinListenerPortRanges ensures all listener ports used in port overrides are
// contained within the listener's port ranges
func (b *defaultEndpointGroupBuilder) validateListenerPortOverrideWithinListenerPortRanges(listener *agamodel.Listener, portOverrides []agamodel.PortOverride) error {
	if len(portOverrides) == 0 {
		return nil
	}

	for _, portOverride := range portOverrides {
		listenerPort := portOverride.ListenerPort
		if !IsPortInRanges(listenerPort, listener.Spec.PortRanges) {
			return fmt.Errorf("port override listener port %d is not within any listener port ranges - this will cause AWS Global Accelerator to reject the configuration", listenerPort)
		}
	}
	return nil
}

// determineRegion determines the region for the endpoint group
func (b *defaultEndpointGroupBuilder) determineRegion(endpointGroup agaapi.GlobalAcceleratorEndpointGroup) (string, error) {
	// Use explicit region from endpoint group if specified
	if endpointGroup.Region != nil && awssdk.ToString(endpointGroup.Region) != "" {
		return awssdk.ToString(endpointGroup.Region), nil
	}

	// Default to cluster region if available
	if b.clusterRegion != "" {
		return b.clusterRegion, nil
	}
	return "", fmt.Errorf("region is required for endpoint group but neither specified in the endpoint group nor available from cluster configuration")
}

// buildPortOverrides builds the port overrides for the endpoint group
func (b *defaultEndpointGroupBuilder) buildPortOverrides(_ context.Context, listener *agamodel.Listener, endpointGroup agaapi.GlobalAcceleratorEndpointGroup) ([]agamodel.PortOverride, error) {
	if endpointGroup.PortOverrides == nil {
		return nil, nil
	}

	var portOverrides []agamodel.PortOverride
	for _, po := range *endpointGroup.PortOverrides {
		portOverrides = append(portOverrides, agamodel.PortOverride{
			ListenerPort: po.ListenerPort,
			EndpointPort: po.EndpointPort,
		})
	}

	// Validate all port override rules
	if err := b.validatePortOverrides(listener, portOverrides); err != nil {
		return []agamodel.PortOverride{}, err
	}

	return portOverrides, nil
}

// validateNoDuplicatePorts checks both listener and endpoint ports for duplicates in a single pass
func (b *defaultEndpointGroupBuilder) validateNoDuplicatePorts(portOverrides []agamodel.PortOverride) error {
	if len(portOverrides) <= 1 {
		return nil
	}

	listenerPorts := make(map[int32]bool)
	endpointPorts := make(map[int32]bool)

	for _, portOverride := range portOverrides {
		// Check for duplicate listener ports
		listenerPort := portOverride.ListenerPort
		if listenerPorts[listenerPort] {
			return fmt.Errorf("duplicate listener port %d in port overrides: each listener port can only be used once in port overrides for an endpoint group", listenerPort)
		}
		listenerPorts[listenerPort] = true

		// Check for duplicate endpoint ports
		endpointPort := portOverride.EndpointPort
		if endpointPorts[endpointPort] {
			return fmt.Errorf("duplicate endpoint port %d in port overrides: each endpoint port can only be used once in port overrides for an endpoint group", endpointPort)
		}
		endpointPorts[endpointPort] = true
	}

	return nil
}

// validatePortOverrides is a wrapper function that runs all port override validation rules
func (b *defaultEndpointGroupBuilder) validatePortOverrides(listener *agamodel.Listener, portOverrides []agamodel.PortOverride) error {
	// Validate listener port overrides against listener port ranges
	if err := b.validateListenerPortOverrideWithinListenerPortRanges(listener, portOverrides); err != nil {
		return err
	}

	// Check for duplicate listener and endpoint ports within this endpoint group's port overrides
	if err := b.validateNoDuplicatePorts(portOverrides); err != nil {
		return err
	}

	return nil
}
