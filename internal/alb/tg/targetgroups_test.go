package tg

import (
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

func init() {
	albec2.EC2svc = &mocks.EC2API{}
}

func TestNewDesiredTargetGroups(t *testing.T) {
	ing := dummy.NewIngress()
	tg, err := NewDesiredTargetGroups(&NewDesiredTargetGroupsOptions{
		Ingress:        ing,
		LoadBalancerID: "lbid",
		Store:          store.NewDummy(),
		CommonTags:     tags.NewTags(),
		Logger:         log.New("logger"),
	})
	if err != nil {
		t.Errorf(err.Error())
	}

	expected := len(ing.Spec.Rules) + 1 // +1 for default backend

	if len(tg) != expected {
		t.Errorf("%v target groups were expected, got %v", expected, len(tg))
	}
}
