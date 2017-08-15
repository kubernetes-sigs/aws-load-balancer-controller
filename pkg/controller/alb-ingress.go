package controller

import (
	"fmt"
	"sort"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/alb"
	"github.com/coreos/alb-ingress-controller/pkg/config"
	awsutil "github.com/coreos/alb-ingress-controller/pkg/util/aws"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
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
	LoadBalancer *alb.LoadBalancer
	tainted      bool // represents that parsing or validation this ingress resource failed
	logger       *log.Logger
}

// ALBIngressesT is a list of ALBIngress. It is held by the ALBController instance and evaluated
// against to determine what should be created, deleted, and modified.
type ALBIngressesT []*ALBIngress

// NewALBIngress returns a minimal ALBIngress instance with a generated name that allows for lookup
// when new ALBIngress objects are created to determine if an instance of that ALBIngress already
// exists.
func NewALBIngress(namespace, name, clustername string) *ALBIngress {
	ingressID := fmt.Sprintf("%s/%s", namespace, name)
	return &ALBIngress{
		id:          aws.String(ingressID),
		namespace:   aws.String(namespace),
		clusterName: aws.String(clustername),
		ingressName: aws.String(name),
		lock:        new(sync.Mutex),
		logger:      log.New(ingressID),
	}
}

// NewALBIngressFromIngress builds ALBIngress's based off of an Ingress object
// https://godoc.org/k8s.io/kubernetes/pkg/apis/extensions#Ingress. Creates a new ingress object,
// and looks up to see if a previous ingress object with the same id is known to the ALBController.
// If there is an issue and the ingress is invalid, nil is returned.
func NewALBIngressFromIngress(ingress *extensions.Ingress, ac *ALBController) (*ALBIngress, error) {
	var err error

	// Create newIngress ALBIngress object holding the resource details and some cluster information.
	newIngress := NewALBIngress(ingress.GetNamespace(), ingress.Name, ac.clusterName)
	newIngress.recorder = ac.recorder

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

	newIngress.LoadBalancer = alb.NewLoadBalancer(ac.clusterName, ingress.GetNamespace(), ingress.Name, newIngress.logger, newIngress.annotations, newIngress.Tags())
	lb := newIngress.LoadBalancer

	for _, rule := range ingress.Spec.Rules {
		// Create a new TargetGroup and Listener, associated with a LoadBalancer for every item in
		// rule.HTTP.Paths. TargetGroups are constructed based on namespace, ingress name, and port.
		// Listeners are constructed based on path and port.
		for _, path := range rule.HTTP.Paths {
			serviceKey := fmt.Sprintf("%s/%s", *newIngress.namespace, path.Backend.ServiceName)
			port, err := ac.GetServiceNodePort(serviceKey, path.Backend.ServicePort.IntVal)
			if err != nil {
				newIngress.logger.Errorf(err.Error())
				continue
			}

			// Start with a new target group with a new Desired state.
			targetGroup := alb.NewTargetGroup(newIngress.annotations, newIngress.Tags(), newIngress.clusterName, lb.ID, port, newIngress.logger, path.Backend.ServiceName)
			// If this rule/path matches an existing target group, pull it out so we can work on it.
			if i := lb.TargetGroups.Find(targetGroup); i >= 0 {
				// Save the Desired state to our old TargetGroup
				lb.TargetGroups[i].DesiredTags = targetGroup.DesiredTags
				lb.TargetGroups[i].DesiredTargetGroup = targetGroup.DesiredTargetGroup
				// Set targetGroup to our old but updated TargetGroup.
				targetGroup = lb.TargetGroups[i]
				// Remove the old TG from our list.
				lb.TargetGroups = append(lb.TargetGroups[:i], lb.TargetGroups[i+1:]...)
			}

			// Add desired targets set to the targetGroup.
			targetGroup.DesiredTargets = GetNodes(ac)
			lb.TargetGroups = append(lb.TargetGroups, targetGroup)

			// Start with a new listener
			listenerList := alb.NewListener(newIngress.annotations, newIngress.logger)
			for _, listener := range listenerList {
				// If this listener matches an existing listener, pull it out so we can work on it.
				// TODO: We should refine the lookup. Find is really not adequate as this could be a first
				// statrt where no Listeners have CurrentListeners attached. In other words, find should be
				// rewritten.
				if i := lb.Listeners.Find(listener.DesiredListener); i >= 0 {
					// Save the Desired state to our old Listener.
					lb.Listeners[i].DesiredListener = listener.DesiredListener
					// Set listener to our old but updated Listener.
					listener = lb.Listeners[i]
					// Remove the old Listener from our list.
					lb.Listeners = append(lb.Listeners[:i], lb.Listeners[i+1:]...)
				}
				lb.Listeners = append(lb.Listeners, listener)

				// Start with a new rule
				rule := alb.NewRule(path.Path, path.Backend.ServiceName, newIngress.logger)
				// If this rule matches an existing rule, pull it out so we can work on it
				if i := listener.Rules.Find(rule.DesiredRule); i >= 0 {
					// Save the Desired state to our old Rule
					listener.Rules[i].DesiredRule = rule.DesiredRule
					// Set rule to our old but updated Rule
					rule = listener.Rules[i]
					// Remove the old Rule from our list.
					listener.Rules = append(listener.Rules[:i], listener.Rules[i+1:]...)
				}
				listener.Rules = append(listener.Rules, rule)
			}
		}
	}

	return newIngress, nil
}

// NewALBIngressFromLoadBalancer builds ALBIngress's based off of an elbv2.LoadBalancer
func NewALBIngressFromLoadBalancer(loadBalancer *elbv2.LoadBalancer, clusterName string) (*ALBIngress, bool) {
	logger.Debugf("Fetching Tags for %s", *loadBalancer.LoadBalancerArn)
	tags, err := awsutil.ALBsvc.DescribeTagsForArn(loadBalancer.LoadBalancerArn)
	if err != nil {
		logger.Fatalf(err.Error())
	}

	ingressName, ok := tags.Get("IngressName")
	if !ok {
		logger.Infof("The LoadBalancer %s does not have an IngressName tag, can't import", *loadBalancer.LoadBalancerName)
		return nil, false
	}

	namespace, ok := tags.Get("Namespace")
	if !ok {
		logger.Infof("The LoadBalancer %s does not have an Namespace tag, can't import", *loadBalancer.LoadBalancerName)
		return nil, false
	}

	ingress := NewALBIngress(namespace, ingressName, clusterName)

	ingress.LoadBalancer = alb.NewLoadBalancer(clusterName, namespace, ingressName, ingress.logger, &config.Annotations{}, tags)
	lb := ingress.LoadBalancer
	lb.CurrentTags = tags
	lb.CurrentLoadBalancer = loadBalancer
	lb.DesiredTags = nil
	lb.DesiredLoadBalancer = nil

	targetGroups, err := awsutil.ALBsvc.DescribeTargetGroupsForLoadBalancer(loadBalancer.LoadBalancerArn)
	if err != nil {
		ingress.logger.Fatalf(err.Error())
	}

	for _, targetGroup := range targetGroups {
		tags, err := awsutil.ALBsvc.DescribeTagsForArn(targetGroup.TargetGroupArn)
		if err != nil {
			ingress.logger.Fatalf(err.Error())
		}

		svcName, ok := tags.Get("ServiceName")
		if !ok {
			ingress.logger.Infof("The LoadBalancer %s does not have an Namespace tag, can't import", *loadBalancer.LoadBalancerName)
			return nil, false
		}

		tg := alb.NewTargetGroup(&config.Annotations{BackendProtocol: targetGroup.Protocol}, tags, aws.String(clusterName), lb.ID, targetGroup.Port, ingress.logger, svcName)
		tg.CurrentTags = tags
		tg.CurrentTargetGroup = targetGroup
		tg.DesiredTags = nil
		tg.DesiredTargetGroup = nil

		ingress.logger.Infof("Fetching Targets for Target Group %s", *targetGroup.TargetGroupArn)

		targets, err := awsutil.ALBsvc.DescribeTargetGroupTargetsForArn(targetGroup.TargetGroupArn)
		if err != nil {
			ingress.logger.Fatalf(err.Error())
		}
		tg.CurrentTargets = targets
		lb.TargetGroups = append(lb.TargetGroups, tg)
	}

	listeners, err := awsutil.ALBsvc.DescribeListenersForLoadBalancer(loadBalancer.LoadBalancerArn)
	if err != nil {
		ingress.logger.Fatalf(err.Error())
	}

	for _, listener := range listeners {
		ingress.logger.Infof("Fetching Rules for Listener %s", *listener.ListenerArn)
		rules, err := awsutil.ALBsvc.DescribeRules(&elbv2.DescribeRulesInput{ListenerArn: listener.ListenerArn})
		if err != nil {
			ingress.logger.Fatalf(err.Error())
		}

		// this is super lame, need to rework annotations and parameters
		annotations := &config.Annotations{Ports: []config.ListenerPort{config.ListenerPort{Port: *listener.Port}}}
		listeners := alb.NewListener(annotations, ingress.logger)

		l := listeners[0]
		l.CurrentListener = listener
		l.DesiredListener = nil

		for _, rule := range rules.Rules {
			var svcName string
			for _, tg := range lb.TargetGroups {
				if *rule.Actions[0].TargetGroupArn == *tg.CurrentTargetGroup.TargetGroupArn {
					svcName = tg.SvcName
				}
			}

			ingress.logger.Debugf("Assembling rule with svc name: %s", svcName)
			r := alb.NewRule("", svcName, ingress.logger)
			r.CurrentRule = rule
			r.DesiredRule = nil

			l.Rules = append(l.Rules, r)
		}

		// Set the highest known priority to the amount of current rules plus 1
		lb.LastRulePriority = int64(len(l.Rules)) + 1

		ingress.logger.Infof("Ingress rebuilt from existing ALB in AWS")
		lb.Listeners = append(lb.Listeners, l)
	}

	return ingress, true
}

// Eventf writes an event to the ALBIngress's Kubernetes ingress resource
func (a *ALBIngress) Eventf(eventtype, reason, messageFmt string, args ...interface{}) {
	if a.ingress == nil || a.recorder == nil {
		return
	}
	a.recorder.Eventf(a.ingress, eventtype, reason, messageFmt, args...)
}

// Reconcile begins the state sync for all AWS resource satisfying this ALBIngress instance.
func (a *ALBIngress) Reconcile(rOpts *alb.ReconcileOptions) {
	a.lock.Lock()
	defer a.lock.Unlock()
	// If the ingress resource failed to assemble, don't attempt reconcile
	if a.tainted {
		return
	}

	rOpts.SetLoadBalancer(a.LoadBalancer)

	errors := a.LoadBalancer.Reconcile(rOpts)
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

// GetNodes returns a list of the cluster node external ids
func GetNodes(ac *ALBController) util.AWSStringSlice {
	var result util.AWSStringSlice
	nodes := ac.storeLister.Node.List()
	for _, node := range nodes {
		result = append(result, aws.String(node.(*api.Node).Spec.ExternalID))
	}
	sort.Sort(result)
	return result
}
