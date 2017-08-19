package rules

import (
	"testing"

	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/alb/rule"
	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroup"
	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroups"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
)

var (
	paths []string
	svcs  []string
)

func init() {
	paths = []string{
		"/path1",
		"/path2",
		"/path3",
	}
	svcs = []string{
		"1service",
		"2service",
		"3service",
	}
}

func TestNewRulesFromIngress(t *testing.T) {
	cases := []struct {
		Pass    bool
		Options *NewRulesFromIngressOptions
	}{
		{ // ingress with empty paths
			Pass: false,
			Options: &NewRulesFromIngressOptions{
				ListenerRules: Rules{
					&rule.Rule{Desired: &elbv2.Rule{IsDefault: aws.Bool(true), Priority: aws.String("default")}},
				},
				Logger: log.New("test"),
				Rule: &extensions.IngressRule{
					IngressRuleValue: extensions.IngressRuleValue{
						HTTP: &extensions.HTTPIngressRuleValue{
							Paths: []extensions.HTTPIngressPath{},
						},
					},
				},
			},
		},
		{ // Listener has one default rule
			// No hostname, some path
			Pass: true,
			Options: &NewRulesFromIngressOptions{
				ListenerRules: Rules{
					&rule.Rule{Desired: &elbv2.Rule{IsDefault: aws.Bool(true), Priority: aws.String("default")}},
				},
				Logger: log.New("test"),
				Rule: &extensions.IngressRule{
					IngressRuleValue: extensions.IngressRuleValue{
						HTTP: &extensions.HTTPIngressRuleValue{
							Paths: []extensions.HTTPIngressPath{
								{
									Path: paths[2],
									Backend: extensions.IngressBackend{
										ServiceName: svcs[2],
									},
								},
							},
						},
					},
				},
			},
		},
		{ // Listener has one existing non-default rule
			// No hostname, some path
			Pass: true,
			Options: &NewRulesFromIngressOptions{
				ListenerRules: Rules{
					&rule.Rule{Desired: &elbv2.Rule{IsDefault: aws.Bool(true), Priority: aws.String("default")}},
					&rule.Rule{Desired: &elbv2.Rule{IsDefault: aws.Bool(false), Priority: aws.String("1")}},
				},
				Logger: log.New("test"),
				Rule: &extensions.IngressRule{
					IngressRuleValue: extensions.IngressRuleValue{
						HTTP: &extensions.HTTPIngressRuleValue{
							Paths: []extensions.HTTPIngressPath{
								{
									Path: paths[2],
									Backend: extensions.IngressBackend{
										ServiceName: svcs[2],
									},
								},
							},
						},
					},
				},
			},
		},
		{ // Listener has no existing rules, no existing priorities
			// With two paths
			Pass: true,
			Options: &NewRulesFromIngressOptions{
				Logger: log.New("test"),
				Rule: &extensions.IngressRule{
					Host: "hostname",
					IngressRuleValue: extensions.IngressRuleValue{
						HTTP: &extensions.HTTPIngressRuleValue{
							Paths: []extensions.HTTPIngressPath{
								{
									Path: paths[0],
									Backend: extensions.IngressBackend{
										ServiceName: svcs[0],
									},
								},
								{
									Path: paths[1],
									Backend: extensions.IngressBackend{
										ServiceName: svcs[1],
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for i, c := range cases {
		newRules, err := NewRulesFromIngress(c.Options)
		if err != nil && !c.Pass {
			continue
		}
		if err != nil && c.Pass {
			t.Errorf("NewRulesFromIngress.%v returned an error but should have passed: %s", i, err.Error())
			continue
		}
		if err == nil && !c.Pass {
			t.Errorf("NewRulesFromIngress.%v passed but should have returned an error.", i)
			continue
		}

		// check default rule
		d := newRules[0].Desired
		if !*d.IsDefault {
			t.Errorf("NewRulesFromIngress.%v first rule was not the default rule.", i)
		}

		if d.Conditions != nil {
			t.Errorf("NewRulesFromIngress.%v first rule (default rule) had conditions.", i)
		}

		if *d.Priority != "default" {
			t.Errorf("NewRulesFromIngress.%v first rule (default rule) did not have 'default' priority.", i)
		}

		skip := 1
		if c.Options.ListenerRules != nil {
			skip = len(c.Options.ListenerRules)
		}
		for n, p := range c.Options.Rule.IngressRuleValue.HTTP.Paths {
			r := newRules[n+skip] // skip existing rules
			if *r.Desired.IsDefault {
				t.Errorf("NewRulesFromIngress.%v path %v is a default rule but should not be.", i, n)
			}
			for _, condition := range r.Desired.Conditions {
				field := *condition.Field
				value := *condition.Values[0]

				if field == "host-header" && value != c.Options.Rule.Host {
					t.Errorf("NewRulesFromIngress.%v path %v host-header condition is %v, should be %v.", i, n, value, c.Options.Rule.Host)
				}

				if field == "path-pattern" && value != p.Path {
					t.Errorf("NewRulesFromIngress.%v path %v path-pattern condition is %v, should be %v.", i, n, value, p.Path)
				}
			}
		}
	}
}

func TestReconcile(t *testing.T) {
	cases := []struct {
		Rules        Rules
		OutputLength int
	}{
		{
			Rules: Rules{
				rule.NewRule(0, "hostname", paths[0], svcs[0], log.New("test")),
			},
			OutputLength: 1,
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
		Eventf: func(a, b, c string, d ...interface{}) {},
	}

	for i, c := range cases {
		rules, _ := c.Rules.Reconcile(rOpts)
		if len(rules) != c.OutputLength {
			t.Errorf("rules.Reconcile.%v output length %v, should be %v.", i, len(rules), c.OutputLength)
		}
	}
}

func TestStripDesiredState(t *testing.T) {
	rs := Rules{&rule.Rule{Desired: &elbv2.Rule{}}}

	rs.StripDesiredState()

	for _, r := range rs {
		if r.Desired != nil {
			t.Errorf("rules.StripDesiredState failed to strip the desired state from the rule")
		}
	}
}

func TestStripCurrentState(t *testing.T) {
	rs := Rules{&rule.Rule{Current: &elbv2.Rule{}}}

	rs.StripCurrentState()

	for _, r := range rs {
		if r.Current != nil {
			t.Errorf("rules.StripCurrentState failed to strip the current state from the rule")
		}
	}
}
