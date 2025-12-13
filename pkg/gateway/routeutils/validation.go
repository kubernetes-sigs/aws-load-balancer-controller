package routeutils

import (
	"fmt"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ValidationError represents a validation error with context
type ValidationError struct {
	Field   string
	Message string
	Rule    RouteRule
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error in field %s: %s", e.Field, e.Message)
}

// ValidateHTTPRouteRule validates an HTTPRoute rule configuration
func ValidateHTTPRouteRule(rule RouteRule) []ValidationError {
	var errors []ValidationError

	// Validate redirect-only rules
	if IsRedirectOnlyRule(rule) {
		errors = append(errors, validateRedirectOnlyRule(rule)...)
	}

	// Validate rules with backends
	if len(rule.GetBackends()) > 0 {
		errors = append(errors, validateBackendRule(rule)...)
	}

	// Validate mixed rules (both redirect and backends)
	if HasRequestRedirectFilter(rule) && len(rule.GetBackends()) > 0 {
		errors = append(errors, validateMixedRule(rule)...)
	}

	// Validate that rule has either backends or redirect filters (not empty)
	if len(rule.GetBackends()) == 0 && !HasRequestRedirectFilter(rule) {
		errors = append(errors, ValidationError{
			Field:   "rule",
			Message: "rule must have either backends or redirect filters",
			Rule:    rule,
		})
	}

	return errors
}

// validateRedirectOnlyRule validates redirect-only rule configuration
func validateRedirectOnlyRule(rule RouteRule) []ValidationError {
	var errors []ValidationError

	rawRule := rule.GetRawRouteRule()
	httpRule, ok := rawRule.(*gwv1.HTTPRouteRule)
	if !ok {
		errors = append(errors, ValidationError{
			Field:   "rule.type",
			Message: "redirect-only rule must be HTTPRouteRule",
			Rule:    rule,
		})
		return errors
	}

	// Validate that redirect filters are properly configured
	redirectFilterCount := 0
	for i, filter := range httpRule.Filters {
		if filter.Type == gwv1.HTTPRouteFilterRequestRedirect {
			redirectFilterCount++
			if filter.RequestRedirect == nil {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("rule.filters[%d].requestRedirect", i),
					Message: "RequestRedirect filter must have configuration",
					Rule:    rule,
				})
			} else {
				// Validate redirect configuration
				errors = append(errors, validateRedirectConfiguration(filter.RequestRedirect, rule, i)...)
			}
		}
	}

	if redirectFilterCount == 0 {
		errors = append(errors, ValidationError{
			Field:   "rule.filters",
			Message: "redirect-only rule must have at least one RequestRedirect filter",
			Rule:    rule,
		})
	}

	// Validate that there are no backends
	if len(rule.GetBackends()) > 0 {
		errors = append(errors, ValidationError{
			Field:   "rule.backends",
			Message: "redirect-only rule must not have backends",
			Rule:    rule,
		})
	}

	return errors
}

// validateBackendRule validates rules with backends
func validateBackendRule(rule RouteRule) []ValidationError {
	var errors []ValidationError

	backends := rule.GetBackends()
	if len(backends) == 0 {
		errors = append(errors, ValidationError{
			Field:   "rule.backends",
			Message: "backend rule must have at least one backend",
			Rule:    rule,
		})
		return errors
	}

	// Validate backend weights
	totalWeight := 0
	for i, backend := range backends {
		if backend.Weight < 0 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("rule.backends[%d].weight", i),
				Message: "backend weight must be non-negative",
				Rule:    rule,
			})
		}
		if backend.Weight > 999 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("rule.backends[%d].weight", i),
				Message: "backend weight must not exceed 999",
				Rule:    rule,
			})
		}
		totalWeight += backend.Weight
	}

	// Validate that at least one backend has positive weight
	if totalWeight == 0 {
		errors = append(errors, ValidationError{
			Field:   "rule.backends",
			Message: "at least one backend must have positive weight",
			Rule:    rule,
		})
	}

	return errors
}

// validateMixedRule validates rules with both redirect filters and backends
func validateMixedRule(rule RouteRule) []ValidationError {
	var errors []ValidationError

	// Mixed rules are valid according to Gateway API spec
	// Both redirect and backend processing should be supported
	
	// Validate that both components are properly configured
	if !HasRequestRedirectFilter(rule) {
		errors = append(errors, ValidationError{
			Field:   "rule.filters",
			Message: "mixed rule must have redirect filter",
			Rule:    rule,
		})
	}

	if len(rule.GetBackends()) == 0 {
		errors = append(errors, ValidationError{
			Field:   "rule.backends",
			Message: "mixed rule must have backends",
			Rule:    rule,
		})
	}

	return errors
}

// validateRedirectConfiguration validates redirect filter configuration
func validateRedirectConfiguration(redirect *gwv1.HTTPRequestRedirectFilter, rule RouteRule, filterIndex int) []ValidationError {
	var errors []ValidationError

	if redirect == nil {
		errors = append(errors, ValidationError{
			Field:   fmt.Sprintf("rule.filters[%d].requestRedirect", filterIndex),
			Message: "redirect configuration cannot be nil",
			Rule:    rule,
		})
		return errors
	}

	// Validate scheme
	if redirect.Scheme != nil {
		scheme := *redirect.Scheme
		if scheme != "http" && scheme != "https" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("rule.filters[%d].requestRedirect.scheme", filterIndex),
				Message: fmt.Sprintf("invalid scheme '%s', must be 'http' or 'https'", scheme),
				Rule:    rule,
			})
		}
	}

	// Validate status code
	if redirect.StatusCode != nil {
		code := *redirect.StatusCode
		if code < 300 || code > 399 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("rule.filters[%d].requestRedirect.statusCode", filterIndex),
				Message: fmt.Sprintf("invalid status code %d, must be between 300-399", code),
				Rule:    rule,
			})
		}
	}

	// Validate port
	if redirect.Port != nil {
		port := *redirect.Port
		if port < 1 || port > 65535 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("rule.filters[%d].requestRedirect.port", filterIndex),
				Message: fmt.Sprintf("invalid port %d, must be between 1-65535", port),
				Rule:    rule,
			})
		}
	}

	return errors
}

// FormatValidationErrors formats validation errors into a user-friendly message
func FormatValidationErrors(errors []ValidationError) error {
	if len(errors) == 0 {
		return nil
	}

	if len(errors) == 1 {
		return errors[0]
	}

	message := fmt.Sprintf("multiple validation errors (%d):", len(errors))
	for i, err := range errors {
		message += fmt.Sprintf("\n  %d. %s", i+1, err.Error())
	}

	return fmt.Errorf("%s", message)
}