package rs

import (
	"fmt"
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

func TestNewDesiredRule(t *testing.T) {
	cases := []struct {
		Priority     int
		Hostname     string
		Path         string
		SvcName      string
		SvcPort      intstr.IntOrString
		TargetPort   int
		Ingress      *extensions.Ingress
		Store        store.Storer
		ExpectedRule Rule
	}{
		{
			Priority:   1,
			SvcName:    "fixed-response-action",
			SvcPort:    intstr.FromString(action.UseActionAnnotation),
			Ingress:    dummy.NewIngress(),
			Store:      store.NewDummy(),
			TargetPort: 0,
			ExpectedRule: Rule{
				svc: svc{desired: service{name: "fixed-response-action", port: intstr.FromString(action.UseActionAnnotation)}},
				rs: rs{
					desired: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Actions: []*elbv2.Action{
							{
								Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
								FixedResponseConfig: &elbv2.FixedResponseActionConfig{
									ContentType: aws.String("text/plain"),
									StatusCode:  aws.String("503"),
									MessageBody: aws.String("message body"),
								},
							},
						},
					},
				},
			},
		},
		{
			Priority:   1,
			SvcName:    "redirect",
			SvcPort:    intstr.FromString(action.UseActionAnnotation),
			Ingress:    dummy.NewIngress(),
			Store:      store.NewDummy(),
			TargetPort: 0,
			ExpectedRule: Rule{
				svc: svc{desired: service{name: "redirect", port: intstr.FromString(action.UseActionAnnotation)}},
				rs: rs{
					desired: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Actions: []*elbv2.Action{
							{
								Type: aws.String(elbv2.ActionTypeEnumRedirect),
								RedirectConfig: &elbv2.RedirectActionConfig{
									Host:       aws.String("#{host}"),
									Path:       aws.String("/#{path}"),
									Port:       aws.String("#{port}"),
									Query:      aws.String("#{query}"),
									Protocol:   aws.String(elbv2.ProtocolEnumHttps),
									StatusCode: aws.String(elbv2.RedirectActionStatusCodeEnumHttp301),
								},
							},
						},
					},
				},
			},
		},
		{
			Priority:   1,
			SvcName:    "namespace-service",
			SvcPort:    intstr.FromInt(8080),
			TargetPort: 8080,
			ExpectedRule: Rule{
				svc: svc{desired: service{name: "namespace-service", port: intstr.FromInt(8080)}},
				rs: rs{
					desired: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Actions:   []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
					},
				},
			},
		},
		{
			Priority:   1,
			Path:       "/path",
			SvcName:    "namespace-service",
			SvcPort:    intstr.FromInt(8080),
			TargetPort: 8080,
			ExpectedRule: Rule{
				svc: svc{desired: service{name: "namespace-service", port: intstr.FromInt(8080)}},
				rs: rs{
					desired: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Conditions: []*elbv2.RuleCondition{
							{
								Field:  aws.String("path-pattern"),
								Values: []*string{aws.String("/path")},
							},
						},
						Actions: []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
					},
				},
			},
		},
		{
			Priority:   1,
			Hostname:   "hostname",
			SvcName:    "namespace-service",
			SvcPort:    intstr.FromInt(8080),
			TargetPort: 8080,
			ExpectedRule: Rule{
				svc: svc{desired: service{name: "namespace-service", port: intstr.FromInt(8080)}},
				rs: rs{
					desired: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Conditions: []*elbv2.RuleCondition{
							{
								Field:  aws.String("host-header"),
								Values: []*string{aws.String("hostname")},
							},
						},
						Actions: []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
					},
				},
			},
		},
		{
			Priority:   1,
			Hostname:   "hostname",
			Path:       "/path",
			SvcName:    "namespace-service",
			SvcPort:    intstr.FromInt(8080),
			TargetPort: 8080,
			ExpectedRule: Rule{
				svc: svc{desired: service{name: "namespace-service", port: intstr.FromInt(8080)}},
				rs: rs{
					desired: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Conditions: []*elbv2.RuleCondition{
							{
								Field:  aws.String("host-header"),
								Values: []*string{aws.String("hostname")},
							},
							{
								Field:  aws.String("path-pattern"),
								Values: []*string{aws.String("/path")},
							},
						},
						Actions: []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
					},
				},
			},
		},
	}

	for i, c := range cases {
		rule, err := NewDesiredRule(&NewDesiredRuleOptions{
			Priority: c.Priority,
			Hostname: c.Hostname,
			Path:     c.Path,
			SvcName:  c.SvcName,
			SvcPort:  c.SvcPort,
			Ingress:  c.Ingress,
			Store:    c.Store,
			Logger:   log.New("test"),
		})
		if err != nil {
			t.Error(err)
		}
		if rule.String() != c.ExpectedRule.String() {
			t.Errorf("TestNewDesiredRule.%v returned an unexpected rule:\n%s\n!=\n%s", i, rule.String(), c.ExpectedRule.String())
		}
	}
}

func TestNewCurrentRule(t *testing.T) {
	r := &elbv2.Rule{RuleArn: aws.String("arn")}
	logger := log.New("test")

	newRule := NewCurrentRule(&NewCurrentRuleOptions{
		Rule:   r,
		Logger: logger,
	})

	if r != newRule.rs.current {
		t.Errorf("NewCurrentRule failed to set the Current to the rule argument")
	}
	if logger != newRule.logger {
		t.Errorf("NewCurrentRule failed to set the logger to the logger argument")
	}
}

func TestRuleReconcile(t *testing.T) {
	cases := []struct {
		Rule             Rule
		Pass             bool
		CreateRuleOutput *elbv2.CreateRuleOutput
		CreateRuleError  error
		ModifyRuleOutput *elbv2.ModifyRuleOutput
		ModifyRuleError  error
		DeleteRuleOutput *elbv2.DeleteRuleOutput
		DeleteRuleError  error
	}{
		{ // test empty rule, no current/desired rules
			Rule: Rule{
				svc:    svc{desired: service{name: "service", port: intstr.FromInt(8080)}},
				logger: log.New("test"),
			},
			Pass: true,
		},
		{ // test Current is default, doesnt delete
			Rule: Rule{
				svc:    svc{desired: service{name: "service", port: intstr.FromInt(8080)}},
				logger: log.New("test"),
				rs: rs{
					current: &elbv2.Rule{
						Priority:  aws.String("default"),
						IsDefault: aws.Bool(true),
						Actions:   []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
					},
				},
			},
			Pass: true,
		},
		{ // test delete
			Rule: Rule{
				svc:    svc{desired: service{name: "service", port: intstr.FromInt(8080)}},
				logger: log.New("test"),
				rs: rs{
					current: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Actions:   []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
					},
				},
			},
			DeleteRuleOutput: &elbv2.DeleteRuleOutput{},
			Pass:             true,
		},
		{ // test delete, fail
			Rule: Rule{
				svc:    svc{desired: service{name: "service", port: intstr.FromInt(8080)}},
				logger: log.New("test"),
				rs: rs{
					current: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Actions:   []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
					},
				},
			},
			DeleteRuleError: fmt.Errorf("fail"),
			Pass:            false,
		},
		{ // test desired rule is default, we do nothing
			Rule: Rule{
				svc:    svc{desired: service{name: "service", port: intstr.FromInt(8080)}},
				logger: log.New("test"),
				rs: rs{
					desired: &elbv2.Rule{
						Priority:  aws.String("default"),
						IsDefault: aws.Bool(true),
						Actions:   []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
					},
				},
			},
			CreateRuleOutput: &elbv2.CreateRuleOutput{
				Rules: []*elbv2.Rule{
					&elbv2.Rule{
						Priority: aws.String("1"),
					},
				},
			},
			Pass: true,
		},
		{ // test current rule is nil, desired rule exists, runs create
			Rule: Rule{
				svc:    svc{desired: service{name: "service", port: intstr.FromInt(8080)}},
				logger: log.New("test"),
				rs: rs{
					desired: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Actions:   []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
					},
				},
			},
			CreateRuleOutput: &elbv2.CreateRuleOutput{
				Rules: []*elbv2.Rule{
					&elbv2.Rule{
						Priority: aws.String("1"),
					},
				},
			},
			Pass: true,
		},
		{ // test current rule is nil, desired rule exists, runs create, fails
			Rule: Rule{
				svc:    svc{desired: service{name: "service", port: intstr.FromInt(8080)}},
				logger: log.New("test"),
				rs: rs{
					desired: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Actions:   []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
					},
				},
			},
			CreateRuleOutput: &elbv2.CreateRuleOutput{
				Rules: []*elbv2.Rule{
					&elbv2.Rule{
						Priority: aws.String("1"),
					},
				},
			},
			CreateRuleError: fmt.Errorf("fail"),
			Pass:            false,
		},
		{ // test current rule and desired rule are different, modify current rule
			Rule: Rule{
				svc:    svc{desired: service{name: "service", port: intstr.FromInt(8080)}},
				logger: log.New("test"),
				rs: rs{
					current: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Actions:   []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
						Conditions: []*elbv2.RuleCondition{
							{
								Field:  aws.String("path-pattern"),
								Values: []*string{aws.String("/path")},
							},
						},
					},
					desired: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Actions:   []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
						Conditions: []*elbv2.RuleCondition{
							{
								Field:  aws.String("path-pattern"),
								Values: []*string{aws.String("/otherpath")},
							},
						},
					},
				},
			},
			ModifyRuleOutput: &elbv2.ModifyRuleOutput{
				Rules: []*elbv2.Rule{
					&elbv2.Rule{
						Priority: aws.String("1"),
					},
				},
			},
			Pass: true,
		},
		{ // test current rule and desired rule are different, modify current rule, fail
			Rule: Rule{
				svc:    svc{desired: service{name: "service", port: intstr.FromInt(8080)}},
				logger: log.New("test"),
				rs: rs{
					current: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						RuleArn:   aws.String("arn"),
						Actions:   []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
						Conditions: []*elbv2.RuleCondition{
							{
								Field:  aws.String("path-pattern"),
								Values: []*string{aws.String("/path")},
							},
						},
					},
					desired: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Actions:   []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
						Conditions: []*elbv2.RuleCondition{
							{
								Field:  aws.String("path-pattern"),
								Values: []*string{aws.String("/otherpath")},
							},
						},
					},
				},
			},
			ModifyRuleOutput: &elbv2.ModifyRuleOutput{
				Rules: []*elbv2.Rule{
					&elbv2.Rule{
						Priority: aws.String("1"),
					},
				},
			},
			ModifyRuleError: fmt.Errorf("fail"),
			Pass:            false,
		},
		{ // test current rule and desired rule are the same, default case
			Rule: Rule{
				svc: svc{
					desired: service{name: "service", port: intstr.FromInt(8080)},
					current: service{name: "service", port: intstr.FromInt(8080)},
				},
				logger: log.New("test"),
				rs: rs{
					current: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Actions:   []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String("arn")}},
						Conditions: []*elbv2.RuleCondition{
							{
								Field:  aws.String("path-pattern"),
								Values: []*string{aws.String("/path")},
							},
						},
					},
					desired: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Actions:   []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String("arn")}},
						Conditions: []*elbv2.RuleCondition{
							{
								Field:  aws.String("path-pattern"),
								Values: []*string{aws.String("/path")},
							},
						},
					},
				},
			},
			Pass: true,
		},
	}

	rOpts := &ReconcileOptions{
		ListenerArn: aws.String(":)"),
		TargetGroups: tg.TargetGroups{
			tg.DummyTG("arn", "service"),
		},
		Eventf: mockEventf,
	}

	for i, c := range cases {
		albelbv2.ELBV2svc = albelbv2.NewDummy()
		albelbv2.ELBV2svc.SetField("CreateRuleOutput", c.CreateRuleOutput)
		albelbv2.ELBV2svc.SetField("CreateRuleError", c.CreateRuleError)
		albelbv2.ELBV2svc.SetField("ModifyRuleOutput", c.ModifyRuleOutput)
		albelbv2.ELBV2svc.SetField("ModifyRuleError", c.ModifyRuleError)
		albelbv2.ELBV2svc.SetField("DeleteRuleOutput", c.DeleteRuleOutput)
		albelbv2.ELBV2svc.SetField("DeleteRuleError", c.DeleteRuleError)
		err := c.Rule.Reconcile(rOpts)
		if err != nil && c.Pass {
			t.Errorf("rule.Reconcile.%v returned an error but should have succeeded.", i)
		}
		if err == nil && !c.Pass {
			t.Errorf("rule.Reconcile.%v succeeded but should have returned an error.", i)
		}
	}
}
func TestTargetGroupArn(t *testing.T) {

	cases := []struct {
		Expected     *string
		TargetGroups tg.TargetGroups
		Rule         Rule
	}{
		{ // svcname is found in the targetgroups list, returns the targetgroup arn
			Expected: aws.String("arn"),
			TargetGroups: tg.TargetGroups{
				tg.DummyTG("arn", "service"),
			},
			Rule: Rule{
				svc:    svc{desired: service{name: "service", port: intstr.FromInt(8080)}},
				logger: log.New("test"),
			},
		},
		{ // svcname isn't found in targetgroups list, returns a nil
			Expected: nil,
			TargetGroups: tg.TargetGroups{
				tg.DummyTG("arn", "missing svc service"),
			},
			Rule: Rule{
				svc:    svc{desired: service{name: "missing service", port: intstr.FromInt(8080)}},
				logger: log.New("test"),
			},
		},
	}

	for i, c := range cases {
		s := c.Rule.TargetGroupArn(c.TargetGroups)
		if s == nil && c.Expected == nil {
			continue
		}
		if s == nil && c.Expected != nil {
			t.Errorf("rule.targetGroupArn.%v returned nil but should have returned '%s'.", i, *c.Expected)
			continue
		}
		if s != nil && c.Expected == nil {
			t.Errorf("rule.targetGroupArn.%v returned '%s' but should have returned nil.", i, *s)
			continue
		}
		if *s != *c.Expected {
			t.Errorf("rule.targetGroupArn.%v returned '%s' but should have returned '%s'.", i, *s, *c.Expected)
			continue
		}
	}
}

func TestCreate(t *testing.T) {
}

func TestModify(t *testing.T) {
}

func TestRuleDelete(t *testing.T) {
	cases := []struct {
		Priority             int
		Hostname             string
		Path                 string
		SvcName              string
		CopyDesiredToCurrent bool
		Pass                 bool
		DeleteRuleError      error
	}{
		{ // test Current == nil
			Priority:             1,
			Hostname:             "hostname",
			Path:                 "/path",
			SvcName:              "namespace-service",
			CopyDesiredToCurrent: false,
			Pass:                 true,
		},
		{ // test deleting a default rule
			Priority:             0,
			Hostname:             "hostname",
			Path:                 "/path",
			SvcName:              "namespace-service",
			CopyDesiredToCurrent: true,
			Pass:                 true,
		},
		{ // test a successful delete
			Priority:             1,
			Hostname:             "hostname",
			Path:                 "/path",
			SvcName:              "namespace-service",
			CopyDesiredToCurrent: true,
			Pass:                 true,
		},
		{ // test a delete that returns an error
			Priority:             1,
			Hostname:             "hostname",
			Path:                 "/path",
			SvcName:              "namespace-service",
			CopyDesiredToCurrent: true,
			DeleteRuleError:      fmt.Errorf("Failed deleting rule"),
			Pass:                 false,
		},
	}

	rOpts := &ReconcileOptions{
		ListenerArn:  aws.String(":)"),
		TargetGroups: nil,
		Eventf:       mockEventf,
	}

	for i, c := range cases {
		rule, err := NewDesiredRule(&NewDesiredRuleOptions{
			Priority: c.Priority,
			Hostname: c.Hostname,
			Path:     c.Path,
			SvcName:  c.SvcName,
			Logger:   log.New("test"),
		})
		if err != nil {
			t.Error(err)
		}

		albelbv2.ELBV2svc.SetField("DeleteRuleOutput", &elbv2.DeleteRuleOutput{})
		albelbv2.ELBV2svc.SetField("DeleteRuleError", c.DeleteRuleError)

		if c.CopyDesiredToCurrent {
			rule.rs.current = rule.rs.desired
		}

		err = rule.delete(rOpts)
		if err != nil && c.Pass {
			t.Errorf("rule.delete.%v returned an error but should have succeeded.", i)
		}
		if err == nil && !c.Pass {
			t.Errorf("rule.delete.%v succeeded but should have returned an error.", i)
		}
	}
}

func TestNeedsModification(t *testing.T) {
	cases := []struct {
		NeedsModification bool
		Current           *elbv2.Rule
		Desired           *elbv2.Rule
	}{
		{ // new rule, current rule is empty
			NeedsModification: true,
			Desired: &elbv2.Rule{
				Conditions: []*elbv2.RuleCondition{
					{
						Field:  aws.String("host-header"),
						Values: []*string{aws.String("hostname")},
					},
					{
						Field:  aws.String("path-pattern"),
						Values: []*string{aws.String("/path")},
					},
				},
			},
		},
		{ // conditions removed from desired rule
			NeedsModification: true,
			Current: &elbv2.Rule{
				Conditions: []*elbv2.RuleCondition{
					{
						Field:  aws.String("host-header"),
						Values: []*string{aws.String("hostname")},
					},
					{
						Field:  aws.String("path-pattern"),
						Values: []*string{aws.String("/path")},
					},
				},
			},
			Desired: &elbv2.Rule{},
		},
		{ // conditions are the same
			NeedsModification: false,
			Current: &elbv2.Rule{
				Conditions: []*elbv2.RuleCondition{
					{
						Field:  aws.String("host-header"),
						Values: []*string{aws.String("hostname")},
					},
					{
						Field:  aws.String("path-pattern"),
						Values: []*string{aws.String("/path")},
					},
				},
			},
			Desired: &elbv2.Rule{
				Conditions: []*elbv2.RuleCondition{
					{
						Field:  aws.String("host-header"),
						Values: []*string{aws.String("hostname")},
					},
					{
						Field:  aws.String("path-pattern"),
						Values: []*string{aws.String("/path")},
					},
				},
			},
		},
		{ // conditions changed on desired rule
			NeedsModification: true,
			Current: &elbv2.Rule{
				Conditions: []*elbv2.RuleCondition{
					{
						Field:  aws.String("host-header"),
						Values: []*string{aws.String("hostname")},
					},
					{
						Field:  aws.String("path-pattern"),
						Values: []*string{aws.String("/path")},
					},
				},
			},
			Desired: &elbv2.Rule{
				Conditions: []*elbv2.RuleCondition{
					{
						Field:  aws.String("changed"),
						Values: []*string{aws.String("changed")},
					},
					{
						Field:  aws.String("changed"),
						Values: []*string{aws.String("changed")},
					},
				},
			},
		},
	}

	for i, c := range cases {
		rule := &Rule{
			logger: log.New("test"),
			rs: rs{
				current: c.Current,
				desired: c.Desired,
			},
		}

		if rule.needsModification() != c.NeedsModification {
			t.Errorf("rule.needsModification.%v returned %v but should have returned %v.", i, rule.needsModification(), c.NeedsModification)
		}
	}
}

func TestRuleStripDesiredState(t *testing.T) {
	r := &Rule{rs: rs{desired: &elbv2.Rule{}}}

	r.stripDesiredState()

	if r.rs.desired != nil {
		t.Errorf("rule.StripDesiredState failed to strip the desired state from the rule")
	}
}

func TestRuleStripCurrentState(t *testing.T) {
	r := &Rule{rs: rs{current: &elbv2.Rule{}}}

	r.stripCurrentState()

	if r.rs.current != nil {
		t.Errorf("rule.StripCurrentState failed to strip the current state from the rule")
	}
}

func TestPriority(t *testing.T) {
	cases := []struct {
		String string
		Int    int64
	}{
		{
			String: "default",
			Int:    0,
		},
		{
			String: "5",
			Int:    5,
		},
	}

	for i, c := range cases {
		out := priority(&c.String)
		if *out != c.Int {
			t.Errorf("rule.priority.%v returned %v but should have returned %v.", i, *out, c.Int)
		}
	}
}

func TestIgnoreHostHeader(t *testing.T) {
	cases := []struct {
		Priority         int
		Hostname         string
		IgnoreHostHeader *bool
		Path             string
		SvcName          string
		SvcPort          intstr.IntOrString
		TargetPort       int
		ExpectedRule     Rule
	}{
		{
			Priority:         1,
			Hostname:         "hostname",
			IgnoreHostHeader: aws.Bool(false),
			Path:             "/path",
			SvcName:          "namespace-service",
			SvcPort:          intstr.FromInt(8080),
			TargetPort:       8080,
			ExpectedRule: Rule{
				svc: svc{desired: service{name: "namespace-service", port: intstr.FromInt(8080)}},
				rs: rs{
					desired: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Conditions: []*elbv2.RuleCondition{
							{
								Field:  aws.String("host-header"),
								Values: []*string{aws.String("hostname")},
							},
							{
								Field:  aws.String("path-pattern"),
								Values: []*string{aws.String("/path")},
							},
						},
						Actions: []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
					},
				},
			},
		},
		{
			Priority:         1,
			Hostname:         "hostname",
			IgnoreHostHeader: aws.Bool(true),
			Path:             "/path",
			SvcName:          "namespace-service",
			SvcPort:          intstr.FromInt(8080),
			TargetPort:       8080,
			ExpectedRule: Rule{
				svc: svc{desired: service{name: "namespace-service", port: intstr.FromInt(8080)}},
				rs: rs{
					desired: &elbv2.Rule{
						Priority:  aws.String("1"),
						IsDefault: aws.Bool(false),
						Conditions: []*elbv2.RuleCondition{
							{
								Field:  aws.String("path-pattern"),
								Values: []*string{aws.String("/path")},
							},
						},
						Actions: []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward)}},
					},
				},
			},
		},
	}

	for i, c := range cases {
		rule, err := NewDesiredRule(&NewDesiredRuleOptions{
			Priority:         c.Priority,
			Hostname:         c.Hostname,
			IgnoreHostHeader: c.IgnoreHostHeader,
			Path:             c.Path,
			SvcName:          c.SvcName,
			SvcPort:          c.SvcPort,
			Logger:           log.New("test"),
		})
		if err != nil {
			t.Error(err)
		}
		if rule.String() != c.ExpectedRule.String() {
			t.Errorf("TestNewDesiredRule.%v returned an unexpected rule:\n%s\n!=\n%s", i, rule.String(), c.ExpectedRule.String())
		}
	}
}
func TestRuleValid(t *testing.T) {
	cases := []struct {
		Priority   int
		Hostname   string
		Path       string
		SvcName    string
		SvcPort    intstr.IntOrString
		TargetPort int
		Protocol   *string
		Valid      bool
		Ingress    *extensions.Ingress
		Store      store.Storer
	}{
		{
			Priority:   1,
			SvcName:    "redirect", // redirect https to https, invalid
			SvcPort:    intstr.FromString(action.UseActionAnnotation),
			Ingress:    dummy.NewIngress(),
			Store:      store.NewDummy(),
			TargetPort: 0,
			Protocol:   aws.String(elbv2.ProtocolEnumHttps),
			Valid:      false,
		},
		{
			Priority:   1,
			SvcName:    "redirect", // redirect http to https, valid
			SvcPort:    intstr.FromString(action.UseActionAnnotation),
			Ingress:    dummy.NewIngress(),
			Store:      store.NewDummy(),
			TargetPort: 0,
			Protocol:   aws.String(elbv2.ProtocolEnumHttp),
			Valid:      true,
		},
		{
			Priority:   1,
			SvcName:    "redirect-path2", // redirect https to https, non-standard path, valid
			SvcPort:    intstr.FromString(action.UseActionAnnotation),
			Ingress:    dummy.NewIngress(),
			Store:      store.NewDummy(),
			TargetPort: 0,
			Protocol:   aws.String(elbv2.ProtocolEnumHttps),
			Valid:      true,
		},
	}

	for i, c := range cases {
		rule, err := NewDesiredRule(&NewDesiredRuleOptions{
			Priority: c.Priority,
			Hostname: c.Hostname,
			Path:     c.Path,
			SvcName:  c.SvcName,
			SvcPort:  c.SvcPort,
			Ingress:  c.Ingress,
			Store:    c.Store,
			Logger:   log.New("test"),
		})
		if err != nil {
			t.Error(err)
		}
		if rule.valid(int64(c.TargetPort), c.Protocol) != c.Valid {
			t.Errorf("TestRuleValid.%v.valid was %v, expected %v", i, rule.valid(int64(c.TargetPort), c.Protocol), c.Valid)
		}
	}
}

func mockEventf(a, b, c string, d ...interface{}) {
}
