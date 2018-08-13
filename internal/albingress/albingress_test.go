package albingress

import (
	"os"
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
)

func init() {
	os.Setenv("AWS_VPC_ID", "vpc-id")
}

func TestNewALBIngressFromIngress(t *testing.T) {
	ing := dummy.NewIngress()
	store := store.NewDummy()

	options := &NewALBIngressFromIngressOptions{
		Ingress: ing,
		Store:   store,
	}
	ingress := NewALBIngressFromIngress(options)
	if ingress == nil {
		t.Errorf("NewALBIngressFromIngress returned nil")
	}
}
