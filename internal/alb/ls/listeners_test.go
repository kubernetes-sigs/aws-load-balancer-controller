package ls

import (
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
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
	ing := dummy.NewIngress()
	ing.Spec.Rules = ing.Spec.Rules[:1]
	dummyStore := store.NewDummy()
	// annos.LoadBalancer = &loadbalancer.Config{Ports: []loadbalancer.PortData{{Port: ports[0], Scheme: "HTTP"}}}

	tgs, _ := tg.NewDesiredTargetGroups(&tg.NewDesiredTargetGroupsOptions{
		Ingress:        ing,
		LoadBalancerID: "lbid",
		Store:          store.NewDummy(),
		CommonTags:     tags.NewTags(),
	})

	// mock ingress options
	o := &NewDesiredListenersOptions{
		Ingress:      ing,
		Store:        dummyStore,
		TargetGroups: tgs,
	}

	// validate expected listener results vs actual
	ls, err := NewDesiredListeners(o)
	if err != nil {
		t.Errorf("Failed to create listeners. Error: %s", err.Error())
	}
	expProto := elbv2.ProtocolEnumHttp
	if schemes[0] {
		expProto = elbv2.ProtocolEnumHttps
	}

	switch {
	case len(ls) != 1:
		t.Errorf("Created %d listeners, should have been %d", len(ls), 1)
	case *ls[0].ls.desired.Port != ports[0]:
		t.Errorf("Port was %d should have been %d", *ls[0].ls.desired.Port, ports[0])
	case *ls[0].ls.desired.Protocol != expProto:
		t.Errorf("Invalid protocol was %s should have been %s", *ls[0].ls.desired.Protocol, expProto)
	}
}

func TestMultipleListeners(t *testing.T) {
	ing := dummy.NewIngress()
	dummyStore := store.NewDummy()

	dummyStore.GetIngressAnnotationsResponse.LoadBalancer.Ports = nil
	// create annotations and listeners
	for i := range ports {
		dummyStore.GetIngressAnnotationsResponse.LoadBalancer.Ports = append(dummyStore.GetIngressAnnotationsResponse.LoadBalancer.Ports, loadbalancer.PortData{Port: ports[i], Scheme: elbv2.ProtocolEnumHttp})
		if schemes[i] {
			dummyStore.GetIngressAnnotationsResponse.LoadBalancer.Scheme = aws.String("HTTPS")
		}
	}

	tgs, _ := tg.NewDesiredTargetGroups(&tg.NewDesiredTargetGroupsOptions{
		Ingress:        ing,
		LoadBalancerID: "lbid",
		Store:          store.NewDummy(),
		CommonTags:     tags.NewTags(),
	})

	// mock ingress options
	o := &NewDesiredListenersOptions{
		Ingress:      ing,
		Store:        dummyStore,
		TargetGroups: tgs,
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
		}
	}
}
