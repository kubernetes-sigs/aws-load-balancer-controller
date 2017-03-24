package controller

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"

	"k8s.io/ingress/core/pkg/ingress"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
)

var (
	route53svc *Route53
	elbv2svc   *ELBV2
	ec2svc     *EC2
	AWSDebug   bool
)

// ALBController is our main controller
type ALBController struct {
	storeLister  ingress.StoreLister
	ALBIngresses ALBIngressesT
	clusterName  *string
}

// NewALBController returns an ALBController
func NewALBController(awsconfig *aws.Config, config *Config) *ALBController {
	ac := &ALBController{
		clusterName: aws.String(config.ClusterName),
	}

	AWSDebug = config.AWSDebug
	route53svc = newRoute53(awsconfig)
	elbv2svc = newELBV2(awsconfig)
	ec2svc = newEC2(awsconfig)
	ac.ALBIngresses = assembleIngresses(ac)

	return ingress.Controller(ac).(*ALBController)
}

func (ac *ALBController) OnUpdate(ingressConfiguration ingress.Configuration) ([]byte, error) {
	OnUpdateCount.Add(float64(1))

	var ALBIngresses ALBIngressesT
	for _, ingress := range ac.storeLister.Ingress.List() {
		// Create an ALBIngress from a Kubernetes Ingress
		ALBIngress := newALBIngressFromIngress(ingress.(*extensions.Ingress), ac)
		ALBIngresses = append(ALBIngresses, ALBIngress)
		go ALBIngress.createOrModify()
	}

	ManagedIngresses.Set(float64(len(ALBIngresses)))

	// Delete ALBIngress's that no longer exist
	for _, ALBIngress := range ac.ALBIngresses {
		if ALBIngresses.find(ALBIngress) < 0 {
			go ALBIngress.delete()
		}
	}

	ac.ALBIngresses = ALBIngresses
	return []byte(""), nil
}

func (ac *ALBController) GetServiceNodePort(serviceKey string, backendPort int32) (*int64, error) {
	// Verify the service (namespace/service-name) exists in Kubernetes.
	item, exists, _ := ac.storeLister.Service.Indexer.GetByKey(serviceKey)
	if !exists {
		return nil, fmt.Errorf("Unable to find the %v service", serviceKey)
	}

	// Verify the service type is Node port.
	if item.(*api.Service).Spec.Type != api.ServiceTypeNodePort {
		return nil, fmt.Errorf("%v service is not of type NodePort", serviceKey)

	}

	// Find associated target port to ensure correct NodePort is assigned.
	for _, p := range item.(*api.Service).Spec.Ports {
		if p.Port == backendPort {
			return aws.Int64(int64(p.NodePort)), nil
		}
	}

	return nil, fmt.Errorf("Unable to find a port defined in the %v service", serviceKey)
}
