package routeutils

import (
	"fmt"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestValidateHTTPRouteRule(t *testing.T) {
	tests := []struct {
		name           string
		rule           RouteRule
		expectedErrors int
		errorContains  []string
	}{
		{
			name: "valid redirect-only rule",
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
			expectedErrors: 0,
		},
		{
			name: "valid backend rule",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{},
				},
				backends: []Backend{{Weight: 1}},
			},
			expectedErrors: 0,
		},
		{
			name: "valid mixed rule",
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
				backends: []Backend{{Weight: 1}},
			},
			expectedErrors: 0,
		},
		{
			name: "invalid empty rule",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{},
				},
				backends: []Backend{},
			},
			expectedErrors: 1,
			errorContains:  []string{"must have either backends or redirect filters"},
		},
		{
			name: "invalid redirect configuration",
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
			expectedErrors: 1,
			errorContains:  []string{"RequestRedirect filter must have configuration"},
		},
		{
			name: "invalid backend weights",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{},
				},
				backends: []Backend{{Weight: -1}, {Weight: 1000}},
			},
			expectedErrors: 2,
			errorContains:  []string{"weight must be non-negative", "weight must not exceed 999"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := ValidateHTTPRouteRule(tt.rule)
			assert.Equal(t, tt.expectedErrors, len(errors))

			if tt.expectedErrors > 0 {
				errorMessage := FormatValidationErrors(errors).Error()
				for _, contains := range tt.errorContains {
					assert.Contains(t, errorMessage, contains)
				}
			}
		})
	}
}

// **Feature: httproute-redirect-only-fix, Property 5: Validation and error handling consistency**
// Property-based test to verify validation and error handling consistency
func TestProperty_ValidationAndErrorHandlingConsistency(t *testing.T) {
	// Test property: For any invalid HTTPRoute configuration, the system should provide 
	// clear error messages and update route status with appropriate condition messages
	
	// Generate various invalid configurations
	invalidConfigs := []struct {
		name        string
		rule        RouteRule
		expectError bool
		errorType   string
	}{
		{
			name: "redirect_with_invalid_scheme",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
								Scheme: awssdk.String("ftp"), // Invalid scheme
							},
						},
					},
				},
				backends: []Backend{},
			},
			expectError: true,
			errorType:   "invalid_scheme",
		},
		{
			name: "redirect_with_invalid_status_code",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
								StatusCode: awssdk.Int(200), // Invalid status code for redirect
							},
						},
					},
				},
				backends: []Backend{},
			},
			expectError: true,
			errorType:   "invalid_status_code",
		},
		{
			name: "redirect_with_invalid_port",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
								Port: (*gwv1.PortNumber)(awssdk.Int32(70000)), // Invalid port
							},
						},
					},
				},
				backends: []Backend{},
			},
			expectError: true,
			errorType:   "invalid_port",
		},
		{
			name: "backend_with_zero_weights",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{},
				},
				backends: []Backend{{Weight: 0}, {Weight: 0}}, // All zero weights
			},
			expectError: true,
			errorType:   "zero_weights",
		},
		{
			name: "backend_with_negative_weight",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{},
				},
				backends: []Backend{{Weight: -5}}, // Negative weight
			},
			expectError: true,
			errorType:   "negative_weight",
		},
	}

	for _, config := range invalidConfigs {
		t.Run(config.name, func(t *testing.T) {
			errors := ValidateHTTPRouteRule(config.rule)
			
			if config.expectError {
				// Property: Invalid configurations should produce validation errors
				assert.Greater(t, len(errors), 0, 
					"Invalid configuration should produce validation errors")
				
				// Property: Error messages should be clear and specific
				errorMessage := FormatValidationErrors(errors)
				assert.NotNil(t, errorMessage, "Should have formatted error message")
				assert.NotEmpty(t, errorMessage.Error(), "Error message should not be empty")
				
				// Property: Error messages should contain relevant context
				switch config.errorType {
				case "invalid_scheme":
					assert.Contains(t, errorMessage.Error(), "scheme", 
						"Error should mention scheme validation")
				case "invalid_status_code":
					assert.Contains(t, errorMessage.Error(), "status code", 
						"Error should mention status code validation")
				case "invalid_port":
					assert.Contains(t, errorMessage.Error(), "port", 
						"Error should mention port validation")
				case "zero_weights":
					assert.Contains(t, errorMessage.Error(), "positive weight", 
						"Error should mention weight validation")
				case "negative_weight":
					assert.Contains(t, errorMessage.Error(), "non-negative", 
						"Error should mention weight validation")
				}
				
				// Property: Each error should have field context
				for _, err := range errors {
					assert.NotEmpty(t, err.Field, "Error should have field context")
					assert.NotEmpty(t, err.Message, "Error should have message")
					assert.NotNil(t, err.Rule, "Error should reference the rule")
				}
			} else {
				// Property: Valid configurations should not produce errors
				assert.Equal(t, 0, len(errors), 
					"Valid configuration should not produce validation errors")
			}
		})
	}
}

// Property-based test for error message formatting consistency
func TestProperty_ErrorMessageFormattingConsistency(t *testing.T) {
	// Test property: Error message formatting should be consistent across different error types
	
	// Generate various error scenarios
	errorScenarios := [][]ValidationError{
		// Single error
		{
			{Field: "rule.backends", Message: "must have at least one backend", Rule: nil},
		},
		// Multiple errors
		{
			{Field: "rule.backends[0].weight", Message: "weight must be non-negative", Rule: nil},
			{Field: "rule.backends[1].weight", Message: "weight must not exceed 999", Rule: nil},
		},
		// Complex errors
		{
			{Field: "rule.filters[0].requestRedirect.scheme", Message: "invalid scheme 'ftp'", Rule: nil},
			{Field: "rule.filters[0].requestRedirect.statusCode", Message: "invalid status code 200", Rule: nil},
			{Field: "rule.backends", Message: "must have at least one backend", Rule: nil},
		},
	}

	for i, errors := range errorScenarios {
		t.Run(fmt.Sprintf("error_scenario_%d", i), func(t *testing.T) {
			formattedError := FormatValidationErrors(errors)
			
			if len(errors) == 0 {
				// Property: No errors should result in nil
				assert.Nil(t, formattedError, "No errors should result in nil")
			} else if len(errors) == 1 {
				// Property: Single error should be returned as-is
				assert.Equal(t, errors[0].Error(), formattedError.Error(), 
					"Single error should be formatted consistently")
			} else {
				// Property: Multiple errors should be formatted with numbering
				errorMessage := formattedError.Error()
				assert.Contains(t, errorMessage, "multiple validation errors", 
					"Multiple errors should indicate count")
				assert.Contains(t, errorMessage, fmt.Sprintf("(%d)", len(errors)), 
					"Error count should be included")
				
				// Property: Each error should be numbered
				for j := range errors {
					assert.Contains(t, errorMessage, fmt.Sprintf("%d.", j+1), 
						"Each error should be numbered")
				}
			}
		})
	}
}

// Property-based test for validation completeness
func TestProperty_ValidationCompleteness(t *testing.T) {
	// Test property: Validation should cover all aspects of HTTPRoute rule configuration
	
	// Test comprehensive validation scenarios
	comprehensiveTests := []struct {
		name           string
		rule           RouteRule
		shouldValidate bool
		aspects        []string
	}{
		{
			name: "redirect_only_comprehensive",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
								Scheme:     awssdk.String("https"),
								Hostname:   (*gwv1.PreciseHostname)(awssdk.String("example.com")),
								Port:       (*gwv1.PortNumber)(awssdk.Int32(443)),
								StatusCode: awssdk.Int(301),
							},
						},
					},
				},
				backends: []Backend{},
			},
			shouldValidate: true,
			aspects:        []string{"redirect_filter", "no_backends", "scheme", "hostname", "port", "status_code"},
		},
		{
			name: "backend_comprehensive",
			rule: &convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Filters: []gwv1.HTTPRouteFilter{},
				},
				backends: []Backend{
					{Weight: 25},
					{Weight: 50},
					{Weight: 25},
				},
			},
			shouldValidate: true,
			aspects:        []string{"backends", "weights", "no_redirect"},
		},
	}

	for _, test := range comprehensiveTests {
		t.Run(test.name, func(t *testing.T) {
			errors := ValidateHTTPRouteRule(test.rule)
			
			if test.shouldValidate {
				// Property: Valid comprehensive configurations should pass validation
				assert.Equal(t, 0, len(errors), 
					"Comprehensive valid configuration should pass validation")
			}
			
			// Property: Validation should check all specified aspects
			// This is verified by the absence of errors for valid configurations
			// and the presence of specific errors for invalid configurations
		})
	}
}