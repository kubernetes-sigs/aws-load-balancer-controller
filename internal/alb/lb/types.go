package lb

import (
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/ls"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

// LoadBalancer contains the overarching configuration for the ALB
type LoadBalancer struct {
	id           string
	lb           lb
	tags         tags
	attributes   attributes
	targetgroups tg.TargetGroups
	listeners    ls.Listeners
	options      options

	deleted bool // flag representing the LoadBalancer instance was fully deleted.
	logger  *log.Logger
}

type lb struct {
	current *elbv2.LoadBalancer // current version of load balancer in AWS
	desired *elbv2.LoadBalancer // desired version of load balancer in AWS
}

type attributes struct {
	current albelbv2.LoadBalancerAttributes
	desired albelbv2.LoadBalancerAttributes
}

type tags struct {
	current util.ELBv2Tags
	desired util.ELBv2Tags
}

type options struct {
	current opts
	desired opts
}

type opts struct {
	ports             portList
	inboundCidrs      util.Cidrs
	webACLId          *string
	managedSG         *string
	managedInstanceSG *string
}

type loadBalancerChange uint

const (
	securityGroupsModified loadBalancerChange = 1 << iota
	subnetsModified
	tagsModified
	schemeModified
	attributesModified
	managedSecurityGroupsModified
	ipAddressTypeModified
	webACLAssociationModified
)

type ReconcileOptions struct {
	Eventf func(string, string, string, ...interface{})
}

type portList []int64

func (a portList) Len() int           { return len(a) }
func (a portList) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a portList) Less(i, j int) bool { return a[i] < a[j] }
