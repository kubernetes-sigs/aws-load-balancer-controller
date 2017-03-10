package controller

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
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
	annotations   *annotationsT
	LoadBalancers []*LoadBalancer
}

type albIngressesT []*albIngress

// Builds albIngress's based off of an Ingress object
// https://godoc.org/k8s.io/kubernetes/pkg/apis/extensions#Ingress
// NOTE: each rule is a different elbv2 load balancer
// NOTE: each path is a different elbv2 listener rule
func newAlbIngressesFromIngress(ingress *extensions.Ingress, ac *ALBController) []*albIngress {
	var albIngresses []*albIngress

	annotations, err := ac.parseAnnotations(ingress.Annotations)
	if err != nil {
		glog.Errorf("Error parsing annotations %v: %v", ingress.Annotations, err)
		return nil
	}

	vpcID, err := ec2svc.getVPCID(annotations.subnets)
	if err != nil {
		glog.Errorf("Error fetching VPC for subnets %v: %v", annotations.subnets, err)
		return nil
	}

	a := &albIngress{
		id:          aws.String(fmt.Sprintf("%s-%s", ingress.GetNamespace(), ingress.Name)),
		namespace:   aws.String(ingress.GetNamespace()),
		clusterName: ac.clusterName,
		ingressName: &ingress.Name,
		annotations: annotations,
	}

	nodeIds := GetNodes(ac)
	for _, rule := range ingress.Spec.Rules {
		lb := &LoadBalancer{
			id:        LoadBalancerID(*ac.clusterName, ingress.GetNamespace(), ingress.Name, rule.Host),
			namespace: aws.String(ingress.GetNamespace()),
			hostname:  &rule.Host,
			vpcID:     vpcID,
			// loadbalancer
		}

		// make targetgroups around namespace, ingressname, and port
		// make listeners for path/port
		for _, path := range rule.HTTP.Paths {
			var port *int64
			serviceName := path.Backend.ServiceName
			serviceKey := fmt.Sprintf("%s/%s", *a.namespace, serviceName)

			item, exists, _ := ac.storeLister.Service.Indexer.GetByKey(serviceKey)
			if !exists {
				glog.Errorf("%s: Unable to find the %v service", a.Name(), serviceKey)
				continue
			}

			service := item.(*api.Service)
			if service.Spec.Type != api.ServiceTypeNodePort {
				glog.Infof("%s: %v service is not of type NodePort", a.Name(), serviceKey)
				continue
			}

			for _, p := range service.Spec.Ports {
				if p.Port == path.Backend.ServicePort.IntVal {
					port = aws.Int64(int64(p.Port))
				}
			}

			if port == nil {
				glog.Errorf("%s: Unable to find a port defined in the %v service", a.Name(), serviceKey)
			}

			targetGroup := &TargetGroup{
				id:      TargetGroupID(*ac.clusterName),
				port:    port,
				targets: nodeIds,
			}

			listener := &Listener{
			// listeners
			// rules
			}

			lb.TargetGroups = append(lb.TargetGroups, targetGroup)
			lb.Listeners = append(lb.Listeners, listener)
		}
		a.LoadBalancers = append(a.LoadBalancers, lb)
	}

	albIngresses = append(albIngresses, a)

	return albIngresses
}

// assembleIngresses builds a list of existing ingresses from resources in AWS
func assembleIngresses(ac *ALBController) albIngressesT {
	var albIngresses albIngressesT
	ingresses := make(map[string][]*LoadBalancer)

	glog.Info("Build up list of existing ingresses")

	for _, loadBalancer := range elbv2svc.describeLoadBalancers(ac.clusterName) {
		tags, err := elbv2svc.describeTags(loadBalancer.LoadBalancerArn)
		if err != nil {
			glog.Fatal(err)
		}

		ingressName, ok := tags["IngressName"]
		if !ok {
			glog.Infof("The LoadBalancer %s does not have an IngressName tag, can't import", *loadBalancer.LoadBalancerName)
			continue
		}

		lb := &LoadBalancer{
			id:        loadBalancer.LoadBalancerName,
			arn:       loadBalancer.LoadBalancerArn,
			namespace: aws.String(tags["Namespace"]),
			// hostname     string // should this be a tag on the ALB?
			vpcID:        loadBalancer.VpcId,
			LoadBalancer: loadBalancer,
		}

		targetGroups := elbv2svc.describeTargetGroups(loadBalancer.LoadBalancerArn)
		for _, targetGroup := range targetGroups {
			tg := &TargetGroup{
				id:          targetGroup.TargetGroupName,
				arn:         targetGroup.TargetGroupArn,
				port:        targetGroup.Port,
				TargetGroup: targetGroup,
			}

			targets, err := elbv2svc.describeTargetGroupTargets(tg.TargetGroup.TargetGroupArn)
			if err != nil {
				glog.Fatal(err)
			}
			tg.targets = targets
			lb.TargetGroups = append(lb.TargetGroups, tg)
		}

		listeners, err := elbv2svc.describeListeners(loadBalancer.LoadBalancerArn)
		if err != nil {
			glog.Fatal(err)
		}

		for _, listener := range listeners {
			lb.Listeners = append(lb.Listeners, &Listener{
				Listener: listener,
				// serviceName:
				arn:   listener.ListenerArn,
				Rules: elbv2svc.describeRules(listener.ListenerArn),
			})
		}
		ingresses[ingressName] = append(ingresses[ingressName], lb)
	}

	for ingressName, loadBalancers := range ingresses {
		albIngresses = append(albIngresses,
			&albIngress{
				id:            aws.String(fmt.Sprintf("%s-%s", *loadBalancers[0].namespace, ingressName)),
				namespace:     loadBalancers[0].namespace,
				ingressName:   aws.String(ingressName),
				clusterName:   ac.clusterName,
				LoadBalancers: loadBalancers,
				// annotations   *annotationsT
			},
		)
	}

	return albIngresses
}

func (a *albIngress) createOrModify() error {
	for _, lb := range a.LoadBalancers {
		exists, loadBalancer, err := lb.loadBalancerExists(a)
		if err != nil {
			return err
		}

		if exists {
			lb.LoadBalancer = loadBalancer
			err := a.modify(lb)
			if err != nil {
				return err
			}
		} else {
			err := a.create(lb)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Starts the process of creating a new ALB. If successful, this will create a TargetGroup (TG), register targets in
// the TG, create a ALB, and create a Listener that maps the ALB to the TG in AWS.
func (a *albIngress) create(lb *LoadBalancer) error {
	glog.Infof("%s: Creating new load balancer %s", a.Name(), *lb.id)
	if err := lb.create(a); err != nil { // this will set lb.LoadBalancer
		return err
	}

	if err := route53svc.UpsertRecord(a, lb); err != nil {
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

// Modifies an ingress
func (a *albIngress) modify(lb *LoadBalancer) error {
	glog.Infof("%s: Modifying load balancer %s", a.Name(), *lb.id)
	if err := lb.modify(a); err != nil {
		return err
	}

	if err := route53svc.UpsertRecord(a, lb); err != nil {
		return err
	}

	for _, targetGroup := range lb.TargetGroups {
		if err := targetGroup.modify(a, lb); err != nil {
			return err
		}
		for _, listener := range lb.Listeners {
			if err := listener.modify(a, lb, targetGroup); err != nil {
				return err
			}
		}
	}

	return nil
}

// Deletes an ingress
func (a *albIngress) delete() error {
	glog.Infof("%s: Deleting ingress", a.Name())

	for _, lb := range a.LoadBalancers {
		for _, targetGroup := range lb.TargetGroups {
			if err := targetGroup.delete(a); err != nil {
				glog.Infof("%s: Unable to delete %s target group %s: %s",
					a.Name(),
					*lb.id,
					*targetGroup.arn,
					err)
			}
		}

		for _, listener := range lb.Listeners {
			if err := listener.delete(a); err != nil {
				glog.Infof("%s: Unable to delete %s listener %s: %s",
					a.Name(),
					*lb.id,
					*listener.arn,
					err)
			}
		}

		if err := route53svc.DeleteRecord(a, lb); err != nil {
			return err
		}

		if err := lb.delete(a); err != nil {
			glog.Infof("%s: Unable to delete load balancer %s: %s",
				a.Name(),
				*lb.arn,
				err)
		}
	}

	glog.Infof("%s: Ingress has been deleted", a.Name())
	return nil
}

// Returns true if both albIngress's are equal
func (a *albIngress) Equals(b *albIngress) bool {
	// switch {
	// case a.namespace != b.namespace:
	// 	// glog.Infof("%v != %v", a.namespace, b.namespace)
	// 	return false
	// case a.serviceName != b.serviceName:
	// 	// glog.Infof("%v != %v", a.serviceName, b.serviceName)
	// 	return false
	// case a.clusterName != b.clusterName:
	// 	// glog.Infof("%v != %v", a.clusterName, b.clusterName)
	// 	return false
	// case a.hostname != b.hostname:
	// 	// glog.Infof("%v != %v", a.hostname, b.hostname)
	// 	return false
	// case pretty.Compare(a.nodeIds, b.nodeIds) != "":
	// 	// glog.Info(pretty.Compare(a.nodeIds, b.nodeIds))
	// 	return false
	// // case pretty.Compare(a.annotations, b.annotations) != "":
	// // 	glog.Info(pretty.Compare(a.annotations, b.annotations))
	// // 	return false
	// case a.nodePort != b.nodePort:
	// 	// glog.Infof("%v != %v", a.nodePort, b.nodePort)
	// 	return false
	// }
	return true
}

func (a *albIngress) Name() string {
	return fmt.Sprintf("%s/%s", *a.namespace, *a.ingressName)
}

func (a albIngressesT) find(b *albIngress) int {
	for p, v := range a {
		if v.id == b.id {
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
