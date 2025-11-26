package aga

import (
	"context"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	agatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sort"
	"strings"
)

// NewListenerSynthesizer constructs listenerSynthesizer
func NewListenerSynthesizer(gaClient services.GlobalAccelerator, listenerManager ListenerManager,
	logger logr.Logger, stack core.Stack) *listenerSynthesizer {
	return &listenerSynthesizer{
		gaClient:        gaClient,
		listenerManager: listenerManager,
		logger:          logger,
		stack:           stack,
	}
}

// listenerSynthesizer is responsible for synthesize Listener resources for a stack.
type listenerSynthesizer struct {
	gaClient        services.GlobalAccelerator
	listenerManager ListenerManager
	logger          logr.Logger
	stack           core.Stack
}

func (s *listenerSynthesizer) Synthesize(ctx context.Context) error {
	// Get the accelerator resource from the stack
	var resAccelerators []*agamodel.Accelerator
	if err := s.stack.ListResources(&resAccelerators); err != nil {
		return err
	}
	if len(resAccelerators) == 0 {
		return errors.New("no accelerator resource found in stack")
	}
	accelerator := resAccelerators[0]

	// Get the accelerator ARN from the spec token
	acceleratorARN, err := accelerator.AcceleratorARN().Resolve(ctx)
	if err != nil {
		return errors.Wrapf(err, "unable to resolve accelerator ARN for stack %s", s.stack.StackID())
	}

	var resListeners []*agamodel.Listener
	s.stack.ListResources(&resListeners)

	// Process all listeners for this accelerator
	if err := s.synthesizeListenersOnAccelerator(ctx, acceleratorARN, resListeners); err != nil {
		return err
	}

	return nil
}

func (s *listenerSynthesizer) PostSynthesize(ctx context.Context) error {
	// PostSynthesize is called after all resources in the stack have been synthesized.
	// This is a good place to handle any cleanup or verification tasks.
	//
	// For listeners, we could use this to verify that all expected listeners
	// are properly created and configured, but this is already handled in the
	// main Synthesize method.
	//
	// Note: To minimize traffic disruption during reconciliation, we've already:
	// 1. Deleted unneeded/conflicting listeners to free up capacity and avoid conflicts
	// 2. Updated existing listeners to maintain their ARNs and associated resources
	// 3. Created new listeners as needed
	//
	// This order ensures that we maintain maximum stability across reconciliations
	// while also avoiding listener limit errors.

	return nil
}

func (s *listenerSynthesizer) synthesizeListenersOnAccelerator(ctx context.Context, accARN string, resListeners []*agamodel.Listener) error {
	// Get existing listeners for this accelerator
	sdkListeners, err := s.findSDKListenersOnAccelerator(ctx, accARN)
	if err != nil {
		return err
	}

	// Match resource listeners with existing SDK listeners
	// - matchedResAndSDKListeners: pairs of resource and SDK listeners that will be updated
	// - unmatchedResListeners: resource listeners that don't match any SDK listeners and will be created
	// - unmatchedSDKListeners: SDK listeners that don't match any resource listeners and will be deleted
	matchedResAndSDKListeners, unmatchedResListeners, unmatchedSDKListeners := s.matchResAndSDKListeners(resListeners, sdkListeners)

	// Improved operation order to minimize traffic disruption:
	// 1. Delete only conflicting listeners (that would block updates)
	// 2. Process all port overrides which may block listener updates
	//   - Remove endpoint port overrides that overlap with any new listener port ranges
	//   - Remove listener port overrides from existing listener which is outside desired listener port ranges
	// 3. Update matched listeners
	// 4. Delete unneeded (non-conflicting) listeners
	// 5. Create new listeners

	// STEP 1: Find SDK listeners that have port conflicts with planned updates
	conflictingListeners, nonConflictingListeners := s.findConflictingAndNonConflictingListeners(matchedResAndSDKListeners, unmatchedSDKListeners)

	// STEP 2: Execute operations in correct order

	// First, delete ONLY conflicting listeners (those that would block updates)
	// TODO: When we implement endpoint groups, for a more comprehensive solution, we might also want to add the ability to
	// migrate endpoint groups from these conflicting listeners to non-conflicting ones as much as possible.
	for _, listener := range conflictingListeners {
		s.logger.Info("Deleting conflicting listener to allow updates",
			"listenerARN", *listener.Listener.ListenerArn,
			"protocol", listener.Listener.Protocol)

		if err := s.listenerManager.Delete(ctx, *listener.Listener.ListenerArn); err != nil {
			s.logger.Error(err, "Failed to delete conflicting listener",
				"listenerARN", *listener.Listener.ListenerArn)
			return err
		}
	}

	// Next, Process all port overrides BEFORE updating listeners
	allResListenerPortRanges, allSDKListenersToProcess, updatePortRangesByListener := s.preparePortOverrideProcessing(resListeners, matchedResAndSDKListeners, nonConflictingListeners)

	// Consolidated port override processing
	if err := s.ProcessEndpointGroupPortOverrides(ctx, allSDKListenersToProcess, allResListenerPortRanges, updatePortRangesByListener); err != nil {
		s.logger.Error(err, "Failed to process endpoint group port overrides")
		return err
	}

	// Next, update existing matched listeners (now conflict-free)
	for _, pair := range matchedResAndSDKListeners {
		s.logger.Info("Updating existing listener",
			"listenerARN", *pair.sdkListener.Listener.ListenerArn,
			"protocol", pair.resListener.Spec.Protocol,
			"portRanges", s.portRangesToString(pair.resListener.Spec.PortRanges))

		listenerStatus, err := s.listenerManager.Update(ctx, pair.resListener, pair.sdkListener)
		if err != nil {
			s.logger.Error(err, "Failed to update listener",
				"listenerARN", *pair.sdkListener.Listener.ListenerArn)
			return err
		}
		pair.resListener.SetStatus(listenerStatus)
	}

	// Then, delete non-conflicting but unneeded listeners to free up the space
	for _, listener := range nonConflictingListeners {
		s.logger.Info("Deleting unneeded listener",
			"listenerARN", *listener.Listener.ListenerArn,
			"protocol", listener.Listener.Protocol)

		if err := s.listenerManager.Delete(ctx, *listener.Listener.ListenerArn); err != nil {
			s.logger.Error(err, "Failed to delete unneeded listener",
				"listenerARN", *listener.Listener.ListenerArn)
			return err
		}
	}

	// Finally, create any new listeners needed
	for _, resListener := range unmatchedResListeners {
		s.logger.Info("Creating new listener",
			"protocol", resListener.Spec.Protocol,
			"portRanges", s.portRangesToString(resListener.Spec.PortRanges))

		listenerStatus, err := s.listenerManager.Create(ctx, resListener)
		if err != nil {
			// If we hit a listener limit error, log it clearly
			var apiErr *agatypes.LimitExceededException
			if errors.As(err, &apiErr) {
				s.logger.Error(err,
					"Reached listener limit on accelerator. Tried to create a listener after deleting unmatched ones.")
			}
			return err
		}
		resListener.SetStatus(listenerStatus)
	}

	return nil
}

// findSDKListenersOnAccelerator returns all listeners for the given accelerator
func (s *listenerSynthesizer) findSDKListenersOnAccelerator(ctx context.Context, accARN string) ([]*ListenerResource, error) {
	// List listeners for the accelerator
	listInput := &globalaccelerator.ListListenersInput{
		AcceleratorArn: awssdk.String(accARN),
	}
	sdkListeners, err := s.gaClient.ListListenersAsList(ctx, listInput)
	if err != nil {
		var apiErr *agatypes.AcceleratorNotFoundException
		if errors.As(err, &apiErr) {
			s.logger.Info("Accelerator not found in AWS, skipping listener listing",
				"acceleratorARN", accARN)
			return nil, nil
		}
		return nil, errors.Wrapf(err, "failed to list listeners for accelerator %s", accARN)
	}

	var listeners []*ListenerResource
	for _, listener := range sdkListeners {
		listeners = append(listeners, &ListenerResource{
			Listener: &listener,
		})
	}
	return listeners, nil
}

// resAndSDKListenerPair holds a matched pair of resource and SDK listener
type resAndSDKListenerPair struct {
	resListener *agamodel.Listener
	sdkListener *ListenerResource
}

// matchResAndSDKListeners matches resource listeners with SDK listeners using a multi-phase approach.
//
// The algorithm implements a two-phase matching process:
//  1. First phase (Exact Matching): Matches listeners with identical protocol and port ranges
//  2. Second phase (Similarity Matching): For remaining unmatched listeners, uses a similarity-based
//     algorithm to find the best matches based on protocol and port range overlap
//
// Returns three groups:
// - matchedResAndSDKListeners: pairs of resource and SDK listeners that will be updated
// - unmatchedResListeners: resource listeners that don't match any SDK listeners and will be created
// - unmatchedSDKListeners: SDK listeners that don't match any resource listeners and will be deleted
func (s *listenerSynthesizer) matchResAndSDKListeners(resListeners []*agamodel.Listener, sdkListeners []*ListenerResource) (
	[]resAndSDKListenerPair, []*agamodel.Listener, []*ListenerResource) {

	// First, try to match by exact protocol and port ranges
	exactMatches, remainingResListeners, remainingSDKListeners := s.findExactMatches(resListeners, sdkListeners)

	// For remaining listeners, use similarity-based matching
	similarityMatches, unmatchedResListeners, unmatchedSDKListeners := s.findSimilarityMatches(
		remainingResListeners, remainingSDKListeners)

	// Combine exact and similarity matches
	matchedPairs := append(exactMatches, similarityMatches...)

	s.logger.V(1).Info("Matched listeners",
		"exactMatches", len(exactMatches),
		"similarityMatches", len(similarityMatches),
		"unmatchedResListeners", len(unmatchedResListeners),
		"unmatchedSDKListeners", len(unmatchedSDKListeners))

	return matchedPairs, unmatchedResListeners, unmatchedSDKListeners
}

// findExactMatches matches listeners that have identical protocol and port ranges.
//
// This function:
// 1. Creates a unique key for each listener based on protocol and port ranges
// 2. Sorts port ranges for consistent key generation
// 3. Matches listeners with identical keys (exact protocol and port range matches)
// 4. Returns matched pairs and remaining unmatched listeners
//
// The key generation ensures that port ranges in different order but with identical
// values still match correctly.
func (s *listenerSynthesizer) findExactMatches(resListeners []*agamodel.Listener, sdkListeners []*ListenerResource) (
	[]resAndSDKListenerPair, []*agamodel.Listener, []*ListenerResource) {

	var matchedPairs []resAndSDKListenerPair
	var unmatchedResListeners []*agamodel.Listener
	var unmatchedSDKListeners []*ListenerResource

	// Create maps with protocol+portRanges as key
	resListenerByKey := make(map[string]*agamodel.Listener)
	sdkListenerByKey := make(map[string]*ListenerResource)

	// Map resource listeners
	for _, resListener := range resListeners {
		key := s.generateResListenerKey(resListener)
		resListenerByKey[key] = resListener
	}

	// Map SDK listeners
	for _, sdkListener := range sdkListeners {
		key := s.generateSDKListenerKey(sdkListener)
		sdkListenerByKey[key] = sdkListener
	}

	// Find matched and unmatched listeners
	resListenerKeys := sets.StringKeySet(resListenerByKey)
	sdkListenerKeys := sets.StringKeySet(sdkListenerByKey)

	// Create compact log entries for exact matches
	var exactMatchDescriptions []string
	// Find matches
	exactMatches := resListenerKeys.Intersection(sdkListenerKeys).List()

	for _, key := range exactMatches {
		resListener := resListenerByKey[key]
		sdkListener := sdkListenerByKey[key]
		matchedPairs = append(matchedPairs, resAndSDKListenerPair{
			resListener: resListener,
			sdkListener: sdkListener,
		})

		// Add compact description for this match
		exactMatchDescriptions = append(exactMatchDescriptions,
			fmt.Sprintf("%s→%s(key:%s)", resListener.ID(),
				awssdk.ToString(sdkListener.Listener.ListenerArn), key))
	}

	// Log all exact matches
	if len(exactMatchDescriptions) > 0 {
		s.logger.V(1).Info("Exact matches found",
			"matches", strings.Join(exactMatchDescriptions, ", "))
	}

	// Find unmatched resource listeners
	for _, key := range resListenerKeys.Difference(sdkListenerKeys).List() {
		unmatchedResListeners = append(unmatchedResListeners, resListenerByKey[key])
	}

	// Find unmatched SDK listeners
	for _, key := range sdkListenerKeys.Difference(resListenerKeys).List() {
		unmatchedSDKListeners = append(unmatchedSDKListeners, sdkListenerByKey[key])
	}

	return matchedPairs, unmatchedResListeners, unmatchedSDKListeners
}

// listenerPairScore holds a potential match with its similarity score
type listenerPairScore struct {
	resListener *agamodel.Listener
	sdkListener *ListenerResource
	score       int
}

// findSimilarityMatches matches remaining listeners based on similarity score.
//
// This function:
// 1. Calculates similarity scores between all possible pairings of unmatched resource and SDK listeners
// 2. Filters pairs that don't meet the minimum similarity threshold (15%)
// 3. Sorts pairs by decreasing similarity score
// 4. Greedily matches the highest-scoring pairs first, ensuring no listener is matched more than once
// 5. Returns similarity-based matched pairs and remaining unmatched listeners
//
// The minimum similarity threshold of 15% was chosen as a balance between allowing some
// flexibility in matching while avoiding false positive matches between listeners with
// minimal similarity.
func (s *listenerSynthesizer) findSimilarityMatches(resListeners []*agamodel.Listener, sdkListeners []*ListenerResource) (
	[]resAndSDKListenerPair, []*agamodel.Listener, []*ListenerResource) {

	// Define minimum similarity threshold - below this, we don't consider it a match
	const minSimilarityThreshold = 15 // 15%

	var matchedPairs []resAndSDKListenerPair

	// Return early if either list is empty
	if len(resListeners) == 0 || len(sdkListeners) == 0 {
		return matchedPairs, resListeners, sdkListeners
	}

	// Calculate similarity scores for all possible pairings
	var scoredPairs []listenerPairScore
	for _, resListener := range resListeners {
		for _, sdkListener := range sdkListeners {
			// Calculate similarity score for this pair
			score := s.calculateSimilarityScore(resListener, sdkListener)

			// Only consider pairs with meaningful similarity (score >= minSimilarityThreshold)
			if score >= minSimilarityThreshold {
				scoredPairs = append(scoredPairs, listenerPairScore{
					resListener: resListener,
					sdkListener: sdkListener,
					score:       score,
				})
			}
		}
	}

	// Sort pairs by score (highest first)
	sort.Slice(scoredPairs, func(i, j int) bool {
		return scoredPairs[i].score > scoredPairs[j].score
	})

	// Track which listeners have been matched
	matchedResListenerIDs := sets.NewString()
	matchedSDKListenerARNs := sets.NewString()

	// Create compact log entries for similarity matches
	var similarityMatchDescriptions []string

	// Match greedily by highest score first
	for _, pair := range scoredPairs {
		resID := pair.resListener.ID()
		sdkARN := awssdk.ToString(pair.sdkListener.Listener.ListenerArn)

		// Skip if either listener is already matched
		if matchedResListenerIDs.Has(resID) || matchedSDKListenerARNs.Has(sdkARN) {
			continue
		}

		// Add this pair to matches
		matchedPairs = append(matchedPairs, resAndSDKListenerPair{
			resListener: pair.resListener,
			sdkListener: pair.sdkListener,
		})

		// Mark as matched
		matchedResListenerIDs.Insert(resID)
		matchedSDKListenerARNs.Insert(sdkARN)

		// Add compact description for this match
		similarityMatchDescriptions = append(similarityMatchDescriptions,
			fmt.Sprintf("%s→%s(score:%d)", resID,
				sdkARN, pair.score))
	}

	// Log all similarity matches in a single line if there are any
	if len(similarityMatchDescriptions) > 0 {
		s.logger.V(1).Info("Similarity matches found",
			"matches", strings.Join(similarityMatchDescriptions, ", "))
	}

	// Collect unmatched resource listeners
	var unmatchedResListeners []*agamodel.Listener
	for _, resListener := range resListeners {
		if !matchedResListenerIDs.Has(resListener.ID()) {
			unmatchedResListeners = append(unmatchedResListeners, resListener)
		}
	}

	// Collect unmatched SDK listeners
	var unmatchedSDKListeners []*ListenerResource
	for _, sdkListener := range sdkListeners {
		if !matchedSDKListenerARNs.Has(awssdk.ToString(sdkListener.Listener.ListenerArn)) {
			unmatchedSDKListeners = append(unmatchedSDKListeners, sdkListener)
		}
	}

	return matchedPairs, unmatchedResListeners, unmatchedSDKListeners
}

// findConflictingAndNonConflictingListeners separates unmatched SDK listeners into those that have
// port conflicts with planned updates and those that don't
func (s *listenerSynthesizer) findConflictingAndNonConflictingListeners(
	matchedResAndSDKListeners []resAndSDKListenerPair,
	unmatchedSDKListeners []*ListenerResource) ([]*ListenerResource, []*ListenerResource) {

	var conflictingListeners []*ListenerResource
	var nonConflictingListeners []*ListenerResource

	// Track which listeners have port conflicts with our updates
	conflictMap := make(map[string][]*ListenerResource)

	// For each update we're planning to do...
	for _, pair := range matchedResAndSDKListeners {
		var conflicts []*ListenerResource

		// Check against all unmatched SDK listeners for conflicts
		for _, sdkListener := range unmatchedSDKListeners {
			if s.hasPortRangeConflict(pair.resListener, sdkListener) {
				conflicts = append(conflicts, sdkListener)
			}
		}

		// If there are conflicts, add them to our conflict map
		if len(conflicts) > 0 {
			conflictMap[pair.resListener.ID()] = conflicts
		}
	}

	// Build list of conflicting and non-conflicting listeners
	listenerIsConflicting := make(map[string]bool)

	// Add all listeners with port conflicts to the conflicting list
	for _, conflicts := range conflictMap {
		for _, listener := range conflicts {
			arn := *listener.Listener.ListenerArn
			if !listenerIsConflicting[arn] {
				conflictingListeners = append(conflictingListeners, listener)
				listenerIsConflicting[arn] = true
			}
		}
	}

	// Sort remaining unmatched listeners into non-conflicting
	for _, sdkListener := range unmatchedSDKListeners {
		arn := *sdkListener.Listener.ListenerArn
		if !listenerIsConflicting[arn] {
			nonConflictingListeners = append(nonConflictingListeners, sdkListener)
		}
	}

	return conflictingListeners, nonConflictingListeners
}

// calculateSimilarityScore calculates how similar two listeners are
// Higher scores indicate better matches
// calculateSimilarityScore calculates how similar two listeners are based on their attributes.
//
// The scoring system uses these components:
//
// 1. Base Protocol Score:
//   - If protocols match: +40 points (significant bonus)
//   - If protocols don't match: 0 points (no bonus)
//
// 2. Port Overlap Score:
//   - Uses Jaccard similarity: (intersection / union) * 100
//   - Calculates the percentage of common ports between the two listeners
//   - Converts port ranges into individual port sets for precise comparison
//
// 3. Client Affinity Score:
//   - If both listeners have client affinity specified and they match: +10 points
//   - Otherwise: 0 points (no bonus)
//
// Note: In the future, we might need to add endpoint matching as well as one of the
// score components so that we match the listeners with the most endpoint matches
// in order to avoid creation-deletion of endpoint groups.
//
// The total similarity score is the sum of the protocol score, port overlap score,
// and client affinity score.
func (s *listenerSynthesizer) calculateSimilarityScore(resListener *agamodel.Listener, sdkListener *ListenerResource) int {
	// Start with base score
	score := 0

	// Protocol match is highly valuable - give significant bonus
	if string(resListener.Spec.Protocol) == string(sdkListener.Listener.Protocol) {
		score += 40 // Strong bonus for protocol match
	}

	// Calculate port overlap
	resPortSet := s.makeResPortSet(resListener.Spec.PortRanges)
	sdkPortSet := s.makeSDKPortSet(sdkListener.Listener.PortRanges)

	// Find common ports (intersection)
	commonPorts := 0
	for port := range resPortSet {
		if sdkPortSet[port] {
			commonPorts++
		}
	}

	// Calculate total unique ports (union)
	totalPorts := len(resPortSet) + len(sdkPortSet) - commonPorts

	// Jaccard similarity: intersection / union (as a percentage)
	if totalPorts > 0 {
		score += (commonPorts * 100) / totalPorts
	}

	// If client affinity matches and is specified, add bonus points
	resClientAffinity := string(resListener.Spec.ClientAffinity)
	sdkClientAffinity := string(sdkListener.Listener.ClientAffinity)

	// Only add bonus if both have affinity set and they match
	if resClientAffinity != "" && sdkClientAffinity != "" && resClientAffinity == sdkClientAffinity {
		score += 10
	}

	return score
}

// makeResPortSet converts resource model port ranges to a set of individual ports.
func (s *listenerSynthesizer) makeResPortSet(portRanges []agamodel.PortRange) map[int32]bool {
	portSet := make(map[int32]bool)
	ResPortRangesToSet(portRanges, portSet)
	return portSet
}

// makeSDKPortSet converts SDK port ranges to a set of individual ports.
func (s *listenerSynthesizer) makeSDKPortSet(portRanges []agatypes.PortRange) map[int32]bool {
	portSet := make(map[int32]bool)
	SDKPortRangesToSet(portRanges, portSet)
	return portSet
}

// generateResListenerKey creates a unique key for a resource listener based on protocol and port ranges
func (s *listenerSynthesizer) generateResListenerKey(listener *agamodel.Listener) string {
	protocol := string(listener.Spec.Protocol)

	// Sort port ranges before generating key to ensure consistent matching
	sortedPortRanges := make([]agamodel.PortRange, len(listener.Spec.PortRanges))
	copy(sortedPortRanges, listener.Spec.PortRanges)
	SortModelPortRanges(sortedPortRanges)

	portRanges := ResPortRangesToString(sortedPortRanges)
	return protocol + ":" + portRanges
}

// generateSDKListenerKey creates a unique key for an SDK listener based on protocol and port ranges
func (s *listenerSynthesizer) generateSDKListenerKey(listener *ListenerResource) string {
	protocol := string(listener.Listener.Protocol)

	// Sort port ranges before generating key to ensure consistent matching
	sortedPortRanges := make([]agatypes.PortRange, len(listener.Listener.PortRanges))
	copy(sortedPortRanges, listener.Listener.PortRanges)
	SortSDKPortRanges(sortedPortRanges)

	portRanges := SDKPortRangesToString(sortedPortRanges)
	return protocol + ":" + portRanges
}

// hasPortRangeConflict checks if there's any overlap between port ranges of two listeners
func (s *listenerSynthesizer) hasPortRangeConflict(resListener *agamodel.Listener, sdkListener *ListenerResource) bool {
	// Different protocols can use the same ports without conflict
	if string(resListener.Spec.Protocol) != string(sdkListener.Listener.Protocol) {
		return false
	}

	// Build port sets for both listeners
	resPortSet := s.makeResPortSet(resListener.Spec.PortRanges)
	sdkPortSet := s.makeSDKPortSet(sdkListener.Listener.PortRanges)

	// Check for any port overlap
	for port := range resPortSet {
		if sdkPortSet[port] {
			return true // Found an overlapping port
		}
	}

	return false
}

// portRangesToString serializes port ranges to a string - deprecated, use ResPortRangesToString instead
func (s *listenerSynthesizer) portRangesToString(portRanges []agamodel.PortRange) string {
	return ResPortRangesToString(portRanges)
}

// havePortRangesChanged checks if port ranges have changed between resource and SDK listener
func (s *listenerSynthesizer) havePortRangesChanged(resListener *agamodel.Listener, sdkListener *ListenerResource) bool {
	if len(resListener.Spec.PortRanges) != len(sdkListener.Listener.PortRanges) {
		return true
	}

	// Build maps for easy comparison
	resPortSet := s.makeResPortSet(resListener.Spec.PortRanges)
	sdkPortSet := s.makeSDKPortSet(sdkListener.Listener.PortRanges)

	// If port sets have different sizes, they've changed
	if len(resPortSet) != len(sdkPortSet) {
		return true
	}

	// Check if any port exists in one set but not the other
	for port := range resPortSet {
		if !sdkPortSet[port] {
			return true
		}
	}

	for port := range sdkPortSet {
		if !resPortSet[port] {
			return true
		}
	}

	// Port ranges are the same
	return false
}

// ProcessEndpointGroupPortOverrides handles all port override validations and updates using a two-phase approach
// Phase 1: Collect all endpoint groups and analyze all port overrides for conflicts
// Phase 2: Execute updates for all identified conflicts
//
// The two-phase approach ensures consistent behavior regardless of processing order, since all
// analysis is completed before any modifications are made.
//
// It handles these validations:
//   - Remove port overrides with endpoint port that overlap with any desired listener port ranges
//   - Remove port overrides with listener port outside desired listener port ranges
func (s *listenerSynthesizer) ProcessEndpointGroupPortOverrides(
	ctx context.Context,
	listeners []*ListenerResource,
	allListenerPortRanges []agamodel.PortRange,
	updatePortRangesByListener map[string][]agamodel.PortRange) error {

	s.logger.V(1).Info("Processing all endpoint port overrides before updating listeners")

	// PHASE 1: Collection and Analysis
	// Map of endpoint group ARN to its conflict information
	type endpointGroupConflicts struct {
		endpointGroup        agatypes.EndpointGroup
		listenerARN          string
		validPortOverrides   []agatypes.PortOverride
		invalidPortOverrides []agatypes.PortOverride
	}

	// Store all conflicts to be resolved
	conflictsByEndpointGroupARN := make(map[string]*endpointGroupConflicts)

	// Process each listener to collect all endpoint groups and analyze port overrides
	for _, listener := range listeners {
		listenerARN := awssdk.ToString(listener.Listener.ListenerArn)

		// List endpoint groups per listener
		endpointGroups, err := s.listenerManager.ListEndpointGroups(ctx, listenerARN)
		if err != nil {
			return fmt.Errorf("failed to list endpoint groups for listener %s: %w", listenerARN, err)
		}

		// Skip if no endpoint groups
		if len(endpointGroups) == 0 {
			continue
		}

		// Get the updated port ranges for this listener if it's being updated
		updatedPortRanges := updatePortRangesByListener[listenerARN]

		// Analyze each endpoint group's port overrides
		for _, eg := range endpointGroups {
			endpointGroupARN := awssdk.ToString(eg.EndpointGroupArn)

			// Skip if no port overrides to check
			if eg.PortOverrides == nil || len(eg.PortOverrides) == 0 {
				continue
			}

			s.logger.V(1).Info("Analyzing endpoint group port overrides for conflicts",
				"listenerARN", listenerARN,
				"endpointGroupARN", endpointGroupARN)

			// Apply all validation rules and collect valid/invalid port overrides
			validPortOverrides, invalidPortOverrides := s.processPortOverridesWithAllRules(
				eg.PortOverrides,
				allListenerPortRanges,
				updatedPortRanges)

			// Only store conflicts if we found invalid overrides
			if len(invalidPortOverrides) > 0 {
				conflictsByEndpointGroupARN[endpointGroupARN] = &endpointGroupConflicts{
					endpointGroup:        eg,
					listenerARN:          listenerARN,
					validPortOverrides:   validPortOverrides,
					invalidPortOverrides: invalidPortOverrides,
				}
			}
		}
	}

	// PHASE 2: Execution - Update all endpoint groups with conflicts
	// Process all conflicts
	for endpointGroupARN := range conflictsByEndpointGroupARN {
		conflictInfo := conflictsByEndpointGroupARN[endpointGroupARN]

		s.logger.V(1).Info("Updating endpoint group to remove conflicting port overrides",
			"endpointGroupARN", endpointGroupARN,
			"listenerARN", conflictInfo.listenerARN,
			"conflictCount", len(conflictInfo.invalidPortOverrides))

		// Update this endpoint group to remove the invalid port overrides
		if err := s.updateEndpointGroupPortOverrides(
			ctx,
			conflictInfo.endpointGroup,
			conflictInfo.validPortOverrides,
			conflictInfo.invalidPortOverrides); err != nil {
			return fmt.Errorf("failed to update endpoint group %s to remove conflicts: %w",
				endpointGroupARN, err)
		}
	}

	return nil
}

// processPortOverridesWithAllRules applies all validation rules to port overrides:
// 1. Endpoint ports must not overlap with any listener port ranges (if listener is being updated)
// 2. Listener ports must be within listener port ranges (if listener is being updated)
func (s *listenerSynthesizer) processPortOverridesWithAllRules(
	portOverrides []agatypes.PortOverride,
	allListenerPortRanges []agamodel.PortRange,
	updatedListenerPortRanges []agamodel.PortRange) ([]agatypes.PortOverride, []agatypes.PortOverride) {

	validPortOverrides := make([]agatypes.PortOverride, 0)
	invalidPortOverrides := make([]agatypes.PortOverride, 0)
	for _, po := range portOverrides {
		isValid := true

		// Rule 1: Endpoint port must not overlap with ANY listener port range
		if aga.IsPortInRanges(awssdk.ToInt32(po.EndpointPort), allListenerPortRanges) {
			isValid = false
			s.logger.V(1).Info("Found port override with endpoint port that overlaps with a listener port range",
				"endpointPort", awssdk.ToInt32(po.EndpointPort),
				"listenerPort", awssdk.ToInt32(po.ListenerPort))
		}

		// Rule 2: If listener is being updated, listener port must be within updated port ranges
		if isValid && len(updatedListenerPortRanges) > 0 && !aga.IsPortInRanges(awssdk.ToInt32(po.ListenerPort), updatedListenerPortRanges) {
			isValid = false
			s.logger.V(1).Info("Found port override with listener port outside updated listener port range",
				"listenerPort", awssdk.ToInt32(po.ListenerPort),
				"endpointPort", awssdk.ToInt32(po.EndpointPort))
		}

		// Add to appropriate collection based on validation result
		if isValid {
			validPortOverrides = append(validPortOverrides, po)
		} else {
			invalidPortOverrides = append(invalidPortOverrides, po)
		}
	}

	return validPortOverrides, invalidPortOverrides
}

// preparePortOverrideProcessing collects all the port override processing requirements:
// - all res listener port ranges
// - all SDK listeners to process
// - map of listeners being updated with their new port ranges
func (s *listenerSynthesizer) preparePortOverrideProcessing(
	resListeners []*agamodel.Listener,
	matchedResAndSDKListeners []resAndSDKListenerPair,
	nonConflictingListeners []*ListenerResource) ([]agamodel.PortRange, []*ListenerResource, map[string][]agamodel.PortRange) {

	// Collect all port ranges from resource listeners
	var allResListenerPortRanges []agamodel.PortRange
	for _, resListener := range resListeners {
		allResListenerPortRanges = append(allResListenerPortRanges, resListener.Spec.PortRanges...)
	}

	// Extract the SDK listeners from matchedResAndSDKListeners
	var allSDKListenersToProcess []*ListenerResource
	for _, pair := range matchedResAndSDKListeners {
		allSDKListenersToProcess = append(allSDKListenersToProcess, pair.sdkListener)
	}

	// Combine with nonConflictingListeners
	allSDKListenersToProcess = append(allSDKListenersToProcess, nonConflictingListeners...)

	// Prepare map of listeners being updated with their new port ranges
	updatePortRangesByListener := make(map[string][]agamodel.PortRange)
	for _, pair := range matchedResAndSDKListeners {
		if s.havePortRangesChanged(pair.resListener, pair.sdkListener) {
			listenerARN := awssdk.ToString(pair.sdkListener.Listener.ListenerArn)
			updatePortRangesByListener[listenerARN] = pair.resListener.Spec.PortRanges
		}
	}

	return allResListenerPortRanges, allSDKListenersToProcess, updatePortRangesByListener
}

// updateEndpointGroupPortOverrides updates an endpoint group with valid port overrides
// and logs information about the removed invalid ones
func (s *listenerSynthesizer) updateEndpointGroupPortOverrides(
	ctx context.Context,
	endpointGroup agatypes.EndpointGroup,
	validPortOverrides []agatypes.PortOverride,
	invalidPortOverrides []agatypes.PortOverride) error {

	endpointGroupARN := awssdk.ToString(endpointGroup.EndpointGroupArn)

	// For logging purposes, record each removed override
	for _, po := range invalidPortOverrides {
		s.logger.V(1).Info("Removing port override",
			"listenerPort", awssdk.ToInt32(po.ListenerPort),
			"endpointPort", awssdk.ToInt32(po.EndpointPort),
			"endpointGroupARN", endpointGroupARN)
	}

	// Update the endpoint group with only valid port overrides
	_, err := s.gaClient.UpdateEndpointGroupWithContext(ctx, &globalaccelerator.UpdateEndpointGroupInput{
		EndpointGroupArn: endpointGroup.EndpointGroupArn,
		PortOverrides:    validPortOverrides,
	})

	if err != nil {
		return fmt.Errorf("failed to update endpoint group %s for port overrides to remove conflicts: %w", endpointGroupARN, err)
	}

	s.logger.Info("Successfully updated endpoint group port overrides to remove conflicts",
		"endpointGroupARN", endpointGroupARN,
		"removedCount", len(invalidPortOverrides),
		"remainingCount", len(validPortOverrides))

	return nil
}
