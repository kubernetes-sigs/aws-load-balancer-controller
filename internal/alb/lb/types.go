package lb

import (
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/sg"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/ls"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	extensions "k8s.io/api/extensions/v1beta1"
)

// LoadBalancer contains the overarching configuration for the ALB
type LoadBalancer struct {
	id            string
	lb            lb
	tags          *tags.Tags
	attributes    *Attributes
	targetgroups  tg.TargetGroups
	listeners     ls.Listeners
	sgAssociation sg.Association
	options       options

	deleted bool // flag representing the LoadBalancer instance was fully deleted.
	logger  *log.Logger
}

type lb struct {
	current *elbv2.LoadBalancer // current version of load balancer in AWS
	desired *elbv2.LoadBalancer // desired version of load balancer in AWS
}

type options struct {
	current opts
	desired opts
}

type opts struct {
	webACLId *string
}

func (o options) needsModification() loadBalancerChange {
	var changes loadBalancerChange
	if o.desired.webACLId != nil && o.current.webACLId == nil ||
		o.desired.webACLId == nil && o.current.webACLId != nil ||
		(o.current.webACLId != nil && o.desired.webACLId != nil && *o.current.webACLId != *o.desired.webACLId) {
		changes |= webACLAssociationModified
	}
	return changes
}

type loadBalancerChange uint

const (
	subnetsModified loadBalancerChange = 1 << iota
	schemeModified
	ipAddressTypeModified
	webACLAssociationModified
)

type ReconcileOptions struct {
	Store                   store.Storer
	Ingress                 *extensions.Ingress
	SgAssociationController sg.AssociationController
	LbAttributesController  AttributesController
	TgAttributesController  tg.AttributesController
	TagsController          tags.Controller
	Eventf                  func(string, string, string, ...interface{})
}

type portList []int64

func (a portList) Len() int           { return len(a) }
func (a portList) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a portList) Less(i, j int) bool { return a[i] < a[j] }
