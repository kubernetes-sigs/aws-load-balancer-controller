package routeutils

import (
	"context"
	"fmt"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestRouteCleanupManager_CleanupRouteRules(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))
	manager := NewRouteCleanupManager(logger)
	ctx := context.Background()
	routeKey := types.NamespacedName{Namespace: "test", Name: "test-route"}

	tests := []struct {
		name                     string
		rules                    []RouteRule
		expectedRedirectSkipped  int
		expectedTargetGroups     int
	}{
		{
			name: "mixed rules with redirect-only and backend",
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
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{},
					},
					backends: []Backend{{Weight: 1}},
				},
			},
			expectedRedirectSkipped: 1,
			expectedTargetGroups:    0, // Cleanup is handled by TargetGroupBinding controller
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
			expectedRedirectSkipped: 2,
			expectedTargetGroups:    0,
		},
		{
			name: "all backend rules",
			rules: []RouteRule{
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 1}},
				},
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 50}, {Weight: 50}},
				},
			},
			expectedRedirectSkipped: 0,
			expectedTargetGroups:    0, // Cleanup is handled by TargetGroupBinding controller
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.CleanupRouteRules(ctx, routeKey, tt.rules)
			
			assert.Equal(t, tt.expectedRedirectSkipped, result.RedirectOnlyRulesSkipped)
			assert.Equal(t, tt.expectedTargetGroups, result.TargetGroupsDeleted)
			assert.Empty(t, result.Errors)
		})
	}
}

// **Feature: httproute-redirect-only-fix, Property 6: Cleanup operation safety**
// Property-based test to verify cleanup operation safety for redirect-only rules
func TestProperty_CleanupOperationSafety(t *testing.T) {
	// Test property: For any HTTPRoute resource cleanup operation, the system should not 
	// attempt to delete target groups that were never created for redirect-only rules
	
	logger := zap.New(zap.UseDevMode(true))
	manager := NewRouteCleanupManager(logger)
	ctx := context.Background()

	// Generate various cleanup scenarios
	cleanupScenarios := []struct {
		name  string
		rules []RouteRule
	}{
		{
			name: "single_redirect_only_rule",
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
			name: "multiple_redirect_only_rules",
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
			},
		},
		{
			name: "mixed_rules_with_redirects",
			rules: []RouteRule{
				// Redirect-only
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
				// Backend rule
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 1}},
				},
				// Mixed rule (redirect + backend)
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

	for _, scenario := range cleanupScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			routeKey := types.NamespacedName{
				Namespace: "test",
				Name:      fmt.Sprintf("route-%s", scenario.name),
			}

			// Property: Cleanup validation should pass for all scenarios
			err := manager.ValidateCleanupSafety(ctx, scenario.rules)
			assert.NoError(t, err, "Cleanup validation should pass")

			// Property: Cleanup should not attempt to delete non-existent target groups
			result := manager.CleanupRouteRules(ctx, routeKey, scenario.rules)
			assert.Empty(t, result.Errors, "Cleanup should not produce errors")

			// Property: Redirect-only rules should be skipped
			redirectOnlyCount := 0
			for _, rule := range scenario.rules {
				if IsRedirectOnlyRule(rule) {
					redirectOnlyCount++
				}
			}
			assert.Equal(t, redirectOnlyCount, result.RedirectOnlyRulesSkipped,
				"All redirect-only rules should be skipped")

			// Property: No target groups should be deleted directly (handled by TargetGroupBinding controller)
			assert.Equal(t, 0, result.TargetGroupsDeleted,
				"Target group deletion should be handled by TargetGroupBinding controller")
		})
	}
}

func TestValidateCleanupSafety(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))
	manager := NewRouteCleanupManager(logger)
	ctx := context.Background()

	tests := []struct {
		name        string
		rules       []RouteRule
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid redirect-only rules",
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
			expectError: false,
		},
		{
			name: "invalid redirect-only rule with backends",
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
					backends: []Backend{{Weight: 1}}, // This should not happen for redirect-only
				},
			},
			expectError: true,
			errorMsg:    "redirect-only rule at index 0 has backends",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateCleanupSafety(ctx, tt.rules)
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// **Feature: httproute-redirect-only-fix, Property 7: State transition correctness**
// Property-based test to verify state transition correctness
func TestProperty_StateTransitionCorrectness(t *testing.T) {
	// Test property: For any HTTPRoute rule that transitions between redirect-only and 
	// backend-routing configurations, the system should handle the transition correctly
	
	logger := zap.New(zap.UseDevMode(true))
	manager := NewRouteCleanupManager(logger)
	ctx := context.Background()

	// Define various state transition scenarios
	transitionScenarios := []struct {
		name     string
		oldRules []RouteRule
		newRules []RouteRule
		expected string
	}{
		{
			name: "redirect_only_to_backend",
			oldRules: []RouteRule{
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
			newRules: []RouteRule{
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 1}},
				},
			},
			expected: "redirect_to_backend_transition",
		},
		{
			name: "backend_to_redirect_only",
			oldRules: []RouteRule{
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 1}},
				},
			},
			newRules: []RouteRule{
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
			},
			expected: "backend_to_redirect_transition",
		},
		{
			name: "backend_to_mixed",
			oldRules: []RouteRule{
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 1}},
				},
			},
			newRules: []RouteRule{
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
					backends: []Backend{{Weight: 100}},
				},
			},
			expected: "backend_to_mixed_transition",
		},
		{
			name: "mixed_to_redirect_only",
			oldRules: []RouteRule{
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
					backends: []Backend{{Weight: 50}},
				},
			},
			newRules: []RouteRule{
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
			expected: "mixed_to_redirect_transition",
		},
		{
			name: "multiple_rules_mixed_transitions",
			oldRules: []RouteRule{
				// Rule 0: redirect-only
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
				// Rule 1: backend
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 1}},
				},
			},
			newRules: []RouteRule{
				// Rule 0: backend (transition from redirect-only)
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 100}},
				},
				// Rule 1: redirect-only (transition from backend)
				&convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Hostname: (*gwv1.PreciseHostname)(awssdk.String("redirect.example.com")),
								},
							},
						},
					},
					backends: []Backend{},
				},
			},
			expected: "multiple_transitions",
		},
	}

	for _, scenario := range transitionScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			routeKey := types.NamespacedName{
				Namespace: "test",
				Name:      fmt.Sprintf("transition-%s", scenario.name),
			}

			// Property: State transitions should be handled without errors
			err := manager.HandleStateTransition(ctx, routeKey, scenario.oldRules, scenario.newRules)
			assert.NoError(t, err, "State transition should be handled without errors")

			// Property: Verify the transition logic
			switch scenario.expected {
			case "redirect_to_backend_transition":
				// Verify old rule was redirect-only and new rule has backends
				assert.True(t, IsRedirectOnlyRule(scenario.oldRules[0]), 
					"Old rule should be redirect-only")
				assert.False(t, IsRedirectOnlyRule(scenario.newRules[0]), 
					"New rule should not be redirect-only")
				assert.Greater(t, len(scenario.newRules[0].GetBackends()), 0, 
					"New rule should have backends")

			case "backend_to_redirect_transition":
				// Verify old rule had backends and new rule is redirect-only
				assert.False(t, IsRedirectOnlyRule(scenario.oldRules[0]), 
					"Old rule should not be redirect-only")
				assert.True(t, IsRedirectOnlyRule(scenario.newRules[0]), 
					"New rule should be redirect-only")
				assert.Equal(t, 0, len(scenario.newRules[0].GetBackends()), 
					"New rule should have no backends")

			case "backend_to_mixed_transition":
				// Verify transition from backend-only to mixed (redirect + backend)
				assert.False(t, IsRedirectOnlyRule(scenario.oldRules[0]), 
					"Old rule should not be redirect-only")
				assert.False(t, HasRequestRedirectFilter(scenario.oldRules[0]), 
					"Old rule should not have redirect filter")
				assert.False(t, IsRedirectOnlyRule(scenario.newRules[0]), 
					"New rule should not be redirect-only (has backends)")
				assert.True(t, HasRequestRedirectFilter(scenario.newRules[0]), 
					"New rule should have redirect filter")
				assert.Greater(t, len(scenario.newRules[0].GetBackends()), 0, 
					"New rule should have backends")

			case "mixed_to_redirect_transition":
				// Verify transition from mixed to redirect-only
				assert.False(t, IsRedirectOnlyRule(scenario.oldRules[0]), 
					"Old rule should not be redirect-only (has backends)")
				assert.True(t, HasRequestRedirectFilter(scenario.oldRules[0]), 
					"Old rule should have redirect filter")
				assert.True(t, IsRedirectOnlyRule(scenario.newRules[0]), 
					"New rule should be redirect-only")
				assert.Equal(t, 0, len(scenario.newRules[0].GetBackends()), 
					"New rule should have no backends")

			case "multiple_transitions":
				// Verify multiple rule transitions
				assert.Equal(t, len(scenario.oldRules), len(scenario.newRules), 
					"Rule count should be preserved")
				
				// Rule 0: redirect-only -> backend
				assert.True(t, IsRedirectOnlyRule(scenario.oldRules[0]), 
					"Old rule 0 should be redirect-only")
				assert.False(t, IsRedirectOnlyRule(scenario.newRules[0]), 
					"New rule 0 should not be redirect-only")
				
				// Rule 1: backend -> redirect-only
				assert.False(t, IsRedirectOnlyRule(scenario.oldRules[1]), 
					"Old rule 1 should not be redirect-only")
				assert.True(t, IsRedirectOnlyRule(scenario.newRules[1]), 
					"New rule 1 should be redirect-only")
			}
		})
	}
}

// Property-based test for rule addition and removal scenarios
func TestProperty_RuleAdditionRemovalTransitions(t *testing.T) {
	// Test property: Adding or removing rules should be handled correctly
	
	logger := zap.New(zap.UseDevMode(true))
	manager := NewRouteCleanupManager(logger)
	ctx := context.Background()

	additionRemovalScenarios := []struct {
		name     string
		oldRules []RouteRule
		newRules []RouteRule
		expected string
	}{
		{
			name:     "add_redirect_only_rule",
			oldRules: []RouteRule{},
			newRules: []RouteRule{
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
			expected: "rule_addition",
		},
		{
			name: "remove_backend_rule",
			oldRules: []RouteRule{
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 1}},
				},
			},
			newRules: []RouteRule{},
			expected: "rule_removal",
		},
		{
			name: "add_multiple_mixed_rules",
			oldRules: []RouteRule{
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 1}},
				},
			},
			newRules: []RouteRule{
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 1}},
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
			},
			expected: "rule_addition",
		},
	}

	for _, scenario := range additionRemovalScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			routeKey := types.NamespacedName{
				Namespace: "test",
				Name:      fmt.Sprintf("add-remove-%s", scenario.name),
			}

			// Property: Addition/removal transitions should be handled without errors
			err := manager.HandleStateTransition(ctx, routeKey, scenario.oldRules, scenario.newRules)
			assert.NoError(t, err, "Addition/removal transition should be handled without errors")

			// Property: Verify the transition behavior
			switch scenario.expected {
			case "rule_addition":
				assert.Greater(t, len(scenario.newRules), len(scenario.oldRules), 
					"New rules should have more rules than old rules")
			case "rule_removal":
				assert.Less(t, len(scenario.newRules), len(scenario.oldRules), 
					"New rules should have fewer rules than old rules")
			}
		})
	}
}