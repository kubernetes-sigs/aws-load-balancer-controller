package controller

import (
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
)

// albIngress contains all information needed to assemble an ALB
type albIngress struct {
	id            *string
	namespace     *string
	ingressName   *string
	clusterName   *string
	lock          sync.Mutex
	nodes         AwsStringSlice
	annotations   *annotationsT
	LoadBalancers []*LoadBalancer
}

type albIngressesT []*albIngress

// Builds albIngress's based off of an Ingress object
// https://godoc.org/k8s.io/kubernetes/pkg/apis/extensions#Ingress
func newAlbIngressesFromIngress(ingress *extensions.Ingress, ac *ALBController) []*albIngress {

	var albIngresses []*albIngress

	annotations, err := ac.parseAnnotations(ingress.Annotations)
	if err != nil {
		glog.Errorf("Error parsing annotations %v: %v", ingress.Annotations, err)
		return nil
	}

	vpcID, err := ec2svc.getVPCID(annotations.subnets)
	if err != nil {
		glog.Errorf("Error fetching VPC for subnets %v: %v", awsutil.Prettify(annotations.subnets), err)
		return nil
	}

	a := &albIngress{
		id:          aws.String(fmt.Sprintf("%s-%s", ingress.GetNamespace(), ingress.Name)),
		namespace:   aws.String(ingress.GetNamespace()),
		clusterName: ac.clusterName,
		ingressName: &ingress.Name,
		annotations: annotations,
		nodes:       GetNodes(ac),
	}

	prevIngress := &albIngress{LoadBalancers: []*LoadBalancer{}}
	if i := ac.lastAlbIngresses.find(a); i >= 0 {
		prevIngress = ac.lastAlbIngresses[i]
	}

	for _, rule := range ingress.Spec.Rules {
		prevLoadBalancer := &LoadBalancer{TargetGroups: TargetGroups{}, Listeners: Listeners{}}

		lb := &LoadBalancer{
			id:        LoadBalancerID(*ac.clusterName, ingress.GetNamespace(), ingress.Name, rule.Host),
			namespace: aws.String(ingress.GetNamespace()),
			hostname:  &rule.Host,
			vpcID:     vpcID,
			// loadbalancer
		}

		for _, loadBalancer := range prevIngress.LoadBalancers {
			if *loadBalancer.id == *lb.id {
				prevLoadBalancer = loadBalancer
				lb.LoadBalancer = prevLoadBalancer.LoadBalancer
				break
			}
		}

		// make targetgroups around namespace, ingressname, and port
		// make listeners for path/port
		for _, path := range rule.HTTP.Paths {
			var port *int64
			serviceKey := fmt.Sprintf("%s/%s", *a.namespace, path.Backend.ServiceName)

			item, exists, _ := ac.storeLister.Service.Indexer.GetByKey(serviceKey)
			if !exists {
				glog.Errorf("%s: Unable to find the %v service", a.Name(), serviceKey)
				continue
			}

			// operator only works on NodePort ingresses
			if item.(*api.Service).Spec.Type != api.ServiceTypeNodePort {
				glog.Infof("%s: %v service is not of type NodePort", a.Name(), serviceKey)
				continue
			}

			// find target port
			for _, p := range item.(*api.Service).Spec.Ports {
				if p.Port == path.Backend.ServicePort.IntVal {
					port = aws.Int64(int64(p.NodePort))
				}
			}

			if port == nil {
				glog.Errorf("%s: Unable to find a port defined in the %v service", a.Name(), serviceKey)
				continue
			}

			// not even sure if its possible to specific non HTTP backends rn
			targetGroup := NewTargetGroup(a.clusterName, aws.String("HTTP"), lb.id, port)
			targetGroup.DesiredTargets = a.nodes

			if i := prevLoadBalancer.TargetGroups.find(targetGroup); i >= 0 {
				targetGroup.CurrentTargetGroup = prevLoadBalancer.TargetGroups[i].CurrentTargetGroup
				targetGroup.CurrentTargets = prevLoadBalancer.TargetGroups[i].CurrentTargets
			}

			lb.TargetGroups = append(lb.TargetGroups, targetGroup)

			// TODO: Revisit, this will never modify a listener
			listener := &Listener{DesiredListener: NewListener(a.annotations)}
			for _, previousListener := range prevLoadBalancer.Listeners {
				if previousListener.Equals(listener.DesiredListener) {
					listener.CurrentListener = previousListener.CurrentListener
					listener.DesiredListener = previousListener.DesiredListener
				}
			}
			lb.Listeners = append(lb.Listeners, listener)

			// TODO: Revisit, this will never modify a rule
			r := &Rule{DesiredRule: NewRule(targetGroup.CurrentTargetGroup.TargetGroupArn, aws.String(path.Path))}
			for _, previousRule := range prevLoadBalancer.Rules {
				if previousRule.Equals(r.DesiredRule) {
					r.CurrentRule = previousRule.CurrentRule
					r.DesiredRule = previousRule.CurrentRule
				}
			}
			lb.Rules = append(lb.Rules, r)
		}

		for _, tg := range prevLoadBalancer.TargetGroups {
			if lb.TargetGroups.find(tg) < 0 {
				tg.DesiredTargetGroup = nil
				lb.TargetGroups = append(lb.TargetGroups, tg)
			}
		}

		// Find any listeners that are no longer defined and set them for deletion
		for _, l := range prevLoadBalancer.Listeners {
			if lb.Listeners.find(l) < 0 {
				l.DesiredListener = nil
				lb.Listeners = append(lb.Listeners, l)
			}
		}

		// Find any rules that are no longer defined and set them for deletion
		for _, r := range prevLoadBalancer.Rules {
			if lb.Rules.find(r) < 0 {
				r.DesiredRule = nil
				lb.Rules = append(lb.Rules, r)
			}
		}

		a.LoadBalancers = append(a.LoadBalancers, lb)
	}

	albIngresses = append(albIngresses, a)

	return albIngresses
}

// assembleIngresses builds a list of existing ingresses from resources in AWS
func assembleIngresses(ac *ALBController) albIngressesT {

	var albIngresses albIngressesT
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
		resourceRecordSets, err := route53svc.describeResourceRecordSets(zone.Id, &hostname)
		if err != nil {
			glog.Errorf("Failed to find %s in AWS Route53", hostname)
		}

		rs := &ResourceRecordSet{
			name:              &hostname,
			zoneid:            zone.Id,
			ResourceRecordSet: resourceRecordSets,
		}

		lb := &LoadBalancer{
			id:           loadBalancer.LoadBalancerName,
			namespace:    aws.String(namespace),
			hostname:     aws.String(hostname),
			vpcID:        loadBalancer.VpcId,
			LoadBalancer: loadBalancer,
			recordSet:    rs,
			Tags:         tags,
		}

		targetGroups, err := elbv2svc.describeTargetGroups(loadBalancer.LoadBalancerArn)
		if err != nil {
			glog.Fatal(err)
		}

		for _, targetGroup := range targetGroups {
			tg := &TargetGroup{
				id:                 targetGroup.TargetGroupName,
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

			lb.Listeners = append(lb.Listeners, &Listener{
				CurrentListener: listener,
				DesiredListener: listener,
			})

			for _, rule := range rules {
				lb.Rules = append(lb.Rules, &Rule{
					CurrentRule: rule,
					DesiredRule: rule,
				})
			}
		}

		a := &albIngress{
			id:            aws.String(fmt.Sprintf("%s-%s", namespace, ingressName)),
			namespace:     aws.String(namespace),
			ingressName:   aws.String(ingressName),
			clusterName:   ac.clusterName,
			LoadBalancers: []*LoadBalancer{lb},
			// annotations   *annotationsT
		}

		if i := albIngresses.find(a); i >= 0 {
			a = albIngresses[i]
			a.LoadBalancers = append(a.LoadBalancers, lb)
		} else {
			albIngresses = append(albIngresses, a)
		}
	}

	glog.Infof("Assembled %d ingresses from existing AWS resources", len(albIngresses))
	return albIngresses
}

func (a *albIngress) createOrModify() {
	a.lock.Lock()
	defer a.lock.Unlock()
	for _, lb := range a.LoadBalancers {
		if lb.LoadBalancer != nil {
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
func (a *albIngress) create(lb *LoadBalancer) error {
	glog.Infof("%s: Creating new load balancer %s", a.Name(), *lb.id)
	if err := lb.create(a); err != nil { // this will set lb.LoadBalancer
		return err
	}

	if err := lb.recordSet.create(a, lb); err != nil {
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

	glog.Infof("%s: LoadBalancer %s created", a.Name(), *lb.hostname)

	return nil
}

// Handles the changes to an ingress
func (a *albIngress) modify(lb *LoadBalancer) error {
	if err := lb.modify(a); err != nil {
		return err
	}

	if err := lb.recordSet.modify(lb, route53.RRTypeA, "UPSERT"); err != nil {
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
func (a *albIngress) delete() error {
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

		if err := lb.recordSet.delete(a, lb); err != nil {
			return err
		}

		if err := lb.delete(a); err != nil {
			glog.Infof("%s: Unable to delete load balancer %s: %s",
				a.Name(),
				*lb.LoadBalancer.LoadBalancerArn,
				err)
		}
	}

	glog.Infof("%s: Ingress has been deleted", a.Name())
	return nil
}

func (a *albIngress) Name() string {
	return fmt.Sprintf("%s/%s", *a.namespace, *a.ingressName)
}

func (a albIngressesT) find(b *albIngress) int {
	for p, v := range a {
		if *v.id == *b.id {
			return p
		}
	}
	return -1
}

// useful for generating a starting point for tags
func (a *albIngress) Tags() []*elbv2.Tag {
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
