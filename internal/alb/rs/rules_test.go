package rs

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func Test_NewRules(t *testing.T) {
	for _, tc := range []struct {
		Name    string
		Ingress *extensions.Ingress
	}{
		{
			Name:    "std params",
			Ingress: &extensions.Ingress{},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			output := NewRules(tc.Ingress)

			assert.Equal(t, tc.Ingress, output.Ingress)
		})
	}
}

func Test_priority(t *testing.T) {
	for _, tc := range []struct {
		Name     string
		Value    string
		Expected int64
	}{
		{
			Name:     "valid integer priority",
			Value:    "3",
			Expected: 3,
		},
		{
			Name:     "invalid priority",
			Value:    "invalid",
			Expected: 0,
		},
		{
			Name:     "valid default priority",
			Value:    "default",
			Expected: 0,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			o := priority(aws.String(tc.Value))
			assert.Equal(t, tc.Expected, aws.Int64Value(o))
		})
	}
}

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
			r := &Rule{}

			if tc.RedirectActionConfig != nil {
				r.Actions = append(r.Actions, &elbv2.Action{RedirectConfig: tc.RedirectActionConfig})
			}
			r.Conditions = tc.Conditions

			assert.Equal(t, tc.Expected, createsRedirectLoop(r, l))
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
	rules := NewRules(dummy.NewIngress())
	rules.ListenerArn = "listenerArn"
	listenerArn := aws.String(rules.ListenerArn)
	tgArn := aws.String("tgArn")

	for _, tc := range []struct {
		Name           string
		Rules          *Rules
		Current        []*Rule
		Desired        []*Rule
		CreateRuleCall *CreateRuleCall
		ModifyRuleCall *ModifyRuleCall
		DeleteRuleCall *DeleteRuleCall
		ExpectedError  error
	}{
		{
			Name:    "Empty ruleset for current and desired, no actions",
			Rules:   rules,
			Current: []*Rule{},
			Desired: []*Rule{},
		},
		{
			Name:  "Add one rule",
			Rules: rules,
			Current: []*Rule{
				{
					Rule: elbv2.Rule{
						Conditions: conditions(condition("path-pattern", "/*")),
						Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
						Priority:   aws.String("1")},
				},
			},
			Desired: []*Rule{
				{
					Rule: elbv2.Rule{
						Conditions: conditions(condition("path-pattern", "/*")),
						Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
						Priority:   aws.String("1")},
				},
				{
					Rule: elbv2.Rule{
						Conditions: conditions(condition("path-pattern", "/newPath/*")),
						Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
						Priority:   aws.String("2")},
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
			Rules:   rules,
			Current: []*Rule{},
			Desired: []*Rule{
				{
					Rule: elbv2.Rule{
						Conditions: conditions(condition("path-pattern", "/*")),
						Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
						Priority:   aws.String("1")},
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
			ExpectedError: errors.New("create rule error"),
		}, {
			Name:  "Remove one rule",
			Rules: rules,
			Current: []*Rule{
				{
					Rule: elbv2.Rule{
						Conditions: conditions(condition("path-pattern", "/*")),
						Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
						Priority:   aws.String("1")},
				},
				{
					Rule: elbv2.Rule{
						RuleArn:    aws.String("Rule arn"),
						Conditions: conditions(condition("path-pattern", "/newPath/*")),
						Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
						Priority:   aws.String("2")},
				},
			},
			Desired: []*Rule{
				{
					Rule: elbv2.Rule{
						Conditions: conditions(condition("path-pattern", "/*")),
						Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
						Priority:   aws.String("1")},
				},
			},
			DeleteRuleCall: &DeleteRuleCall{
				Input: &elbv2.DeleteRuleInput{
					RuleArn: aws.String("Rule arn"),
				},
			},
		},
		{
			Name:  "DeleteRule error",
			Rules: rules,
			Current: []*Rule{
				{
					Rule: elbv2.Rule{
						RuleArn:    aws.String("Rule arn"),
						Conditions: conditions(condition("path-pattern", "/*")),
						Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
						Priority:   aws.String("1")},
				},
			},
			Desired: []*Rule{},
			DeleteRuleCall: &DeleteRuleCall{
				Input: &elbv2.DeleteRuleInput{
					RuleArn: aws.String("Rule arn"),
				},
				Error: errors.New("delete rule error"),
			},
			ExpectedError: errors.New("delete rule error"),
		},
		{
			Name:  "Modify one rule",
			Rules: rules,
			Current: []*Rule{
				{
					Rule: elbv2.Rule{
						RuleArn:    aws.String("Rule arn"),
						Conditions: conditions(condition("path-pattern", "/*")),
						Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
						Priority:   aws.String("1")},
				},
			},
			Desired: []*Rule{
				{
					Rule: elbv2.Rule{
						Conditions: conditions(condition("path-pattern", "/new/*")),
						Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
						Priority:   aws.String("1")},
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
			Name:  "ModifyRule error",
			Rules: rules,
			Current: []*Rule{
				{
					Rule: elbv2.Rule{
						RuleArn:    aws.String("Rule arn"),
						Conditions: conditions(condition("path-pattern", "/*")),
						Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
						Priority:   aws.String("1")},
				},
			},
			Desired: []*Rule{
				{
					Rule: elbv2.Rule{
						Conditions: conditions(condition("path-pattern", "/new/*")),
						Actions:    actions(&elbv2.Action{TargetGroupArn: tgArn}, elbv2.ActionTypeEnumForward),
						Priority:   aws.String("1")},
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
			ExpectedError: errors.New("error modifying rule 1: modify rule error"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			store := &mocks.Storer{}
			elbv2svc := &mocks.ELBV2API{}
			if tc.CreateRuleCall != nil {
				elbv2svc.On("CreateRule", tc.CreateRuleCall.Input).Return(nil, tc.CreateRuleCall.Error)
			}
			if tc.ModifyRuleCall != nil {
				elbv2svc.On("ModifyRule", tc.ModifyRuleCall.Input).Return(nil, tc.ModifyRuleCall.Error)
			}
			if tc.DeleteRuleCall != nil {
				elbv2svc.On("DeleteRule", tc.DeleteRuleCall.Input).Return(nil, tc.DeleteRuleCall.Error)
			}

			controller := NewRulesController(elbv2svc, store)
			controller.getCurrentRulesFunc = func(string) ([]*Rule, error) { return tc.Current, nil }
			controller.getDesiredRulesFunc = func(*extensions.Ingress, tg.TargetGroups, *elbv2.Listener) ([]*Rule, error) { return tc.Desired, nil }

			err := controller.Reconcile(context.Background(), tc.Rules, &elbv2.Listener{})

			if tc.ExpectedError != nil {
				assert.Equal(t, tc.ExpectedError, err)
			} else {
				assert.NoError(t, err)
			}
			elbv2svc.AssertExpectations(t)
			store.AssertExpectations(t)

		})
	}
}

func rulesWithTg(rules *Rules, tg tg.TargetGroups) *Rules {
	rules.TargetGroups = tg
	return rules
}

type GetRulesCall struct {
	Output []*elbv2.Rule
	Error  error
}

type DescribeTagsCall struct {
	Output *elbv2.DescribeTagsOutput
	Error  error
}

func tag(k, v string) *elbv2.Tag {
	return &elbv2.Tag{Key: aws.String(k), Value: aws.String(v)}
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
	i := dummy.NewIngress()
	i.Spec.Rules = rules
	return i
}

type GetIngressAnnotationsCall struct {
	Annotations *annotations.Ingress
	Error       error
}

func Test_getDesiredRules(t *testing.T) {
	for _, tc := range []struct {
		Name                      string
		Ingress                   *extensions.Ingress
		TargetGroups              tg.TargetGroups
		GetIngressAnnotationsCall *GetIngressAnnotationsCall
		Expected                  []*Rule
		ExpectedError             error
	}{
		{
			Name:          "No paths in ingress",
			Ingress:       ingRules(ingRule()),
			ExpectedError: errors.New("ingress doesn't have any paths defined"),
		},
		{
			Name: "One path with an annotation backed service",
			Ingress: ingRules(ingRule(extensions.HTTPIngressPath{
				Path:    "/*",
				Backend: backend("fixed-response-action", intstr.FromString("use-annotation")),
			})),
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Annotations: annotations.NewIngressDummy(),
			},
			Expected: []*Rule{
				{
					Rule: elbv2.Rule{
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
					Backend: backend("fixed-response-action", intstr.FromString("use-annotation")),
				},
			},
		},
		{
			Name: "Get Ingress annotation error",
			Ingress: ingRules(ingRule(extensions.HTTPIngressPath{
				Path:    "/*",
				Backend: backend("fixed-response-action", intstr.FromString("use-annotation")),
			})),
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Error: errors.New("error"),
			},
			ExpectedError: errors.New("error"),
		},
		{
			Name: "Action annotation refers to invalid action",
			Ingress: ingRules(ingRule(extensions.HTTPIngressPath{
				Path:    "/*",
				Backend: backend("missing-service", intstr.FromString("use-annotation")),
			})),
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Annotations: annotations.NewIngressDummy(),
			},
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
			TargetGroups: tg.TargetGroups{
				&tg.TargetGroup{SvcName: "service2", SvcPort: intstr.FromString("443")},
				&tg.TargetGroup{SvcName: "service1", SvcPort: intstr.FromString("http")},
			},
			Expected: []*Rule{
				{
					Rule: elbv2.Rule{
						IsDefault:  aws.Bool(false),
						Conditions: conditions(condition("path-pattern", "/path1/*")),
						Actions:    actions(&elbv2.Action{}, "forward"),
						Priority:   aws.String("1")},
					Backend: backend("service1", intstr.FromString("http")),
				},
				{
					Rule: elbv2.Rule{
						IsDefault: aws.Bool(false),
						Conditions: conditions(
							condition("host-header", "hostname"),
							condition("path-pattern", "/path2/*"),
						),
						Actions:  actions(&elbv2.Action{}, "forward"),
						Priority: aws.String("2")},
					Backend: backend("service2", intstr.FromString("443")),
				},
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			elbv2svc := &mocks.ELBV2API{}
			store := &mocks.Storer{}
			if tc.GetIngressAnnotationsCall != nil {
				store.On("GetIngressAnnotations", k8s.MetaNamespaceKey(tc.Ingress)).Return(tc.GetIngressAnnotationsCall.Annotations, tc.GetIngressAnnotationsCall.Error)
			}
			controller := &rulesController{
				elbv2: elbv2svc,
				store: store,
			}
			results, err := controller.getDesiredRules(tc.Ingress, tc.TargetGroups, &elbv2.Listener{})
			assert.Equal(t, tc.Expected, results)
			assert.Equal(t, tc.ExpectedError, err)
			elbv2svc.AssertExpectations(t)
			store.AssertExpectations(t)
		})
	}
}

func Test_getCurrentRules(t *testing.T) {
	listenerArn := "listenerArn"
	tgArn := "tgArn"

	for _, tc := range []struct {
		Name             string
		GetRulesCall     *GetRulesCall
		DescribeTagsCall *DescribeTagsCall
		Expected         []*Rule
		ExpectedError    error
	}{
		{
			Name:          "DescribeRulesRequest returns an error",
			GetRulesCall:  &GetRulesCall{Output: nil, Error: errors.New("Some error")},
			ExpectedError: errors.New("Some error"),
		},
		{
			Name: "DescribeTags returns an error",
			GetRulesCall: &GetRulesCall{Output: []*elbv2.Rule{
				{
					Priority:   aws.String("1"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: conditions(condition("path-pattern", "/*")),
				},
			}},
			DescribeTagsCall: &DescribeTagsCall{
				Error: errors.New("Some error"),
			},
			ExpectedError: errors.New("Some error"),
		},
		{
			Name: "DescribeRulesRequest returns one rule without any actions",
			GetRulesCall: &GetRulesCall{Output: []*elbv2.Rule{
				{
					Priority:   aws.String("1"),
					Conditions: conditions(condition("path-pattern", "/*")),
				},
			}},
			ExpectedError: errors.New("invalid amount of actions on rule for listener listenerArn"),
		}, {
			Name: "DescribeRulesRequest returns one rule",
			GetRulesCall: &GetRulesCall{Output: []*elbv2.Rule{
				{
					Priority:   aws.String("1"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: conditions(condition("path-pattern", "/*")),
				},
			}},
			DescribeTagsCall: &DescribeTagsCall{Output: &elbv2.DescribeTagsOutput{TagDescriptions: []*elbv2.TagDescription{
				{
					ResourceArn: aws.String(tgArn),
					Tags: []*elbv2.Tag{
						tag(tags.ServiceName, "ServiceName"),
						tag(tags.ServicePort, "http"),
					},
				},
			}}},
			Expected: []*Rule{
				{
					Rule: elbv2.Rule{
						Priority:   aws.String("1"),
						Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
						Conditions: conditions(condition("path-pattern", "/*")),
					},
					Backend: extensions.IngressBackend{
						ServiceName: "ServiceName",
						ServicePort: intstr.FromString("http"),
					},
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
			DescribeTagsCall: &DescribeTagsCall{Output: &elbv2.DescribeTagsOutput{TagDescriptions: []*elbv2.TagDescription{
				{
					ResourceArn: aws.String(tgArn),
					Tags: []*elbv2.Tag{
						tag(tags.ServiceName, "ServiceName"),
						tag(tags.ServicePort, "http"),
					},
				},
			}}},
			Expected: []*Rule{
				{
					Rule: elbv2.Rule{
						Priority: aws.String("1"),
						Actions:  []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
						Conditions: []*elbv2.RuleCondition{
							condition("path-pattern", "/*"),
						},
					},
					Backend: extensions.IngressBackend{
						ServiceName: "ServiceName",
						ServicePort: intstr.FromString("http"),
					},
				},
				{
					Rule: elbv2.Rule{
						Priority:   aws.String("3"),
						Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
						Conditions: conditions(condition("path-pattern", "/2*")),
					},
					Backend: extensions.IngressBackend{
						ServiceName: "ServiceName",
						ServicePort: intstr.FromString("http"),
					},
				},
				{
					Rule: elbv2.Rule{
						Priority:   aws.String("4"),
						Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumFixedResponse)}},
						Conditions: conditions(condition("path-pattern", "/3*")),
					},
					Backend: extensions.IngressBackend{
						ServicePort: intstr.FromString(action.UseActionAnnotation),
					},
				},
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			elbv2svc := &mocks.ELBV2API{}
			if tc.GetRulesCall != nil {
				elbv2svc.On("GetRules", listenerArn).Return(tc.GetRulesCall.Output, tc.GetRulesCall.Error)
			}
			if tc.DescribeTagsCall != nil {
				elbv2svc.On("DescribeTags",
					&elbv2.DescribeTagsInput{ResourceArns: []*string{aws.String(tgArn)}}).Return(tc.DescribeTagsCall.Output, tc.DescribeTagsCall.Error)
			}

			store := &mocks.Storer{}
			controller := &rulesController{
				elbv2: elbv2svc,
				store: store,
			}
			results, err := controller.getCurrentRules(listenerArn)
			assert.Equal(t, tc.Expected, results)
			assert.Equal(t, tc.ExpectedError, err)
			elbv2svc.AssertExpectations(t)
			store.AssertExpectations(t)
		})
	}
}
