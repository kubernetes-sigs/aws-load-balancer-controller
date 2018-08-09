package albingress

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"

	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/record"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/lb"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

const MaxRetryTime = 2 * time.Hour
const restrictIngressConfigMap = "alb-ingress-controller-internet-facing-ingresses"

type NewALBIngressOptions struct {
	Namespace  string
	Name       string
	Ingress    *extensions.Ingress
	Recorder   record.EventRecorder
	Reconciled bool
	Store      store.Storer
}

// NewALBIngress returns a minimal ALBIngress instance with a generated name that allows for lookup
// when new ALBIngress objects are created to determine if an instance of that ALBIngress already
// exists.
func NewALBIngress(o *NewALBIngressOptions) *ALBIngress {
	var id string
	if o.Ingress != nil {
		id = k8s.MetaNamespaceKey(o.Ingress)
	} else {
		id = fmt.Sprintf(o.Namespace + "/" + o.Name)
	}

	b := backoff.NewExponentialBackOff()
	b.InitialInterval = time.Minute
	b.MaxInterval = time.Hour
	b.MaxElapsedTime = MaxRetryTime
	b.RandomizationFactor = 0.01
	b.Multiplier = 2
	return &ALBIngress{
		id:          id,
		namespace:   o.Namespace,
		ingressName: o.Name,
		lock:        new(sync.Mutex),
		logger:      log.New(id),
		store:       o.Store,
		recorder:    o.Recorder,
		reconciled:  o.Reconciled,
		ingress:     o.Ingress,
		backoff:     b,
		nextAttempt: 0,
		prevAttempt: 0,
	}
}

type NewALBIngressFromIngressOptions struct {
	Ingress         *extensions.Ingress
	ExistingIngress *ALBIngress
	Recorder        record.EventRecorder
	Store           store.Storer
}

// NewALBIngressFromIngress builds ALBIngress's based off of an Ingress object
// https://godoc.org/k8s.io/kubernetes/pkg/apis/extensions#Ingress. Creates a new ingress object,
// and looks up to see if a previous ingress object with the same id is known to the ALBController.
// If there is an issue and the ingress is invalid, nil is returned.
func NewALBIngressFromIngress(o *NewALBIngressFromIngressOptions) *ALBIngress {
	var err error

	// Create newIngress ALBIngress object holding the resource details and some cluster information.
	newIngress := NewALBIngress(&NewALBIngressOptions{
		Namespace: o.Ingress.GetNamespace(),
		Name:      o.Ingress.Name,
		Recorder:  o.Recorder,
		Ingress:   o.Ingress,
		Store:     o.Store,
	})

	if o.ExistingIngress != nil {
		if !o.ExistingIngress.ready() && !o.ExistingIngress.valid {
			// silently fail to assemble the ingress if we are not ready to retry it
			o.ExistingIngress.reconciled = false
			return o.ExistingIngress
		}

		// Acquire a lock to prevent race condition if existing ingress's state is currently being synced
		// with Amazon..
		newIngress = o.ExistingIngress
		newIngress.lock.Lock()
		defer newIngress.lock.Unlock()
		// reattach k8s ingress as if assembly happened through aws sync, it may be missing.
		newIngress.ingress = o.Ingress
		// Ensure all desired state is removed from the copied ingress. The desired state of each
		// component will be generated later in this function.
		newIngress.stripDesiredState()
		newIngress.valid = false
	}

	newIngress.reconciled = false

	// Load up the ingress with our current annotations.
	newIngress.annotations, err = o.Store.GetIngressAnnotations(newIngress.id)
	if err != nil {
		if _, ok := err.(store.NotExistsError); ok {
			newIngress.resetBackoff() // Don't blame the ingress for the annotations not being in sync
			return newIngress
		}
		msg := fmt.Sprintf("error parsing annotations: %s", err.Error())
		newIngress.incrementBackoff()
		newIngress.Eventf(api.EventTypeWarning, "ERROR", msg)
		newIngress.logger.Errorf(msg)
		newIngress.logger.Errorf("Will retry in %v", newIngress.nextAttempt)
		return newIngress
	}

	tags := append(newIngress.annotations.Tags.LoadBalancer, newIngress.Tags()...)

	// Check if we are restricting internet facing ingresses and if this ingress is allowed
	if o.Store.GetConfig().RestrictScheme && *newIngress.annotations.LoadBalancer.Scheme == "internet-facing" {
		allowed, err := newIngress.ingressAllowedExternal(o.Store.GetConfig().RestrictSchemeNamespace)
		if err != nil {
			msg := fmt.Sprintf("error getting restricted ingresses ConfigMap: %s", err.Error())
			newIngress.incrementBackoff()
			newIngress.logger.Errorf(msg)
			newIngress.logger.Errorf("Will retry in %v", newIngress.nextAttempt)
			return newIngress
		}
		if !allowed {
			msg := fmt.Sprintf("ingress %s/%s is not allowed to be internet-facing", o.Ingress.GetNamespace(), o.Ingress.Name)
			newIngress.incrementBackoff()
			newIngress.Eventf(api.EventTypeWarning, "ERROR", msg)
			newIngress.logger.Errorf(msg)
			newIngress.logger.Errorf("Will retry in %v", newIngress.nextAttempt)
			return newIngress
		}
	}

	// Assemble the load balancer
	newIngress.loadBalancer, err = lb.NewDesiredLoadBalancer(&lb.NewDesiredLoadBalancerOptions{
		ExistingLoadBalancer: newIngress.loadBalancer,
		Ingress:              o.Ingress,
		Logger:               newIngress.logger,
		Store:                o.Store,
		CommonTags:           tags,
	})

	if err != nil {
		msg := fmt.Sprintf("error instantiating load balancer: %s", err.Error())
		newIngress.incrementBackoff()
		newIngress.Eventf(api.EventTypeWarning, "ERROR", msg)
		newIngress.logger.Errorf(msg)
		newIngress.logger.Errorf("Will retry in %v", newIngress.nextAttempt)
		return newIngress
	}

	newIngress.valid = true
	return newIngress
}

func tagsFromIngress(r util.ELBv2Tags) (string, string, error) {
	var ingressName, namespace string

	// Support legacy tags
	if v, ok := r.Get("IngressName"); ok {
		ingressName = v
	}
	if v, ok := r.Get("Namespace"); ok {
		namespace = v
	}

	if v, ok := r.Get("kubernetes.io/ingress-name"); ok {
		p := strings.Split(v, "/")
		if len(p) == 2 {
			return p[0], p[1], nil
		}
		ingressName = v
	}

	if v, ok := r.Get("kubernetes.io/namespace"); ok {
		namespace = v
	}

	if ingressName == "" {
		return namespace, ingressName, fmt.Errorf("kubernetes.io/ingress-name tag is missing")
	}

	if namespace == "" {
		return namespace, ingressName, fmt.Errorf("kubernetes.io/namespace tag is missing")
	}

	return namespace, ingressName, nil
}

type NewALBIngressFromAWSLoadBalancerOptions struct {
	LoadBalancer *elbv2.LoadBalancer
	Store        store.Storer
	Recorder     record.EventRecorder
	TargetGroups map[string][]*elbv2.TargetGroup
}

// NewALBIngressFromAWSLoadBalancer builds ALBIngress's based off of an elbv2.LoadBalancer
func NewALBIngressFromAWSLoadBalancer(o *NewALBIngressFromAWSLoadBalancerOptions) (*ALBIngress, error) {
	resourceTags, err := albrgt.RGTsvc.GetClusterResources()
	if err != nil {
		return nil, fmt.Errorf("Failed to get AWS tags. Error: %s", err.Error())
	}

	namespace, ingressName, err := tagsFromIngress(resourceTags.LoadBalancers[*o.LoadBalancer.LoadBalancerArn])
	if err != nil {
		return nil, fmt.Errorf("The LoadBalancer %s does not have the proper tags, can't import: %s", *o.LoadBalancer.LoadBalancerName, err.Error())
	}

	// Assemble ingress
	ingress := NewALBIngress(&NewALBIngressOptions{
		Namespace:  namespace,
		Name:       ingressName,
		Recorder:   o.Recorder,
		Store:      o.Store,
		Reconciled: true,
	})

	// Assemble load balancer
	ingress.loadBalancer, err = lb.NewCurrentLoadBalancer(&lb.NewCurrentLoadBalancerOptions{
		LoadBalancer: o.LoadBalancer,
		TargetGroups: o.TargetGroups,
		Logger:       ingress.logger,
	})
	if err != nil {
		return nil, err
	}

	// Assembly complete
	ingress.logger.Infof("Ingress rebuilt from existing ALB in AWS")
	ingress.valid = true
	return ingress, nil
}

// Eventf writes an event to the ALBIngress's Kubernetes ingress resource
func (a *ALBIngress) Eventf(eventtype, reason, messageFmt string, args ...interface{}) {
	if a.ingress == nil || a.recorder == nil {
		return
	}
	a.recorder.Eventf(a.ingress, eventtype, reason, messageFmt, args...)
}

// Hostnames returns the AWS hostnames for the load balancer
func (a *ALBIngress) Hostnames() ([]api.LoadBalancerIngress, error) {
	var hostnames []api.LoadBalancerIngress

	if a.reconciled != true {
		return nil, fmt.Errorf("ingress is not in sync, hostname invalid")
	}

	if a.loadBalancer == nil {
		return hostnames, nil
	}

	if a.loadBalancer.Hostname() != nil {
		hostnames = append(hostnames, api.LoadBalancerIngress{Hostname: *a.loadBalancer.Hostname()})
	}

	if len(hostnames) == 0 {
		return nil, fmt.Errorf("No ALB hostnames for ingress")
	}

	return hostnames, nil
}

// Reconcile begins the state sync for all AWS resource satisfying this ALBIngress instance.
func (a *ALBIngress) Reconcile(rOpts *ReconcileOptions) {
	a.lock.Lock()
	defer a.lock.Unlock()

	// Ingress isn't a valid state, skip it
	if !a.valid {
		return
	}

	// Check if we are ready to attempt a reconcile
	if !a.ready() {
		return
	}

	errors := a.loadBalancer.Reconcile(
		&lb.ReconcileOptions{
			Store:  rOpts.Store,
			Eventf: rOpts.Eventf,
		})
	if len(errors) > 0 {
		// marks reconciled state as false so UpdateIngressStatus won't operate
		a.reconciled = false
		a.logger.Errorf("Failed to reconcile state on this ingress")
		for _, err := range errors {
			a.logger.Errorf(" - %s", err.Error())
		}
		a.incrementBackoff()
		a.logger.Errorf("Will retry to reconcile in %v", a.nextAttempt)
		return
	}
	// marks reconciled state as true so that UpdateIngressStatus will operate
	a.reconciled = true
	a.resetBackoff()
}

// StripDesiredState strips all desired objects from an ALBIngress
func (a *ALBIngress) stripDesiredState() {
	if a.loadBalancer != nil {
		a.loadBalancer.StripDesiredState()
	}
}

// Tags returns an elbv2.Tag slice of standard tags for the ingress AWS resources
func (a *ALBIngress) Tags() (tags []*elbv2.Tag) {
	// https://github.com/kubernetes/kubernetes/blob/master/pkg/cloudprovider/providers/aws/tags.go
	tags = append(tags, &elbv2.Tag{
		Key:   aws.String("kubernetes.io/cluster/" + a.store.GetConfig().ClusterName),
		Value: aws.String("owned"),
	})

	tags = append(tags, &elbv2.Tag{
		Key:   aws.String("kubernetes.io/namespace"),
		Value: aws.String(a.namespace),
	})

	tags = append(tags, &elbv2.Tag{
		Key:   aws.String("kubernetes.io/ingress-name"),
		Value: aws.String(a.ingressName),
	})

	return tags
}

func (a *ALBIngress) resetBackoff() {
	a.backoff.Reset()
	a.nextAttempt = 0
	a.prevAttempt = 0
}

func (a *ALBIngress) ready() bool {
	if a.backoff.GetElapsedTime() < a.nextAttempt+a.prevAttempt {
		return false
	}
	return true
}

func (a *ALBIngress) incrementBackoff() {
	a.prevAttempt = a.backoff.GetElapsedTime()
	a.nextAttempt = a.backoff.NextBackOff()
	if a.nextAttempt == backoff.Stop {
		a.nextAttempt = MaxRetryTime
	}
}

func (a *ALBIngress) ingressAllowedExternal(configNamespace string) (bool, error) {
	configMap, err := a.store.GetConfigMap(configNamespace + "/" + restrictIngressConfigMap)
	if err != nil {
		return false, err
	}
	for ns, ingressString := range configMap.Data {
		ingressString := strings.Replace(ingressString, " ", "", -1)
		ingresses := strings.Split(ingressString, ",")
		for _, ing := range ingresses {
			if a.namespace == ns && a.ingressName == ing {
				return true, nil
			}
		}
	}
	return false, nil
}
