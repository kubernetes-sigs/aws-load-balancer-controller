package ls

import (
	"context"
	"fmt"
	"strconv"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/conditions"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/auth"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// RulesController provides functionality to manage rules on listeners
type RulesController interface {
	// Reconcile ensures the listener rules in AWS match the rules configured in the Ingress resource.
	Reconcile(ctx context.Context, listener *elbv2.Listener, ingress *extensions.Ingress, ingressAnnos *annotations.Ingress, tgGroup tg.TargetGroupGroup) error
}

// NewRulesController constructs RulesController
func NewRulesController(cloud aws.CloudAPI, authModule auth.Module) RulesController {
	return &rulesController{
		cloud:      cloud,
		authModule: authModule,
	}
}

type rulesController struct {
	cloud      aws.CloudAPI
	authModule auth.Module
}

// Reconcile modifies AWS resources to match the rules defined in the Ingress
func (c *rulesController) Reconcile(ctx context.Context, listener *elbv2.Listener, ingress *extensions.Ingress, ingressAnnos *annotations.Ingress, tgGroup tg.TargetGroupGroup) error {
	desired, err := c.getDesiredRules(ctx, listener, ingress, ingressAnnos, tgGroup)
	if err != nil {
		return err
	}
	lsArn := aws.StringValue(listener.ListenerArn)
	current, err := c.getCurrentRules(ctx, lsArn)
	if err != nil {
		return err
	}
	return c.reconcileRules(ctx, lsArn, current, desired)
}

func (c *rulesController) reconcileRules(ctx context.Context, lsArn string, current []elbv2.Rule, desired []elbv2.Rule) error {
	additions, modifies, removals := rulesChangeSets(current, desired)

	for _, rule := range additions {
		albctx.GetLogger(ctx).Infof("creating rule %v on %v", aws.StringValue(rule.Priority), lsArn)
		priority, _ := strconv.ParseInt(aws.StringValue(rule.Priority), 10, 64)
		in := &elbv2.CreateRuleInput{
			ListenerArn: aws.String(lsArn),
			Actions:     rule.Actions,
			Conditions:  rule.Conditions,
			Priority:    aws.Int64(priority),
		}

		if _, err := c.cloud.CreateRuleWithContext(ctx, in); err != nil {
			msg := fmt.Sprintf("failed creating rule %v on %v due to %v", aws.StringValue(rule.Priority), lsArn, err)
			albctx.GetLogger(ctx).Errorf(msg)
			albctx.GetEventf(ctx)(corev1.EventTypeWarning, "ERROR", msg)
			return fmt.Errorf(msg)
		}

		msg := fmt.Sprintf("rule %v created with conditions %v", aws.StringValue(rule.Priority), log.Prettify(rule.Conditions))
		albctx.GetLogger(ctx).Infof(msg)
		albctx.GetEventf(ctx)(corev1.EventTypeNormal, "CREATE", msg)
	}

	for _, rule := range modifies {
		albctx.GetLogger(ctx).Infof("modifying rule %v on %v", aws.StringValue(rule.Priority), lsArn)
		in := &elbv2.ModifyRuleInput{
			Actions:    rule.Actions,
			Conditions: rule.Conditions,
			RuleArn:    rule.RuleArn,
		}

		if _, err := c.cloud.ModifyRuleWithContext(ctx, in); err != nil {
			msg := fmt.Sprintf("failed modifying rule %v on %v due to %v", aws.StringValue(rule.Priority), lsArn, err)
			albctx.GetLogger(ctx).Errorf(msg)
			albctx.GetEventf(ctx)(corev1.EventTypeWarning, "ERROR", msg)
			return fmt.Errorf(msg)
		}

		msg := fmt.Sprintf("rule %v modified with conditions %v", aws.StringValue(rule.Priority), log.Prettify(rule.Conditions))
		albctx.GetEventf(ctx)(corev1.EventTypeNormal, "MODIFY", msg)
		albctx.GetLogger(ctx).Infof(msg)
	}

	for _, rule := range removals {
		albctx.GetLogger(ctx).Infof("deleting rule %v on %v", aws.StringValue(rule.Priority), lsArn)

		in := &elbv2.DeleteRuleInput{RuleArn: rule.RuleArn}
		if _, err := c.cloud.DeleteRuleWithContext(ctx, in); err != nil {
			msg := fmt.Sprintf("failed deleting rule %v on %v due to %v", aws.StringValue(rule.Priority), lsArn, err)
			albctx.GetLogger(ctx).Errorf(msg)
			albctx.GetEventf(ctx)(corev1.EventTypeWarning, "ERROR", msg)
			return fmt.Errorf(msg)
		}

		msg := fmt.Sprintf("rule %v deleted with conditions %v", aws.StringValue(rule.Priority), log.Prettify(rule.Conditions))
		albctx.GetEventf(ctx)(corev1.EventTypeNormal, "DELETE", msg)
		albctx.GetLogger(ctx).Infof(msg)
	}
	return nil
}

func (c *rulesController) getDesiredRules(ctx context.Context, listener *elbv2.Listener, ingress *extensions.Ingress, ingressAnnos *annotations.Ingress, tgGroup tg.TargetGroupGroup) ([]elbv2.Rule, error) {
	var output []elbv2.Rule

	nextPriority := 1
	for _, ingressRule := range ingress.Spec.Rules {
		// Ingress spec allows empty HTTP, and we will 'route all traffic to the default backend'(which relies on default action of listeners)
		if ingressRule.HTTP == nil {
			continue
		}

		seenUnconditionalRedirect := false

		for _, path := range ingressRule.HTTP.Paths {
			if seenUnconditionalRedirect {
				// Ignore rules that follow a unconditional redirect, they are moot
				continue
			}
			authCfg, err := c.authModule.NewConfig(ctx, ingress, path.Backend, aws.StringValue(listener.Protocol))
			if err != nil {
				return nil, err
			}
			elbActions, err := buildActions(ctx, authCfg, ingressAnnos, path.Backend, tgGroup)
			if err != nil {
				return nil, err
			}
			elbConditions := buildConditions(ctx, ingressAnnos, ingressRule, path)
			elbRule := elbv2.Rule{
				IsDefault:  aws.Bool(false),
				Priority:   aws.String(strconv.Itoa(nextPriority)),
				Actions:    elbActions,
				Conditions: elbConditions,
			}
			if createsRedirectLoop(listener, elbRule) {
				continue
			} else if isUnconditionalRedirect(listener, elbRule) {
				seenUnconditionalRedirect = true
			}
			output = append(output, elbRule)
			nextPriority++
		}
	}
	return output, nil
}

func (c *rulesController) getCurrentRules(ctx context.Context, listenerArn string) ([]elbv2.Rule, error) {
	rules, err := c.cloud.GetRules(ctx, listenerArn)
	if err != nil {
		return nil, err
	}

	var output []elbv2.Rule
	for _, rule := range rules {
		if aws.BoolValue(rule.IsDefault) {
			// Ignore these, let the listener manage it
			continue
		}
		output = append(output, *rule)
	}

	return output, nil
}

// buildActions will build listener rule actions for specific authCfg and backend
func buildActions(ctx context.Context, authCfg auth.Config, ingressAnnos *annotations.Ingress, backend extensions.IngressBackend, tgGroup tg.TargetGroupGroup) ([]*elbv2.Action, error) {
	var elbActions []*elbv2.Action

	// Handle auth actions
	authAction := buildAuthAction(ctx, authCfg)
	if authAction != nil {
		elbActions = append(elbActions, authAction)
	}

	// Handle backend actions
	if action.Use(backend.ServicePort.String()) {
		// backend is based on annotation
		annotationAction, err := ingressAnnos.Action.GetAction(backend.ServiceName)
		if err != nil {
			return nil, err
		}
		annotationELBAction, err := buildAnnotationAction(ctx, annotationAction, tgGroup)
		if err != nil {
			return nil, err
		}
		elbActions = append(elbActions, annotationELBAction)
	} else {
		// backend is based on service
		targetGroup, ok := tgGroup.TGByBackend[backend]
		if !ok {
			return nil, fmt.Errorf("unable to find targetGroup for backend %v:%v",
				backend.ServiceName, backend.ServicePort.String())
		}
		backendAction := elbv2.Action{
			Type: aws.String(elbv2.ActionTypeEnumForward),
			ForwardConfig: &elbv2.ForwardActionConfig{
				TargetGroups: []*elbv2.TargetGroupTuple{
					{
						TargetGroupArn: aws.String(targetGroup.Arn),
						Weight:         aws.Int64(1),
					},
				},
				TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
					Enabled: aws.Bool(false),
				},
			},
		}
		elbActions = append(elbActions, &backendAction)
	}

	for index, elbAction := range elbActions {
		elbAction.Order = aws.Int64(int64(index) + 1)
	}
	return elbActions, nil
}

// buildConditions will build listener rule conditions for specific ingressRule
func buildConditions(ctx context.Context, ingressAnnos *annotations.Ingress, rule extensions.IngressRule, path extensions.HTTPIngressPath) []*elbv2.RuleCondition {
	var elbConditions []*elbv2.RuleCondition

	hostHeaderConfig := &elbv2.HostHeaderConditionConfig{
		Values: nil,
	}
	pathPatternConfig := &elbv2.PathPatternConditionConfig{
		Values: nil,
	}
	if rule.Host != "" {
		hostHeaderConfig.Values = append(hostHeaderConfig.Values, aws.String(rule.Host))
	}
	if path.Path != "" {
		pathPatternConfig.Values = append(pathPatternConfig.Values, aws.String(path.Path))
	}
	annotationConditions := ingressAnnos.Conditions.GetConditions(path.Backend.ServiceName)
	for _, condition := range annotationConditions {
		switch aws.StringValue(condition.Field) {
		case conditions.FieldHostHeader:
			hostHeaderConfig.Values = append(hostHeaderConfig.Values, condition.HostHeaderConfig.Values...)
		case conditions.FieldPathPattern:
			pathPatternConfig.Values = append(pathPatternConfig.Values, condition.PathPatternConfig.Values...)
		case conditions.FieldHTTPRequestMethod:
			elbConditions = append(elbConditions, &elbv2.RuleCondition{
				Field: aws.String(conditions.FieldHTTPRequestMethod),
				HttpRequestMethodConfig: &elbv2.HttpRequestMethodConditionConfig{
					Values: condition.HttpRequestMethodConfig.Values,
				},
			})
		case conditions.FieldSourceIP:
			elbConditions = append(elbConditions, &elbv2.RuleCondition{
				Field: aws.String(conditions.FieldSourceIP),
				SourceIpConfig: &elbv2.SourceIpConditionConfig{
					Values: condition.SourceIpConfig.Values,
				},
			})
		case conditions.FieldHTTPHeader:
			elbConditions = append(elbConditions, &elbv2.RuleCondition{
				Field: aws.String(conditions.FieldHTTPHeader),
				HttpHeaderConfig: &elbv2.HttpHeaderConditionConfig{
					HttpHeaderName: condition.HttpHeaderConfig.HttpHeaderName,
					Values:         condition.HttpHeaderConfig.Values,
				},
			})
		case conditions.FieldQueryString:
			var queryStringKVPairs []*elbv2.QueryStringKeyValuePair
			for _, kv := range condition.QueryStringConfig.Values {
				queryStringKVPairs = append(queryStringKVPairs, &elbv2.QueryStringKeyValuePair{
					Key:   kv.Key,
					Value: kv.Value,
				})
			}
			elbConditions = append(elbConditions, &elbv2.RuleCondition{
				Field: aws.String(conditions.FieldQueryString),
				QueryStringConfig: &elbv2.QueryStringConditionConfig{
					Values: queryStringKVPairs,
				},
			})
		}
	}

	if len(hostHeaderConfig.Values) != 0 {
		elbConditions = append(elbConditions, &elbv2.RuleCondition{
			Field:            aws.String(conditions.FieldHostHeader),
			HostHeaderConfig: hostHeaderConfig,
		})
	}
	if len(pathPatternConfig.Values) != 0 {
		elbConditions = append(elbConditions, &elbv2.RuleCondition{
			Field:             aws.String(conditions.FieldPathPattern),
			PathPatternConfig: pathPatternConfig,
		})
	}
	if len(elbConditions) == 0 {
		elbConditions = append(elbConditions, &elbv2.RuleCondition{
			Field: aws.String(conditions.FieldPathPattern),
			PathPatternConfig: &elbv2.PathPatternConditionConfig{
				Values: []*string{aws.String("/*")},
			},
		})
	}
	return elbConditions
}

// buildAuthAction builds ELB action for specific authCfg.
// null will be returned if no auth is required.
func buildAuthAction(ctx context.Context, authCfg auth.Config) *elbv2.Action {
	switch authCfg.Type {
	case auth.TypeCognito:
		{
			return &elbv2.Action{
				Type: aws.String(elbv2.ActionTypeEnumAuthenticateCognito),
				AuthenticateCognitoConfig: &elbv2.AuthenticateCognitoActionConfig{
					AuthenticationRequestExtraParams: aws.StringMap(authCfg.IDPCognito.AuthenticationRequestExtraParams),
					UserPoolArn:                      aws.String(authCfg.IDPCognito.UserPoolArn),
					UserPoolClientId:                 aws.String(authCfg.IDPCognito.UserPoolClientId),
					UserPoolDomain:                   aws.String(authCfg.IDPCognito.UserPoolDomain),

					OnUnauthenticatedRequest: aws.String(string(authCfg.OnUnauthenticatedRequest)),
					Scope:                    aws.String(authCfg.Scope),
					SessionCookieName:        aws.String(authCfg.SessionCookie),
					SessionTimeout:           aws.Int64(authCfg.SessionTimeout),
				},
			}
		}
	case auth.TypeOIDC:
		{
			return &elbv2.Action{
				Type: aws.String(elbv2.ActionTypeEnumAuthenticateOidc),
				AuthenticateOidcConfig: &elbv2.AuthenticateOidcActionConfig{
					AuthenticationRequestExtraParams: aws.StringMap(authCfg.IDPOIDC.AuthenticationRequestExtraParams),
					AuthorizationEndpoint:            aws.String(authCfg.IDPOIDC.AuthorizationEndpoint),
					ClientId:                         aws.String(authCfg.IDPOIDC.ClientId),
					ClientSecret:                     aws.String(authCfg.IDPOIDC.ClientSecret),
					Issuer:                           aws.String(authCfg.IDPOIDC.Issuer),
					TokenEndpoint:                    aws.String(authCfg.IDPOIDC.TokenEndpoint),
					UserInfoEndpoint:                 aws.String(authCfg.IDPOIDC.UserInfoEndpoint),

					OnUnauthenticatedRequest: aws.String(string(authCfg.OnUnauthenticatedRequest)),
					Scope:                    aws.String(authCfg.Scope),
					SessionCookieName:        aws.String(authCfg.SessionCookie),
					SessionTimeout:           aws.Int64(authCfg.SessionTimeout),
				},
			}
		}
	}
	return nil
}

func buildAnnotationAction(ctx context.Context, action action.Action, tgGroup tg.TargetGroupGroup) (*elbv2.Action, error) {
	switch aws.StringValue(action.Type) {
	case elbv2.ActionTypeEnumFixedResponse:
		return &elbv2.Action{
			Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
			FixedResponseConfig: &elbv2.FixedResponseActionConfig{
				ContentType: action.FixedResponseConfig.ContentType,
				MessageBody: action.FixedResponseConfig.MessageBody,
				StatusCode:  action.FixedResponseConfig.StatusCode,
			},
		}, nil
	case elbv2.ActionTypeEnumRedirect:
		return &elbv2.Action{
			Type: aws.String(elbv2.ActionTypeEnumRedirect),
			RedirectConfig: &elbv2.RedirectActionConfig{
				Host:       action.RedirectConfig.Host,
				Path:       action.RedirectConfig.Path,
				Port:       action.RedirectConfig.Port,
				Protocol:   action.RedirectConfig.Protocol,
				Query:      action.RedirectConfig.Query,
				StatusCode: action.RedirectConfig.StatusCode,
			},
		}, nil
	case elbv2.ActionTypeEnumForward:
		return buildAnnotationForwardAction(ctx, action, tgGroup)
	}
	return nil, errors.Errorf("unknown action type: %v", aws.StringValue(action.Type))
}

func buildAnnotationForwardAction(ctx context.Context, action action.Action, tgGroup tg.TargetGroupGroup) (*elbv2.Action, error) {
	var elbTGs []*elbv2.TargetGroupTuple
	for _, tgt := range action.ForwardConfig.TargetGroups {
		normalizedWeight := tgt.Weight
		if normalizedWeight == nil {
			normalizedWeight = aws.Int64(1)
		}
		if tgt.TargetGroupArn != nil {
			elbTGs = append(elbTGs, &elbv2.TargetGroupTuple{
				TargetGroupArn: tgt.TargetGroupArn,
				Weight:         normalizedWeight,
			})
		} else {
			backend := extensions.IngressBackend{
				ServiceName: aws.StringValue(tgt.ServiceName),
				ServicePort: intstr.Parse(aws.StringValue(tgt.ServicePort)),
			}
			targetGroup, ok := tgGroup.TGByBackend[backend]
			if !ok {
				return nil, errors.Errorf("unable to find targetGroup for backend %v:%v",
					backend.ServiceName, backend.ServicePort.String())
			}
			elbTGs = append(elbTGs, &elbv2.TargetGroupTuple{
				TargetGroupArn: aws.String(targetGroup.Arn),
				Weight:         normalizedWeight,
			})
		}
	}
	elbAction := &elbv2.Action{
		Type: aws.String(elbv2.ActionTypeEnumForward),
		ForwardConfig: &elbv2.ForwardActionConfig{
			TargetGroups: elbTGs,
		},
	}

	if action.ForwardConfig.TargetGroupStickinessConfig != nil {
		elbAction.ForwardConfig.TargetGroupStickinessConfig = &elbv2.TargetGroupStickinessConfig{
			DurationSeconds: action.ForwardConfig.TargetGroupStickinessConfig.DurationSeconds,
			Enabled:         action.ForwardConfig.TargetGroupStickinessConfig.Enabled,
		}
	} else {
		elbAction.ForwardConfig.TargetGroupStickinessConfig = &elbv2.TargetGroupStickinessConfig{
			Enabled: aws.Bool(false),
		}
	}
	return elbAction, nil
}

// rulesChangeSets compares desired to current, returning a list of rules to add, modify and remove from current to match desired
func rulesChangeSets(current, desired []elbv2.Rule) (add []elbv2.Rule, modify []elbv2.Rule, remove []elbv2.Rule) {
	currentMap := make(map[string]elbv2.Rule, len(current))
	desiredMap := make(map[string]elbv2.Rule, len(desired))
	for _, i := range current {
		currentMap[aws.StringValue(i.Priority)] = i
	}
	for _, i := range desired {
		desiredMap[aws.StringValue(i.Priority)] = i
	}
	currentKeys := sets.StringKeySet(currentMap)
	desiredKeys := sets.StringKeySet(desiredMap)
	for key := range desiredKeys.Difference(currentKeys) {
		add = append(add, desiredMap[key])
	}
	for key := range currentKeys.Difference(desiredKeys) {
		remove = append(remove, currentMap[key])
	}

	for key := range currentKeys.Intersection(desiredKeys) {
		currentRule := currentMap[key]
		desiredRule := desiredMap[key]
		desiredRule.RuleArn = currentRule.RuleArn

		if !ruleMatches(desiredRule, currentRule) {
			modify = append(modify, desiredRule)
		}
	}
	return add, modify, remove
}

// createsRedirectLoop checks whether specified rule creates redirectionLoop for listener of protocol & port
func createsRedirectLoop(listener *elbv2.Listener, r elbv2.Rule) bool {
	for _, action := range r.Actions {
		rc := action.RedirectConfig
		if rc == nil {
			continue
		}

		var hosts []string
		var paths []string
		for _, c := range r.Conditions {
			switch aws.StringValue(c.Field) {
			case conditions.FieldHostHeader:
				hosts = append(hosts, aws.StringValueSlice(c.HostHeaderConfig.Values)...)
			case conditions.FieldPathPattern:
				paths = append(paths, aws.StringValueSlice(c.PathPatternConfig.Values)...)
			}
		}

		if len(hosts) == 0 && aws.StringValue(rc.Host) != "#{host}" {
			return false
		}

		if len(hosts) != 0 && aws.StringValue(rc.Host) != "#{host}" {
			hostMatches := false
			for _, host := range hosts {
				if aws.StringValue(rc.Host) == host {
					hostMatches = true
					break
				}
			}
			// it won't be redirect loop if none of the host condition matches
			if !hostMatches {
				return false
			}
		}

		if len(paths) == 0 && aws.StringValue(rc.Path) != "/#{path}" {
			return false
		}
		if len(paths) != 0 && aws.StringValue(rc.Path) != "/#{path}" {
			pathMatches := false
			for _, path := range paths {
				if aws.StringValue(rc.Path) == path {
					pathMatches = true
					break
				}
			}
			// it won't be redirect loop if none of the path condition matches
			if !pathMatches {
				return false
			}
		}

		if aws.StringValue(rc.Port) != "#{port}" && aws.StringValue(rc.Port) != fmt.Sprintf("%v", aws.Int64Value(listener.Port)) {
			return false
		}
		if aws.StringValue(rc.Query) != "#{query}" {
			return false
		}
		if aws.StringValue(rc.Protocol) != "#{protocol}" && aws.StringValue(rc.Protocol) != aws.StringValue(listener.Protocol) {
			return false
		}
		return true
	}
	return false
}

// isUnconditionalRedirect checks whether specified rule always redirects
// We consider the rule is a unconditional redirect if
// 1) The Path condition is nil, or at least one Path condition is /*
// 2) All other rule conditions are nil (ignoring the Host condition).
// 3) RedirectConfig is not nil.
func isUnconditionalRedirect(listener *elbv2.Listener, r elbv2.Rule) bool {
	for _, action := range r.Actions {
		rc := action.RedirectConfig
		if rc == nil {
			continue
		}

		var paths []string
		for _, c := range r.Conditions {
			switch aws.StringValue(c.Field) {
			case conditions.FieldPathPattern:
				paths = append(paths, aws.StringValueSlice(c.PathPatternConfig.Values)...)
			case conditions.FieldHTTPRequestMethod, conditions.FieldSourceIP, conditions.FieldHTTPHeader, conditions.FieldQueryString:
				// If there are any conditions, then the redirect is not unconditional
				return false
			}
		}

		if len(paths) != 0 {
			// ALB path conditions are ORed, so if any of them are a wildcard, the redirect is unconditional
			for _, path := range paths {
				if path == "/*" {
					return true
				}
			}
			// The redirect isn't unconditional if none of the path conditions are a wildcard
			return false
		}

		return true
	}
	return false
}
