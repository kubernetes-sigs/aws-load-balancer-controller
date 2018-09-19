package albingress

import (
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
)

func init() {
	albec2.EC2svc = albec2.NewMockEC2()
}

func TestHostnames(t *testing.T) {
	ing := dummy.NewIngress()
	store := store.NewDummy()

	options := &NewALBIngressFromIngressOptions{
		Ingress: ing,
		Store:   store,
	}
	ingress, err := NewALBIngressFromIngress(options)
	if ingress == nil {
		t.Errorf("NewALBIngressFromIngress returned nil")
	}
	if err != nil {
		t.Errorf(err.Error())
	}
	ingress.reconciled = true
	ingress.loadBalancer = nil
	_, err = ingress.Hostnames()
	if err == nil {
		t.Errorf("A nil ingress status should result in hostname retrieval error")
	}
}

func TestNewALBIngressFromIngress(t *testing.T) {
	ing := dummy.NewIngress()
	store := store.NewDummy()

	options := &NewALBIngressFromIngressOptions{
		Ingress: ing,
		Store:   store,
	}
	ingress, err := NewALBIngressFromIngress(options)
	if ingress == nil {
		t.Errorf("NewALBIngressFromIngress returned nil")
	}
	if err != nil {
		t.Error(err)
	}
}
