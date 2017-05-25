package controller

import (
	"fmt"
	"sort"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/awsutil"
	"github.com/coreos/alb-ingress-controller/controller/alb"
	"github.com/coreos/alb-ingress-controller/controller/config"
	"github.com/coreos/alb-ingress-controller/controller/util"
	"github.com/coreos/alb-ingress-controller/log"
	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/apis/extensions"
)

// ALBIngress contains all information above the cluster, ingress resource, and AWS resources
// needed to assemble an ALB, TargetGroup, Listener, Rules, and Route53 Resource Records.
type ALBIngress struct {
	id            *string
	namespace     *string
	ingressName   *string
	clusterName   *string
	lock          *sync.Mutex
	annotations   *config.Annotations
	LoadBalancers alb.LoadBalancers
	tainted       bool // represents that parsing or validation this ingress resource failed
}

// ALBIngressesT is a list of ALBIngress. It is held by the ALBController instance and evaluated
// against to determine what should be created, deleted, and modified.
type ALBIngressesT []*ALBIngress

// NewALBIngress returns a minimal ALBIngress instance with a generated name that allows for lookup
// when new ALBIngress objects are created to determine if an instance of that ALBIngress already
// exists.
func NewALBIngress(namespace, name, clustername string) *ALBIngress {
	return &ALBIngress{
		id:          aws.String(fmt.Sprintf("%s-%s", namespace, name)),
		namespace:   aws.String(namespace),
		clusterName: aws.String(clustername),
		ingressName: aws.String(name),
		lock:        new(sync.Mutex),
	}
}

// NewALBIngressFromIngress builds ALBIngress's based off of an Ingress object
// https://godoc.org/k8s.io/kubernetes/pkg/apis/extensions#Ingress. Creates a new ingress object,
// and looks up to see if a previous ingress object with the same id is known to the ALBController.
// If there is an issue and the ingress is invalid, nil is returned.
func NewALBIngressFromIngress(ingress *extensions.Ingress, ac *ALBController) (*ALBIngress, error) {
	var err error

	// Create newIngress ALBIngress object holding the resource details and some cluster information.
	newIngress := NewALBIngress(ingress.GetNamespace(), ingress.Name, *ac.clusterName)

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

	// Load up the ingress with our current annotations.
	newIngress.annotations, err = config.ParseAnnotations(ingress.Annotations)
	if err != nil {
		log.Errorf("Error parsing annotations for ingress %v. Error: %s", "controller", newIngress.Name(), err.Error())
		return newIngress, err
	}

	// If annotation set is nil, its because it was cached as an invalid set before. Stop processing
	// and return nil.
	if newIngress.annotations == nil {
		log.Debugf("%s-%s: Skipping processing due to a history of bad annotations", newIngress.Name(), ingress.GetNamespace(), ingress.Name)
		return newIngress, err
	}

	// Create a new LoadBalancer instance for every item in ingress.Spec.Rules. This means that for
	// each host specified (1 per ingress.Spec.Rule) a new load balancer is expected.
	for _, rule := range ingress.Spec.Rules {
		// Start with a new LoadBalancer with a new DesiredState.
		// TODO: RETURNING NIL SHOULD NOT BE AN OPTION HERE, otherwise memory access violations will
		// occur.
		lb := alb.NewLoadBalancer(*ac.clusterName, ingress.GetNamespace(), ingress.Name, rule.Host, newIngress.id, newIngress.annotations, newIngress.Tags())

		// If this rule is for a previously defined loadbalancer, pull it out so we can work on it
		if i := newIngress.LoadBalancers.Find(lb); i >= 0 {
			// Save the Desired state to our old Loadbalancer.
			newIngress.LoadBalancers[i].DesiredLoadBalancer = lb.DesiredLoadBalancer
			newIngress.LoadBalancers[i].DesiredTags = lb.DesiredTags
			newIngress.LoadBalancers[i].Hostname = lb.Hostname
			// Set lb to our old but updated LoadBalancer.
			lb = newIngress.LoadBalancers[i]
			// Remove the old LoadBalancer from the list.
			newIngress.LoadBalancers = append(newIngress.LoadBalancers[:i], newIngress.LoadBalancers[i+1:]...)
		}

		// Create a new TargetGroup and Listener, associated with a LoadBalancer for every item in
		// rule.HTTP.Paths. TargetGroups are constructed based on namespace, ingress name, and port.
		// Listeners are constructed based on path and port.
		for _, path := range rule.HTTP.Paths {
			serviceKey := fmt.Sprintf("%s/%s", *newIngress.namespace, path.Backend.ServiceName)
			port, err := ac.GetServiceNodePort(serviceKey, path.Backend.ServicePort.IntVal)
			if err != nil {
				glog.Infof("%s: %s", newIngress.Name(), err)
				continue
			}

			// Start with a new target group with a new Desired state.
			targetGroup := alb.NewTargetGroup(newIngress.annotations, newIngress.Tags(), newIngress.clusterName, lb.ID, port, newIngress.id, path.Backend.ServiceName)
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
			listenerList := alb.NewListener(newIngress.annotations, newIngress.id)
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
				rule := alb.NewRule(path, newIngress.id)
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

			// Create a new ResourceRecordSet for the hostname.
			resourceRecordSet := alb.NewResourceRecordSet(lb.Hostname, lb.IngressID)
			// If the load balancer has a CurrentResourceRecordSet, set
			// this value inside our new resourceRecordSet.
			if lb.ResourceRecordSet != nil {
				resourceRecordSet.CurrentResourceRecordSet = lb.ResourceRecordSet.CurrentResourceRecordSet
			}

			// Assign the resourceRecordSet to the load balancer
			lb.ResourceRecordSet = resourceRecordSet

		}

		// Add the newly constructed LoadBalancer to the new ALBIngress's Loadbalancer list.
		newIngress.LoadBalancers = append(newIngress.LoadBalancers, lb)
	}

	return newIngress, nil
}

// assembleIngresses builds a list of existing ingresses from resources in AWS
func assembleIngresses(ac *ALBController) ALBIngressesT {

	var ALBIngresses ALBIngressesT
	log.Infof("Build up list of existing ingresses", "controller")

	loadBalancers, err := awsutil.ALBsvc.DescribeLoadBalancers(ac.clusterName)
	if err != nil {
		glog.Fatal(err)
	}

	for _, loadBalancer := range loadBalancers {

		log.Debugf("Fetching Tags for %s", "controller", *loadBalancer.LoadBalancerArn)
		tags, err := awsutil.ALBsvc.DescribeTags(loadBalancer.LoadBalancerArn)
		if err != nil {
			glog.Fatal(err)
		}

		ingressName, ok := tags.Get("IngressName")
		if !ok {
			log.Infof("The LoadBalancer %s does not have an IngressName tag, can't import", "controller", *loadBalancer.LoadBalancerName)
			continue
		}

		namespace, ok := tags.Get("Namespace")
		if !ok {
			log.Infof("The LoadBalancer %s does not have an Namespace tag, can't import", "controller", *loadBalancer.LoadBalancerName)
			continue
		}

		hostname, ok := tags.Get("Hostname")
		if !ok {
			log.Infof("The LoadBalancer %s does not have a Hostname tag, can't import", "controller", *loadBalancer.LoadBalancerName)
			continue
		}

		zone, err := awsutil.Route53svc.GetZoneID(&hostname)
		if err != nil {
			log.Infof("Failed to resolve %s zoneID. Returned error %s", "controller", hostname, err.Error())
			continue
		}

		log.Infof("Fetching resource recordset for %s/%s %s", "controller", namespace, ingressName, hostname)
		resourceRecordSet, err := awsutil.Route53svc.DescribeResourceRecordSets(zone.Id,
			&hostname)
		if err != nil {
			log.Errorf("Failed to find %s in AWS Route53", "controller", hostname)
		}

		ingressID := namespace + "-" + ingressName

		rs := &alb.ResourceRecordSet{
			IngressID: &ingressID,
			ZoneID:    zone.Id,
			CurrentResourceRecordSet: resourceRecordSet,
		}

		lb := &alb.LoadBalancer{
			ID:                  loadBalancer.LoadBalancerName,
			IngressID:           &ingressID,
			Hostname:            aws.String(hostname),
			CurrentLoadBalancer: loadBalancer,
			ResourceRecordSet:   rs,
			CurrentTags:         tags,
		}

		targetGroups, err := awsutil.ALBsvc.DescribeTargetGroups(loadBalancer.LoadBalancerArn)
		if err != nil {
			glog.Fatal(err)
		}

		for _, targetGroup := range targetGroups {
			tags, err := awsutil.ALBsvc.DescribeTags(targetGroup.TargetGroupArn)
			if err != nil {
				glog.Fatal(err)
			}

			svcName, ok := tags.Get("ServiceName")
			if !ok {
				log.Infof("The LoadBalancer %s does not have an Namespace tag, can't import", "controller", *loadBalancer.LoadBalancerName)
				continue
			}

			tg := &alb.TargetGroup{
				ID:                 targetGroup.TargetGroupName,
				IngressID:          &ingressID,
				SvcName:            svcName,
				CurrentTags:        tags,
				CurrentTargetGroup: targetGroup,
			}
			log.Infof("Fetching Targets for Target Group %s", "controller", *targetGroup.TargetGroupArn)

			targets, err := awsutil.ALBsvc.DescribeTargetGroupTargets(targetGroup.TargetGroupArn)
			if err != nil {
				glog.Fatal(err)
			}
			tg.CurrentTargets = targets
			lb.TargetGroups = append(lb.TargetGroups, tg)
		}

		listeners, err := awsutil.ALBsvc.DescribeListeners(loadBalancer.LoadBalancerArn)
		if err != nil {
			glog.Fatal(err)
		}

		for _, listener := range listeners {
			log.Infof("Fetching Rules for Listener %s", "controller", *listener.ListenerArn)
			rules, err := awsutil.ALBsvc.DescribeRules(listener.ListenerArn)
			if err != nil {
				glog.Fatal(err)
			}

			l := &alb.Listener{
				CurrentListener: listener,
				IngressID:       &ingressID,
			}

			for _, rule := range rules {
				var svcName string
				for _, tg := range lb.TargetGroups {
					if *rule.Actions[0].TargetGroupArn == *tg.CurrentTargetGroup.TargetGroupArn {
						svcName = tg.SvcName
					}
				}

				log.Debugf("Assembling rule with svc name: %s", "controller", svcName)
				l.Rules = append(l.Rules, &alb.Rule{
					IngressID:   &ingressID,
					SvcName:     svcName,
					CurrentRule: rule,
				})
			}

			// Set the highest known priority to the amount of current rules plus 1
			lb.LastRulePriority = int64(len(l.Rules)) + 1

			lb.Listeners = append(lb.Listeners, l)
		}

		a := NewALBIngress(namespace, ingressName, *ac.clusterName)
		a.LoadBalancers = []*alb.LoadBalancer{lb}

		if i := ALBIngresses.find(a); i >= 0 {
			a = ALBIngresses[i]
			a.LoadBalancers = append(a.LoadBalancers, lb)
		} else {
			ALBIngresses = append(ALBIngresses, a)
		}
	}

	log.Infof("Assembled %d ingresses from existing AWS resources", "controller", len(ALBIngresses))
	return ALBIngresses
}

// Reconcile begins the state sync for all AWS resource satisfying this ALBIngress instance.
func (a *ALBIngress) Reconcile() {
	a.lock.Lock()
	defer a.lock.Unlock()
	// If the ingress resource failed to assemble, don't attempt reconcile
	if a.tainted {
		return
	}
	var err error

	a.LoadBalancers, err = a.LoadBalancers.Reconcile()
	if err != nil {
		log.Errorf("Sync stopped due to error. Error: %s", *a.id, err.Error())
	}
}

// Name returns the name of the ingress
func (a *ALBIngress) Name() string {
	return fmt.Sprintf("%s-%s", *a.namespace, *a.ingressName)
}

// StripDesiredState strips all desired objects from an ALBIngress
func (a *ALBIngress) StripDesiredState() {
	a.LoadBalancers.StripDesiredState()
	for _, lb := range a.LoadBalancers {
		lb.Listeners.StripDesiredState()
		lb.TargetGroups.StripDesiredState()
		for _, listener := range lb.Listeners {
			listener.Rules.StripDesiredState()
		}
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
	nodes, _ := ac.storeLister.Node.List()
	for _, node := range nodes.Items {
		if label, ok := node.ObjectMeta.Labels["kubernetes.io/role"]; ok {
			if label == "master" {
				continue
			}
		}
		result = append(result, aws.String(node.Spec.ExternalID))
	}
	sort.Sort(result)
	return result
}
