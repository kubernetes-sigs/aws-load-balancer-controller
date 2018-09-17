package rs

import (
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"

	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

var (
	paths           []string
	ingressBackends []extensions.IngressBackend
)

func init() {
	paths = []string{
		"/path1",
		"/path2",
		"/path3",
	}
	ingressBackends = []extensions.IngressBackend{
		extensions.IngressBackend{
			ServiceName: "1service",
			ServicePort: intstr.FromInt(8080),
		},
		extensions.IngressBackend{
			ServiceName: "2service",
			ServicePort: intstr.FromInt(8080),
		},
		extensions.IngressBackend{
			ServiceName: "3service",
			ServicePort: intstr.FromInt(8080),
		},
	}

	albelbv2.ELBV2svc = albelbv2.NewDummy()
	albrgt.RGTsvc = &albrgt.Dummy{}
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
		{ // Listener has no rules
			// No hostname, some path
			Pass: true,
			Options: &NewDesiredRulesOptions{
				Logger: log.New("test"),
				Rule: &extensions.IngressRule{
					IngressRuleValue: extensions.IngressRuleValue{
						HTTP: &extensions.HTTPIngressRuleValue{
							Paths: []extensions.HTTPIngressPath{
								{
									Path:    paths[2],
									Backend: ingressBackends[2],
								},
							},
						},
					},
				},
			},
		},
		{ // Listener has one existing rule
			// No hostname, some path
			Pass: true,
			Options: &NewDesiredRulesOptions{
				ListenerRules: Rules{
					&Rule{rs: rs{current: &elbv2.Rule{IsDefault: aws.Bool(false), Priority: aws.String("1")}}},
				},
				Logger: log.New("test"),
				Rule: &extensions.IngressRule{
					IngressRuleValue: extensions.IngressRuleValue{
						HTTP: &extensions.HTTPIngressRuleValue{
							Paths: []extensions.HTTPIngressPath{
								{
									Path:    paths[2],
									Backend: ingressBackends[2],
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
									Path:    paths[0],
									Backend: ingressBackends[0],
								},
								{
									Path:    paths[1],
									Backend: ingressBackends[1],
								},
							},
						},
					},
				},
			},
		},
	}

	for i, c := range cases {
		ing := dummy.NewIngress()
		ing.Spec.Rules = []extensions.IngressRule{*c.Options.Rule}
		tgs, err := tg.NewDesiredTargetGroups(&tg.NewDesiredTargetGroupsOptions{
			Ingress:        ing,
			LoadBalancerID: "lbid",
			Store:          store.NewDummy(),
			CommonTags:     util.ELBv2Tags{},
			Logger:         log.New("logger"),
		})
		c.Options.TargetGroups = tgs

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

		for n, p := range c.Options.Rule.IngressRuleValue.HTTP.Paths {
			r := newRules[n]
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
	rule, _ := NewDesiredRule(&NewDesiredRuleOptions{
		Priority: 0,
		Hostname: "hostname",
		Path:     paths[0],
		SvcName:  ingressBackends[0].ServiceName,
		SvcPort:  ingressBackends[0].ServicePort,
		Logger:   log.New("test"),
	})

	cases := []struct {
		Rules            Rules
		OutputLength     int
		CreateRuleOutput elbv2.CreateRuleOutput
	}{
		{
			Rules: Rules{
				rule,
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
			tg.DummyTG("arn", "service"),
		},
		Eventf: func(a, b, c string, d ...interface{}) {},
	}

	for i, c := range cases {
		albelbv2.ELBV2svc.SetField("CreateRuleOutput", &c.CreateRuleOutput)
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
