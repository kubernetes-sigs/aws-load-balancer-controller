package rs

import (
	"context"
	"errors"
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

			assert.Equal(t, output.Ingress, tc.Ingress)
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
			assert.Equal(t, aws.Int64Value(o), tc.Expected)
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
			assert.Equal(t, o, tc.Expected)
		})
	}
}

func Test_Reconcile(t *testing.T) {
	rules := NewRules(dummy.NewIngress())
	rules.ListenerArn = "listenerArn"

	for _, tc := range []struct {
		Name          string
		Rules         *Rules
		Current       []*Rule
		Desired       []*Rule
		ExpectedError error
	}{
		{
			Name:    "Empty ruleset for current and desired, no actions",
			Rules:   rules,
			Current: []*Rule{},
			Desired: []*Rule{},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			elbv2svc := &mocks.ELBV2API{}
			store := &mocks.Storer{}

			controller := NewRulesController(elbv2svc, store)
			controller.getCurrentRulesFunc = func(string) ([]*Rule, error) { return tc.Current, nil }
			controller.getDesiredRulesFunc = func(*extensions.Ingress, tg.TargetGroups) ([]*Rule, error) { return tc.Desired, nil }

			err := controller.Reconcile(context.Background(), tc.Rules)

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
						Conditions: conditions(condition("path-pattern", "/path1/*")),
						Actions:    actions(&elbv2.Action{}, "forward"),
						Priority:   aws.String("1")},
					Backend: backend("service1", intstr.FromString("http")),
				},
				{
					Rule: elbv2.Rule{
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
			results, err := controller.getDesiredRules(tc.Ingress, tc.TargetGroups)
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
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
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
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
				},
			}},
			ExpectedError: errors.New("invalid amount of actions on rule for listener listenerArn"),
		}, {
			Name: "DescribeRulesRequest returns one rule",
			GetRulesCall: &GetRulesCall{Output: []*elbv2.Rule{
				{
					Priority:   aws.String("1"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
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
			},
		},
		{
			Name: "DescribeRulesRequest returns four rules, default rule is ignored",
			GetRulesCall: &GetRulesCall{Output: []*elbv2.Rule{
				{
					Priority:   aws.String("default"),
					IsDefault:  aws.Bool(true),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
				},
				{
					Priority:   aws.String("1"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
				},
				{
					Priority:   aws.String("3"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/2*")},
				},
				{
					Priority:   aws.String("4"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumFixedResponse)}},
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/3*")},
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
						Priority: aws.String("3"),
						Actions:  []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
						Conditions: []*elbv2.RuleCondition{
							condition("path-pattern", "/2*"),
						},
					},
					Backend: extensions.IngressBackend{
						ServiceName: "ServiceName",
						ServicePort: intstr.FromString("http"),
					},
				},
				{
					Rule: elbv2.Rule{
						Priority: aws.String("4"),
						Actions:  []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumFixedResponse)}},
						Conditions: []*elbv2.RuleCondition{
							condition("path-pattern", "/3*"),
						},
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
