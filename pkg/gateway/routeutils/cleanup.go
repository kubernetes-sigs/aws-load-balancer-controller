package routeutils

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
)

// CleanupResult represents the result of a cleanup operation
type CleanupResult struct {
	RedirectOnlyRulesSkipped int
	TargetGroupsDeleted      int
	Errors                   []error
}

// RouteCleanupManager handles cleanup operations for HTTPRoute rules
type RouteCleanupManager struct {
	logger logr.Logger
}

// NewRouteCleanupManager creates a new RouteCleanupManager
func NewRouteCleanupManager(logger logr.Logger) *RouteCleanupManager {
	return &RouteCleanupManager{
		logger: logger,
	}
}

// CleanupRouteRules performs cleanup for a set of route rules
func (m *RouteCleanupManager) CleanupRouteRules(ctx context.Context, routeKey types.NamespacedName, rules []RouteRule) CleanupResult {
	result := CleanupResult{
		Errors: make([]error, 0),
	}

	for i, rule := range rules {
		if IsRedirectOnlyRule(rule) {
			// Skip cleanup for redirect-only rules as they don't have target groups
			result.RedirectOnlyRulesSkipped++
			m.logger.Info("Skipping cleanup for redirect-only rule", 
				"route", routeKey,
				"ruleIndex", i,
				"reason", "no target groups created for redirect-only rules")
			continue
		}

		// For rules with backends, normal cleanup would apply
		// This is handled by the existing TargetGroupBinding cleanup logic
		if len(rule.GetBackends()) > 0 {
			m.logger.Info("Rule has backends, cleanup will be handled by TargetGroupBinding controller", 
				"route", routeKey,
				"ruleIndex", i,
				"backendCount", len(rule.GetBackends()))
		}
	}

	return result
}

// ValidateCleanupSafety validates that cleanup operations are safe for redirect-only rules
func (m *RouteCleanupManager) ValidateCleanupSafety(ctx context.Context, rules []RouteRule) error {
	for i, rule := range rules {
		if IsRedirectOnlyRule(rule) {
			// Validate that redirect-only rules don't have any associated target groups
			if len(rule.GetBackends()) > 0 {
				return fmt.Errorf("redirect-only rule at index %d has backends, this should not happen", i)
			}

			// Additional safety checks can be added here
			m.logger.V(1).Info("Validated redirect-only rule safety", 
				"ruleIndex", i,
				"hasRedirectFilter", HasRequestRedirectFilter(rule))
		}
	}

	return nil
}

// HandleStateTransition handles transitions between redirect-only and backend configurations
func (m *RouteCleanupManager) HandleStateTransition(ctx context.Context, routeKey types.NamespacedName, oldRules, newRules []RouteRule) error {
	// Track state changes for each rule
	for i := 0; i < len(oldRules) && i < len(newRules); i++ {
		oldRule := oldRules[i]
		newRule := newRules[i]

		oldIsRedirectOnly := IsRedirectOnlyRule(oldRule)
		newIsRedirectOnly := IsRedirectOnlyRule(newRule)

		if oldIsRedirectOnly != newIsRedirectOnly {
			m.logger.Info("Detected rule state transition", 
				"route", routeKey,
				"ruleIndex", i,
				"oldIsRedirectOnly", oldIsRedirectOnly,
				"newIsRedirectOnly", newIsRedirectOnly)

			if oldIsRedirectOnly && !newIsRedirectOnly {
				// Transition from redirect-only to backend rule
				// Target groups will be created by the normal reconciliation process
				m.logger.Info("Rule transitioning from redirect-only to backend rule", 
					"route", routeKey,
					"ruleIndex", i,
					"newBackendCount", len(newRule.GetBackends()))
			} else if !oldIsRedirectOnly && newIsRedirectOnly {
				// Transition from backend rule to redirect-only
				// Target groups will be cleaned up by the TargetGroupBinding controller
				m.logger.Info("Rule transitioning from backend rule to redirect-only", 
					"route", routeKey,
					"ruleIndex", i,
					"oldBackendCount", len(oldRule.GetBackends()))
			}
		}
	}

	// Handle added or removed rules
	if len(newRules) > len(oldRules) {
		for i := len(oldRules); i < len(newRules); i++ {
			newRule := newRules[i]
			if IsRedirectOnlyRule(newRule) {
				m.logger.Info("New redirect-only rule added", 
					"route", routeKey,
					"ruleIndex", i)
			}
		}
	} else if len(oldRules) > len(newRules) {
		for i := len(newRules); i < len(oldRules); i++ {
			oldRule := oldRules[i]
			if !IsRedirectOnlyRule(oldRule) {
				m.logger.Info("Backend rule removed, cleanup will be handled by TargetGroupBinding controller", 
					"route", routeKey,
					"ruleIndex", i,
					"oldBackendCount", len(oldRule.GetBackends()))
			}
		}
	}

	return nil
}