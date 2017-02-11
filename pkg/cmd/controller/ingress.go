package controller

import (
	"log"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
)

// albIngress contains all information needed to assemble an ALB
type albIngress struct {
	// ingressSpec extensions.IngressSpe
	// server  *ingress.Server
	namespace   string
	serviceName string
	clusterName string
	hostname    string
	alb         *ALB
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
			clusterName: ingress.GetClusterName(),
			namespace:   ingress.GetNamespace(),
			serviceName: path.Backend.ServiceName,
			hostname:    rule.Host,
		}

		item, exists, _ := ac.storeLister.Service.Indexer.GetByKey(a.ServiceKey())
		if !exists {
			log.Printf("Unable to find the %v service", a.ServiceKey())
			continue
		}

		service := item.(*api.Service)
		if service.Spec.Type != api.ServiceTypeNodePort {
			log.Printf("%v service is not of type NodePort", a.ServiceKey())
			continue
		}

		nodeport := service.Spec.Ports[0].NodePort

		// build up list of nodes, i dont know where to find these
		nodes := []string{"1.2.3.4", "2.3.4.5", "4.5.6.7"}

		// build up list of ingress.Endpoint, but I don't know where to get the
		// node ips
		if false {
			for _, node := range nodes {
				log.Printf("%v:%v", node, nodeport)
			}
		}

		// TODO: locate EC2 instances from ips

		albIngresses = append(albIngresses, &a)
	}

	return albIngresses
}

func (a *albIngress) ServiceKey() string {
	return a.namespace + "/" + a.serviceName
}

func (a *albIngress) Build() error {
	log.Printf("Creating an ASG for %v", a.serviceName)
	log.Printf("Creating an ALB for %v", a.serviceName)
	log.Printf("Creating a Route53 record for %v", a.hostname)
	return nil
}
