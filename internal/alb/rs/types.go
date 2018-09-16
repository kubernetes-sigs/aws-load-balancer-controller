package rs

import (
	"strings"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	"k8s.io/apimachinery/pkg/util/intstr"
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

func (r *Rule) String() string {
	if r == nil {
		return "<nil>"
	}

	return "[" + strings.Join([]string{
		"CurrentRule: " + log.String(r.rs.current),
		"DesiredRule: " + log.String(r.rs.desired),
		"CurrentService: " + log.String(r.svc.current),
		"DesiredService: " + log.String(r.svc.desired),
	}, ", ") + "]"
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
	port intstr.IntOrString
}

func (s service) String() string {
	return "[" + strings.Join([]string{
		"name: " + s.name,
		"port: " + log.String(&s.port),
	}, ", ") + "]"
}

type ReconcileOptions struct {
	Eventf        func(string, string, string, ...interface{})
	ListenerArn   *string
	ListenerRules *Rules
	TargetGroups  tg.TargetGroups
}
