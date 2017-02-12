package controller

import (
	"sort"

	"github.com/golang/glog"
	"github.com/kylelemons/godebug/pretty"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
)

// albIngress contains all information needed to assemble an ALB
type albIngress struct {
	namespace   string
	serviceName string
	clusterName string
	hostname    string
	nodeIds     []string
	nodePort    int32
	annotations map[string]string
	elbv2       *ELBV2
	route53     *Route53
}

// Builds albIngress's based off of an Ingress object
func newAlbIngressesFromIngress(ingress *extensions.Ingress, ac *ALBController) []*albIngress {
	var albIngresses []*albIngress

	for _, rule := range ingress.Spec.Rules {
		// There are multiple rule.HTTP.Paths, but I don't think this can be done
		// with an ALB, only using the first one..
		path := rule.HTTP.Paths[0]

		a := albIngress{
			// TODO: Remove once resolving correctly
			clusterName: "TEMPCLUSTERNAME", // TODO why is this empty, might be good for ELB names
			namespace:   ingress.GetNamespace(),
			serviceName: path.Backend.ServiceName,
			hostname:    rule.Host,
			annotations: ingress.Annotations,
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

		albIngresses = append(albIngresses, &a)
	}

	return albIngresses
}

func (a *albIngress) ServiceKey() string {
	return a.namespace + "/" + a.serviceName
}

func (a *albIngress) Create() error {
	glog.Infof("Creating an ASG for %v", a.serviceName)
	glog.Infof("Creating an ALB for %v", a.serviceName)
	glog.Infof("Creating a Route53 record for %v", a.hostname)

	if err := a.elbv2.alterALB(a); err != nil {
		return err
	}

	if err := a.route53.upsertRecord(a); err != nil {
		return err
	}
	return nil
}

func (a *albIngress) Destroy() error {
	glog.Infof("Deleting the ALB for %v", a.serviceName)
	glog.Infof("Deleting the Route53 record for %v", a.hostname)
	if err := a.route53.deleteRecord(a); err != nil {
		return err
	}

	return nil
}

// Returns true if both albIngress's are equal
// TODO: Test annotations
func (a *albIngress) Equals(b *albIngress) bool {
	sort.Strings(a.nodeIds)
	sort.Strings(b.nodeIds)
	switch {
	case a.namespace != b.namespace:
		glog.Infof("%v != %v", a.namespace, b.namespace)
		return false
	case a.serviceName != b.serviceName:
		glog.Infof("%v != %v", a.serviceName, b.serviceName)
		return false
	case a.clusterName != b.clusterName:
		glog.Infof("%v != %v", a.clusterName, b.clusterName)
		return false
	case a.hostname != b.hostname:
		glog.Infof("%v != %v", a.hostname, b.hostname)
		return false
	case pretty.Compare(a.nodeIds, b.nodeIds) != "":
		glog.Info(pretty.Compare(a.nodeIds, b.nodeIds))
		return false
	case pretty.Compare(a.annotations, b.annotations) != "":
		glog.Info(pretty.Compare(a.annotations, b.annotations))
		return false
	case a.nodePort != b.nodePort:
		glog.Infof("%v != %v", a.nodePort, b.nodePort)
		return false
	}
	return true
}
