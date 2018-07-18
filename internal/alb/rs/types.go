package rs

import (
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

// Rules contains a slice of Rules
type Rules []*Rule

// Rule contains a current/desired Rule
type Rule struct {
	rs      rs
	svc     svc
	deleted bool
	logger  *log.Logger
}

type rs struct {
	current *elbv2.Rule
	desired *elbv2.Rule
}

type svc struct {
	current service
	desired service
}

type service struct {
	name string
	port int32
}

type ReconcileOptions struct {
	Eventf        func(string, string, string, ...interface{})
	ListenerArn   *string
	ListenerRules *Rules
	TargetGroups  tg.TargetGroups
}
