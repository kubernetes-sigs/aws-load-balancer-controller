package rs

import (
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

// Rules contains a slice of Rules
type Rules []*Rule

// Rule contains a current/desired Rule
type Rule struct {
	rs      rs
	svcname svcname
	deleted bool
	logger  *log.Logger
}

type rs struct {
	current *elbv2.Rule
	desired *elbv2.Rule
}

type svcname struct {
	current string
	desired string
}

type ReconcileOptions struct {
	Eventf        func(string, string, string, ...interface{})
	ListenerArn   *string
	ListenerRules *Rules
	TargetGroups  tg.TargetGroups
}
