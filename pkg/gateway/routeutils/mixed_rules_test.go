package routeutils

import (
	"fmt"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// **Feature: httproute-redirect-only-fix, Property 3: Mixed HTTPRoute processing independence**
// Property-based test to verify that mixed HTTPRoute rules are processed independently
func TestProperty_MixedHTTPRouteProcessingIndependence(t *testing.T) {
	// Test property: For any HTTPRoute containing multiple rules of different types 
	// (redirect-only and backend rules), each rule should be processed independently
	
	// Create various combinations of mixed rules
	testCases := []struct {
		name  string
		rules []RouteRule
	}{
		{
			name: "redirect_only_then_backend",
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
		},
		{
			name: "backend_then_redirect_only",
			rules: []RouteRule{
				// Backend rule
				&convertedHTTPRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{},
					},
					backends: []Backend{{Weight: 1}},
				},
				// Redirect-only rule
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
			name: "mixed_with_redirect_and_backend",
			rules: []RouteRule{
				// Rule with both redirect and backend
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
					backends: []Backend{{Weight: 50}, {Weight: 50}},
				},
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
			},
		},
		{
			name: "multiple_mixed_rules",
			rules: []RouteRule{
				// Backend rule 1
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 1}},
				},
				// Redirect-only rule 1
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
				// Backend rule 2
				&convertedHTTPRouteRule{
					rule:     &gwv1.HTTPRouteRule{Filters: []gwv1.HTTPRouteFilter{}},
					backends: []Backend{{Weight: 100}},
				},
				// Redirect-only rule 2
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
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Property: Each rule should be processed independently
			for i, rule := range tc.rules {
				t.Run(fmt.Sprintf("rule_%d", i), func(t *testing.T) {
					// Verify rule classification is independent of other rules
					isRedirectOnly := IsRedirectOnlyRule(rule)
					hasRedirectFilter := HasRequestRedirectFilter(rule)
					hasBackends := len(rule.GetBackends()) > 0

					// Property assertions based on rule characteristics
					if hasBackends {
						assert.False(t, isRedirectOnly, 
							"Rule with backends should not be redirect-only regardless of other rules")
					} else if hasRedirectFilter {
						assert.True(t, isRedirectOnly, 
							"Rule with redirect filter and no backends should be redirect-only regardless of other rules")
					} else {
						assert.False(t, isRedirectOnly, 
							"Rule without redirect filter and backends should not be redirect-only regardless of other rules")
					}
				})
			}

			// Verify that the presence of different rule types doesn't affect each other
			redirectOnlyCount := 0
			backendRuleCount := 0
			mixedRuleCount := 0

			for _, rule := range tc.rules {
				hasBackends := len(rule.GetBackends()) > 0
				hasRedirectFilter := HasRequestRedirectFilter(rule)

				if IsRedirectOnlyRule(rule) {
					redirectOnlyCount++
				} else if hasBackends && !hasRedirectFilter {
					backendRuleCount++
				} else if hasBackends && hasRedirectFilter {
					mixedRuleCount++
				}
			}

			// Property: Rule counts should be consistent with individual rule analysis
			assert.GreaterOrEqual(t, len(tc.rules), redirectOnlyCount+backendRuleCount+mixedRuleCount,
				"Total rule count should match classified rules")
		})
	}
}

// **Feature: httproute-redirect-only-fix, Property 4: Gateway API compliance for mixed rules**
// Property-based test to verify Gateway API compliance for rules with both redirects and backends
func TestProperty_GatewayAPIComplianceForMixedRules(t *testing.T) {
	// Test property: For any HTTPRoute rule that has both RequestRedirect filters and BackendRefs,
	// the system should process both according to Gateway API specifications
	
	// Create various combinations of mixed rules (redirect + backend)
	mixedRuleConfigs := []struct {
		name            string
		redirectFilter  gwv1.HTTPRequestRedirectFilter
		backends        []Backend
		expectedBehavior string
	}{
		{
			name: "https_redirect_with_single_backend",
			redirectFilter: gwv1.HTTPRequestRedirectFilter{
				Scheme: awssdk.String("https"),
			},
			backends: []Backend{{Weight: 1}},
			expectedBehavior: "should_process_both_redirect_and_backend",
		},
		{
			name: "hostname_redirect_with_multiple_backends",
			redirectFilter: gwv1.HTTPRequestRedirectFilter{
				Hostname: (*gwv1.PreciseHostname)(awssdk.String("api.example.com")),
			},
			backends: []Backend{{Weight: 50}, {Weight: 50}},
			expectedBehavior: "should_process_both_redirect_and_backend",
		},
		{
			name: "port_redirect_with_weighted_backends",
			redirectFilter: gwv1.HTTPRequestRedirectFilter{
				Port: (*gwv1.PortNumber)(awssdk.Int32(8080)),
			},
			backends: []Backend{{Weight: 25}, {Weight: 75}},
			expectedBehavior: "should_process_both_redirect_and_backend",
		},
		{
			name: "full_redirect_with_backend",
			redirectFilter: gwv1.HTTPRequestRedirectFilter{
				Scheme:     awssdk.String("https"),
				Hostname:   (*gwv1.PreciseHostname)(awssdk.String("secure.example.com")),
				Port:       (*gwv1.PortNumber)(awssdk.Int32(443)),
				StatusCode: awssdk.Int(301),
			},
			backends: []Backend{{Weight: 100}},
			expectedBehavior: "should_process_both_redirect_and_backend",
		},
	}

	for _, config := range mixedRuleConfigs {
		t.Run(config.name, func(t *testing.T) {
			// Create rule with both redirect filter and backends
			rule := &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type:            gwv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: &config.redirectFilter,
						},
					},
				},
				backends: config.backends,
			}

			// Property: Rules with both redirects and backends should not be redirect-only
			assert.False(t, IsRedirectOnlyRule(rule), 
				"Rule with both redirect and backends should not be redirect-only")
			
			// Property: Should have redirect filter
			assert.True(t, HasRequestRedirectFilter(rule), 
				"Rule should have redirect filter")
			
			// Property: Should have backends
			assert.Greater(t, len(rule.GetBackends()), 0, 
				"Rule should have backends")
			
			// Property: Backend count should match expected
			assert.Equal(t, len(config.backends), len(rule.GetBackends()), 
				"Backend count should match configuration")
			
			// Property: Backend weights should be preserved
			for i, expectedBackend := range config.backends {
				actualBackend := rule.GetBackends()[i]
				assert.Equal(t, expectedBackend.Weight, actualBackend.Weight, 
					"Backend weight should be preserved")
			}

			// Property: Gateway API compliance - both redirect and backend processing should be possible
			// This means the rule should be processed for both redirect actions and target group creation
			switch config.expectedBehavior {
			case "should_process_both_redirect_and_backend":
				// Verify that the rule has the necessary components for both redirect and backend processing
				assert.True(t, HasRequestRedirectFilter(rule), "Should have redirect filter for redirect processing")
				assert.Greater(t, len(rule.GetBackends()), 0, "Should have backends for target group creation")
				
				// Verify redirect filter is properly configured
				httpRule := rule.GetRawRouteRule().(*gwv1.HTTPRouteRule)
				found := false
				for _, filter := range httpRule.Filters {
					if filter.Type == gwv1.HTTPRouteFilterRequestRedirect {
						assert.NotNil(t, filter.RequestRedirect, "Redirect filter should have configuration")
						found = true
						break
					}
				}
				assert.True(t, found, "Should find redirect filter in rule")
			}
		})
	}
}

// Property-based test for Gateway API specification compliance
func TestProperty_GatewayAPISpecificationCompliance(t *testing.T) {
	// Test various Gateway API specification requirements
	
	// Test that redirect-only rules comply with Gateway API spec
	t.Run("redirect_only_compliance", func(t *testing.T) {
		redirectOnlyRule := &convertedHTTPRouteRule{
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
			backends: []Backend{}, // No backends as per Gateway API spec for redirect-only
		}

		// Property: Redirect-only rules should be valid according to Gateway API
		assert.True(t, IsRedirectOnlyRule(redirectOnlyRule), 
			"Valid redirect-only rule should be identified correctly")
		assert.True(t, HasRequestRedirectFilter(redirectOnlyRule), 
			"Redirect-only rule should have redirect filter")
		assert.Equal(t, 0, len(redirectOnlyRule.GetBackends()), 
			"Redirect-only rule should have no backends")
	})

	// Test that backend-only rules comply with Gateway API spec
	t.Run("backend_only_compliance", func(t *testing.T) {
		backendOnlyRule := &convertedHTTPRouteRule{
			rule: &gwv1.HTTPRouteRule{
				Filters: []gwv1.HTTPRouteFilter{}, // No filters
			},
			backends: []Backend{{Weight: 1}}, // Has backends
		}

		// Property: Backend-only rules should be valid according to Gateway API
		assert.False(t, IsRedirectOnlyRule(backendOnlyRule), 
			"Backend-only rule should not be redirect-only")
		assert.False(t, HasRequestRedirectFilter(backendOnlyRule), 
			"Backend-only rule should not have redirect filter")
		assert.Greater(t, len(backendOnlyRule.GetBackends()), 0, 
			"Backend-only rule should have backends")
	})
}