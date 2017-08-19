package rule

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/davecgh/go-spew/spew"

	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroup"
	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroups"
	albelbv2 "github.com/coreos/alb-ingress-controller/pkg/aws/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
)

func TestNewRule(t *testing.T) {
	cases := []struct {
		Priority     int
		Hostname     string
		Path         string
		SvcName      string
		ExpectedRule Rule
	}{
		{
			Priority: 0,
			Hostname: "hostname",
			Path:     "/path",
			SvcName:  "namespace-service",
			ExpectedRule: Rule{
				svcName: "namespace-service",
				DesiredRule: &elbv2.Rule{
					Priority:  aws.String("default"),
					IsDefault: aws.Bool(true),
					Actions:   []*elbv2.Action{{Type: aws.String("forward")}},
				},
			},
		},
		{
			Priority: 1,
			Hostname: "hostname",
			Path:     "/path",
			SvcName:  "namespace-service",
			ExpectedRule: Rule{
				svcName: "namespace-service",
				DesiredRule: &elbv2.Rule{
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
					Actions: []*elbv2.Action{{Type: aws.String("forward")}},
				},
			},
		},
	}

	for i, c := range cases {
		rule := NewRule(c.Priority, c.Hostname, c.Path, c.SvcName, log.New("test"))
		if log.Prettify(rule) != log.Prettify(c.ExpectedRule) {
			t.Errorf("NewRule.%v returned an unexpected rule:\n%s\n!=\n%s", i, log.Prettify(rule), log.Prettify(c.ExpectedRule))
		}
	}
}

func TestNewRuleFromAWSRule(t *testing.T) {
	r := &elbv2.Rule{RuleArn: aws.String("arn")}
	logger := log.New("test")

	newRule := NewRuleFromAWSRule(r, logger)

	if r != newRule.CurrentRule {
		t.Errorf("NewRuleFromAWSRule failed to set the CurrentRule to the rule argument")
	}
	if logger != newRule.logger {
		t.Errorf("NewRuleFromAWSRule failed to set the logger to the logger argument")
	}
}

func TestReconcile(t *testing.T) {
	cases := []struct {
		Rule             Rule
		Pass             bool
		CreateRuleOutput elbv2.CreateRuleOutput
		CreateRuleError  error
		ModifyRuleOutput elbv2.ModifyRuleOutput
		ModifyRuleError  error
		DeleteRuleOutput elbv2.DeleteRuleOutput
		DeleteRuleError  error
	}{
		{ // test empty rule, no current/desired rules
			Rule: Rule{
				svcName: "namespace-service",
				logger:  log.New("test"),
			},
			Pass: true,
		},
		{ // test currentrule is default, doesnt delete
			Rule: Rule{
				svcName: "namespace-service",
				logger:  log.New("test"),
				CurrentRule: &elbv2.Rule{
					Priority:  aws.String("default"),
					IsDefault: aws.Bool(true),
					Actions:   []*elbv2.Action{{Type: aws.String("forward")}},
				},
			},
			Pass: true,
		},
		{ // test delete
			Rule: Rule{
				svcName: "namespace-service",
				logger:  log.New("test"),
				CurrentRule: &elbv2.Rule{
					Priority:  aws.String("1"),
					IsDefault: aws.Bool(false),
					Actions:   []*elbv2.Action{{Type: aws.String("forward")}},
				},
			},
			Pass: true,
		},
		{ // test delete, fail
			Rule: Rule{
				svcName: "namespace-service",
				logger:  log.New("test"),
				CurrentRule: &elbv2.Rule{
					Priority:  aws.String("1"),
					IsDefault: aws.Bool(false),
					Actions:   []*elbv2.Action{{Type: aws.String("forward")}},
				},
			},
			DeleteRuleError: fmt.Errorf("fail"),
			Pass:            false,
		},
		{ // test desired rule is default, we do nothing
			Rule: Rule{
				svcName: "namespace-service",
				logger:  log.New("test"),
				DesiredRule: &elbv2.Rule{
					Priority:  aws.String("default"),
					IsDefault: aws.Bool(true),
					Actions:   []*elbv2.Action{{Type: aws.String("forward")}},
				},
			},
			Pass: true,
		},
		{ // test current rule is nil, desired rule exists, runs create
			Rule: Rule{
				svcName: "namespace-service",
				logger:  log.New("test"),
				DesiredRule: &elbv2.Rule{
					Priority:  aws.String("1"),
					IsDefault: aws.Bool(false),
					Actions:   []*elbv2.Action{{Type: aws.String("forward")}},
				},
			},
			CreateRuleOutput: elbv2.CreateRuleOutput{
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
				svcName: "namespace-service",
				logger:  log.New("test"),
				DesiredRule: &elbv2.Rule{
					Priority:  aws.String("1"),
					IsDefault: aws.Bool(false),
					Actions:   []*elbv2.Action{{Type: aws.String("forward")}},
				},
			},
			CreateRuleOutput: elbv2.CreateRuleOutput{
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
				svcName: "namespace-service",
				logger:  log.New("test"),
				CurrentRule: &elbv2.Rule{
					Priority:  aws.String("1"),
					IsDefault: aws.Bool(false),
					Actions:   []*elbv2.Action{{Type: aws.String("forward")}},
					Conditions: []*elbv2.RuleCondition{
						{
							Field:  aws.String("path-pattern"),
							Values: []*string{aws.String("/path")},
						},
					},
				},
				DesiredRule: &elbv2.Rule{
					Priority:  aws.String("1"),
					IsDefault: aws.Bool(false),
					Actions:   []*elbv2.Action{{Type: aws.String("forward")}},
					Conditions: []*elbv2.RuleCondition{
						{
							Field:  aws.String("path-pattern"),
							Values: []*string{aws.String("/otherpath")},
						},
					},
				},
			},
			ModifyRuleOutput: elbv2.ModifyRuleOutput{
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
				svcName: "namespace-service",
				logger:  log.New("test"),
				CurrentRule: &elbv2.Rule{
					Priority:  aws.String("1"),
					IsDefault: aws.Bool(false),
					RuleArn:   aws.String("arn"),
					Actions:   []*elbv2.Action{{Type: aws.String("forward")}},
					Conditions: []*elbv2.RuleCondition{
						{
							Field:  aws.String("path-pattern"),
							Values: []*string{aws.String("/path")},
						},
					},
				},
				DesiredRule: &elbv2.Rule{
					Priority:  aws.String("1"),
					IsDefault: aws.Bool(false),
					Actions:   []*elbv2.Action{{Type: aws.String("forward")}},
					Conditions: []*elbv2.RuleCondition{
						{
							Field:  aws.String("path-pattern"),
							Values: []*string{aws.String("/otherpath")},
						},
					},
				},
			},
			ModifyRuleOutput: elbv2.ModifyRuleOutput{
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
				svcName: "namespace-service",
				logger:  log.New("test"),
				CurrentRule: &elbv2.Rule{
					Priority:  aws.String("1"),
					IsDefault: aws.Bool(false),
					Actions:   []*elbv2.Action{{Type: aws.String("forward")}},
					Conditions: []*elbv2.RuleCondition{
						{
							Field:  aws.String("path-pattern"),
							Values: []*string{aws.String("/path")},
						},
					},
				},
				DesiredRule: &elbv2.Rule{
					Priority:  aws.String("1"),
					IsDefault: aws.Bool(false),
					Actions:   []*elbv2.Action{{Type: aws.String("forward")}},
					Conditions: []*elbv2.RuleCondition{
						{
							Field:  aws.String("path-pattern"),
							Values: []*string{aws.String("/path")},
						},
					},
				},
			},
			Pass: true,
		},
	}

	rOpts := &ReconcileOptions{
		ListenerArn: aws.String(":)"),
		TargetGroups: targetgroups.TargetGroups{
			&targetgroup.TargetGroup{
				SvcName: "namespace-service",
				CurrentTargetGroup: &elbv2.TargetGroup{
					TargetGroupArn: aws.String(":)"),
				},
			},
		},
		Eventf: mockEventf,
	}

	for i, c := range cases {
		albelbv2.ELBV2svc = mockedELBV2{
			CreateRuleOutput: c.CreateRuleOutput,
			ModifyRuleOutput: c.ModifyRuleOutput,
			DeleteRuleOutput: c.DeleteRuleOutput,
			CreateRuleError:  c.CreateRuleError,
			ModifyRuleError:  c.ModifyRuleError,
			DeleteRuleError:  c.DeleteRuleError,
		}

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
		TargetGroups targetgroups.TargetGroups
		Rule         Rule
	}{
		{ // Current rule already contains the TG, ignore the TargetGroups param
			Expected: aws.String(":)"),
			Rule: Rule{
				svcName: "namespace-service",
				logger:  log.New("test"),
				CurrentRule: &elbv2.Rule{
					Actions: []*elbv2.Action{{TargetGroupArn: aws.String(":)")}},
				},
			},
		},
		{ // svcname is found in the targetgroups list, returns the targetgroup arn
			Expected: aws.String(":)"),
			TargetGroups: targetgroups.TargetGroups{
				&targetgroup.TargetGroup{
					SvcName: "namespace-service",
					CurrentTargetGroup: &elbv2.TargetGroup{
						TargetGroupArn: aws.String(":)"),
					},
				},
			},
			Rule: Rule{
				svcName: "namespace-service",
				logger:  log.New("test"),
			},
		},
		{ // svcname isn't found in targetgroups list, returns a nil
			Expected: nil,
			TargetGroups: targetgroups.TargetGroups{
				&targetgroup.TargetGroup{
					SvcName: "missing svc name",
				},
			},
			Rule: Rule{
				svcName: "namespace-service",
				logger:  log.New("test"),
			},
		},
	}

	for i, c := range cases {
		s := c.Rule.TargetGroupArn(c.TargetGroups)
		if s == nil && c.Expected == nil {
			continue
		}
		if s == nil && c.Expected != nil {
			t.Errorf("rule.TargetGroupArn.%v returned nil but should have returned '%s'.", i, *c.Expected)
			continue
		}
		if s != nil && c.Expected == nil {
			t.Errorf("rule.TargetGroupArn.%v returned '%s' but should have returned nil.", i, *s)
			continue
		}
		if *s != *c.Expected {
			t.Errorf("rule.TargetGroupArn.%v returned '%s' but should have returned '%s'.", i, *s, *c.Expected)
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
		Priority                 int
		Hostname                 string
		Path                     string
		SvcName                  string
		CopyDesiredToCurrentRule bool
		Pass                     bool
		DeleteRuleError          error
	}{
		{ // test CurrentRule == nil
			Priority:                 1,
			Hostname:                 "hostname",
			Path:                     "/path",
			SvcName:                  "namespace-service",
			CopyDesiredToCurrentRule: false,
			Pass: true,
		},
		{ // test deleting a default rule
			Priority:                 0,
			Hostname:                 "hostname",
			Path:                     "/path",
			SvcName:                  "namespace-service",
			CopyDesiredToCurrentRule: true,
			Pass: true,
		},
		{ // test a successful delete
			Priority:                 1,
			Hostname:                 "hostname",
			Path:                     "/path",
			SvcName:                  "namespace-service",
			CopyDesiredToCurrentRule: true,
			Pass: true,
		},
		{ // test a delete that returns an error
			Priority:                 1,
			Hostname:                 "hostname",
			Path:                     "/path",
			SvcName:                  "namespace-service",
			CopyDesiredToCurrentRule: true,
			DeleteRuleError:          fmt.Errorf("Failed deleting rule"),
			Pass:                     false,
		},
	}

	rOpts := &ReconcileOptions{
		ListenerArn:  aws.String(":)"),
		TargetGroups: nil,
		Eventf:       mockEventf,
	}

	for i, c := range cases {
		rule := NewRule(c.Priority, c.Hostname, c.Path, c.SvcName, log.New("test"))

		albelbv2.ELBV2svc = mockedELBV2{
			DeleteRuleOutput: elbv2.DeleteRuleOutput{},
			DeleteRuleError:  c.DeleteRuleError,
		}

		if c.CopyDesiredToCurrentRule {
			rule.CurrentRule = rule.DesiredRule
		}

		err := rule.delete(rOpts)
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
		CurrentRule       *elbv2.Rule
		DesiredRule       *elbv2.Rule
	}{
		{ // new rule, current rule is empty
			NeedsModification: true,
			DesiredRule: &elbv2.Rule{
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
			CurrentRule: &elbv2.Rule{
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
			DesiredRule: &elbv2.Rule{},
		},
		{ // conditions are the same
			NeedsModification: false,
			CurrentRule: &elbv2.Rule{
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
			DesiredRule: &elbv2.Rule{
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
			CurrentRule: &elbv2.Rule{
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
			DesiredRule: &elbv2.Rule{
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
			logger:      log.New("test"),
			CurrentRule: c.CurrentRule,
			DesiredRule: c.DesiredRule,
		}

		if rule.needsModification() != c.NeedsModification {
			t.Errorf("rule.needsModification.%v returned %v but should have returned %v.", i, rule.needsModification(), c.NeedsModification)
		}
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

func TestReconcileOptions(t *testing.T) {
	cases := []struct {
		Eventf       func(string, string, string, ...interface{})
		ListenerArn  *string
		TargetGroups targetgroups.TargetGroups
	}{
		{
			Eventf:       mockEventf,
			ListenerArn:  aws.String("arn"),
			TargetGroups: targetgroups.TargetGroups{&targetgroup.TargetGroup{ID: aws.String(":)")}},
		},
	}

	for i, c := range cases {
		rOpts := NewReconcileOptions().SetListenerArn(c.ListenerArn).SetEventf(c.Eventf).SetTargetGroups(c.TargetGroups)

		if spew.Sdump(rOpts.Eventf) != spew.Sdump(c.Eventf) {
			t.Errorf("TestReconcileOptions.%v failed to set Eventf, '%v' != '%v'", i, rOpts.Eventf, c.Eventf)
		}

		if rOpts.ListenerArn != c.ListenerArn {
			t.Errorf("TestReconcileOptions.%v failed to set ListenerArn, '%v' != '%v'", i, rOpts.ListenerArn, c.ListenerArn)
		}

		if spew.Sdump(rOpts.TargetGroups) != spew.Sdump(c.TargetGroups) {
			t.Errorf("TestReconcileOptions.%v failed to set TargetGroups, '%v' != '%v'", i, rOpts.TargetGroups, c.TargetGroups)
		}
	}
}

func mockEventf(a, b, c string, d ...interface{}) {
}

type mockedELBV2 struct {
	albelbv2.ELBV2API
	CreateRuleOutput elbv2.CreateRuleOutput
	CreateRuleError  error
	ModifyRuleOutput elbv2.ModifyRuleOutput
	ModifyRuleError  error
	DeleteRuleOutput elbv2.DeleteRuleOutput
	DeleteRuleError  error
}

func (m mockedELBV2) CreateRule(input *elbv2.CreateRuleInput) (*elbv2.CreateRuleOutput, error) {
	return &m.CreateRuleOutput, m.CreateRuleError
}

func (m mockedELBV2) ModifyRule(input *elbv2.ModifyRuleInput) (*elbv2.ModifyRuleOutput, error) {
	return &m.ModifyRuleOutput, m.ModifyRuleError
}

func (m mockedELBV2) DeleteRule(input *elbv2.DeleteRuleInput) (*elbv2.DeleteRuleOutput, error) {
	return &m.DeleteRuleOutput, m.DeleteRuleError
}
