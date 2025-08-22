package routeutils

import (
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/pkg/errors"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"strconv"
	"strings"
)

// BuildRulePreRoutingActions returns pre-routing action for rule
// The assumption is that the ListenerRuleConfiguration CRD makes sure we only have one of the actions (authenticate-cognito, authenticate-oidc) defined
func BuildRulePreRoutingAction(route RouteDescriptor, crdPreRoutingAction *elbv2gw.Action) (*elbv2model.Action, error) {
	switch crdPreRoutingAction.Type {
	case elbv2gw.ActionTypeAuthenticateOIDC:
		return buildAuthenticateOIDCAction(crdPreRoutingAction.AuthenticateOIDCConfig, route)
	case elbv2gw.ActionTypeAuthenticateCognito:
		return buildAuthenticateCognitoAction(crdPreRoutingAction.AuthenticateCognitoConfig)

	}
	return nil, errors.Errorf("unsupported action type %s", crdPreRoutingAction.Type)
}

// BuildRuleRoutingAction returns routing action for rule
// The assumption is that the ListenerRuleConfiguration CRD makes sure we only have one of the actions (forward, redirect, fixed-response) defined
func BuildRuleRoutingAction(rule RouteRule, route RouteDescriptor, routingAction *elbv2gw.Action, targetGroupTuples []elbv2model.TargetGroupTuple) (*elbv2model.Action, error) {
	var action *elbv2model.Action
	// Build Rule Routing Actions - Fixed Response
	if routingAction != nil && routingAction.Type == elbv2gw.ActionTypeFixedResponse {
		fixedResponseActions, err := buildFixedResponseRoutingAction(routingAction.FixedResponseConfig)
		if err != nil {
			return nil, err
		}
		if fixedResponseActions != nil {
			action = fixedResponseActions
		}
	} else {
		// Build Rule Routing Actions - Forward
		forwardActions, err := buildForwardRoutingAction(routingAction, targetGroupTuples)
		if err != nil {
			return nil, err
		}
		if forwardActions != nil {
			action = forwardActions
		}

		// Build Rule Routing Actions - Redirect
		redirectActions, err := buildRedirectRoutingAction(rule, route, routingAction)
		if err != nil {
			return nil, err
		}
		if redirectActions != nil {
			action = redirectActions
		}
	}
	return action, nil
}

func buildFixedResponseRoutingAction(fixedResponseConfig *elbv2gw.FixedResponseActionConfig) (*elbv2model.Action, error) {
	action := elbv2model.Action{
		Type: elbv2model.ActionTypeFixedResponse,
		FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
			ContentType: fixedResponseConfig.ContentType,
			StatusCode:  strconv.Itoa(int(fixedResponseConfig.StatusCode)),
			MessageBody: fixedResponseConfig.MessageBody,
		},
	}
	return &action, nil
}

func buildAuthenticateCognitoAction(authCognitoActionConfig *elbv2gw.AuthenticateCognitoActionConfig) (*elbv2model.Action, error) {
	return &elbv2model.Action{
		Type: elbv2model.ActionTypeAuthenticateCognito,
		AuthenticateCognitoConfig: &elbv2model.AuthenticateCognitoActionConfig{
			UserPoolARN:                      authCognitoActionConfig.UserPoolArn,
			UserPoolClientID:                 authCognitoActionConfig.UserPoolClientID,
			UserPoolDomain:                   authCognitoActionConfig.UserPoolDomain,
			AuthenticationRequestExtraParams: *authCognitoActionConfig.AuthenticationRequestExtraParams,
			OnUnauthenticatedRequest:         elbv2model.AuthenticateCognitoActionConditionalBehavior(*authCognitoActionConfig.OnUnauthenticatedRequest),
			Scope:                            authCognitoActionConfig.Scope,
			SessionCookieName:                authCognitoActionConfig.SessionCookieName,
			SessionTimeout:                   authCognitoActionConfig.SessionTimeout,
		},
	}, nil
}

func buildAuthenticateOIDCAction(autheticateOIDCActionConfig *elbv2gw.AuthenticateOidcActionConfig, route RouteDescriptor) (*elbv2model.Action, error) {
	// TODO
	return nil, nil
}

func buildForwardRoutingAction(routingAction *elbv2gw.Action, targetGroupTuples []elbv2model.TargetGroupTuple) (*elbv2model.Action, error) {
	if shouldProvisionActions(targetGroupTuples) {
		var forwardConfig *elbv2gw.ForwardActionConfig
		if routingAction != nil {
			forwardConfig = routingAction.ForwardConfig
		}
		return buildL7ListenerForwardActions(targetGroupTuples, forwardConfig), nil
	}
	return nil, nil
}

func buildRedirectRoutingAction(rule RouteRule, route RouteDescriptor, routingAction *elbv2gw.Action) (*elbv2model.Action, error) {
	switch route.GetRouteKind() {
	case HTTPRouteKind:
		httpRule := rule.GetRawRouteRule().(*gwv1.HTTPRouteRule)
		if len(httpRule.Filters) > 0 {
			var redirectConfig *elbv2gw.RedirectActionConfig
			if routingAction != nil {
				redirectConfig = routingAction.RedirectConfig
			}
			redirectActions, err := buildHttpRuleRedirectActionsBasedOnFilter(httpRule.Filters, redirectConfig)
			if err != nil {
				return nil, err
			}
			return redirectActions, nil
		}
	}
	return nil, nil
}

func buildL7ListenerForwardActions(targetGroupTuple []elbv2model.TargetGroupTuple, forwardActionConfig *elbv2gw.ForwardActionConfig) *elbv2model.Action {
	forwardConfig := &elbv2model.ForwardActionConfig{
		TargetGroups: targetGroupTuple,
	}

	// if forwardActionConfig is not nil, enabled and durationSecond will at least have default value
	if forwardActionConfig != nil {
		forwardConfig.TargetGroupStickinessConfig = &elbv2model.TargetGroupStickinessConfig{
			Enabled:         awssdk.Bool(*forwardActionConfig.TargetGroupStickinessConfig.Enabled),
			DurationSeconds: awssdk.Int32(*forwardActionConfig.TargetGroupStickinessConfig.DurationSeconds),
		}
	}

	return &elbv2model.Action{
		Type:          elbv2model.ActionTypeForward,
		ForwardConfig: forwardConfig,
	}
}

// buildHttpRuleRedirectActionsBasedOnFilter only request redirect is supported, header modification is limited due to ALB support level.
func buildHttpRuleRedirectActionsBasedOnFilter(filters []gwv1.HTTPRouteFilter, redirectConfig *elbv2gw.RedirectActionConfig) (*elbv2model.Action, error) {
	// edge case: filters only defines ExtensionRef with Kind ListenerRuleConfiguration and ListenerRuleConfiguration type is redirect
	if len(filters) == 1 && filters[0].Type == gwv1.HTTPRouteFilterExtensionRef && redirectConfig != nil {
		return nil, errors.Errorf("HTTPRouteFilterRequestRedirect must be provided if RedirectActionConfig in ListenerRuleConfiguration is provided")

	}
	for _, filter := range filters {
		switch filter.Type {
		case gwv1.HTTPRouteFilterRequestRedirect:
			return buildHttpRedirectAction(filter.RequestRedirect, redirectConfig)
		case gwv1.HTTPRouteFilterExtensionRef:
			continue
		default:
			return nil, errors.Errorf("Unsupported filter type: %v. Only request redirect is supported. To specify header modification, please configure it through LoadBalancerConfiguration.", filter.Type)
		}
	}
	return nil, nil
}

// buildHttpRedirectAction configure filter attributes to RedirectActionConfig
// gateway api has no attribute to specify query, use listener rule configuration
func buildHttpRedirectAction(filter *gwv1.HTTPRequestRedirectFilter, redirectConfig *elbv2gw.RedirectActionConfig) (*elbv2model.Action, error) {
	isComponentSpecified := false
	var statusCode string
	if filter.StatusCode != nil {
		statusCodeStr := fmt.Sprintf("HTTP_%d", *filter.StatusCode)
		statusCode = statusCodeStr
	}

	var port *string
	if filter.Port != nil {
		portStr := fmt.Sprintf("%d", *filter.Port)
		port = &portStr
		isComponentSpecified = true
	}

	var protocol *string
	if filter.Scheme != nil {
		upperScheme := strings.ToUpper(*filter.Scheme)
		if upperScheme != "HTTP" && upperScheme != "HTTPS" {
			return nil, errors.Errorf("unsupported redirect scheme: %v", upperScheme)
		}
		protocol = &upperScheme
		isComponentSpecified = true
	}

	var path *string
	if filter.Path != nil {
		if filter.Path.ReplaceFullPath != nil {
			pathValue := *filter.Path.ReplaceFullPath
			if strings.ContainsAny(pathValue, "*?") {
				return nil, errors.Errorf("ReplaceFullPath shouldn't contain wildcards: %v", pathValue)
			}
			path = filter.Path.ReplaceFullPath
			isComponentSpecified = true
		} else if filter.Path.ReplacePrefixMatch != nil {
			pathValue := *filter.Path.ReplacePrefixMatch
			if strings.ContainsAny(pathValue, "*?") {
				return nil, errors.Errorf("ReplacePrefixMatch shouldn't contain wildcards: %v", pathValue)
			}
			processedPath := fmt.Sprintf("%s/*", pathValue)
			path = &processedPath
			isComponentSpecified = true
		}
	}

	var hostname *string
	if filter.Hostname != nil {
		hostname = (*string)(filter.Hostname)
		isComponentSpecified = true
	}

	if !isComponentSpecified {
		return nil, errors.Errorf("To avoid a redirect loop, you must modify at least one of the following components: protocol, port, hostname or path.")
	}

	var query *string
	if redirectConfig != nil {
		query = redirectConfig.Query
	}

	action := elbv2model.Action{
		Type: elbv2model.ActionTypeRedirect,
		RedirectConfig: &elbv2model.RedirectActionConfig{
			Host:       hostname,
			Path:       path,
			Port:       port,
			Protocol:   protocol,
			StatusCode: statusCode,
			Query:      query,
		},
	}
	return &action, nil
}

// shouldProvisionActions -- determine if the given target groups are acceptable for ELB Actions.
// The criteria -
// 1/ One or more target groups are present.
// 2/ At least one target group has a weight greater than zero.
func shouldProvisionActions(targetGroups []elbv2model.TargetGroupTuple) bool {
	for _, tg := range targetGroups {
		if tg.Weight == nil || *tg.Weight != 0 {
			return true
		}
	}
	return false
}
