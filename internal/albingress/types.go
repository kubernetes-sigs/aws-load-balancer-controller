package albingress

import (
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/lb"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/record"
)

// ALBIngresses is a list of ALBIngress. It is held by the ALBController instance and evaluated
// against to determine what should be created, deleted, and modified.
type ALBIngresses []*ALBIngress

// ALBIngress contains all information above the cluster, ingress resource, and AWS resources
// needed to assemble an ALB, TargetGroup, Listener and Rules.
type ALBIngress struct {
	id           string
	namespace    string
	ingressName  string
	backoff      *backoff.ExponentialBackOff
	nextAttempt  time.Duration
	prevAttempt  time.Duration
	store        store.Storer
	recorder     record.EventRecorder
	ingress      *extensions.Ingress
	lock         *sync.Mutex
	annotations  *annotations.Ingress
	loadBalancer *lb.LoadBalancer
	valid        bool
	logger       *log.Logger
	reconciled   bool
}

type ReconcileOptions struct {
	Store  store.Storer
	Eventf func(string, string, string, ...interface{})
}

func (a *ALBIngress) ID() string {
	return a.id
}
