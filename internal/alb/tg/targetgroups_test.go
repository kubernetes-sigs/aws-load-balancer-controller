package tg

import (
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

func init() {
	albec2.EC2svc = albec2.NewMockEC2()
}

func TestNewDesiredTargetGroups(t *testing.T) {
	ing := dummy.NewIngress()
	tg, err := NewDesiredTargetGroups(&NewDesiredTargetGroupsOptions{
		Ingress:        ing,
		LoadBalancerID: "lbid",
		Store:          store.NewDummy(),
		CommonTags:     util.ELBv2Tags{},
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
