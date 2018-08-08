package albingress

import (
	"time"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	pool "gopkg.in/go-playground/pool.v3"

	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/record"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/class"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"
)

// NewALBIngressesFromIngressesOptions are the options to NewALBIngressesFromIngresses
type NewALBIngressesFromIngressesOptions struct {
	Recorder     record.EventRecorder
	Store        store.Storer
	ALBIngresses ALBIngresses
}

// NewALBIngressesFromIngresses returns a ALBIngresses created from the Kubernetes ingress state.
func NewALBIngressesFromIngresses(o *NewALBIngressesFromIngressesOptions) ALBIngresses {
	var ALBIngresses ALBIngresses

	// Find every ingress currently in Kubernetes.
	for _, ingResource := range o.Store.ListIngresses() {
		// Ensure the ingress resource found contains an appropriate ingress class.
		if !class.IsValid(ingResource) {
			continue
		}

		applyDefaults(ingResource)

		// Find the existing ingress for this Kubernetes ingress (if it existed).
		id := k8s.MetaNamespaceKey(ingResource)
		_, existingIngress := o.ALBIngresses.FindByID(id)

		// Produce a new ALBIngress instance for every ingress found. If ALBIngress returns nil, there
		// was an issue with the ingress (e.g. bad annotations) and should not be added to the list.
		ALBIngress := NewALBIngressFromIngress(&NewALBIngressFromIngressOptions{
			Ingress:         ingResource,
			ExistingIngress: existingIngress,
			Store:           o.Store,
			Recorder:        o.Recorder,
		})

		// Add the new ALBIngress instance to the new ALBIngress list.
		ALBIngresses = append(ALBIngresses, ALBIngress)
	}
	return ALBIngresses
}

// AssembleIngressesFromAWSOptions are the options to AssembleIngressesFromAWS
type AssembleIngressesFromAWSOptions struct {
	Store    store.Storer
	Recorder record.EventRecorder
}

// AssembleIngressesFromAWS builds a list of existing ingresses from resources in AWS
func AssembleIngressesFromAWS(o *AssembleIngressesFromAWSOptions) ALBIngresses {

	glog.Infof("Building list of existing ALBs")
	t0 := time.Now()

	// Fetch the list of load balancers
	loadBalancers, err := albelbv2.ELBV2svc.ClusterLoadBalancers()
	if err != nil {
		glog.Fatalf(err.Error())
	}
	glog.Infof("Fetching information on %d ALBs", len(loadBalancers))

	// Fetch the list of target groups
	targetGroups, err := albelbv2.ELBV2svc.ClusterTargetGroups()
	if err != nil {
		glog.Fatalf(err.Error())
	}
	glog.V(2).Infof("Retrieved information on %v target groups", len(targetGroups))

	ingresses := newIngressesFromLoadBalancers(&newIngressesFromLoadBalancersOptions{
		LoadBalancers: loadBalancers,
		Recorder:      o.Recorder,
		Store:         o.Store,
		TargetGroups:  targetGroups,
	})

	glog.Infof("Assembled %d ingresses from existing AWS resources in %v", len(ingresses), time.Now().Sub(t0))
	if len(loadBalancers) != len(ingresses) {
		glog.Fatalf("Assembled %d ingresses from %v load balancers", len(ingresses), len(loadBalancers))
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

// Reconcile syncs the desired state to the current state
func (a ALBIngresses) Reconcile() {
	p := pool.NewLimited(20)
	defer p.Close()

	batch := p.Batch()

	for _, ingress := range a {
		batch.Queue(func(ingress *ALBIngress) pool.WorkFunc {
			return func(wu pool.WorkUnit) (interface{}, error) {
				if wu.IsCancelled() {
					return nil, nil
				}

				ingress.Reconcile(&ReconcileOptions{Eventf: ingress.Eventf, Store: ingress.store})
				return nil, nil
			}
		}(ingress))
	}

	batch.QueueComplete()
	for e := range batch.Results() {
		err := e.Error()
		if _, ok := err.(*pool.ErrRecovery); ok {
			glog.Fatal(err.Error())
		}
	}
}

// IngressesByNamespace returns the count of ingresses per namespace
func (a ALBIngresses) IngressesByNamespace() map[string]int {
	ingressesByNamespace := map[string]int{}
	for _, ingress := range a {
		ingressesByNamespace[ingress.namespace]++
	}
	return ingressesByNamespace
}

type newIngressesFromLoadBalancersOptions struct {
	LoadBalancers []*elbv2.LoadBalancer
	TargetGroups  map[string][]*elbv2.TargetGroup
	Recorder      record.EventRecorder
	Store         store.Storer
}

func newIngressesFromLoadBalancers(o *newIngressesFromLoadBalancersOptions) ALBIngresses {
	var ingresses ALBIngresses
	// Generate the list of ingresses from those load balancers

	p := pool.NewLimited(10)
	defer p.Close()

	batch := p.Batch()

	for _, loadBalancer := range o.LoadBalancers {
		batch.Queue(func(loadBalancer *elbv2.LoadBalancer) pool.WorkFunc {
			return func(wu pool.WorkUnit) (interface{}, error) {
				if wu.IsCancelled() {
					return nil, nil
				}

				albIngress, err := NewALBIngressFromAWSLoadBalancer(&NewALBIngressFromAWSLoadBalancerOptions{
					LoadBalancer: loadBalancer,
					Recorder:     o.Recorder,
					TargetGroups: o.TargetGroups,
					Store:        o.Store,
				})
				if err != nil {
					glog.Infof(err.Error())
					return nil, err
				}
				ingresses = append(ingresses, albIngress)
				return nil, nil
			}
		}(loadBalancer))
	}

	batch.QueueComplete()
	for e := range batch.Results() {
		err := e.Error()
		if _, ok := err.(*pool.ErrRecovery); ok {
			glog.Fatal(err.Error())
		}
	}

	return ingresses
}

func applyDefaults(i *extensions.Ingress) {
	if i.Spec.Backend == nil {
	BACKEND:
		for _, r := range i.Spec.Rules {
			if r.HTTP == nil {
				continue
			}
			for _, p := range r.HTTP.Paths {
				i.Spec.Backend = &p.Backend
				break BACKEND
			}
		}
	}
}
