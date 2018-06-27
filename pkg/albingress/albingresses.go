package albingress

import (
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/service/elbv2"

	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/record"
	"k8s.io/ingress/core/pkg/ingress/annotations/class"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/albrgt"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

// NewALBIngressesFromIngressesOptions are the options to NewALBIngressesFromIngresses
type NewALBIngressesFromIngressesOptions struct {
	Recorder              record.EventRecorder
	ClusterName           string
	ALBNamePrefix         string
	Ingresses             []interface{}
	ALBIngresses          ALBIngresses
	IngressClass          string
	DefaultIngressClass   string
	GetServiceNodePort    func(string, int32) (*int64, error)
	GetServiceAnnotations func(string, string) (*map[string]string, error)
	GetNodes              func() util.AWSStringSlice
	AnnotationFactory     annotations.AnnotationFactory
}

// NewALBIngressesFromIngresses returns a ALBIngresses created from the Kubernetes ingress state.
func NewALBIngressesFromIngresses(o *NewALBIngressesFromIngressesOptions) ALBIngresses {
	var ALBIngresses ALBIngresses

	// Find every ingress currently in Kubernetes.
	for _, k8singress := range o.Ingresses {
		ingResource := k8singress.(*extensions.Ingress)

		// Ensure the ingress resource found contains an appropriate ingress class.
		if !class.IsValid(ingResource, o.IngressClass, o.DefaultIngressClass) {
			continue
		}

		// Find the existing ingress for this Kubernetes ingress (if it existed).
		id := GenerateID(ingResource.GetNamespace(), ingResource.Name)
		_, existingIngress := o.ALBIngresses.FindByID(id)

		// Produce a new ALBIngress instance for every ingress found. If ALBIngress returns nil, there
		// was an issue with the ingress (e.g. bad annotations) and should not be added to the list.
		ALBIngress := NewALBIngressFromIngress(&NewALBIngressFromIngressOptions{
			Ingress:               ingResource,
			ExistingIngress:       existingIngress,
			ClusterName:           o.ClusterName,
			ALBNamePrefix:         o.ALBNamePrefix,
			GetServiceNodePort:    o.GetServiceNodePort,
			GetServiceAnnotations: o.GetServiceAnnotations,
			GetNodes:              o.GetNodes,
			Recorder:              o.Recorder,
			AnnotationFactory:     o.AnnotationFactory,
		})

		// Add the new ALBIngress instance to the new ALBIngress list.
		ALBIngresses = append(ALBIngresses, ALBIngress)
	}
	return ALBIngresses
}

// AssembleIngressesFromAWSOptions are the options to AssembleIngressesFromAWS
type AssembleIngressesFromAWSOptions struct {
	Recorder      record.EventRecorder
	ClusterName   string
	ALBNamePrefix string
}

// AssembleIngressesFromAWS builds a list of existing ingresses from resources in AWS
func AssembleIngressesFromAWS(o *AssembleIngressesFromAWSOptions) ALBIngresses {
	var ingresses ALBIngresses
	var wg sync.WaitGroup

	logger.Infof("Building list of existing ALBs")
	t0 := time.Now()

	// Grab all of the tags for our cluster resources
	resources, err := albrgt.RGTsvc.GetResources(&o.ClusterName)
	if err != nil {
		logger.Fatalf(err.Error())
	}

	// Fetch the list of load balancers
	loadBalancers, err := albelbv2.ELBV2svc.ClusterLoadBalancers(resources)
	if err != nil {
		logger.Fatalf(err.Error())
	}

	// Fetch the list of target groups
	targetGroups, err := albelbv2.ELBV2svc.ClusterTargetGroups(resources)
	if err != nil {
		logger.Fatalf(err.Error())
	}

	logger.Infof("Fetching information on %d ALBs", len(loadBalancers))

	// Generate the list of ingresses from those load balancers
	for _, loadBalancer := range loadBalancers {
		wg.Add(1)
		go func(wg *sync.WaitGroup, loadBalancer *elbv2.LoadBalancer) {
			defer wg.Done()

			albIngress, err := NewALBIngressFromAWSLoadBalancer(&NewALBIngressFromAWSLoadBalancerOptions{
				LoadBalancer:  loadBalancer,
				ALBNamePrefix: o.ALBNamePrefix,
				Recorder:      o.Recorder,
				ResourceTags:  resources,
				TargetGroups:  targetGroups,
			})
			if err != nil {
				logger.Infof(err.Error())
				return
			}
			ingresses = append(ingresses, albIngress)
		}(&wg, loadBalancer)
	}
	wg.Wait()

	logger.Infof("Assembled %d ingresses from existing AWS resources in %v", len(ingresses), time.Now().Sub(t0))
	if len(loadBalancers) != len(ingresses) {
		logger.Fatalf("Assembled %d ingresses from %v load balancers", len(ingresses), len(loadBalancers))
	}
	return ingresses
}

// FindByID locates the ingress by the id parameter and returns its position
func (a ALBIngresses) FindByID(id string) (int, *ALBIngress) {
	for p, v := range a {
		if v.id == id {
			return p, v
		}
	}
	return -1, nil
}

// RemovedIngresses compares the ingress list to the ingress list in the type, returning any ingresses that
// are not in the ingress list parameter.
func (a ALBIngresses) RemovedIngresses(newList ALBIngresses) ALBIngresses {
	var deleteableIngress ALBIngresses

	// Loop through every ingress in current (old) ingress list known to ALBController
	for _, ingress := range a {
		// Ingress objects not found in newList might qualify for deletion.
		if i, _ := newList.FindByID(ingress.id); i < 0 {
			// If the ALBIngress still contains a LoadBalancer, it still needs to be deleted.
			// In this case, strip all desired state and add it to the deleteableIngress list.
			// If the ALBIngress contains no LoadBalancer, it was previously deleted and is
			// no longer relevant to the ALBController.
			if ingress.loadBalancer != nil {
				ingress.stripDesiredState()
				deleteableIngress = append(deleteableIngress, ingress)
			}
		}
	}
	return deleteableIngress
}
