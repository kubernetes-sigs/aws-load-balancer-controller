package rs

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func actionConfig(b *elbv2.RedirectActionConfig) *elbv2.RedirectActionConfig {
	r := &elbv2.RedirectActionConfig{
		Host:       aws.String("#{host}"),
		Path:       aws.String("/#{path}"),
		Port:       aws.String("#{port}"),
		Protocol:   aws.String("#{protocol}"),
		Query:      aws.String("#{query}"),
		StatusCode: aws.String("301"),
	}
	if b.Host != nil {
		r.Host = b.Host
	}
	if b.Path != nil {
		r.Path = b.Path
	}
	if b.Port != nil {
		r.Port = b.Port
	}
	if b.Protocol != nil {
		r.Protocol = b.Protocol
	}
	if b.Query != nil {
		r.Query = b.Query
	}
	if b.StatusCode != nil {
		r.StatusCode = b.StatusCode
	}
	return r
}

func conditions(conditions ...*elbv2.RuleCondition) []*elbv2.RuleCondition {
	return conditions
}

func backend(serviceName string, servicePort intstr.IntOrString) extensions.IngressBackend {
	return extensions.IngressBackend{
		ServiceName: serviceName,
		ServicePort: servicePort,
	}
}

func actions(a *elbv2.Action, t string) []*elbv2.Action {
	a.Type = aws.String(t)
	return []*elbv2.Action{a}
}

func Test_createsRedirectLoop(t *testing.T) {
	l := &elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)}

	for _, tc := range []struct {
		Name                 string
		Expected             bool
		RedirectActionConfig *elbv2.RedirectActionConfig
		Conditions           []*elbv2.RuleCondition
	}{
		{
			Name:     "No RedirectConfig",
			Expected: false,
		},
		{
			Name:                 "Host variable set to new hostname (no host-header)",
			Expected:             false,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Host: aws.String("new.hostname")}),
		},
		{
			Name:                 "Host variable set to new hostname (with host-header)",
			Expected:             false,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Host: aws.String("new.hostname")}),
			Conditions:           conditions(condition("host-header", "old.hostname")),
		},
		{
			Name:                 "Host variable set to #{host}",
			Expected:             true,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Host: aws.String("#{host}")}),
		},
		{
			Name:                 "Host variable set to same value as host-header",
			Expected:             true,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Host: aws.String("reused.hostname")}),
			Conditions:           conditions(condition("host-header", "reused.hostname")),
		},
		{
			Name:                 "Path variable set to new path",
			Expected:             false,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/newpath")}),
		},
		{
			Name:                 "Path variable set to /#{path}",
			Expected:             true,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/#{path}")}),
		},
		{
			Name:                 "Path variable set to different value as path-pattern",
			Expected:             false,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/newpath")}),
			Conditions:           conditions(condition("path-pattern", "/reused.path")),
		},
		{
			Name:                 "Path variable set to same value as path-pattern",
			Expected:             true,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/reused.path")}),
			Conditions:           conditions(condition("path-pattern", "/reused.path")),
		},
		{
			Name:                 "Port variable set to new port",
			Expected:             false,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Port: aws.String("999")}),
		},
		{
			Name:                 "Port variable set to #{port}",
			Expected:             true,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Port: aws.String("#{port}")}),
		},
		{
			Name:                 "Port variable set to listener port",
			Expected:             true,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Port: aws.String(fmt.Sprintf("%v", aws.Int64Value(l.Port)))}),
		},
		{
			Name:                 "Query variable set to new query",
			Expected:             false,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Query: aws.String("new query")}),
		},
		{
			Name:                 "Query variable set to #{query}",
			Expected:             true,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Query: aws.String("#{query}")}),
		},
		{
			Name:                 "Protocol variable set to new protocol",
			Expected:             false,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Protocol: aws.String("HTTPS")}),
		},
		{
			Name:                 "Protocol variable set to #{protocol}",
			Expected:             true,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Protocol: aws.String("#{protocol}")}),
		},
		{
			Name:                 "Protocol variable set to the same protocol",
			Expected:             true,
			RedirectActionConfig: actionConfig(&elbv2.RedirectActionConfig{Protocol: l.Protocol}),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			r := elbv2.Rule{}
			if tc.RedirectActionConfig != nil {
				r.Actions = append(r.Actions, &elbv2.Action{RedirectConfig: tc.RedirectActionConfig})
			}
			r.Conditions = tc.Conditions

			assert.Equal(t, tc.Expected, createsRedirectLoop(l, r))
		})
	}
}

func Test_condition(t *testing.T) {
	for _, tc := range []struct {
		Name     string
		Field    string
		Values   []string
		Expected *elbv2.RuleCondition
	}{
		{
			Name:     "one value",
			Field:    "field name",
			Values:   []string{"val1"},
			Expected: &elbv2.RuleCondition{Field: aws.String("field name"), Values: aws.StringSlice([]string{"val1"})},
		},
		{
			Name:     "three Values",
			Field:    "field name",
			Values:   []string{"val1", "val2", "val3"},
			Expected: &elbv2.RuleCondition{Field: aws.String("field name"), Values: aws.StringSlice([]string{"val1", "val2", "val3"})},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			o := condition(tc.Field, tc.Values...)
			assert.Equal(t, tc.Expected, o)
		})
	}
}

type CreateRuleCall struct {
	Input *elbv2.CreateRuleInput
	Error error
}

type ModifyRuleCall struct {
	Input *elbv2.ModifyRuleInput
	Error error
}

type DeleteRuleCall struct {
	Input *elbv2.DeleteRuleInput
	Error error
}

func Test_Reconcile(t *testing.T) {
	listenerArn := aws.String("lsArn")
	tgArn := aws.String("tgArn")
	for _, tc := range []struct {
		Name           string
		Current        []elbv2.Rule
		Desired        []elbv2.Rule
		CreateRuleCall *CreateRuleCall
		ModifyRuleCall *ModifyRuleCall
		DeleteRuleCall *DeleteRuleCall
		ExpectedError  error
	}{
		{
			Name:    "Empty ruleset for current and desired, no actions",
			Current: []elbv2.Rule{},
			Desired: []elbv2.Rule{},
		},
		{
			Name: "Add one rule",
			Current: []elbv2.Rule{
				{

					Conditions: conditions(condition("path-pattern", "/*")),
					Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
					Priority:   aws.String("1"),
				},
			},
			Desired: []elbv2.Rule{
				{

					Conditions: conditions(condition("path-pattern", "/*")),
					Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
					Priority:   aws.String("1"),
				},
				{

					Conditions: conditions(condition("path-pattern", "/newPath/*")),
					Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
					Priority:   aws.String("2"),
				},
			},
			CreateRuleCall: &CreateRuleCall{
				Input: &elbv2.CreateRuleInput{
					ListenerArn: listenerArn,
					Priority:    aws.Int64(2),
					Conditions:  conditions(condition("path-pattern", "/newPath/*")),
					Actions:     actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
				},
			},
		},
		{
			Name:    "CreateRule error",
			Current: []elbv2.Rule{},
			Desired: []elbv2.Rule{
				{

					Conditions: conditions(condition("path-pattern", "/*")),
					Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
					Priority:   aws.String("1"),
				},
			},
			CreateRuleCall: &CreateRuleCall{
				Input: &elbv2.CreateRuleInput{
					ListenerArn: listenerArn,
					Priority:    aws.Int64(1),
					Conditions:  conditions(condition("path-pattern", "/*")),
					Actions:     actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
				},
				Error: errors.New("create rule error"),
			},
			ExpectedError: errors.New("failed creating rule 1 on lsArn due to create rule error"),
		}, {
			Name: "Remove one rule",
			Current: []elbv2.Rule{
				{

					Conditions: conditions(condition("path-pattern", "/*")),
					Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
					Priority:   aws.String("1"),
				},
				{

					RuleArn:    aws.String("Rule arn"),
					Conditions: conditions(condition("path-pattern", "/newPath/*")),
					Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
					Priority:   aws.String("2"),
				},
			},
			Desired: []elbv2.Rule{
				{
					Conditions: conditions(condition("path-pattern", "/*")),
					Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
					Priority:   aws.String("1"),
				},
			},
			DeleteRuleCall: &DeleteRuleCall{
				Input: &elbv2.DeleteRuleInput{
					RuleArn: aws.String("Rule arn"),
				},
			},
		},
		{
			Name: "DeleteRule error",
			Current: []elbv2.Rule{
				{
					RuleArn:    aws.String("Rule arn"),
					Conditions: conditions(condition("path-pattern", "/*")),
					Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
					Priority:   aws.String("1"),
				},
			},
			DeleteRuleCall: &DeleteRuleCall{
				Input: &elbv2.DeleteRuleInput{
					RuleArn: aws.String("Rule arn"),
				},
				Error: errors.New("delete rule error"),
			},
			ExpectedError: errors.New("failed deleting rule 1 on lsArn due to delete rule error"),
		},
		{
			Name: "Modify one rule",
			Current: []elbv2.Rule{
				{
					RuleArn:    aws.String("Rule arn"),
					Conditions: conditions(condition("path-pattern", "/*")),
					Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
					Priority:   aws.String("1"),
				},
			},
			Desired: []elbv2.Rule{
				{
					Conditions: conditions(condition("path-pattern", "/new/*")),
					Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
					Priority:   aws.String("1"),
				},
			},
			ModifyRuleCall: &ModifyRuleCall{
				Input: &elbv2.ModifyRuleInput{
					RuleArn:    aws.String("Rule arn"),
					Conditions: conditions(condition("path-pattern", "/new/*")),
					Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
				},
			},
		},
		{
			Name: "ModifyRule error",
			Current: []elbv2.Rule{
				{
					RuleArn:    aws.String("Rule arn"),
					Conditions: conditions(condition("path-pattern", "/*")),
					Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
					Priority:   aws.String("1"),
				},
			},
			Desired: []elbv2.Rule{
				{
					Conditions: conditions(condition("path-pattern", "/new/*")),
					Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
					Priority:   aws.String("1"),
				},
			},
			ModifyRuleCall: &ModifyRuleCall{
				Input: &elbv2.ModifyRuleInput{
					RuleArn:    aws.String("Rule arn"),
					Conditions: conditions(condition("path-pattern", "/new/*")),
					Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
				},
				Error: errors.New("modify rule error"),
			},
			ExpectedError: errors.New("failed modifying rule 1 on lsArn due to modify rule error"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			cloud := &mocks.CloudAPI{}
			if tc.CreateRuleCall != nil {
				cloud.On("CreateRuleWithContext", ctx, tc.CreateRuleCall.Input).Return(nil, tc.CreateRuleCall.Error)
			}
			if tc.ModifyRuleCall != nil {
				cloud.On("ModifyRuleWithContext", ctx, tc.ModifyRuleCall.Input).Return(nil, tc.ModifyRuleCall.Error)
			}
			if tc.DeleteRuleCall != nil {
				cloud.On("DeleteRuleWithContext", ctx, tc.DeleteRuleCall.Input).Return(nil, tc.DeleteRuleCall.Error)
			}

			controller := &defaultController{
				cloud:               cloud,
				getCurrentRulesFunc: func(context.Context, string) ([]elbv2.Rule, error) { return tc.Current, nil },
				getDesiredRulesFunc: func(*elbv2.Listener, *extensions.Ingress, *annotations.Ingress, tg.TargetGroupGroup) ([]elbv2.Rule, error) {
					return tc.Desired, nil
				},
			}
			err := controller.Reconcile(context.Background(), &elbv2.Listener{ListenerArn: listenerArn}, &extensions.Ingress{}, &annotations.Ingress{}, tg.TargetGroupGroup{})
			if tc.ExpectedError != nil {
				assert.Equal(t, tc.ExpectedError, err)
			} else {
				assert.NoError(t, err)
			}
			cloud.AssertExpectations(t)
		})
	}
}

type GetRulesCall struct {
	Output []*elbv2.Rule
	Error  error
}

func ingRule(paths ...extensions.HTTPIngressPath) extensions.IngressRule {
	return extensions.IngressRule{
		IngressRuleValue: extensions.IngressRuleValue{
			HTTP: &extensions.HTTPIngressRuleValue{
				Paths: paths,
			},
		},
	}
}

func ingHost(r extensions.IngressRule, host string) extensions.IngressRule {
	r.Host = host
	return r
}

func ingRules(rules ...extensions.IngressRule) *extensions.Ingress {
	i := &extensions.Ingress{}
	i.Spec.Rules = rules
	return i
}

func Test_getDesiredRules(t *testing.T) {
	for _, tc := range []struct {
		Name          string
		Ingress       *extensions.Ingress
		IngressAnnos  *annotations.Ingress
		TargetGroups  tg.TargetGroupGroup
		Expected      []elbv2.Rule
		ExpectedError error
	}{
		{
			Name:          "No paths in ingress",
			Ingress:       ingRules(ingRule()),
			ExpectedError: nil,
		},
		{
			Name: "One path with an annotation backed service",
			Ingress: ingRules(ingRule(extensions.HTTPIngressPath{
				Path:    "/*",
				Backend: backend("fixed-response-action", intstr.FromString("use-annotation")),
			})),
			IngressAnnos: annotations.NewIngressDummy(),
			Expected: []elbv2.Rule{
				{
					IsDefault:  aws.Bool(false),
					Conditions: conditions(condition("path-pattern", "/*")),
					Actions: actions(
						&elbv2.Action{
							FixedResponseConfig: &elbv2.FixedResponseActionConfig{
								ContentType: aws.String("text/plain"),
								StatusCode:  aws.String("503"),
								MessageBody: aws.String("message body"),
							},
						}, "fixed-response"),
					Priority: aws.String("1")},
			},
		},
		{
			Name: "Action annotation refers to invalid action",
			Ingress: ingRules(ingRule(extensions.HTTPIngressPath{
				Path:    "/*",
				Backend: backend("missing-service", intstr.FromString("use-annotation")),
			})),
			IngressAnnos:  annotations.NewIngressDummy(),
			ExpectedError: errors.New("backend with `servicePort: use-annotation` was configured with `serviceName: missing-service` but an action annotation for missing-service is not set"),
		},
		{
			Name: "No target group for the selected service",
			Ingress: ingRules(
				ingRule(extensions.HTTPIngressPath{
					Path:    "/path1/*",
					Backend: backend("service1", intstr.FromString("http")),
				}),
			),
			ExpectedError: errors.New("unable to locate a target group for backend service1:http"),
		},
		{
			Name: "Two paths",
			Ingress: ingRules(
				ingRule(extensions.HTTPIngressPath{
					Path:    "/path1/*",
					Backend: backend("service1", intstr.FromString("http")),
				}),
				ingHost(ingRule(extensions.HTTPIngressPath{
					Path:    "/path2/*",
					Backend: backend("service2", intstr.FromString("443")),
				}), "hostname")),
			TargetGroups: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{ServiceName: "service1", ServicePort: intstr.FromString("http")}: {Arn: "arn1"},
					{ServiceName: "service2", ServicePort: intstr.FromString("443")}:  {Arn: "arn2"},
				},
			},
			Expected: []elbv2.Rule{
				{

					IsDefault:  aws.Bool(false),
					Conditions: conditions(condition("path-pattern", "/path1/*")),
					Actions:    actions(&elbv2.Action{TargetGroupArn: aws.String("arn1")}, "forward"),
					Priority:   aws.String("1"),
				},
				{
					IsDefault: aws.Bool(false),
					Conditions: conditions(
						condition("host-header", "hostname"),
						condition("path-pattern", "/path2/*"),
					),
					Actions:  actions(&elbv2.Action{TargetGroupArn: aws.String("arn2")}, "forward"),
					Priority: aws.String("2"),
				},
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			cloud := &mocks.CloudAPI{}
			controller := &defaultController{
				cloud: cloud,
			}
			results, err := controller.getDesiredRules(&elbv2.Listener{}, tc.Ingress, tc.IngressAnnos, tc.TargetGroups)
			assert.Equal(t, tc.Expected, results)
			assert.Equal(t, tc.ExpectedError, err)
			cloud.AssertExpectations(t)
		})
	}
}

func Test_getCurrentRules(t *testing.T) {
	listenerArn := "listenerArn"
	tgArn := "tgArn"

	for _, tc := range []struct {
		Name          string
		GetRulesCall  *GetRulesCall
		Expected      []elbv2.Rule
		ExpectedError error
	}{
		{
			Name:          "DescribeRulesRequest returns an error",
			GetRulesCall:  &GetRulesCall{Output: nil, Error: errors.New("Some error")},
			ExpectedError: errors.New("Some error"),
		},
		{
			Name: "DescribeRulesRequest returns one rule",
			GetRulesCall: &GetRulesCall{Output: []*elbv2.Rule{
				{
					Priority:   aws.String("1"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: conditions(condition("path-pattern", "/*")),
				},
			}},
			Expected: []elbv2.Rule{
				{

					Priority:   aws.String("1"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: conditions(condition("path-pattern", "/*")),
				},
			},
		},
		{
			Name: "DescribeRulesRequest returns four rules, default rule is ignored",
			GetRulesCall: &GetRulesCall{Output: []*elbv2.Rule{
				{
					Priority:   aws.String("default"),
					IsDefault:  aws.Bool(true),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: conditions(condition("path-pattern", "/*")),
				},
				{
					Priority:   aws.String("1"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: conditions(condition("path-pattern", "/*")),
				},
				{
					Priority:   aws.String("3"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: conditions(condition("path-pattern", "/2*")),
				},
				{
					Priority:   aws.String("4"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumFixedResponse)}},
					Conditions: conditions(condition("path-pattern", "/3*")),
				},
			}},
			Expected: []elbv2.Rule{
				{

					Priority: aws.String("1"),
					Actions:  []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: []*elbv2.RuleCondition{
						condition("path-pattern", "/*"),
					},
				},
				{

					Priority:   aws.String("3"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: conditions(condition("path-pattern", "/2*")),
				},
				{

					Priority:   aws.String("4"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumFixedResponse)}},
					Conditions: conditions(condition("path-pattern", "/3*")),
				},
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			cloud := &mocks.CloudAPI{}
			if tc.GetRulesCall != nil {
				cloud.On("GetRules", ctx, listenerArn).Return(tc.GetRulesCall.Output, tc.GetRulesCall.Error)
			}
			controller := &defaultController{
				cloud: cloud,
			}
			results, err := controller.getCurrentRules(ctx, listenerArn)
			assert.Equal(t, tc.Expected, results)
			assert.Equal(t, tc.ExpectedError, err)
			cloud.AssertExpectations(t)
		})
	}
}
