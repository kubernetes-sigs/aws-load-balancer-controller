package controller

import (
	"encoding/base64"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/kylelemons/godebug/pretty"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
)

// albIngress contains all information needed to assemble an ALB
type albIngress struct {
	id                    string
	namespace             string
	serviceName           string
	clusterName           string
	hostname              string
	nodeIds               []string
	nodePort              int32
	vpcID                 string
	loadBalancerDNSName   string
	loadBalancerArn       string
	loadBalancerScheme    string
	targetGroupArn        string
	canonicalHostedZoneId string
	annotations           *annotationsT
}

// Builds albIngress's based off of an Ingress object
func newAlbIngressesFromIngress(ingress *extensions.Ingress, ac *ALBController) []*albIngress {
	var albIngresses []*albIngress

	for _, rule := range ingress.Spec.Rules {
		// There are multiple rule.HTTP.Paths, but I don't think this can be done
		// with an ALB, only using the first one..
		path := rule.HTTP.Paths[0]

		annotations, err := ac.parseAnnotations(ingress.Annotations)
		if err != nil {
			glog.Errorf("Error parsing annotations %v: %v", ingress.Annotations, err)
			continue
		}

		a := albIngress{
			clusterName: ac.clusterName,
			namespace:   ingress.GetNamespace(),
			serviceName: path.Backend.ServiceName,
			hostname:    rule.Host,
			annotations: annotations,
		}

		item, exists, _ := ac.storeLister.Service.Indexer.GetByKey(a.ServiceKey())
		if !exists {
			glog.Errorf("Unable to find the %v service", a.ServiceKey())
			continue
		}

		service := item.(*api.Service)
		if service.Spec.Type != api.ServiceTypeNodePort {
			glog.Infof("%v service is not of type NodePort", a.ServiceKey())
			continue
		}

		a.nodePort = service.Spec.Ports[0].NodePort

		nodes, _ := ac.storeLister.Node.List()
		for _, node := range nodes.Items {
			a.nodeIds = append(a.nodeIds, node.Spec.ExternalID)
		}

		err = ac.ec2svc.setVPC(&a)
		if err != nil {
			glog.Errorf("Error fetching VPC for %v: %v", a, err)
			continue
		}

		a.id = a.resolveID()

		albIngresses = append(albIngresses, &a)
	}

	return albIngresses
}

func (a *albIngress) ServiceKey() string {
	return a.namespace + "/" + a.serviceName
}

// Returns true if both albIngress's are equal
// TODO: Test annotations
func (a *albIngress) Equals(b *albIngress) bool {
	sort.Strings(a.nodeIds)
	sort.Strings(b.nodeIds)
	// sort.Strings(a.annotations)
	// sort.Strings(b.annotations)
	switch {
	case a.namespace != b.namespace:
		// glog.Infof("%v != %v", a.namespace, b.namespace)
		return false
	case a.serviceName != b.serviceName:
		// glog.Infof("%v != %v", a.serviceName, b.serviceName)
		return false
	case a.clusterName != b.clusterName:
		// glog.Infof("%v != %v", a.clusterName, b.clusterName)
		return false
	case a.hostname != b.hostname:
		// glog.Infof("%v != %v", a.hostname, b.hostname)
		return false
	case pretty.Compare(a.nodeIds, b.nodeIds) != "":
		// glog.Info(pretty.Compare(a.nodeIds, b.nodeIds))
		return false
	// case pretty.Compare(a.annotations, b.annotations) != "":
	// 	glog.Info(pretty.Compare(a.annotations, b.annotations))
	// 	return false
	case a.nodePort != b.nodePort:
		// glog.Infof("%v != %v", a.nodePort, b.nodePort)
		return false
	}
	return true
}

func (a *albIngress) setLoadBalancer(lb *elbv2.LoadBalancer) {
	a.loadBalancerArn = *lb.LoadBalancerArn
	a.loadBalancerDNSName = *lb.DNSName
	a.loadBalancerScheme = *lb.Scheme
	a.canonicalHostedZoneId = *lb.CanonicalHostedZoneId
}

// Create a unique ingress ID used for naming ingress controller creations.
func (a *albIngress) resolveID() string {
	encoding := base64.StdEncoding
	output := make([]byte, 100)
	encoding.Encode(output, []byte(a.namespace+a.serviceName))
	// Limit to 15 characters
	if len(output) > 15 {
		output = output[:15]
	}

	return fmt.Sprintf("%s-%s", a.clusterName, output)
}
