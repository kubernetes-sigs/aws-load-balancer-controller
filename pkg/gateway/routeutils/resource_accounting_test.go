package routeutils

import (
	"context"
	"fmt"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestResourceAccountingManager_CalculateResourceUsage(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))
	manager := NewResourceAccountingManager(logger)
	ctx := context.Background()
	routeKey := types.NamespacedName{Namespace: "test", Name: "test-route"}

	tests := []struct {
		name     string
		rules    []RouteRule
		expected ResourceUsage
	}{
		{
			name: "mixed rules scenario",
			rules: []RouteRule{
				// Redirect-only rule
				&convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Scheme: awssdk.String("https"),
								},
							},
						},
					},
					backends: []Backend{},
				},
				// Backend rule
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 1}},
				},
				// Mixed rule
				&convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Hostname: (*gwv1.PreciseHostname)(awssdk.String("example.com")),
								},
							},
						},
					},
					backends: []Backend{{Weight: 50}, {Weight: 50}},
				},
			},
			expected: ResourceUsage{
				TotalRules:           3,
				RedirectOnlyRules:    1,
				BackendRules:         1,
				MixedRules:           1,
				TargetGroupsRequired: 3, // 1 from backend rule + 2 from mixed rule
				TargetGroupsSkipped:  1, // 1 from redirect-only rule
			},
		},
		{
			name: "all redirect-only rules",
			rules: []RouteRule{
				&convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Scheme: awssdk.String("https"),
								},
							},
						},
					},
					backends: []Backend{},
				},
				&convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Port: (*gwv1.PortNumber)(awssdk.Int32(443)),
								},
							},
						},
					},
					backends: []Backend{},
				},
			},
			expected: ResourceUsage{
				TotalRules:           2,
				RedirectOnlyRules:    2,
				BackendRules:         0,
				MixedRules:           0,
				TargetGroupsRequired: 0,
				TargetGroupsSkipped:  2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := manager.CalculateResourceUsage(ctx, routeKey, tt.rules)
			assert.Equal(t, tt.expected, usage)
		})
	}
}

// **Feature: httproute-redirect-only-fix, Property 8: Resource accounting accuracy**
// Property-based test to verify resource accounting accuracy for redirect-only rules
func TestProperty_ResourceAccountingAccuracy(t *testing.T) {
	// Test property: For any redirect-only HTTPRoute rule, the system should not 
	// count it toward target group limits or quotas
	
	logger := zap.New(zap.UseDevMode(true))
	manager := NewResourceAccountingManager(logger)
	ctx := context.Background()

	// Generate various resource accounting scenarios
	accountingScenarios := []struct {
		name  string
		rules []RouteRule
	}{
		{
			name: "single_redirect_only",
			rules: []RouteRule{
				&convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Scheme: awssdk.String("https"),
								},
							},
						},
					},
					backends: []Backend{},
				},
			},
		},
		{
			name: "multiple_redirect_only",
			rules: []RouteRule{
				&convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Scheme: awssdk.String("https"),
								},
							},
						},
					},
					backends: []Backend{},
				},
				&convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Hostname: (*gwv1.PreciseHostname)(awssdk.String("example.com")),
								},
							},
						},
					},
					backends: []Backend{},
				},
				&convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Port: (*gwv1.PortNumber)(awssdk.Int32(443)),
								},
							},
						},
					},
					backends: []Backend{},
				},
			},
		},
		{
			name: "mixed_with_many_redirects",
			rules: []RouteRule{
				// Multiple redirect-only rules
				&convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Scheme: awssdk.String("https"),
								},
							},
						},
					},
					backends: []Backend{},
				},
				&convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Hostname: (*gwv1.PreciseHostname)(awssdk.String("redirect1.example.com")),
								},
							},
						},
					},
					backends: []Backend{},
				},
				&convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Hostname: (*gwv1.PreciseHostname)(awssdk.String("redirect2.example.com")),
								},
							},
						},
					},
					backends: []Backend{},
				},
				// One backend rule
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 1}},
				},
			},
		},
	}

	for _, scenario := range accountingScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			routeKey := types.NamespacedName{
				Namespace: "test",
				Name:      fmt.Sprintf("accounting-%s", scenario.name),
			}

			// Property: Resource usage calculation should be accurate
			usage := manager.CalculateResourceUsage(ctx, routeKey, scenario.rules)

			// Property: Total rules should match input
			assert.Equal(t, len(scenario.rules), usage.TotalRules,
				"Total rules should match input rule count")

			// Property: Redirect-only rules should not count toward target groups
			redirectOnlyCount := 0
			expectedTargetGroups := 0
			for _, rule := range scenario.rules {
				if IsRedirectOnlyRule(rule) {
					redirectOnlyCount++
				} else {
					expectedTargetGroups += len(rule.GetBackends())
				}
			}

			assert.Equal(t, redirectOnlyCount, usage.RedirectOnlyRules,
				"Redirect-only rule count should be accurate")
			assert.Equal(t, redirectOnlyCount, usage.TargetGroupsSkipped,
				"Target groups skipped should equal redirect-only rules")
			assert.Equal(t, expectedTargetGroups, usage.TargetGroupsRequired,
				"Target groups required should exclude redirect-only rules")

			// Property: Effective target group count should exclude redirect-only rules
			effectiveCount := manager.GetEffectiveTargetGroupCount(ctx, scenario.rules)
			assert.Equal(t, expectedTargetGroups, effectiveCount,
				"Effective target group count should exclude redirect-only rules")

			// Property: Rule categorization should be complete and non-overlapping
			totalCategorized := usage.RedirectOnlyRules + usage.BackendRules + usage.MixedRules
			assert.Equal(t, usage.TotalRules, totalCategorized,
				"All rules should be categorized exactly once")
		})
	}
}

func TestValidateResourceLimits(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))
	manager := NewResourceAccountingManager(logger)
	ctx := context.Background()

	limits := ResourceLimits{
		MaxTargetGroups:  5,
		MaxRulesPerRoute: 10,
	}

	tests := []struct {
		name        string
		usage       ResourceUsage
		expectError bool
		errorMsg    string
	}{
		{
			name: "within limits",
			usage: ResourceUsage{
				TotalRules:           3,
				TargetGroupsRequired: 2,
			},
			expectError: false,
		},
		{
			name: "exceeds target group limit",
			usage: ResourceUsage{
				TotalRules:           3,
				TargetGroupsRequired: 6,
			},
			expectError: true,
			errorMsg:    "target group limit exceeded",
		},
		{
			name: "exceeds rule limit",
			usage: ResourceUsage{
				TotalRules:           15,
				TargetGroupsRequired: 2,
			},
			expectError: true,
			errorMsg:    "rule limit exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateResourceLimits(ctx, tt.usage, limits)
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOptimizeResourceUsage(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))
	manager := NewResourceAccountingManager(logger)
	ctx := context.Background()

	tests := []struct {
		name               string
		usage              ResourceUsage
		expectedSuggestions int
		containsSuggestion  string
	}{
		{
			name: "no redirect rules suggests using them",
			usage: ResourceUsage{
				BackendRules:      5,
				RedirectOnlyRules: 0,
			},
			expectedSuggestions: 1,
			containsSuggestion:  "redirect-only rules",
		},
		{
			name: "many target groups suggests consolidation",
			usage: ResourceUsage{
				BackendRules:         2,
				MixedRules:           1,
				TargetGroupsRequired: 10,
			},
			expectedSuggestions: 1,
			containsSuggestion:  "consolidating",
		},
		{
			name: "mixed rules provides guidance",
			usage: ResourceUsage{
				MixedRules: 3,
			},
			expectedSuggestions: 1,
			containsSuggestion:  "Mixed rules",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestions := manager.OptimizeResourceUsage(ctx, tt.usage)
			
			assert.GreaterOrEqual(t, len(suggestions), tt.expectedSuggestions)
			
			if tt.containsSuggestion != "" {
				found := false
				for _, suggestion := range suggestions {
					if strings.Contains(suggestion, tt.containsSuggestion) {
						found = true
						break
					}
				}
				assert.True(t, found, "Should contain expected suggestion")
			}
		})
	}
}

func TestGenerateResourceReport(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))
	manager := NewResourceAccountingManager(logger)
	ctx := context.Background()

	routeKey := types.NamespacedName{Namespace: "test", Name: "test-route"}
	usage := ResourceUsage{
		TotalRules:           5,
		RedirectOnlyRules:    2,
		BackendRules:         2,
		MixedRules:           1,
		TargetGroupsRequired: 4,
		TargetGroupsSkipped:  2,
	}
	limits := DefaultResourceLimits()

	report := manager.GenerateResourceReport(ctx, routeKey, usage, limits)

	// Verify report contains expected information
	assert.Contains(t, report, "Resource Usage Report")
	assert.Contains(t, report, routeKey.String())
	assert.Contains(t, report, "Total Rules: 5")
	assert.Contains(t, report, "Redirect-only Rules: 2")
	assert.Contains(t, report, "Backend Rules: 2")
	assert.Contains(t, report, "Mixed Rules: 1")
	assert.Contains(t, report, "Required: 4")
	assert.Contains(t, report, "Skipped (redirect-only): 2")
	assert.Contains(t, report, "Efficiency:")
}

// Property-based test for resource accounting consistency
func TestProperty_ResourceAccountingConsistency(t *testing.T) {
	// Test property: Resource accounting should be consistent across different rule configurations
	
	logger := zap.New(zap.UseDevMode(true))
	manager := NewResourceAccountingManager(logger)
	ctx := context.Background()

	// Generate various rule configurations for consistency testing
	consistencyTests := []struct {
		name  string
		rules []RouteRule
	}{
		{
			name: "all_redirect_only",
			rules: []RouteRule{
				&convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Scheme: awssdk.String("https"),
								},
							},
						},
					},
					backends: []Backend{},
				},
			},
		},
		{
			name: "all_backend_only",
			rules: []RouteRule{
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 1}},
				},
			},
		},
		{
			name: "all_mixed",
			rules: []RouteRule{
				&convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Scheme: awssdk.String("https"),
								},
							},
						},
					},
					backends: []Backend{{Weight: 100}},
				},
			},
		},
	}

	for _, test := range consistencyTests {
		t.Run(test.name, func(t *testing.T) {
			routeKey := types.NamespacedName{
				Namespace: "test",
				Name:      fmt.Sprintf("consistency-%s", test.name),
			}

			usage := manager.CalculateResourceUsage(ctx, routeKey, test.rules)

			// Property: Consistency checks
			assert.Equal(t, len(test.rules), usage.TotalRules,
				"Total rules should match input")
			
			// Property: Rule categorization should be mutually exclusive and complete
			totalCategorized := usage.RedirectOnlyRules + usage.BackendRules + usage.MixedRules
			assert.Equal(t, usage.TotalRules, totalCategorized,
				"All rules should be categorized exactly once")

			// Property: Target group accounting should be consistent
			expectedSkipped := usage.RedirectOnlyRules
			assert.Equal(t, expectedSkipped, usage.TargetGroupsSkipped,
				"Target groups skipped should equal redirect-only rules")

			// Property: Effective count should match required count
			effectiveCount := manager.GetEffectiveTargetGroupCount(ctx, test.rules)
			assert.Equal(t, usage.TargetGroupsRequired, effectiveCount,
				"Effective target group count should match required count")
		})
	}
}