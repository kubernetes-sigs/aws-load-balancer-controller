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

// OnUpdate is a callback invoked from the sync queue when ingress resources, or resources ingress
// resources touch, change. On each new event a new list of ALBIngresses are created and evaluated
// against the existing ALBIngress list known to the ALBController. Eventually the state of this
// list is synced resulting in new ingresses causing resource creation, modified ingresses having
// resources modified (when appropriate) and ingresses missing from the new list deleted from AWS.
func (ac *ALBController) OnUpdate(ingressConfiguration ingress.Configuration) ([]byte, error) {
	OnUpdateCount.Add(float64(1))

	// Create new ALBIngress list for this invocation.
	var ALBIngresses ALBIngressesT
	// Find every ingress currently in Kubernetes.
	for _, ingress := range ac.storeLister.Ingress.List() {
		// Produce a new ALBIngress instance for every ingress found. If ALBIngress returns nil, there
		// was an issue with the ingress (e.g. bad annotations) and should not be added to the list.
		ALBIngress := NewALBIngressFromIngress(ingress.(*extensions.Ingress), ac)
		if ALBIngress == nil {
			continue
		}
		// Add the new ALBIngress instance to the new ALBIngress list.
		ALBIngresses = append(ALBIngresses, ALBIngress)
	}

	// Caputure any ingresses missing from the new list that qualify for deletion.
	deletable := ac.ingressToDelete(ALBIngresses)
	// If deletable ingresses were found, add them to the list so they'll be deleted when SyncState()
	// is called.
	if len(deletable) > 0 {
		ALBIngresses = append(ALBIngresses, deletable...)
	}

	ManagedIngresses.Set(float64(len(ALBIngresses)))
	// Update the list of ALBIngresses known to the ALBIngress controller to the newly generated list.
	ac.ALBIngresses = ALBIngresses
	return []byte(""), nil
}

func (ac *ALBController) Reload(data []byte) ([]byte, bool, error) {
	ReloadCount.Add(float64(1))

	// Sync the state, resulting in creation, modify, delete, or no action, for every ALBIngress
	// instance known to the ALBIngress controller.
	for _, ALBIngress := range ac.ALBIngresses {
		ALBIngress.SyncState()
	}

	return []byte(""), true, nil
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

// Returns a list of ingress objects that are no longer known to kubernetes and should
// be deleted.
func (ac *ALBController) ingressToDelete(newList ALBIngressesT) ALBIngressesT {
	var deleteableIngress ALBIngressesT

	// Loop through every ingress in current (old) ingress list known to ALBController
	for _, ingress := range ac.ALBIngresses {
		// Ingress objects not found in newList might qualify for deletion.
		if i := newList.find(ingress); i < 0 {
			// If the ALBIngress still contains LoadBalancer(s), it still needs to be deleted.
			// In this case, strip all desired state and add it to the deleteableIngress list.
			// If the ALBIngress contains no LoadBalancer(s), it was previously deleted and is
			// no longer relevant to the ALBController.
			if len(ingress.LoadBalancers) > 0 {
				ingress.StripDesiredState()
				deleteableIngress = append(deleteableIngress, ingress)
			}
		}
	}
	return deleteableIngress
}
