package routeutils

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
)

// ResourceAccountingManager handles resource accounting for HTTPRoute rules
type ResourceAccountingManager struct {
	logger logr.Logger
}

// ResourceUsage represents resource usage statistics
type ResourceUsage struct {
	TotalRules           int
	RedirectOnlyRules    int
	BackendRules         int
	MixedRules           int
	TargetGroupsRequired int
	TargetGroupsSkipped  int
}

// NewResourceAccountingManager creates a new ResourceAccountingManager
func NewResourceAccountingManager(logger logr.Logger) *ResourceAccountingManager {
	return &ResourceAccountingManager{
		logger: logger,
	}
}

// CalculateResourceUsage calculates resource usage for a set of route rules
func (m *ResourceAccountingManager) CalculateResourceUsage(ctx context.Context, routeKey types.NamespacedName, rules []RouteRule) ResourceUsage {
	usage := ResourceUsage{
		TotalRules: len(rules),
	}

	for i, rule := range rules {
		hasBackends := len(rule.GetBackends()) > 0
		hasRedirectFilter := HasRequestRedirectFilter(rule)
		isRedirectOnly := IsRedirectOnlyRule(rule)

		if isRedirectOnly {
			usage.RedirectOnlyRules++
			usage.TargetGroupsSkipped++
			m.logger.V(1).Info("Redirect-only rule does not require target groups", 
				"route", routeKey,
				"ruleIndex", i)
		} else if hasBackends && !hasRedirectFilter {
			usage.BackendRules++
			usage.TargetGroupsRequired += len(rule.GetBackends())
			m.logger.V(1).Info("Backend rule requires target groups", 
				"route", routeKey,
				"ruleIndex", i,
				"targetGroupsRequired", len(rule.GetBackends()))
		} else if hasBackends && hasRedirectFilter {
			usage.MixedRules++
			usage.TargetGroupsRequired += len(rule.GetBackends())
			m.logger.V(1).Info("Mixed rule requires target groups for backends", 
				"route", routeKey,
				"ruleIndex", i,
				"targetGroupsRequired", len(rule.GetBackends()))
		}
	}

	m.logger.Info("Calculated resource usage", 
		"route", routeKey,
		"totalRules", usage.TotalRules,
		"redirectOnlyRules", usage.RedirectOnlyRules,
		"backendRules", usage.BackendRules,
		"mixedRules", usage.MixedRules,
		"targetGroupsRequired", usage.TargetGroupsRequired,
		"targetGroupsSkipped", usage.TargetGroupsSkipped)

	return usage
}

// ValidateResourceLimits validates that resource usage is within acceptable limits
func (m *ResourceAccountingManager) ValidateResourceLimits(ctx context.Context, usage ResourceUsage, limits ResourceLimits) error {
	if usage.TargetGroupsRequired > limits.MaxTargetGroups {
		return fmt.Errorf("target group limit exceeded: required %d, limit %d", 
			usage.TargetGroupsRequired, limits.MaxTargetGroups)
	}

	if usage.TotalRules > limits.MaxRulesPerRoute {
		return fmt.Errorf("rule limit exceeded: total %d, limit %d", 
			usage.TotalRules, limits.MaxRulesPerRoute)
	}

	return nil
}

// ResourceLimits defines resource limits for validation
type ResourceLimits struct {
	MaxTargetGroups   int
	MaxRulesPerRoute  int
}

// DefaultResourceLimits returns default resource limits
func DefaultResourceLimits() ResourceLimits {
	return ResourceLimits{
		MaxTargetGroups:  100, // AWS ALB limit per load balancer
		MaxRulesPerRoute: 100, // Reasonable limit per HTTPRoute
	}
}

// GetEffectiveTargetGroupCount returns the number of target groups that will actually be created
// This excludes redirect-only rules which don't create target groups
func (m *ResourceAccountingManager) GetEffectiveTargetGroupCount(ctx context.Context, rules []RouteRule) int {
	count := 0
	for _, rule := range rules {
		if !IsRedirectOnlyRule(rule) {
			count += len(rule.GetBackends())
		}
	}
	return count
}

// OptimizeResourceUsage provides suggestions for optimizing resource usage
func (m *ResourceAccountingManager) OptimizeResourceUsage(ctx context.Context, usage ResourceUsage) []string {
	var suggestions []string

	// Suggest using redirect-only rules where appropriate
	if usage.BackendRules > 0 && usage.RedirectOnlyRules == 0 {
		suggestions = append(suggestions, 
			"Consider using redirect-only rules for simple redirects to reduce target group usage")
	}

	// Suggest consolidating backends if there are many single-backend rules
	// This would need access to the actual rules to calculate, but we can provide general guidance
	if usage.TargetGroupsRequired > usage.BackendRules+usage.MixedRules {
		suggestions = append(suggestions, 
			"Consider consolidating multiple backends into fewer rules to optimize target group usage")
	}

	// Suggest using mixed rules efficiently
	if usage.MixedRules > 0 {
		suggestions = append(suggestions, 
			"Mixed rules (redirect + backend) are efficient but ensure redirect logic is necessary")
	}

	return suggestions
}

// GenerateResourceReport generates a detailed resource usage report
func (m *ResourceAccountingManager) GenerateResourceReport(ctx context.Context, routeKey types.NamespacedName, usage ResourceUsage, limits ResourceLimits) string {
	report := fmt.Sprintf("Resource Usage Report for Route %s\n", routeKey)
	report += fmt.Sprintf("=====================================\n")
	report += fmt.Sprintf("Total Rules: %d\n", usage.TotalRules)
	report += fmt.Sprintf("  - Redirect-only Rules: %d\n", usage.RedirectOnlyRules)
	report += fmt.Sprintf("  - Backend Rules: %d\n", usage.BackendRules)
	report += fmt.Sprintf("  - Mixed Rules: %d\n", usage.MixedRules)
	report += fmt.Sprintf("\n")
	report += fmt.Sprintf("Target Group Usage:\n")
	report += fmt.Sprintf("  - Required: %d\n", usage.TargetGroupsRequired)
	report += fmt.Sprintf("  - Skipped (redirect-only): %d\n", usage.TargetGroupsSkipped)
	report += fmt.Sprintf("  - Efficiency: %.1f%% (skipped/total)\n", 
		float64(usage.TargetGroupsSkipped)/float64(usage.TotalRules)*100)
	report += fmt.Sprintf("\n")
	report += fmt.Sprintf("Resource Limits:\n")
	report += fmt.Sprintf("  - Target Groups: %d/%d (%.1f%%)\n", 
		usage.TargetGroupsRequired, limits.MaxTargetGroups,
		float64(usage.TargetGroupsRequired)/float64(limits.MaxTargetGroups)*100)
	report += fmt.Sprintf("  - Rules: %d/%d (%.1f%%)\n", 
		usage.TotalRules, limits.MaxRulesPerRoute,
		float64(usage.TotalRules)/float64(limits.MaxRulesPerRoute)*100)

	return report
}