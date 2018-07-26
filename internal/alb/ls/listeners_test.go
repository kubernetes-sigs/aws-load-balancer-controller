package ls

import (
	"fmt"
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	logger   *log.Logger
	ports    []int64
	schemes  []bool
	hosts    []string
	paths    []string
	svcs     []string
	svcPorts []int
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
	svcPorts = []int{
		30001,
		30002,
		30003,
	}
}

func buildIngress() *extensions.Ingress {
	ing := &extensions.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "foo",
			Namespace: api.NamespaceDefault,
		},
		Spec: extensions.IngressSpec{
			Backend: &extensions.IngressBackend{
				ServiceName: "default-backend",
				ServicePort: intstr.FromInt(80),
			},
		},
	}
	for i := range ports {
		extRules := extensions.IngressRule{
			Host: hosts[i],
			IngressRuleValue: extensions.IngressRuleValue{
				HTTP: &extensions.HTTPIngressRuleValue{
					Paths: []extensions.HTTPIngressPath{{
						Path: paths[i],
						Backend: extensions.IngressBackend{
							ServiceName: svcs[i],
							ServicePort: intstr.FromInt(svcPorts[i]),
						},
					},
					},
				},
			},
		}
		ing.Spec.Rules = append(ing.Spec.Rules, extRules)
	}
	return ing
}

func TestNewSingleListener(t *testing.T) {
	ing := buildIngress()
	ing.Spec.Rules = ing.Spec.Rules[:1]
	dummyStore := store.NewDummy()
	// annos.LoadBalancer = &loadbalancer.Config{Ports: []loadbalancer.PortData{{Port: ports[0], Scheme: "HTTP"}}}
	// annos.Rule = &rule.Config{IgnoreHostHeader: aws.Bool(false)}

	// mock ingress options
	o := &NewDesiredListenersOptions{
		Ingress: ing,
		Store:   dummyStore,
		Logger:  logger,
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
	ing := buildIngress()
	dummyStore := store.NewDummy()

	dummyStore.GetIngressAnnotationsResponse.LoadBalancer.Ports = nil
	// create annotations and listeners
	for i := range ports {
		dummyStore.GetIngressAnnotationsResponse.LoadBalancer.Ports = append(dummyStore.GetIngressAnnotationsResponse.LoadBalancer.Ports, loadbalancer.PortData{Port: ports[i], Scheme: "HTTP"})
		if schemes[i] {
			dummyStore.GetIngressAnnotationsResponse.LoadBalancer.Scheme = aws.String("HTTPS")
		}
	}

	// mock ingress options
	o := &NewDesiredListenersOptions{
		Ingress: ing,
		Logger:  logger,
		Store:   dummyStore,
	}
	ls, err := NewDesiredListeners(o)
	if err != nil {
		t.Errorf("Failed to create listeners. Error: %s", err.Error())
	}

	// validate expected listener results vs actual
	for i := range dummyStore.GetIngressAnnotationsResponse.LoadBalancer.Ports {
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
