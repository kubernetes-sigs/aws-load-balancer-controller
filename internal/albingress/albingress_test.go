package albingress

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
)

func init() {
	mockEC2 := &mocks.EC2API{}
	mockEC2.On("GetVPCID").Return(aws.String("vpc-id"), nil)
	albec2.EC2svc = mockEC2
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
