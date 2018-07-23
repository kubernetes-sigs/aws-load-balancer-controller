package rs

import (
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"

	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albcache"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
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

	albrgt.RGTsvc = &albrgt.Dummy{}
	albcache.NewCache(metric.DummyCollector{})
}

func TestNewDesiredRules(t *testing.T) {
	cases := []struct {
		Pass    bool
		Options *NewDesiredRulesOptions
	}{
		{ // ingress with empty paths
			Pass: false,
			Options: &NewDesiredRulesOptions{
				ListenerRules: Rules{
					&Rule{rs: rs{current: &elbv2.Rule{IsDefault: aws.Bool(true), Priority: aws.String("default")}}},
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
			Options: &NewDesiredRulesOptions{
				ListenerRules: Rules{
					&Rule{rs: rs{current: &elbv2.Rule{IsDefault: aws.Bool(true), Priority: aws.String("default")}}},
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
			Options: &NewDesiredRulesOptions{
				ListenerRules: Rules{
					&Rule{rs: rs{current: &elbv2.Rule{IsDefault: aws.Bool(true), Priority: aws.String("default")}}},
					&Rule{rs: rs{current: &elbv2.Rule{IsDefault: aws.Bool(false), Priority: aws.String("1")}}},
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
			Options: &NewDesiredRulesOptions{
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
		newRules, _, err := NewDesiredRules(c.Options)
		if err != nil && !c.Pass {
			continue
		}
		if err != nil && c.Pass {
			t.Errorf("NewDesiredRules.%v returned an error but should have passed: %s", i, err.Error())
			continue
		}
		if err == nil && !c.Pass {
			t.Errorf("NewDesiredRules.%v passed but should have returned an error.", i)
			continue
		}

		// check default rule
		d := newRules[0].rs.desired
		if !*d.IsDefault {
			t.Errorf("NewDesiredRules.%v first rule was not the default rule.", i)
		}

		if d.Conditions != nil {
			t.Errorf("NewDesiredRules.%v first rule (default rule) had conditions.", i)
		}

		if *d.Priority != "default" {
			t.Errorf("NewDesiredRules.%v first rule (default rule) did not have 'default' priority.", i)
		}

		for n, p := range c.Options.Rule.IngressRuleValue.HTTP.Paths {
			r := newRules[n+1] // +1 to skip default rule
			if *r.rs.desired.IsDefault {
				t.Errorf("NewDesiredRules.%v path %v is a default rule but should not be.", i, n)
			}
			for _, condition := range r.rs.desired.Conditions {
				field := *condition.Field
				value := *condition.Values[0]

				if field == "host-header" && value != c.Options.Rule.Host {
					t.Errorf("NewDesiredRules.%v path %v host-header condition is %v, should be %v.", i, n, value, c.Options.Rule.Host)
				}

				if field == "path-pattern" && value != p.Path {
					t.Errorf("NewDesiredRules.%v path %v path-pattern condition is %v, should be %v.", i, n, value, p.Path)
				}
			}
		}
	}
}

func TestRulesReconcile(t *testing.T) {
	cases := []struct {
		Rules            Rules
		OutputLength     int
		CreateRuleOutput elbv2.CreateRuleOutput
	}{
		{
			Rules: Rules{
				NewDesiredRule(&NewDesiredRuleOptions{
					Priority: 0,
					Hostname: "hostname",
					Path:     paths[0],
					SvcName:  svcs[0],
					Logger:   log.New("test"),
				}),
			},
			OutputLength: 1,
			CreateRuleOutput: elbv2.CreateRuleOutput{
				Rules: []*elbv2.Rule{
					&elbv2.Rule{
						Priority: aws.String("1"),
					},
				},
			},
		},
	}

	rOpts := &ReconcileOptions{
		ListenerArn: aws.String(":)"),
		TargetGroups: tg.TargetGroups{
			genTG(":)", "namespace-service"),
		},
		Eventf: func(a, b, c string, d ...interface{}) {},
	}

	for i, c := range cases {
		albelbv2.ELBV2svc = mockedELBV2{
			CreateRuleOutput: c.CreateRuleOutput,
		}
		rules, _ := c.Rules.Reconcile(rOpts)
		if len(rules) != c.OutputLength {
			t.Errorf("rules.Reconcile.%v output length %v, should be %v.", i, len(rules), c.OutputLength)
		}
	}
}

func TestRulesStripDesiredState(t *testing.T) {
	rs := Rules{&Rule{rs: rs{desired: &elbv2.Rule{}}}}

	rs.StripDesiredState()

	for _, r := range rs {
		if r.rs.desired != nil {
			t.Errorf("rules.StripDesiredState failed to strip the desired state from the rule")
		}
	}
}

func TestRulesStripCurrentState(t *testing.T) {
	rs := Rules{&Rule{rs: rs{current: &elbv2.Rule{}}}}

	rs.StripCurrentState()

	for _, r := range rs {
		if r.rs.current != nil {
			t.Errorf("rules.StripCurrentState failed to strip the current state from the rule")
		}
	}
}
