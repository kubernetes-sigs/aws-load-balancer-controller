package ls

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"

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

		for _, path := range ingressRule.HTTP.Paths {
			authCfg, err := c.authModule.NewConfig(ctx, ingress, path.Backend, aws.StringValue(listener.Protocol))
			if err != nil {
				return nil, err
			}
			elbActions, err := buildActions(ctx, authCfg, ingressAnnos, path.Backend, tgGroup)
			if err != nil {
				return nil, err
			}
			elbConditions := buildConditions(ctx, ingressRule, path)
			elbRule := elbv2.Rule{
				IsDefault:  aws.Bool(false),
				Priority:   aws.String(strconv.Itoa(nextPriority)),
				Actions:    elbActions,
				Conditions: elbConditions,
			}
			if createsRedirectLoop(listener, elbRule) {
				continue
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
	var actions []*elbv2.Action

	// Handle auth actions
	authAction := buildAuthAction(ctx, authCfg)
	if authAction != nil {
		actions = append(actions, authAction)
	}

	// Handle backend actions
	if action.Use(backend.ServicePort.String()) {
		// backend is based on annotation
		backendAction, err := ingressAnnos.Action.GetAction(backend.ServiceName)
		if err != nil {
			return nil, err
		}
		actions = append(actions, &backendAction)
	} else {
		// backend is based on service
		targetGroup, ok := tgGroup.TGByBackend[backend]
		if !ok {
			return nil, fmt.Errorf("unable to find targetGroup for backend %v:%v",
				backend.ServiceName, backend.ServicePort.String())
		}
		backendAction := elbv2.Action{
			Type:           aws.String(elbv2.ActionTypeEnumForward),
			TargetGroupArn: aws.String(targetGroup.Arn),
		}
		actions = append(actions, &backendAction)
	}

	for index, action := range actions {
		action.Order = aws.Int64(int64(index) + 1)
	}
	return actions, nil
}

// buildConditions will build listener rule conditions for specific ingressRule
func buildConditions(ctx context.Context, rule extensions.IngressRule, path extensions.HTTPIngressPath) []*elbv2.RuleCondition {
	var conditions []*elbv2.RuleCondition
	if rule.Host != "" {
		conditions = append(conditions, condition("host-header", rule.Host))
	}
	if path.Path != "" {
		conditions = append(conditions, condition("path-pattern", path.Path))
	} else if len(conditions) == 0 {
		conditions = append(conditions, condition("path-pattern", "/*"))
	}
	return conditions
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

		sortConditions(currentRule.Conditions)
		sortConditions(desiredRule.Conditions)
		sortActions(currentRule.Actions)
		sortActions(desiredRule.Actions)
		if !reflect.DeepEqual(currentRule, desiredRule) {
			modify = append(modify, desiredRule)
		}
	}
	return add, modify, remove
}

func condition(field string, values ...string) *elbv2.RuleCondition {
	return &elbv2.RuleCondition{
		Field:  aws.String(field),
		PathPatternConfig: *elbv2.PathPatternConditionConfig{
			Values: aws.StringSlice(values),
		},
		Values: aws.StringSlice(values),
	}
}

func sortConditions(conditions []*elbv2.RuleCondition) {
	for _, cond := range conditions {
		sort.Slice(cond.Values, func(i, j int) bool { return aws.StringValue(cond.Values[i]) < aws.StringValue(cond.Values[j]) })
	}

	sort.Slice(conditions, func(i, j int) bool {
		return aws.StringValue(conditions[i].Field) < aws.StringValue(conditions[j].Field)
	})
}

func sortActions(actions []*elbv2.Action) {
	sort.Slice(actions, func(i, j int) bool {
		return aws.Int64Value(actions[i].Order) < aws.Int64Value(actions[j].Order)
	})
}

// createsRedirectLoop checks whether specified rule creates redirectionLoop for listener of protocol & port
func createsRedirectLoop(listener *elbv2.Listener, r elbv2.Rule) bool {
	for _, action := range r.Actions {
		var host, path *string
		rc := action.RedirectConfig
		if rc == nil {
			continue
		}

		for _, c := range r.Conditions {
			if aws.StringValue(c.Field) == "host-header" {
				host = c.Values[0]
			}
			if aws.StringValue(c.Field) == "path-pattern" {
				path = c.Values[0]
			}
		}

		if host == nil && aws.StringValue(rc.Host) != "#{host}" {
			return false
		}
		if host != nil && aws.StringValue(rc.Host) != aws.StringValue(host) && aws.StringValue(rc.Host) != "#{host}" {
			return false
		}
		if path == nil && aws.StringValue(rc.Path) != "/#{path}" {
			return false
		}
		if path != nil && aws.StringValue(rc.Path) != aws.StringValue(path) && aws.StringValue(rc.Path) != "/#{path}" {
			return false
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
