package ls

import (
	"fmt"
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

var (
	ports   []int64
	schemes []bool
)

func init() {
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
}

func TestNewSingleListener(t *testing.T) {
	ing := store.NewDummyIngress()
	ing.Spec.Rules = ing.Spec.Rules[:1]
	dummyStore := store.NewDummy()
	// annos.LoadBalancer = &loadbalancer.Config{Ports: []loadbalancer.PortData{{Port: ports[0], Scheme: "HTTP"}}}
	// annos.Rule = &rule.Config{IgnoreHostHeader: aws.Bool(false)}

	// mock ingress options
	o := &NewDesiredListenersOptions{
		Ingress: ing,
		Store:   dummyStore,
		Logger:  log.New("test"),
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
	ing := store.NewDummyIngress()
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
		Logger:  log.New("test"),
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
