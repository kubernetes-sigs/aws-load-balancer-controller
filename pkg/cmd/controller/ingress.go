package controller

import (
	"fmt"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"
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
	annotations   *annotationsT
	LoadBalancers LoadBalancers
}

type ALBIngressesT []*ALBIngress

func NewALBIngress(namespace, name, clustername string) *ALBIngress {
	return &ALBIngress{
		id:          aws.String(fmt.Sprintf("%s-%s", namespace, name)),
		namespace:   aws.String(namespace),
		clusterName: aws.String(clustername),
		ingressName: aws.String(name),
	}
}

// Builds ALBIngress's based off of an Ingress object
// https://godoc.org/k8s.io/kubernetes/pkg/apis/extensions#Ingress. Creates a new ingress object,
// and looks up to see if a previous ingress object with the same id is known to the ALBController.
func newALBIngressesFromIngress(ingress *extensions.Ingress, ac *ALBController) []*ALBIngress {
	var ALBIngresses []*ALBIngress
	var err error

	// Create newIngress ALBIngress object holding the resource details and some cluster information.
	newIngress := NewALBIngress(ingress.GetNamespace(), ingress.Name, *ac.clusterName)

	// Find the previous version of this ingress (if it existed) and copy its Current state
	if i := ac.lastALBIngresses.find(newIngress); i >= 0 {
		newIngress.CopyState(ac.lastALBIngresses[i])
	}

	// Load up the ingress with our current annotations
	newIngress.annotations, err = ac.parseAnnotations(ingress.Annotations)
	if err != nil {
		glog.Errorf("%s: Error parsing annotations %v: %v", newIngress.Name(), err, awsutil.Prettify(ingress.Annotations))
		return nil
	}

	// If annotation set is nil, its because it was cached as an invalid set before
	if newIngress.annotations == nil {
		glog.Infof("%s-%s: Skipping processing due to a history of bad annotations", ingress.GetNamespace(), ingress.Name)
		return nil
	}

	// Create a new LoadBalancer instance for every item in ingress.Spec.Rules. This means that for
	// each host specified (1 per ingress.Spec.Rule) a new load balancer is expected.
	for _, rule := range ingress.Spec.Rules {
		// Start with a new load balancer
		lb := NewLoadBalancer(*ac.clusterName, ingress.GetNamespace(), ingress.Name, rule.Host, newIngress.annotations, newIngress.Tags())

		// If this rule is for a previously defined loadbalancer, pull it out so we can work on it
		if i := newIngress.LoadBalancers.find(lb); i >= 0 {
			lb = newIngress.LoadBalancers[i]
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

			// Start with a new target group
			targetGroup := NewTargetGroup(newIngress.annotations, newIngress.Tags(), newIngress.clusterName, lb.id, port)
			// If this rule/path matches an existing target group, pull it out so we can work on it
			if i := lb.TargetGroups.find(targetGroup); i >= 0 {
				targetGroup = lb.TargetGroups[i]
			}

			targetGroup.DesiredTargets = GetNodes(ac)

			// if i := prevLoadBalancer.TargetGroups.find(targetGroup); i >= 0 {
			// 	targetGroup.CurrentTargetGroup = prevLoadBalancer.TargetGroups[i].CurrentTargetGroup
			// 	targetGroup.CurrentTargets = prevLoadBalancer.TargetGroups[i].CurrentTargets
			// 	targetGroup.CurrentTags = prevLoadBalancer.TargetGroups[i].CurrentTags
			// }

			lb.TargetGroups = append(lb.TargetGroups, targetGroup)

			// 		listener := NewListener(newIngress.annotations)
			// 		// We can't used find here because it checks CurrentListener only
			// 		for _, previousListener := range prevLoadBalancer.Listeners {
			// 			if previousListener.Equals(listener.DesiredListener) {
			// 				// listener.Rules = previousListener.Rules
			// 				listener.CurrentListener = previousListener.CurrentListener
			// 				listener.DesiredListener = previousListener.DesiredListener
			// 			}
			// 		}

			// 		r := NewRule(aws.String(path.Path))
			// 		// for _, previousRule := range listener.Rules {
			// 		// 	if previousRule.Equals(r.DesiredRule) {
			// 		// 		r.CurrentRule = previousRule.CurrentRule
			// 		// 		r.DesiredRule = previousRule.CurrentRule
			// 		// 	}
			// 		// }

			// 		// if listener.Rules.find(r) < 0 {
			// 		listener.Rules = append(listener.Rules, r)
			// 		// }

			// 		if i := lb.Listeners.find(listener); i < 0 {
			// 			lb.Listeners = append(lb.Listeners, listener)
			// 			// fmt.Println("appending")
			// 			// } else {
			// 			// 	fmt.Println("existing")
			// 			// 	fmt.Println(awsutil.Prettify(lb.Listeners[i]))
			// 			// 	fmt.Println("new")
			// 			// 	fmt.Println(awsutil.Prettify(listener))
			// 		}

			// 		// Create a new route53.ResourceRecordSet based on lb. This value becomes the new desired
			// 		// state for the ResourceRecordSet.
			// 		lb.ResourceRecordSet, err = NewResourceRecordSet(lb.hostname)
			// 		if err != nil {
			// 			continue
			// 		}

			// 		// Set the new load balancer's current ResourceRecordSet state to the
			// 		// CurrentResourceRecordSet stored in the previous load balancer. Set the new load balancer's
			// 		// desired ResourceRecordSet state to the ResourceRecordSet generate above.
			// 		if prevLoadBalancer.ResourceRecordSet != nil {
			// 			lb.ResourceRecordSet.CurrentResourceRecordSet =
			// 				prevLoadBalancer.ResourceRecordSet.CurrentResourceRecordSet
			// 		}

			// 	}

			// Find any TargetGroups that are no longer defined and set them for deletion.
			// for _, tg := range prevLoadBalancer.TargetGroups {
			// 	if lb.TargetGroups.find(tg) < 0 {
			// 		tg.DesiredTargetGroup = nil
			// 		lb.TargetGroups = append(lb.TargetGroups, tg)
			// 	}
			// }

			// 	// Find any Listeners that are no longer defined and set them for deletion
			// 	// Also handles erasing the rules
			// 	for _, l := range prevLoadBalancer.Listeners {
			// 		if lb.Listeners.find(l) < 0 {
			// 			l.DesiredListener = nil
			// 			for _, r := range l.Rules {
			// 				r.DesiredRule = nil
			// 			}
			// 			lb.Listeners = append(lb.Listeners, l)
			// 		}
			// 	}

			// 	// Find any Rules that are no longer defined and set them for deletion
			// 	// for _, l := range lb.Listeners {
			// 	// 	if i := lb.Listeners.find(l); i >= 0 {
			// 	// 		fmt.Println("found one")
			// 	// 		fmt.Println(awsutil.Prettify(l))
			// 	// 		fmt.Println(awsutil.Prettify(lb.Listeners[i]))
			// 	// 		// l.DesiredListener = nil
			// 	// 		// for _, r := range l.Rules {
			// 	// 		// 	r.DesiredRule = nil
			// 	// 		// }
			// 	// 		// lb.Listeners = append(lb.Listeners, l)
			// 	// 	}
			// 	// }

			// 	// for _, r := range prevLoadBalancer.Rules {
			// 	// 	if lb.Rules.find(r) < 0 {
			// 	// 		r.DesiredRule = nil
			// 	// 		lb.Rules = append(lb.Rules, r)
			// 	// 	}
		}

		// 	fmt.Printf(awsutil.Prettify(lb.Listeners))
		// 	os.Exit(1)

		newIngress.LoadBalancers = append(newIngress.LoadBalancers, lb)
	}

	fmt.Println(awsutil.Prettify(newIngress))
	os.Exit(1)
	ALBIngresses = append(ALBIngresses, newIngress)

	return ALBIngresses
}

// assembleIngresses builds a list of existing ingresses from resources in AWS
func assembleIngresses(ac *ALBController) ALBIngressesT {

	var ALBIngresses ALBIngressesT
	glog.Info("Build up list of existing ingresses")

	loadBalancers, err := elbv2svc.describeLoadBalancers(ac.clusterName)
	if err != nil {
		glog.Fatal(err)
	}

	for _, loadBalancer := range loadBalancers {

		glog.Infof("Fetching tags for %s", *loadBalancer.LoadBalancerArn)
		tags, err := elbv2svc.describeTags(loadBalancer.LoadBalancerArn)
		if err != nil {
			glog.Fatal(err)
		}

		ingressName, ok := tags.Get("IngressName")
		if !ok {
			glog.Infof("The LoadBalancer %s does not have an IngressName tag, can't import", *loadBalancer.LoadBalancerName)
			continue
		}

		namespace, ok := tags.Get("Namespace")
		if !ok {
			glog.Infof("The LoadBalancer %s does not have an Namespace tag, can't import", *loadBalancer.LoadBalancerName)
			continue
		}

		hostname, ok := tags.Get("Hostname")
		if !ok {
			glog.Infof("The LoadBalancer %s does not have a Hostname tag, can't import", *loadBalancer.LoadBalancerName)
			continue
		}

		zone, err := route53svc.getZoneID(&hostname)
		if err != nil {
			glog.Infof("Failed to resolve %s zoneID. Returned error %s", hostname, err.Error())
			continue
		}

		glog.Infof("Fetching resource recordset for %s/%s %s", namespace, ingressName, hostname)
		resourceRecordSet, err := route53svc.describeResourceRecordSets(zone.Id, &hostname)
		if err != nil {
			glog.Errorf("Failed to find %s in AWS Route53", hostname)
		}

		rs := &ResourceRecordSet{
			ZoneId: zone.Id,
			CurrentResourceRecordSet: resourceRecordSet,
			DesiredResourceRecordSet: resourceRecordSet,
		}

		lb := &LoadBalancer{
			id:                  loadBalancer.LoadBalancerName,
			hostname:            aws.String(hostname),
			CurrentLoadBalancer: loadBalancer,
			DesiredLoadBalancer: loadBalancer,
			ResourceRecordSet:   rs,
			Tags:                tags,
		}

		targetGroups, err := elbv2svc.describeTargetGroups(loadBalancer.LoadBalancerArn)
		if err != nil {
			glog.Fatal(err)
		}

		for _, targetGroup := range targetGroups {
			tags, err := elbv2svc.describeTags(targetGroup.TargetGroupArn)
			if err != nil {
				glog.Fatal(err)
			}

			tg := &TargetGroup{
				id:                 targetGroup.TargetGroupName,
				CurrentTags:        tags,
				DesiredTags:        tags,
				CurrentTargetGroup: targetGroup,
				DesiredTargetGroup: targetGroup,
			}
			glog.Infof("Fetching Targets for Target Group %s", *targetGroup.TargetGroupArn)

			targets, err := elbv2svc.describeTargetGroupTargets(targetGroup.TargetGroupArn)
			if err != nil {
				glog.Fatal(err)
			}
			tg.CurrentTargets = targets
			tg.DesiredTargets = targets
			lb.TargetGroups = append(lb.TargetGroups, tg)
		}

		listeners, err := elbv2svc.describeListeners(loadBalancer.LoadBalancerArn)
		if err != nil {
			glog.Fatal(err)
		}

		for _, listener := range listeners {
			glog.Infof("Fetching Rules for Listener %s", *listener.ListenerArn)
			rules, err := elbv2svc.describeRules(listener.ListenerArn)
			if err != nil {
				glog.Fatal(err)
			}

			l := &Listener{
				CurrentListener: listener,
				DesiredListener: listener,
			}

			for _, rule := range rules {
				l.Rules = append(l.Rules, &Rule{
					CurrentRule: rule,
					DesiredRule: rule,
				})
			}

			lb.Listeners = append(lb.Listeners, l)
		}

		a := &ALBIngress{
			id:            aws.String(fmt.Sprintf("%s-%s", namespace, ingressName)),
			namespace:     aws.String(namespace),
			ingressName:   aws.String(ingressName),
			clusterName:   ac.clusterName,
			LoadBalancers: []*LoadBalancer{lb},
			// annotations   *annotationsT
		}

		if i := ALBIngresses.find(a); i >= 0 {
			a = ALBIngresses[i]
			a.LoadBalancers = append(a.LoadBalancers, lb)
		} else {
			ALBIngresses = append(ALBIngresses, a)
		}
	}

	glog.Infof("Assembled %d ingresses from existing AWS resources", len(ALBIngresses))
	return ALBIngresses
}

func (a *ALBIngress) createOrModify() {
	a.lock.Lock()
	defer a.lock.Unlock()
	for _, lb := range a.LoadBalancers {
		if lb.CurrentLoadBalancer != nil {
			err := a.modify(lb)
			if err != nil {
				glog.Errorf("%s: Error modifying ingress load balancer %s: %s", a.Name(), *lb.id, err)
			}
		} else {
			err := a.create(lb)
			if err != nil {
				glog.Errorf("%s: Error creating ingress load balancer %s: %s", a.Name(), *lb.id, err)
			}
		}
	}
}

// Starts the process of creating a new ALB. If successful, this will create a TargetGroup (TG), register targets in
// the TG, create a ALB, and create a Listener that maps the ALB to the TG in AWS.
func (a *ALBIngress) create(lb *LoadBalancer) error {
	glog.Infof("%s: Creating new load balancer %s", a.Name(), *lb.id)
	if err := lb.create(a); err != nil { // this will set lb.LoadBalancer
		return err
	}

	lb.ResourceRecordSet.PopulateFromLoadBalancer(lb.CurrentLoadBalancer)

	if err := lb.ResourceRecordSet.create(a, lb); err != nil {
		return err
	}

	for _, targetGroup := range lb.TargetGroups {
		if err := targetGroup.create(a, lb); err != nil {
			return err
		}

		for _, listener := range lb.Listeners {
			if err := listener.create(a, lb, targetGroup); err != nil {
				return err
			}
		}
	}

	glog.Infof("%s: LoadBalancer %s created", a.Name(), *lb.ResourceRecordSet.CurrentResourceRecordSet.Name)

	return nil
}

// Handles the changes to an ingress
func (a *ALBIngress) modify(lb *LoadBalancer) error {
	if err := lb.modify(a); err != nil {
		return err
	}

	lb.ResourceRecordSet.PopulateFromLoadBalancer(lb.CurrentLoadBalancer)

	if err := lb.ResourceRecordSet.modify(lb, route53.RRTypeA, "UPSERT"); err != nil {
		return err
	}

	if err := lb.TargetGroups.modify(a, lb); err != nil {
		return err
	}

	if err := lb.Listeners.modify(a, lb); err != nil {
		return err
	}

	// TODO: check rules

	return nil
}

// Deletes an ingress
func (a *ALBIngress) delete() error {
	glog.Infof("%s: Deleting ingress", a.Name())
	a.lock.Lock()
	defer a.lock.Unlock()

	for _, lb := range a.LoadBalancers {
		if err := lb.Listeners.delete(a); err != nil {
			glog.Info(err)
		}

		if err := lb.TargetGroups.delete(a); err != nil {
			glog.Info(err)
		}

		if err := lb.ResourceRecordSet.delete(a, lb); err != nil {
			return err
		}

		if err := lb.delete(a); err != nil {
			glog.Infof("%s: Unable to delete load balancer %s: %s",
				a.Name(),
				*lb.CurrentLoadBalancer.LoadBalancerArn,
				err)
		}
	}

	glog.Infof("%s: Ingress has been deleted", a.Name())
	return nil
}

func (a *ALBIngress) Name() string {
	return fmt.Sprintf("%s/%s", *a.namespace, *a.ingressName)
}

// CopyState copies the entire state in `b` to `a` and strips out all
// Desired values
func (a *ALBIngress) CopyState(b *ALBIngress) {
	*a = *b
	for _, lb := range a.LoadBalancers {
		lb.DesiredLoadBalancer = nil
		lb.ResourceRecordSet.DesiredResourceRecordSet = nil
		for _, listener := range lb.Listeners {
			listener.DesiredListener = nil
			for _, rule := range listener.Rules {
				rule.DesiredRule = nil
			}
		}
		for _, targetgroup := range lb.TargetGroups {
			targetgroup.DesiredTags = nil
			targetgroup.DesiredTargetGroup = nil
			targetgroup.DesiredTargets = nil
		}
	}
}

// useful for generating a starting point for tags
func (a *ALBIngress) Tags() []*elbv2.Tag {
	tags := a.annotations.tags

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
