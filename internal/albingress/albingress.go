package albingress

import (
	"fmt"
	"strings"
	"sync"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"

	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/record"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/lb"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

type NewALBIngressOptions struct {
	Namespace     string
	Name          string
	ClusterName   string
	ALBNamePrefix string
	Ingress       *extensions.Ingress
	Recorder      record.EventRecorder
	Reconciled    bool
}

// NewALBIngress returns a minimal ALBIngress instance with a generated name that allows for lookup
// when new ALBIngress objects are created to determine if an instance of that ALBIngress already
// exists.
func NewALBIngress(o *NewALBIngressOptions) *ALBIngress {
	ingressID := GenerateID(o.Namespace, o.Name)
	return &ALBIngress{
		id:            ingressID,
		namespace:     o.Namespace,
		clusterName:   o.ClusterName,
		albNamePrefix: o.ALBNamePrefix,
		ingressName:   o.Name,
		lock:          new(sync.Mutex),
		logger:        log.New(ingressID),
		recorder:      o.Recorder,
		reconciled:    o.Reconciled,
		ingress:       o.Ingress,
	}
}

type NewALBIngressFromIngressOptions struct {
	Ingress               *extensions.Ingress
	ExistingIngress       *ALBIngress
	ClusterName           string
	ALBNamePrefix         string
	GetServiceNodePort    func(string, string, int32) (*int64, error)
	GetServiceAnnotations func(string, string) (*map[string]string, error)
	TargetsFunc           func(*string, string, string, *int64) albelbv2.TargetDescriptions
	Recorder              record.EventRecorder
	ConnectionIdleTimeout *int64
	AnnotationFactory     annotations.AnnotationFactory
	Resources             *albrgt.Resources
}

// NewALBIngressFromIngress builds ALBIngress's based off of an Ingress object
// https://godoc.org/k8s.io/kubernetes/pkg/apis/extensions#Ingress. Creates a new ingress object,
// and looks up to see if a previous ingress object with the same id is known to the ALBController.
// If there is an issue and the ingress is invalid, nil is returned.
func NewALBIngressFromIngress(o *NewALBIngressFromIngressOptions) *ALBIngress {
	var err error

	// Create newIngress ALBIngress object holding the resource details and some cluster information.
	newIngress := NewALBIngress(&NewALBIngressOptions{
		Namespace:     o.Ingress.GetNamespace(),
		Name:          o.Ingress.Name,
		ClusterName:   o.ClusterName,
		ALBNamePrefix: o.ALBNamePrefix,
		Recorder:      o.Recorder,
		Ingress:       o.Ingress,
	})

	if o.ExistingIngress != nil {
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

	// Load up the ingress with our current annotations.
	newIngress.annotations, err = o.AnnotationFactory.ParseAnnotations(&annotations.ParseAnnotationsOptions{
		Annotations: o.Ingress.Annotations,
		Namespace:   o.Ingress.Namespace,
		IngressName: o.Ingress.Name,
		Resources:   o.Resources,
	})
	if err != nil {
		msg := fmt.Sprintf("Error parsing annotations: %s", err.Error())
		newIngress.reconciled = false
		newIngress.Eventf(api.EventTypeWarning, "ERROR", msg)
		newIngress.logger.Errorf(msg)
		return newIngress
	}

	// If annotation set is nil, its because it was cached as an invalid set before. Stop processing
	// and return nil.
	if newIngress.annotations == nil {
		msg := fmt.Sprintf("Skipping processing due to a history of bad annotations")
		newIngress.Eventf(api.EventTypeWarning, "ERROR", msg)
		newIngress.logger.Debugf(msg)
		return newIngress
	}

	// Assemble the load balancer
	newIngress.loadBalancer, err = lb.NewDesiredLoadBalancer(&lb.NewDesiredLoadBalancerOptions{
		ALBNamePrefix:         o.ALBNamePrefix,
		Namespace:             o.Ingress.GetNamespace(),
		ExistingLoadBalancer:  newIngress.loadBalancer,
		IngressName:           o.Ingress.Name,
		IngressRules:          o.Ingress.Spec.Rules,
		Logger:                newIngress.logger,
		Annotations:           newIngress.annotations,
		IngressAnnotations:    &o.Ingress.Annotations,
		CommonTags:            newIngress.Tags(o.ClusterName),
		GetServiceNodePort:    o.GetServiceNodePort,
		GetServiceAnnotations: o.GetServiceAnnotations,
		TargetsFunc:           o.TargetsFunc,
		AnnotationFactory:     o.AnnotationFactory,
		Resources:             o.Resources,
	})

	if err != nil {
		msg := fmt.Sprintf("Error instantiating load balancer: %s", err.Error())
		newIngress.Eventf(api.EventTypeWarning, "ERROR", msg)
		newIngress.logger.Errorf(msg)
		newIngress.reconciled = false
		return newIngress
	}

	newIngress.valid = true
	return newIngress
}

func tagsFromIngress(r util.ELBv2Tags) (string, string, error) {
	v, ok := r.Get("kubernetes.io/ingress-name")
	if ok {
		p := strings.Split(v, "/")
		if len(p) < 2 {
			return "", "", fmt.Errorf("kubernetes.io/ingress-name tag is invalid")
		}
		return p[0], p[1], nil
	}

	// Support legacy tags
	ingressName, ok := r.Get("IngressName")
	if !ok {
		return "", "", fmt.Errorf("IngressName tag is missing")
	}

	namespace, ok := r.Get("Namespace")
	if !ok {
		return "", "", fmt.Errorf("Namespace tag is missing")
	}
	return namespace, ingressName, nil
}

type NewALBIngressFromAWSLoadBalancerOptions struct {
	LoadBalancer  *elbv2.LoadBalancer
	ALBNamePrefix string
	Recorder      record.EventRecorder
	ResourceTags  *albrgt.Resources
	TargetGroups  map[string][]*elbv2.TargetGroup
}

// NewALBIngressFromAWSLoadBalancer builds ALBIngress's based off of an elbv2.LoadBalancer
func NewALBIngressFromAWSLoadBalancer(o *NewALBIngressFromAWSLoadBalancerOptions) (*ALBIngress, error) {
	namespace, ingressName, err := tagsFromIngress(o.ResourceTags.LoadBalancers[*o.LoadBalancer.LoadBalancerArn])
	if err != nil {
		return nil, fmt.Errorf("The LoadBalancer %s does not have the proper tags, can't import: %s", *o.LoadBalancer.LoadBalancerName, err.Error())
	}

	// Assemble ingress
	ingress := NewALBIngress(&NewALBIngressOptions{
		Namespace:     namespace,
		Name:          ingressName,
		ALBNamePrefix: o.ALBNamePrefix,
		Recorder:      o.Recorder,
		Reconciled:    true,
	})

	// Assemble load balancer
	ingress.loadBalancer, err = lb.NewCurrentLoadBalancer(&lb.NewCurrentLoadBalancerOptions{
		LoadBalancer:  o.LoadBalancer,
		ResourceTags:  o.ResourceTags,
		TargetGroups:  o.TargetGroups,
		ALBNamePrefix: o.ALBNamePrefix,
		Logger:        ingress.logger,
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
	// If the ingress resource is invalid, don't attempt reconcile
	if !a.valid {
		return
	}

	errors := a.loadBalancer.Reconcile(
		&lb.ReconcileOptions{
			Eventf: rOpts.Eventf,
		})
	if len(errors) > 0 {
		// marks reconciled state as false so UpdateIngressStatus won't operate
		a.reconciled = false
		a.logger.Errorf("Failed to reconcile state on this ingress")
		for _, err := range errors {
			a.logger.Errorf(" - %s", err.Error())
		}
	}
	// marks reconciled state as true so that UpdateIngressStatus will operate
	a.reconciled = true
}

// Namespace returns the namespace of the ingress
func (a *ALBIngress) Namespace() string {
	return a.namespace
}

// StripDesiredState strips all desired objects from an ALBIngress
func (a *ALBIngress) stripDesiredState() {
	if a.loadBalancer != nil {
		a.loadBalancer.StripDesiredState()
	}
}

// Tags returns an elbv2.Tag slice of standard tags for the ingress AWS resources
func (a *ALBIngress) Tags(clusterName string) []*elbv2.Tag {
	tags := a.annotations.Tags

	// https://github.com/kubernetes/kubernetes/blob/master/pkg/cloudprovider/providers/aws/tags.go
	tags = append(tags, &elbv2.Tag{
		Key:   aws.String("kubernetes.io/cluster/" + clusterName),
		Value: aws.String("owned"),
	})

	tags = append(tags, &elbv2.Tag{
		Key:   aws.String("Namespace"),
		Value: aws.String(a.namespace),
	})

	tags = append(tags, &elbv2.Tag{
		Key:   aws.String("IngressName"),
		Value: aws.String(a.ingressName),
	})

	return tags
}

// id returns an ingress id based off of a namespace and name
func GenerateID(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

func (a *ALBIngress) GetLoadBalancer() *lb.LoadBalancer {
	return a.loadBalancer
}
