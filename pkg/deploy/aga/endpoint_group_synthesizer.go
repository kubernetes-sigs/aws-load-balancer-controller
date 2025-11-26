package aga

import (
	"context"
	"k8s.io/apimachinery/pkg/util/sets"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	agatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

// NewEndpointGroupSynthesizer constructs new EndpointGroupSynthesizer
func NewEndpointGroupSynthesizer(
	gaService services.GlobalAccelerator,
	endpointGroupManager EndpointGroupManager,
	logger logr.Logger,
	stack core.Stack) *endpointGroupSynthesizer {

	return &endpointGroupSynthesizer{
		gaService:            gaService,
		endpointGroupManager: endpointGroupManager,
		logger:               logger,
		stack:                stack,
	}
}

// endpointGroupSynthesizer synthesizes AGA EndpointGroup resources
type endpointGroupSynthesizer struct {
	gaService            services.GlobalAccelerator
	endpointGroupManager EndpointGroupManager
	logger               logr.Logger
	stack                core.Stack
}

// EndpointPortConflict describes a conflict between endpoint ports in different groups
type EndpointPortConflict struct {
	Port              int32
	ConflictingGroups []string // ARNs of endpoint groups with this port
	ListenerARNs      []string // ARNs of listeners that contain the conflicting endpoint groups
}

// endpointGroupAndSDKEndpointGroup contains a pair of endpoint group resource and its SDK endpoint group
type resAndSDKGroupPair struct {
	resEndpointGroup *agamodel.EndpointGroup
	sdkEndpointGroup *agatypes.EndpointGroup
}

// mapEndpointGroupsByListenerARN maps endpoint groups by their parent listener ARN
func (s *endpointGroupSynthesizer) mapEndpointGroupsByListenerARN(ctx context.Context, resEndpointGroups []*agamodel.EndpointGroup) (map[string][]*agamodel.EndpointGroup, error) {
	endpointGroupsByListenerARN := make(map[string][]*agamodel.EndpointGroup)

	for _, eg := range resEndpointGroups {
		listenerARN, err := eg.Spec.ListenerARN.Resolve(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to resolve listener ARN for endpoint group %s", eg.ID())
		}
		endpointGroupsByListenerARN[listenerARN] = append(endpointGroupsByListenerARN[listenerARN], eg)
	}

	return endpointGroupsByListenerARN, nil
}

// areListenersEquivalent checks if two listener ARNs refer to the same underlying listener
// Since AWS Global Accelerator ARNs are consistent across resources, a simple string comparison is sufficient
func areListenersEquivalent(listener1, listener2 string) bool {
	return listener1 == listener2
}

// EndpointPortInfo represents endpoint port usage information for a specific region
type EndpointPortInfo struct {
	// Region where this endpoint port is used
	Region string
	// Port number
	Port int32
	// ListenerARN that uses this port
	ListenerARN string
	// EndpointGroupARN of the group using this port
	EndpointGroupARN string
}

// detectConflictsWithSDKEndpointGroups identifies endpoint port conflicts between desired
// endpoint groups and existing SDK endpoint groups within the same AWS region.
//
// AWS Global Accelerator enforces a critical constraint: within a single region, endpoint ports
// must be unique across endpoint groups belonging to different listeners. This prevents the
// same destination port from being used by multiple listeners in the same region, which would
// create ambiguity for traffic routing.
//
// The function compares endpoint ports used by desired endpoint groups against those
// already in use by existing SDK endpoint groups. It returns a map of conflicting ports
// to their respective conflicting endpoint group ARNs for resolution.
//
// For more details on port override constraints, see:
// https://docs.aws.amazon.com/global-accelerator/latest/dg/about-endpoint-groups-port-override.html
func (s *endpointGroupSynthesizer) detectConflictsWithSDKEndpointGroups(
	ctx context.Context,
	resEndpointGroups []*agamodel.EndpointGroup,
	sdkEndpointGroups []agatypes.EndpointGroup) (map[int32][]string, error) {

	// Step 1: Collect all desired endpoint ports by region and listener
	var desiredPortInfos []EndpointPortInfo

	for _, resGroup := range resEndpointGroups {
		// Skip groups with no port overrides
		if resGroup.Spec.PortOverrides == nil || len(resGroup.Spec.PortOverrides) == 0 {
			continue
		}

		// Get listener ARN for this resource group
		listenerARN, err := resGroup.Spec.ListenerARN.Resolve(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to resolve listener ARN for endpoint group %s", resGroup.ID())
		}

		// Add all endpoint ports this group wants to use
		for _, po := range resGroup.Spec.PortOverrides {
			desiredPortInfos = append(desiredPortInfos, EndpointPortInfo{
				Region:      resGroup.Spec.Region,
				Port:        po.EndpointPort,
				ListenerARN: listenerARN,
			})
		}
	}

	// No desired port overrides means no conflicts return early
	if len(desiredPortInfos) == 0 {
		return nil, nil
	}

	// Step 2: Collect all SDK endpoint ports by region and listener
	var sdkPortInfos []EndpointPortInfo

	for _, sdkGroup := range sdkEndpointGroups {
		region := awssdk.ToString(sdkGroup.EndpointGroupRegion)
		groupARN := awssdk.ToString(sdkGroup.EndpointGroupArn)
		listenerARN := extractListenerARNFromEndpointGroupARN(groupARN)

		// Add all ports for this SDK group
		for _, po := range sdkGroup.PortOverrides {
			port := awssdk.ToInt32(po.EndpointPort)
			sdkPortInfos = append(sdkPortInfos, EndpointPortInfo{
				Region:           region,
				Port:             port,
				ListenerARN:      listenerARN,
				EndpointGroupARN: groupARN,
			})
		}
	}

	// Step 3: Find conflicts by comparing different listeners using the same port in the same region
	conflicts := make(map[int32][]string) // map[port][]conflictingGroupARNs

	for _, desiredInfo := range desiredPortInfos {
		for _, sdkInfo := range sdkPortInfos {
			// Check if they're in the same region and using the same port
			if desiredInfo.Region == sdkInfo.Region && desiredInfo.Port == sdkInfo.Port {
				// Check if they're from different listeners
				if !areListenersEquivalent(desiredInfo.ListenerARN, sdkInfo.ListenerARN) {
					// If different listeners use same port in same region, it's a conflict
					conflicts[desiredInfo.Port] = append(conflicts[desiredInfo.Port], sdkInfo.EndpointGroupARN)

					s.logger.V(1).Info("Detected endpoint port conflict",
						"endpointPort", desiredInfo.Port,
						"region", desiredInfo.Region,
						"conflictingSDKGroup", sdkInfo.EndpointGroupARN)
				}
			}
		}
	}

	return conflicts, nil
}

// extractListenerARNFromEndpointGroupARN extracts the listener ARN portion from an endpoint group ARN
// Returns empty string if the format doesn't match expectations
func extractListenerARNFromEndpointGroupARN(groupARN string) string {
	// Expected format: arn:aws:globalaccelerator::123456789012:accelerator/abcd/listener/l-1234/endpoint-group/eg-1234

	// Check for endpoint group pattern
	if !strings.Contains(groupARN, "/endpoint-group/") {
		return ""
	}

	// Split by "endpoint-group" and take first part
	parts := strings.Split(groupARN, "/endpoint-group/")
	if len(parts) < 2 {
		return ""
	}

	return parts[0]
}

// resolveConflictsWithSDKEndpointGroups resolves endpoint port conflicts by updating
// the existing SDK endpoint groups to remove conflicting port overrides
func (s *endpointGroupSynthesizer) resolveConflictsWithSDKEndpointGroups(
	ctx context.Context,
	conflicts map[int32][]string,
	sdkEndpointGroups []agatypes.EndpointGroup) error {

	if len(conflicts) == 0 {
		return nil
	}

	s.logger.V(1).Info("Detected endpoint port conflicts with existing SDK endpoint groups",
		"conflictCount", len(conflicts))

	// Track which SDK groups need updating
	sdkGroupUpdates := make(map[string][]agatypes.PortOverride)

	// For each conflict, we need to remove the conflicting port override from the SDK groups
	for port, conflictingGroups := range conflicts {
		for _, groupARN := range conflictingGroups {
			// Find the SDK group with this ARN
			for i, sdkGroup := range sdkEndpointGroups {
				if awssdk.ToString(sdkGroup.EndpointGroupArn) == groupARN {
					// Create a filtered list of port overrides excluding the conflicting one
					updatedPortOverrides := make([]agatypes.PortOverride, 0, len(sdkGroup.PortOverrides))

					for _, po := range sdkGroup.PortOverrides {
						if awssdk.ToInt32(po.EndpointPort) != port {
							updatedPortOverrides = append(updatedPortOverrides, po)
						}
					}

					// Store the updated port overrides for this group
					sdkGroupUpdates[groupARN] = updatedPortOverrides

					// Update the SDK endpoint group in our local list to reflect the change
					sdkEndpointGroups[i].PortOverrides = updatedPortOverrides
					break
				}
			}
		}
	}

	// Update all the SDK endpoint groups that need updating
	for groupARN, updatedPortOverrides := range sdkGroupUpdates {
		s.logger.V(1).Info("Updating existing endpoint group to remove conflicting port overrides",
			"endpointGroupARN", groupARN,
			"updatedPortOverridesCount", len(updatedPortOverrides))

		// Create update input
		updateInput := &globalaccelerator.UpdateEndpointGroupInput{
			EndpointGroupArn: awssdk.String(groupARN),
			PortOverrides:    updatedPortOverrides,
		}

		// Update the endpoint group
		_, err := s.gaService.UpdateEndpointGroupWithContext(ctx, updateInput)
		if err != nil {
			return errors.Wrapf(err, "failed to update endpoint group %s to resolve port conflicts", groupARN)
		}
	}

	return nil
}

// getAllEndpointGroupsInListeners returns all endpoint groups across all listeners
func (s *endpointGroupSynthesizer) getAllEndpointGroupsInListeners(ctx context.Context, listenerARNs []string) ([]agatypes.EndpointGroup, error) {
	var allEndpointGroups []agatypes.EndpointGroup
	for _, listenerARN := range listenerARNs {
		endpointGroups, err := s.gaService.ListEndpointGroupsAsList(ctx, &globalaccelerator.ListEndpointGroupsInput{
			ListenerArn: awssdk.String(listenerARN),
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list endpoint groups for listener %s", listenerARN)
		}
		allEndpointGroups = append(allEndpointGroups, endpointGroups...)
	}

	return allEndpointGroups, nil
}

// Synthesize performs the actual synthesis of endpoint group resources
func (s *endpointGroupSynthesizer) Synthesize(ctx context.Context) error {
	var resEndpointGroups []*agamodel.EndpointGroup
	s.stack.ListResources(&resEndpointGroups)

	// Get listener ARNs from stack
	var resListeners []*agamodel.Listener
	s.stack.ListResources(&resListeners)

	// Nothing to process. No Listeners and endpoint groups in stack.
	// This means we have already deleted all the unneeded listeners and its corresponding endpoint groups during listener synthesis.
	if len(resListeners) == 0 {
		return nil
	}

	listenerARNs := make([]string, 0, len(resListeners))
	for _, resListener := range resListeners {
		listenerARN, err := resListener.ListenerARN().Resolve(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to resolve listener ARN for resListener %s", resListener.ID())
		}
		listenerARNs = append(listenerARNs, listenerARN)
	}

	// Group endpoint groups by listener ARN
	endpointGroupsByListenerARN, err := s.mapEndpointGroupsByListenerARN(ctx, resEndpointGroups)
	if err != nil {
		return err
	}

	// Only detect conflicts for endpoint port duplicates if there are any desired endpoint groups in our stack
	if len(resEndpointGroups) > 0 {
		// Get endpoint groups and handle any conflicts before proceeding
		if err := s.detectAndResolveEndpointGroupConflicts(ctx, resEndpointGroups, listenerARNs); err != nil {
			return err
		}
	}

	// Process endpoint groups by listener ARN
	for _, listenerARN := range listenerARNs {
		resEndpointGroups := endpointGroupsByListenerARN[listenerARN]
		if err := s.synthesizeEndpointGroupsOnListener(ctx, listenerARN, resEndpointGroups); err != nil {
			return err
		}
	}

	return nil
}

// PostSynthesize performs cleanup of endpoint group resources
// Currently not needed as deletion happens in synthesizeEndpointGroupsOnListener
func (s *endpointGroupSynthesizer) PostSynthesize(ctx context.Context) error {
	return nil
}

// detectAndResolveEndpointGroupConflicts handles endpoint group conflicts
// It detects conflicts between endpoint groups from different listeners, resolves them if needed
func (s *endpointGroupSynthesizer) detectAndResolveEndpointGroupConflicts(ctx context.Context, resEndpointGroups []*agamodel.EndpointGroup, listenerARNs []string) error {
	s.logger.V(1).Info("Detecting and resolving endpoint group conflicts",
		"endpointGroupCount", len(resEndpointGroups),
		"listenerCount", len(listenerARNs))

	// Get endpoint groups to check for conflicts with our desired state
	allSDKEndpointGroups, err := s.getAllEndpointGroupsInListeners(ctx, listenerARNs)
	if err != nil {
		s.logger.Error(err, "Failed to get endpoint groups for conflict checking",
			"listenerCount", len(listenerARNs))
		return errors.Wrap(err, "failed to get endpoint groups for conflict checking")
	}

	// Detect conflicts between our desired endpoint groups and existing ones in AWS
	sdkConflicts, err := s.detectConflictsWithSDKEndpointGroups(ctx, resEndpointGroups, allSDKEndpointGroups)
	if err != nil {
		s.logger.Error(err, "Failed to detect endpoint group conflicts",
			"endpointGroupCount", len(resEndpointGroups),
			"sdkEndpointGroupCount", len(allSDKEndpointGroups))
		return err
	}

	// If conflicts with existing SDK endpoint groups are found, update them to remove conflicts
	if len(sdkConflicts) > 0 {
		for port, groups := range sdkConflicts {
			s.logger.V(1).Info("Port conflict details",
				"endpointPort", port,
				"conflictingGroupCount", len(groups))
		}

		if err := s.resolveConflictsWithSDKEndpointGroups(ctx, sdkConflicts, allSDKEndpointGroups); err != nil {
			s.logger.Error(err, "Failed to resolve endpoint group port conflicts",
				"conflictCount", len(sdkConflicts))
			return errors.Wrap(err, "failed to resolve endpoint group port conflicts")
		}

		s.logger.Info("Successfully resolved all endpoint group port conflicts",
			"conflictCount", len(sdkConflicts))
	} else {
		s.logger.V(1).Info("No endpoint group port conflicts detected with external groups")
	}

	return nil
}

// matchResAndSDKEndpointGroups matches resource endpoint groups with SDK endpoint groups using region as the unique key
func matchResAndSDKEndpointGroups(resEndpointGroups []*agamodel.EndpointGroup, sdkEndpointGroups []agatypes.EndpointGroup) ([]resAndSDKGroupPair, []*agamodel.EndpointGroup, []*agatypes.EndpointGroup) {
	// Create maps for matching by region (region is the unique key within a listener)
	sdkGroupsByRegion := make(map[string]*agatypes.EndpointGroup)
	resGroupsByRegion := make(map[string]*agamodel.EndpointGroup)

	// Map resource endpoint groups by region
	for _, resGroup := range resEndpointGroups {
		region := resGroup.Spec.Region
		resGroupsByRegion[region] = resGroup
	}

	// Map SDK endpoint groups by region
	for _, sdkGroup := range sdkEndpointGroups {
		region := awssdk.ToString(sdkGroup.EndpointGroupRegion)
		sdkGroupsByRegion[region] = &sdkGroup
	}
	resGroupRegions := sets.StringKeySet(resGroupsByRegion)
	sdkGroupRegions := sets.StringKeySet(sdkGroupsByRegion)

	// Find matches and non-matches
	var matchedResAndSDKGroups []resAndSDKGroupPair
	var unmatchedResGroups []*agamodel.EndpointGroup
	var unmatchedSDKGroups []*agatypes.EndpointGroup

	// Find matched pairs and unmatched resource groups
	for _, region := range resGroupRegions.Intersection(sdkGroupRegions).List() {
		resGroup := resGroupsByRegion[region]
		sdkGroup := sdkGroupsByRegion[region]
		matchedResAndSDKGroups = append(matchedResAndSDKGroups, resAndSDKGroupPair{
			resEndpointGroup: resGroup,
			sdkEndpointGroup: sdkGroup,
		})
	}
	for _, region := range resGroupRegions.Difference(sdkGroupRegions).List() {
		unmatchedResGroups = append(unmatchedResGroups, resGroupsByRegion[region])
	}
	for _, region := range sdkGroupRegions.Difference(resGroupRegions).List() {
		unmatchedSDKGroups = append(unmatchedSDKGroups, sdkGroupsByRegion[region])
	}

	return matchedResAndSDKGroups, unmatchedResGroups, unmatchedSDKGroups
}

// synthesizeEndpointGroupsOnListener processes all endpoint groups for a specific listener
func (s *endpointGroupSynthesizer) synthesizeEndpointGroupsOnListener(ctx context.Context, listenerARN string, resEndpointGroups []*agamodel.EndpointGroup) error {
	// Get existing endpoint groups for this listener from AWS
	sdkEndpointGroups, err := s.getEndpointGroupsForListener(ctx, listenerARN)
	if err != nil {
		return errors.Wrapf(err, "failed to list endpoint groups for listener %s", listenerARN)
	}

	// Match resource endpoint groups with SDK endpoint groups
	matchedEndpointGroups, unmatchedResEndpointGroups, unmatchedSDKEndpointGroups := matchResAndSDKEndpointGroups(resEndpointGroups, sdkEndpointGroups)

	// Handle matched pairs - update them
	for _, pair := range matchedEndpointGroups {
		s.logger.Info("Updating existing endpoint group",
			"endpointGroupArn", *pair.sdkEndpointGroup.EndpointGroupArn,
			"region", pair.resEndpointGroup.Spec.Region)
		status, err := s.endpointGroupManager.Update(ctx, pair.resEndpointGroup, pair.sdkEndpointGroup)
		if err != nil {
			return errors.Wrapf(err, "failed to update endpoint group %v", pair.resEndpointGroup.ID())
		}

		// Update the resource with the returned status
		pair.resEndpointGroup.SetStatus(status)
	}

	// Handle unmatched SDK endpoint groups - delete them
	for _, sdkGroup := range unmatchedSDKEndpointGroups {
		s.logger.Info("Deleting unneeded endpoint group",
			"endpointGroupArn", *sdkGroup.EndpointGroupArn,
			"region", *sdkGroup.EndpointGroupRegion)
		egARN := awssdk.ToString(sdkGroup.EndpointGroupArn)

		if err := s.endpointGroupManager.Delete(ctx, egARN); err != nil {
			return errors.Wrapf(err, "failed to delete unneeded endpoint group: %v", egARN)
		}
	}

	// Handle unmatched resource endpoint groups - create them
	for _, resGroup := range unmatchedResEndpointGroups {
		s.logger.Info("Creating new endpoint group",
			"region", resGroup.Spec.Region)
		status, err := s.endpointGroupManager.Create(ctx, resGroup)
		if err != nil {
			return errors.Wrapf(err, "failed to create endpoint group %v", resGroup.ID())
		}

		// Update the resource with the returned status
		resGroup.SetStatus(status)
	}
	return nil
}

// getEndpointGroupsForListener gets all endpoint groups for a specific listener
func (s *endpointGroupSynthesizer) getEndpointGroupsForListener(ctx context.Context, listenerARN string) ([]agatypes.EndpointGroup, error) {
	listInput := &globalaccelerator.ListEndpointGroupsInput{
		ListenerArn: awssdk.String(listenerARN),
	}

	return s.gaService.ListEndpointGroupsAsList(ctx, listInput)
}
