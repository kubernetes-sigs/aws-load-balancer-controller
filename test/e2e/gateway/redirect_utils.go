package gateway

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elbv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elbv2/types"
	. "github.com/onsi/gomega"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// RedirectTestUtils provides utilities for testing redirect-only HTTPRoute functionality
type RedirectTestUtils struct {
	elbv2Client *elbv2.Client
}

// NewRedirectTestUtils creates a new RedirectTestUtils instance
func NewRedirectTestUtils(elbv2Client *elbv2.Client) *RedirectTestUtils {
	return &RedirectTestUtils{
		elbv2Client: elbv2Client,
	}
}

// VerifyRedirectRule verifies that a redirect rule exists in the ALB listener rules
func (r *RedirectTestUtils) VerifyRedirectRule(ctx context.Context, listenerArn string, expectedPath string, expectedRedirect RedirectExpectation) error {
	// Get listener rules
	resp, err := r.elbv2Client.DescribeRules(ctx, &elbv2.DescribeRulesInput{
		ListenerArn: &listenerArn,
	})
	if err != nil {
		return fmt.Errorf("failed to describe rules: %w", err)
	}

	// Find the rule with matching path condition
	for _, rule := range resp.Rules {
		if r.ruleMatchesPath(rule, expectedPath) {
			return r.verifyRuleRedirectAction(rule, expectedRedirect)
		}
	}

	return fmt.Errorf("no rule found with path condition: %s", expectedPath)
}

// VerifyNoTargetGroupsForRedirectRules verifies that redirect-only rules don't have target groups
func (r *RedirectTestUtils) VerifyNoTargetGroupsForRedirectRules(ctx context.Context, listenerArn string, redirectPaths []string) error {
	// Get listener rules
	resp, err := r.elbv2Client.DescribeRules(ctx, &elbv2.DescribeRulesInput{
		ListenerArn: &listenerArn,
	})
	if err != nil {
		return fmt.Errorf("failed to describe rules: %w", err)
	}

	// Check each redirect path
	for _, path := range redirectPaths {
		for _, rule := range resp.Rules {
			if r.ruleMatchesPath(rule, path) {
				// Verify this rule has redirect action, not forward action
				if err := r.verifyRuleHasRedirectAction(rule); err != nil {
					return fmt.Errorf("redirect rule for path %s: %w", path, err)
				}
			}
		}
	}

	return nil
}

// RedirectExpectation defines expected redirect behavior
type RedirectExpectation struct {
	Scheme     *string
	Hostname   *string
	Port       *string
	StatusCode *string
}

// ruleMatchesPath checks if a rule has a path condition matching the expected path
func (r *RedirectTestUtils) ruleMatchesPath(rule elbv2types.Rule, expectedPath string) bool {
	for _, condition := range rule.Conditions {
		if condition.Field != nil && *condition.Field == "path-pattern" {
			for _, value := range condition.Values {
				if value == expectedPath {
					return true
				}
			}
		}
	}
	return false
}

// verifyRuleRedirectAction verifies that a rule has the expected redirect action
func (r *RedirectTestUtils) verifyRuleRedirectAction(rule elbv2types.Rule, expected RedirectExpectation) error {
	for _, action := range rule.Actions {
		if action.Type == elbv2types.ActionTypeEnumRedirect {
			redirectConfig := action.RedirectConfig
			if redirectConfig == nil {
				return fmt.Errorf("redirect action missing configuration")
			}

			// Verify scheme
			if expected.Scheme != nil {
				if redirectConfig.Protocol == nil || *redirectConfig.Protocol != *expected.Scheme {
					return fmt.Errorf("expected scheme %s, got %v", *expected.Scheme, redirectConfig.Protocol)
				}
			}

			// Verify hostname
			if expected.Hostname != nil {
				if redirectConfig.Host == nil || *redirectConfig.Host != *expected.Hostname {
					return fmt.Errorf("expected hostname %s, got %v", *expected.Hostname, redirectConfig.Host)
				}
			}

			// Verify port
			if expected.Port != nil {
				if redirectConfig.Port == nil || *redirectConfig.Port != *expected.Port {
					return fmt.Errorf("expected port %s, got %v", *expected.Port, redirectConfig.Port)
				}
			}

			// Verify status code
			if expected.StatusCode != nil {
				if redirectConfig.StatusCode == nil || *redirectConfig.StatusCode != *expected.StatusCode {
					return fmt.Errorf("expected status code %s, got %v", *expected.StatusCode, redirectConfig.StatusCode)
				}
			}

			return nil // Found matching redirect action
		}
	}

	return fmt.Errorf("no redirect action found in rule")
}

// verifyRuleHasRedirectAction verifies that a rule has a redirect action (not forward)
func (r *RedirectTestUtils) verifyRuleHasRedirectAction(rule elbv2types.Rule) error {
	hasRedirect := false
	hasForward := false

	for _, action := range rule.Actions {
		switch action.Type {
		case elbv2types.ActionTypeEnumRedirect:
			hasRedirect = true
		case elbv2types.ActionTypeEnumForward:
			hasForward = true
		}
	}

	if !hasRedirect {
		return fmt.Errorf("rule does not have redirect action")
	}

	if hasForward {
		return fmt.Errorf("redirect-only rule should not have forward action")
	}

	return nil
}

// BuildRedirectOnlyHTTPRoute creates an HTTPRoute with only redirect rules (no backends)
func BuildRedirectOnlyHTTPRoute(name string, redirectRules []RedirectRule) *gwv1.HTTPRoute {
	var rules []gwv1.HTTPRouteRule

	for _, redirectRule := range redirectRules {
		rule := gwv1.HTTPRouteRule{
			Matches: []gwv1.HTTPRouteMatch{
				{
					Path: &gwv1.HTTPPathMatch{
						Type:  &redirectRule.PathType,
						Value: &redirectRule.Path,
					},
				},
			},
			Filters: []gwv1.HTTPRouteFilter{
				{
					Type:            gwv1.HTTPRouteFilterRequestRedirect,
					RequestRedirect: &redirectRule.Redirect,
				},
			},
			// No BackendRefs - this makes it redirect-only
		}
		rules = append(rules, rule)
	}

	return &gwv1.HTTPRoute{
		ObjectMeta: buildHTTPRouteObjectMeta(name),
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name: defaultGatewayName,
					},
				},
			},
			Rules: rules,
		},
	}
}

// RedirectRule defines a redirect rule configuration
type RedirectRule struct {
	Path     string
	PathType gwv1.PathMatchType
	Redirect gwv1.HTTPRequestRedirectFilter
}

// BuildMixedHTTPRoute creates an HTTPRoute with both redirect-only and backend rules
func BuildMixedHTTPRoute(name string, redirectRules []RedirectRule, backendRules []BackendRule) *gwv1.HTTPRoute {
	var rules []gwv1.HTTPRouteRule

	// Add redirect-only rules
	for _, redirectRule := range redirectRules {
		rule := gwv1.HTTPRouteRule{
			Matches: []gwv1.HTTPRouteMatch{
				{
					Path: &gwv1.HTTPPathMatch{
						Type:  &redirectRule.PathType,
						Value: &redirectRule.Path,
					},
				},
			},
			Filters: []gwv1.HTTPRouteFilter{
				{
					Type:            gwv1.HTTPRouteFilterRequestRedirect,
					RequestRedirect: &redirectRule.Redirect,
				},
			},
			// No BackendRefs
		}
		rules = append(rules, rule)
	}

	// Add backend rules
	for _, backendRule := range backendRules {
		rule := gwv1.HTTPRouteRule{
			Matches: []gwv1.HTTPRouteMatch{
				{
					Path: &gwv1.HTTPPathMatch{
						Type:  &backendRule.PathType,
						Value: &backendRule.Path,
					},
				},
			},
			BackendRefs: backendRule.BackendRefs,
			// No redirect filters
		}
		rules = append(rules, rule)
	}

	return &gwv1.HTTPRoute{
		ObjectMeta: buildHTTPRouteObjectMeta(name),
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name: defaultGatewayName,
					},
				},
			},
			Rules: rules,
		},
	}
}

// BackendRule defines a backend rule configuration
type BackendRule struct {
	Path        string
	PathType    gwv1.PathMatchType
	BackendRefs []gwv1.HTTPBackendRef
}

// Common redirect configurations for testing
var (
	HTTPSRedirect = gwv1.HTTPRequestRedirectFilter{
		Scheme:     awssdk.String("https"),
		StatusCode: awssdk.Int(301),
	}

	HostnameRedirect = gwv1.HTTPRequestRedirectFilter{
		Hostname:   (*gwv1.PreciseHostname)(awssdk.String("new.example.com")),
		StatusCode: awssdk.Int(302),
	}

	PortRedirect = gwv1.HTTPRequestRedirectFilter{
		Port:       (*gwv1.PortNumber)(awssdk.Int32(8080)),
		StatusCode: awssdk.Int(302),
	}

	ComplexRedirect = gwv1.HTTPRequestRedirectFilter{
		Scheme:     awssdk.String("https"),
		Hostname:   (*gwv1.PreciseHostname)(awssdk.String("secure.example.com")),
		Port:       (*gwv1.PortNumber)(awssdk.Int32(443)),
		StatusCode: awssdk.Int(301),
	}
)

// Verification helpers
func ExpectRedirectRule(ctx context.Context, utils *RedirectTestUtils, listenerArn, path string, expected RedirectExpectation) {
	Eventually(func() error {
		return utils.VerifyRedirectRule(ctx, listenerArn, path, expected)
	}).Should(Succeed(), fmt.Sprintf("Expected redirect rule for path %s", path))
}

func ExpectNoTargetGroupsForRedirects(ctx context.Context, utils *RedirectTestUtils, listenerArn string, redirectPaths []string) {
	Consistently(func() error {
		return utils.VerifyNoTargetGroupsForRedirectRules(ctx, listenerArn, redirectPaths)
	}).Should(Succeed(), "Redirect-only rules should not have target groups")
}