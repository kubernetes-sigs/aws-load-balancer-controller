package ls

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/listener"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/rule"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	logger   *log.Logger
	ports    []int64
	schemes  []bool
	hosts    []string
	paths    []string
	svcs     []string
	svcPorts []int32
)

func init() {
	logger = log.New("test")
	ports = []int64{
		int64(80),
		int64(443),
		int64(8080),
	}
	schemes = []bool{
		false,
		true,
		false,
	}
	hosts = []string{
		"1.test.domain",
		"2.test.domain",
		"3.test.domain",
	}
	paths = []string{
		"/",
		"/store",
		"/store/dev",
	}
	svcs = []string{
		"1service",
		"2service",
		"3service",
	}
	svcPorts = []int32{
		int32(30001),
		int32(30002),
		int32(30003),
	}
}

func TestNewSingleListener(t *testing.T) {
	// mock ingress rules
	rs := []extensions.IngressRule{
		{
			Host: hosts[0],
			IngressRuleValue: extensions.IngressRuleValue{
				HTTP: &extensions.HTTPIngressRuleValue{
					Paths: []extensions.HTTPIngressPath{{
						Path: paths[0],
						Backend: extensions.IngressBackend{
							ServiceName: svcs[0],
							ServicePort: intstr.IntOrString{
								Type:   0,
								IntVal: svcPorts[0],
							},
						},
					},
					},
				},
			},
		},
	}

	// mock ingress options
	o := &NewDesiredListenersOptions{
		Annotations: &annotations.Ingress{
			LoadBalancer: &loadbalancer.Config{
				Ports: []loadbalancer.PortData{{ports[0], "HTTP"}},
			},
			Listener: &listener.Config{},
			Rule: &rule.Config{
				IgnoreHostHeader: aws.Bool(false),
			},
		},
		Logger:       logger,
		IngressRules: rs,
	}

	// validate expected listener results vs actual
	ls, err := NewDesiredListeners(o)
	if err != nil {
		t.Errorf("Failed to create listeners. Error: %s", err.Error())
	}
	expProto := "HTTP"
	if schemes[0] {
		expProto = "HTTPS"
	}

	switch {
	case len(ls) != 1:
		t.Errorf("Created %d listeners, should have been %d", len(ls), 1)
	case *ls[0].ls.desired.Port != ports[0]:
		t.Errorf("Port was %d should have been %d", *ls[0].ls.desired.Port, ports[0])
	case *ls[0].ls.desired.Protocol != expProto:
		t.Errorf("Invalid protocol was %s should have been %s", *ls[0].ls.desired.Protocol, expProto)
	case len(ls[0].rules) != 2:
		t.Errorf("Quantity of rules attached to listener is invalid. Was %d, expected %d.", len(ls[0].rules), 2)

	}
}

func TestMultipleListeners(t *testing.T) {
	as := &annotations.Ingress{
		LoadBalancer: &loadbalancer.Config{},
		Listener:     &listener.Config{},
		Rule:         &rule.Config{},
	}
	rs := []extensions.IngressRule{}

	// create annotations and listeners
	for i := range ports {
		as.LoadBalancer.Ports = append(as.LoadBalancer.Ports, loadbalancer.PortData{ports[i], "HTTP"})
		if schemes[i] {
			as.LoadBalancer.Scheme = aws.String("HTTPS")
		}
		as.Rule.IgnoreHostHeader = aws.Bool(false)

		extRules := extensions.IngressRule{
			Host: hosts[i],
			IngressRuleValue: extensions.IngressRuleValue{
				HTTP: &extensions.HTTPIngressRuleValue{
					Paths: []extensions.HTTPIngressPath{{
						Path: paths[i],
						Backend: extensions.IngressBackend{
							ServiceName: svcs[i],
							ServicePort: intstr.IntOrString{
								Type:   0,
								IntVal: svcPorts[i],
							},
						},
					},
					},
				},
			},
		}
		rs = append(rs, extRules)
	}

	// mock ingress options
	o := &NewDesiredListenersOptions{
		Annotations:  as,
		Logger:       logger,
		IngressRules: rs,
	}
	ls, err := NewDesiredListeners(o)
	if err != nil {
		t.Errorf("Failed to create listeners. Error: %s", err.Error())
	}

	// validate expected listener results vs actual
	for i := range as.LoadBalancer.Ports {
		// expProto := "HTTP"
		// if schemes[i] {
		// 	expProto = "HTTPS"
		// }

		switch {
		case len(ls) != len(ports):
			t.Errorf("Created %d listeners, should have been %d", len(ls), len(ports))
		case *ls[i].ls.desired.Port != ports[i]:
			t.Errorf("Port was %d should have been %d", *ls[i].ls.desired.Port, ports[i])
		// case *ls[i].ls.desired.Protocol != expProto:
		// 	t.Errorf("Invalid protocol was %s should have been %s", *ls[i].ls.desired.Protocol, expProto)
		case len(ls[i].rules) != len(ports)+1:
			t.Errorf("Quantity of rules attached to listener is invalid. Was %d, expected %d.", len(ls[i].rules), len(ports)+1)
		case !ls[i].rules[0].IsDesiredDefault():
			fmt.Println(awsutil.Prettify(ls[i].rules))
			t.Errorf("1st rule wasn't marked as default rule.")
		case ls[i].rules[1].IsDesiredDefault():
			fmt.Println(awsutil.Prettify(ls[i].rules))
			t.Errorf("2nd rule was marked as default, should only be the first")
		}
	}
}
