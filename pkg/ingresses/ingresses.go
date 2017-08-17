package ingresses

import (
	"sync"

	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/record"
	"k8s.io/ingress/core/pkg/ingress/annotations/class"

	"github.com/aws/aws-sdk-go/service/elbv2"
	albelbv2 "github.com/coreos/alb-ingress-controller/pkg/aws/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/ingress"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
)

// ALBIngressesT is a list of ALBIngress. It is held by the ALBController instance and evaluated
// against to determine what should be created, deleted, and modified.
type ALBIngressesT []*ingress.ALBIngress

var logger *log.Logger

func init() {
	logger = log.New("ingresses")
}

type NewALBIngressFromIngressesOptions struct {
	Recorder            record.EventRecorder
	ClusterName         string
	Ingresses           []interface{}
	ALBIngresses        ALBIngressesT
	IngressClass        string
	DefaultIngressClass string
	GetServiceNodePort  func(string, int32) (*int64, error)
	GetNodes            func() util.AWSStringSlice
}

func NewALBIngressFromIngresses(o *NewALBIngressFromIngressesOptions) ALBIngressesT {
	var ALBIngresses ALBIngressesT

	// Find every ingress currently in Kubernetes.
	for _, k8singress := range o.Ingresses {
		ingResource := k8singress.(*extensions.Ingress)

		// Ensure the ingress resource found contains an appropriate ingress class.
		if !class.IsValid(ingResource, o.IngressClass, o.DefaultIngressClass) {
			continue
		}

		// Find the existing ingress for this Kubernetes ingress (if it existed).
		id := ingress.ID(ingResource.GetNamespace(), ingResource.Name)
		_, existingIngress := o.ALBIngresses.FindByID(id)

		// Produce a new ALBIngress instance for every ingress found. If ALBIngress returns nil, there
		// was an issue with the ingress (e.g. bad annotations) and should not be added to the list.
		ALBIngress, err := ingress.NewALBIngressFromIngress(&ingress.NewALBIngressFromIngressOptions{
			Ingress:            ingResource,
			ExistingIngress:    existingIngress,
			ClusterName:        o.ClusterName,
			GetServiceNodePort: o.GetServiceNodePort,
			GetNodes:           o.GetNodes,
			Recorder:           o.Recorder,
		})
		if ALBIngress == nil {
			continue
		}
		if err != nil {
			ALBIngress.Tainted = true
		}
		// Add the new ALBIngress instance to the new ALBIngress list.
		ALBIngresses = append(ALBIngresses, ALBIngress)
	}

	return ALBIngresses
}

type AssembleIngressesFromAWSOptions struct {
	Recorder    record.EventRecorder
	ClusterName string
}

// AssembleIngressesFromAWS builds a list of existing ingresses from resources in AWS
func AssembleIngressesFromAWS(o *AssembleIngressesFromAWSOptions) ALBIngressesT {
	logger.Infof("Build up list of existing ingresses")
	var ingresses ALBIngressesT

	loadBalancers, err := albelbv2.ELBV2svc.GetClusterLoadBalancers(&o.ClusterName)
	if err != nil {
		logger.Fatalf(err.Error())
	}

	var wg sync.WaitGroup
	wg.Add(len(loadBalancers))

	for _, loadBalancer := range loadBalancers {
		go func(wg *sync.WaitGroup, loadBalancer *elbv2.LoadBalancer) {
			defer wg.Done()

			albIngress, err := ingress.NewALBIngressFromAWSLoadBalancer(&ingress.NewALBIngressFromAWSLoadBalancerOptions{
				LoadBalancer: loadBalancer,
				ClusterName:  o.ClusterName,
				Recorder:     o.Recorder,
			})
			if err != nil {
				logger.Fatalf(err.Error())
			}

			ingresses = append(ingresses, albIngress)
		}(&wg, loadBalancer)
	}
	wg.Wait()

	logger.Infof("Assembled %d ingresses from existing AWS resources", len(ingresses))
	return ingresses
}

func (a ALBIngressesT) Find(b *ingress.ALBIngress) int {
	for p, v := range a {
		if *v.Id == *b.Id {
			return p
		}
	}
	return -1
}

// FindByID locates the ingress by the id parameter and returns its position
func (a ALBIngressesT) FindByID(id string) (int, *ingress.ALBIngress) {
	for p, v := range a {
		if *v.Id == id {
			return p, v
		}
	}
	return -1, nil
}
