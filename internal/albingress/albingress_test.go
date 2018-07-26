package albingress

import (
	"os"
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
)

func init() {
	os.Setenv("AWS_VPC_ID", "vpc-id")
}

func TestNewALBIngressFromIngress(t *testing.T) {
	ing := store.NewDummyIngress()
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
