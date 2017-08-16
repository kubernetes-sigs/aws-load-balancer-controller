package controller

import (
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	listenersP "github.com/coreos/alb-ingress-controller/pkg/alb/listeners"
	"github.com/coreos/alb-ingress-controller/pkg/alb/loadbalancer"
	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroups"
	"github.com/coreos/alb-ingress-controller/pkg/config"
	awsutil "github.com/coreos/alb-ingress-controller/pkg/util/aws"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/record"
)

// ALBIngress contains all information above the cluster, ingress resource, and AWS resources
// needed to assemble an ALB, TargetGroup, Listener and Rules.
type ALBIngress struct {
	id           *string
	namespace    *string
	ingressName  *string
	clusterName  *string
	recorder     record.EventRecorder
	ingress      *extensions.Ingress
	lock         *sync.Mutex
	annotations  *config.Annotations
	LoadBalancer *loadbalancer.LoadBalancer
	tainted      bool // represents that parsing or validation this ingress resource failed
	logger       *log.Logger
}

// ALBIngressesT is a list of ALBIngress. It is held by the ALBController instance and evaluated
// against to determine what should be created, deleted, and modified.
type ALBIngressesT []*ALBIngress

// NewALBIngress returns a minimal ALBIngress instance with a generated name that allows for lookup
// when new ALBIngress objects are created to determine if an instance of that ALBIngress already
// exists.
func NewALBIngress(namespace, name string, ac *ALBController) *ALBIngress {
	ingressID := fmt.Sprintf("%s/%s", namespace, name)
	return &ALBIngress{
		id:          aws.String(ingressID),
		namespace:   aws.String(namespace),
		clusterName: aws.String(ac.clusterName),
		ingressName: aws.String(name),
		lock:        new(sync.Mutex),
		logger:      log.New(ingressID),
		recorder:    ac.recorder,
	}
}

// NewALBIngressFromIngress builds ALBIngress's based off of an Ingress object
// https://godoc.org/k8s.io/kubernetes/pkg/apis/extensions#Ingress. Creates a new ingress object,
// and looks up to see if a previous ingress object with the same id is known to the ALBController.
// If there is an issue and the ingress is invalid, nil is returned.
func NewALBIngressFromIngress(ingress *extensions.Ingress, ac *ALBController) (*ALBIngress, error) {
	var err error

	// Create newIngress ALBIngress object holding the resource details and some cluster information.
	newIngress := NewALBIngress(ingress.GetNamespace(), ingress.Name, ac)

	// Find the previous version of this ingress (if it existed) and copy its Current state.
	if i := ac.ALBIngresses.find(newIngress); i >= 0 {
		// Acquire a lock to prevent race condition if existing ingress's state is currently being synced
		// with Amazon..
		*newIngress = *ac.ALBIngresses[i]
		newIngress.lock.Lock()
		defer newIngress.lock.Unlock()
		// Ensure all desired state is removed from the copied ingress. The desired state of each
		// component will be generated later in this function.
		newIngress.StripDesiredState()
	}
	newIngress.ingress = ingress

	// Load up the ingress with our current annotations.
	newIngress.annotations, err = config.ParseAnnotations(ingress.Annotations)
	if err != nil {
		msg := fmt.Sprintf("Error parsing annotations: %s", err.Error())
		newIngress.Eventf(api.EventTypeWarning, "ERROR", msg)
		newIngress.logger.Errorf(msg)
		return newIngress, err
	}

	// If annotation set is nil, its because it was cached as an invalid set before. Stop processing
	// and return nil.
	if newIngress.annotations == nil {
		msg := fmt.Sprintf("Skipping processing due to a history of bad annotations")
		newIngress.Eventf(api.EventTypeWarning, "ERROR", msg)
		newIngress.logger.Debugf(msg)
		return newIngress, err
	}

	// Assemble the load balancer
	newLoadBalancer := loadbalancer.NewLoadBalancer(ac.clusterName, ingress.GetNamespace(), ingress.Name, newIngress.logger, newIngress.annotations, newIngress.Tags())
	if newIngress.LoadBalancer != nil {
		// we had an existing LoadBalancer in ingress, so just copy the desired state over
		newIngress.LoadBalancer.DesiredLoadBalancer = newLoadBalancer.DesiredLoadBalancer
		newIngress.LoadBalancer.DesiredTags = newLoadBalancer.DesiredTags
	} else {
		// no existing LoadBalancer, so use the one we just created
		newIngress.LoadBalancer = newLoadBalancer
	}
	lb := newIngress.LoadBalancer

	// Assemble the target groups
	lb.TargetGroups, err = targetgroups.NewTargetGroupsFromIngress(&targetgroups.NewTargetGroupsFromIngressOptions{
		Ingress:              ingress,
		LoadBalancerID:       newIngress.LoadBalancer.ID,
		ExistingTargetGroups: newIngress.LoadBalancer.TargetGroups,
		Annotations:          newIngress.annotations,
		ClusterName:          &ac.clusterName,
		Namespace:            ingress.GetNamespace(),
		Tags:                 newIngress.Tags(),
		Logger:               newIngress.logger,
		GetServiceNodePort:   ac.GetServiceNodePort,
		GetNodes:             ac.GetNodes,
	})
	if err != nil {
		msg := fmt.Sprintf("Error instantiating target groups: %s", err.Error())
		newIngress.Eventf(api.EventTypeWarning, "ERROR", msg)
		newIngress.logger.Errorf(msg)
		return newIngress, err
	}

	// Assemble the listeners
	lb.Listeners, err = listenersP.NewListenersFromIngress(&listenersP.NewListenersFromIngressOptions{
		Ingress:     ingress,
		Listeners:   &newIngress.LoadBalancer.Listeners,
		Annotations: newIngress.annotations,
		Logger:      newIngress.logger,
	})
	if err != nil {
		msg := fmt.Sprintf("Error instantiating listeners: %s", err.Error())
		newIngress.Eventf(api.EventTypeWarning, "ERROR", msg)
		newIngress.logger.Errorf(msg)
		return newIngress, err
	}

	return newIngress, nil
}

// NewALBIngressFromAWSLoadBalancer builds ALBIngress's based off of an elbv2.LoadBalancer
func NewALBIngressFromAWSLoadBalancer(loadBalancer *elbv2.LoadBalancer, ac *ALBController) (*ALBIngress, error) {
	logger.Debugf("Fetching Tags for %s", *loadBalancer.LoadBalancerArn)
	tags, err := awsutil.ALBsvc.DescribeTagsForArn(loadBalancer.LoadBalancerArn)
	if err != nil {
		return nil, fmt.Errorf("Unable to get tags for %s: %s", *loadBalancer.LoadBalancerName, err.Error())
	}

	ingressName, ok := tags.Get("IngressName")
	if !ok {
		return nil, fmt.Errorf("The LoadBalancer %s does not have an IngressName tag, can't import", *loadBalancer.LoadBalancerName)
	}

	namespace, ok := tags.Get("Namespace")
	if !ok {
		return nil, fmt.Errorf("The LoadBalancer %s does not have an Namespace tag, can't import", *loadBalancer.LoadBalancerName)
	}

	// Assemble ingress
	ingress := NewALBIngress(namespace, ingressName, ac)

	// Assemble load balancer
	ingress.LoadBalancer, err = loadbalancer.NewLoadBalancerFromAWSLoadBalancer(loadBalancer, tags, ac.clusterName, ingress.logger)
	if err != nil {
		return nil, err
	}

	// Assemble target groups
	targetGroups, err := awsutil.ALBsvc.DescribeTargetGroupsForLoadBalancer(loadBalancer.LoadBalancerArn)
	if err != nil {
		return nil, err
	}

	ingress.LoadBalancer.TargetGroups, err = targetgroups.NewTargetGroupsFromAWSTargetGroups(targetGroups, ac.clusterName, ingress.LoadBalancer.ID, ingress.logger)
	if err != nil {
		return nil, err
	}

	// Assemble listeners
	listeners, err := awsutil.ALBsvc.DescribeListenersForLoadBalancer(loadBalancer.LoadBalancerArn)
	if err != nil {
		return nil, err
	}

	ingress.LoadBalancer.Listeners, err = listenersP.NewListenersFromAWSListeners(listeners, ingress.logger)
	if err != nil {
		return nil, err
	}

	// Assembly complete
	logger.Infof("Ingress rebuilt from existing ALB in AWS")

	return ingress, nil
}

// Eventf writes an event to the ALBIngress's Kubernetes ingress resource
func (a *ALBIngress) Eventf(eventtype, reason, messageFmt string, args ...interface{}) {
	if a.ingress == nil || a.recorder == nil {
		return
	}
	a.recorder.Eventf(a.ingress, eventtype, reason, messageFmt, args...)
}

// Reconcile begins the state sync for all AWS resource satisfying this ALBIngress instance.
func (a *ALBIngress) Reconcile(rOpts *ReconcileOptions) {
	a.lock.Lock()
	defer a.lock.Unlock()
	// If the ingress resource failed to assemble, don't attempt reconcile
	if a.tainted {
		return
	}

	errors := a.LoadBalancer.Reconcile(loadbalancer.NewReconcileOptions().SetEventf(rOpts.Eventf).SetLoadBalancer(a.LoadBalancer))
	if len(errors) > 0 {
		a.logger.Errorf("Failed to reconcile state on this ingress")
		for _, err := range errors {
			a.logger.Errorf(" - %s", err.Error())
		}
	}
}

// Name returns the name of the ingress
func (a *ALBIngress) Name() string {
	return fmt.Sprintf("%s-%s", *a.namespace, *a.ingressName)
}

// StripDesiredState strips all desired objects from an ALBIngress
func (a *ALBIngress) StripDesiredState() {
	if a.LoadBalancer != nil {
		a.LoadBalancer.StripDesiredState()
	}
}

// Tags returns an elbv2.Tag slice of standard tags for the ingress AWS resources
func (a *ALBIngress) Tags() []*elbv2.Tag {
	tags := a.annotations.Tags

	tags = append(tags, &elbv2.Tag{
		Key:   aws.String("Namespace"),
		Value: a.namespace,
	})

	tags = append(tags, &elbv2.Tag{
		Key:   aws.String("IngressName"),
		Value: a.ingressName,
	})

	return tags
}

func (a ALBIngressesT) find(b *ALBIngress) int {
	for p, v := range a {
		if *v.id == *b.id {
			return p
		}
	}
	return -1
}

type ReconcileOptions struct {
	Eventf func(string, string, string, ...interface{})
}

func NewReconcileOptions() *ReconcileOptions {
	return &ReconcileOptions{}
}

func (r *ReconcileOptions) SetEventf(f func(string, string, string, ...interface{})) *ReconcileOptions {
	r.Eventf = f
	return r
}
