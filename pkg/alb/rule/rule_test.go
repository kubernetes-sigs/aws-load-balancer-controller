package rule

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"

	albelbv2 "github.com/coreos/alb-ingress-controller/pkg/aws/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
)

func mockEventf(a, b, c string, d ...interface{}) {
}

type mockedELBV2 struct {
	albelbv2.ELBV2API
	ModifyRuleOutput elbv2.ModifyRuleOutput
	DeleteRuleOutput elbv2.DeleteRuleOutput
	Error            error
}

func (m mockedELBV2) ModifyRule(input *elbv2.ModifyRuleInput) (*elbv2.ModifyRuleOutput, error) {
	return &m.ModifyRuleOutput, m.Error
}

func (m mockedELBV2) DeleteRule(input *elbv2.DeleteRuleInput) (*elbv2.DeleteRuleOutput, error) {
	return &m.DeleteRuleOutput, m.Error
}

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

func TestTargetGroupArn(t *testing.T) {
}
func TestReconcile(t *testing.T) {
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
		ResponseError            error
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
			ResponseError:            fmt.Errorf("Failed deleting rule"),
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
			Error:            c.ResponseError,
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
