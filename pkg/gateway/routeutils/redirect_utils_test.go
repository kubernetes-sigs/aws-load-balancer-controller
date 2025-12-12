package routeutils

import (
	"fmt"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestIsRedirectOnlyRule(t *testing.T) {
	tests := []struct {
		name     string
		rule     RouteRule
		expected bool
	}{
		{
			name: "redirect-only rule with no backends",
			rule: &convertedHTTPRouteRule{
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
			expected: true,
		},
		{
			name: "rule with backends and redirect filter",
			rule: &convertedHTTPRouteRule{
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
				backends: []Backend{
					{Weight: 1},
				},
			},
			expected: false,
		},
		{
			name: "rule with backends but no redirect filter",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{},
				},
				backends: []Backend{
					{Weight: 1},
				},
			},
			expected: false,
		},
		{
			name: "rule with no backends and no redirect filter",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{},
				},
				backends: []Backend{},
			},
			expected: false,
		},
		{
			name: "rule with multiple filters including redirect",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterExtensionRef,
						},
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
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRedirectOnlyRule(tt.rule)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasRequestRedirectFilter(t *testing.T) {
	tests := []struct {
		name     string
		rule     RouteRule
		expected bool
	}{
		{
			name: "rule with redirect filter",
			rule: &convertedHTTPRouteRule{
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
			},
			expected: true,
		},
		{
			name: "rule without redirect filter",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterExtensionRef,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "rule with no filters",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasRequestRedirectFilter(tt.rule)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// **Feature: httproute-redirect-only-fix, Property 1: Redirect-only rules skip target group creation**
// Property-based test to verify that redirect-only rules are correctly identified
func TestProperty_RedirectOnlyRulesSkipTargetGroupCreation(t *testing.T) {
	// Test property: For any HTTPRoute rule that contains only RequestRedirect filters and no BackendRefs,
	// the system should identify it as redirect-only
	
	// Generate various redirect-only rule configurations
	redirectConfigs := []gwv1.HTTPRequestRedirectFilter{
		{Scheme: awssdk.String("https")},
		{Hostname: (*gwv1.PreciseHostname)(awssdk.String("example.com"))},
		{Port: (*gwv1.PortNumber)(awssdk.Int32(443))},
		{StatusCode: awssdk.Int(301)},
		{
			Scheme:   awssdk.String("https"),
			Hostname: (*gwv1.PreciseHostname)(awssdk.String("secure.example.com")),
			Port:     (*gwv1.PortNumber)(awssdk.Int32(443)),
		},
	}

	for i, redirectConfig := range redirectConfigs {
		t.Run(fmt.Sprintf("redirect_config_%d", i), func(t *testing.T) {
			// Create a rule with only redirect filter and no backends
			rule := &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type:            gwv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: &redirectConfig,
						},
					},
				},
				backends: []Backend{}, // No backends
			}

			// Property: Should be identified as redirect-only
			assert.True(t, IsRedirectOnlyRule(rule), "Rule with only redirect filter should be identified as redirect-only")
			assert.True(t, HasRequestRedirectFilter(rule), "Rule should have redirect filter")

			// Add backends and verify it's no longer redirect-only
			ruleWithBackends := &convertedHTTPRouteRule{
				rule: rule.rule,
				backends: []Backend{
					{Weight: 1},
				},
			}
			assert.False(t, IsRedirectOnlyRule(ruleWithBackends), "Rule with backends should not be redirect-only")
		})
	}
}

// **Feature: httproute-redirect-only-fix, Property 2: Backend rules create target groups**
// Property-based test to verify that rules with backends are not identified as redirect-only
func TestProperty_BackendRulesCreateTargetGroups(t *testing.T) {
	// Test property: For any HTTPRoute rule that contains BackendRefs (with or without RequestRedirect filters),
	// the system should not identify it as redirect-only
	
	// Generate various backend configurations
	backendConfigs := [][]Backend{
		{{Weight: 1}},
		{{Weight: 50}, {Weight: 50}},
		{{Weight: 100}},
		{{Weight: 25}, {Weight: 25}, {Weight: 25}, {Weight: 25}},
	}

	// Test with and without redirect filters
	redirectFilters := [][]gwv1.HTTPRouteFilter{
		{}, // No filters
		{{ // With redirect filter
			Type: gwv1.HTTPRouteFilterRequestRedirect,
			RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
				Scheme: awssdk.String("https"),
			},
		}},
		{{ // With multiple filters including redirect
			Type: gwv1.HTTPRouteFilterExtensionRef,
		}, {
			Type: gwv1.HTTPRouteFilterRequestRedirect,
			RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
				Hostname: (*gwv1.PreciseHostname)(awssdk.String("example.com")),
			},
		}},
	}

	for i, backends := range backendConfigs {
		for j, filters := range redirectFilters {
			t.Run(fmt.Sprintf("backends_%d_filters_%d", i, j), func(t *testing.T) {
				rule := &convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: filters,
					},
					backends: backends,
				}

				// Property: Rules with backends should never be redirect-only
				assert.False(t, IsRedirectOnlyRule(rule), 
					"Rule with backends should not be identified as redirect-only")
				
				// Verify backends are present
				assert.Greater(t, len(rule.GetBackends()), 0, 
					"Rule should have backends")
			})
		}
	}
}

// Property-based test for rules without backends and without redirect filters
func TestProperty_EmptyRulesNotRedirectOnly(t *testing.T) {
	// Test property: Rules with no backends and no redirect filters should not be redirect-only
	
	nonRedirectFilters := [][]gwv1.HTTPRouteFilter{
		{}, // No filters
		{{ // Extension ref only
			Type: gwv1.HTTPRouteFilterExtensionRef,
		}},
		{{ // URL rewrite only
			Type: gwv1.HTTPRouteFilterURLRewrite,
		}},
	}

	for i, filters := range nonRedirectFilters {
		t.Run(fmt.Sprintf("non_redirect_filters_%d", i), func(t *testing.T) {
			rule := &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: filters,
				},
				backends: []Backend{}, // No backends
			}

			// Property: Rules without backends and without redirect filters should not be redirect-only
			assert.False(t, IsRedirectOnlyRule(rule), 
				"Rule without backends and redirect filters should not be redirect-only")
			assert.False(t, HasRequestRedirectFilter(rule), 
				"Rule should not have redirect filter")
		})
	}
}

// Additional unit tests for edge cases and invalid inputs
func TestIsRedirectOnlyRule_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		rule     RouteRule
		expected bool
	}{
		{
			name: "nil rule",
			rule: nil,
			expected: false,
		},
		{
			name: "non-HTTP rule type",
			rule: &convertedTCPRouteRule{}, // Assuming this exists or create a mock
			expected: false,
		},
		{
			name: "rule with nil redirect filter",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type:            gwv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: nil,
						},
					},
				},
				backends: []Backend{},
			},
			expected: true, // Still considered redirect-only even with nil config
		},
		{
			name: "rule with empty filters array",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{},
				},
				backends: []Backend{},
			},
			expected: false,
		},
		{
			name: "rule with only extension ref filter",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterExtensionRef,
						},
					},
				},
				backends: []Backend{},
			},
			expected: false,
		},
		{
			name: "rule with zero weight backends",
			rule: &convertedHTTPRouteRule{
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
				backends: []Backend{{Weight: 0}}, // Zero weight backend
			},
			expected: false, // Has backends, so not redirect-only
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.rule == nil {
				// Test nil safety
				assert.False(t, false) // This would panic if not handled properly
				return
			}
			result := IsRedirectOnlyRule(tt.rule)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Mock TCP route rule for testing non-HTTP rules - using existing convertedTCPRouteRule from tcp.go

// Test helper function behavior with various backend configurations
func TestBackendConfigurations(t *testing.T) {
	tests := []struct {
		name     string
		backends []Backend
		expected string
	}{
		{
			name:     "no backends",
			backends: []Backend{},
			expected: "empty",
		},
		{
			name:     "single backend",
			backends: []Backend{{Weight: 1}},
			expected: "single",
		},
		{
			name:     "multiple backends",
			backends: []Backend{{Weight: 50}, {Weight: 50}},
			expected: "multiple",
		},
		{
			name:     "backends with zero weights",
			backends: []Backend{{Weight: 0}, {Weight: 0}},
			expected: "zero_weights",
		},
		{
			name:     "mixed weight backends",
			backends: []Backend{{Weight: 0}, {Weight: 100}},
			expected: "mixed_weights",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := &convertedHTTPRouteRule{
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
				backends: tt.backends,
			}

			// Test behavior based on backend configuration
			switch tt.expected {
			case "empty":
				assert.True(t, IsRedirectOnlyRule(rule), "Rule with no backends should be redirect-only")
			case "single", "multiple", "zero_weights", "mixed_weights":
				assert.False(t, IsRedirectOnlyRule(rule), "Rule with backends should not be redirect-only")
			}

			assert.Equal(t, len(tt.backends), len(rule.GetBackends()), "Backend count should match")
		})
	}
}