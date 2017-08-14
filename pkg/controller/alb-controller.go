package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/alb"
	"github.com/coreos/alb-ingress-controller/pkg/config"
	awsutil "github.com/coreos/alb-ingress-controller/pkg/util/aws"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	"github.com/spf13/pflag"

	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/record"
	"k8s.io/ingress/core/pkg/ingress"
	"k8s.io/ingress/core/pkg/ingress/controller"
	"k8s.io/ingress/core/pkg/ingress/defaults"
)

// ALBController is our main controller
type ALBController struct {
	storeLister    ingress.StoreLister
	recorder       record.EventRecorder
	ALBIngresses   ALBIngressesT
	clusterName    *string
	IngressClass   string
	disableRoute53 bool
}

// NewALBController returns an ALBController
func NewALBController(awsconfig *aws.Config, conf *config.Config) *ALBController {
	ac := &ALBController{
		clusterName:    aws.String(conf.ClusterName),
		disableRoute53: conf.DisableRoute53,
	}

	awsutil.AWSDebug = conf.AWSDebug
	awsutil.Session = awsutil.NewSession(awsconfig)
	awsutil.ALBsvc = awsutil.NewELBV2(awsutil.Session)
	awsutil.Ec2svc = awsutil.NewEC2(awsutil.Session)
	awsutil.ACMsvc = awsutil.NewACM(awsutil.Session)
	awsutil.IAMsvc = awsutil.NewIAM(awsutil.Session)

	if !conf.DisableRoute53 {
		awsutil.Route53svc = awsutil.NewRoute53(awsutil.Session)
	}

	return ingress.Controller(ac).(*ALBController)
}

func (ac *ALBController) Configure(ic *controller.GenericController) {
	ac.IngressClass = ic.IngressClass()
	if ac.IngressClass != "" {
		log.Infof("Ingress class set to %s", "controller", ac.IngressClass)
	}

	ac.recorder = ic.GetRecoder()
}

// OnUpdate is a callback invoked from the sync queue when ingress resources, or resources ingress
// resources touch, change. On each new event a new list of ALBIngresses are created and evaluated
// against the existing ALBIngress list known to the ALBController. Eventually the state of this
// list is synced resulting in new ingresses causing resource creation, modified ingresses having
// resources modified (when appropriate) and ingresses missing from the new list deleted from AWS.
func (ac *ALBController) OnUpdate(ingressConfiguration ingress.Configuration) error {
	awsutil.OnUpdateCount.Add(float64(1))

	log.Debugf("OnUpdate event seen by ALB ingress controller.", "controller")

	// Create new ALBIngress list for this invocation.
	var ALBIngresses ALBIngressesT
	// Find every ingress currently in Kubernetes.
	for _, ingress := range ac.storeLister.Ingress.List() {
		ingResource := ingress.(*extensions.Ingress)
		// Ensure the ingress resource found contains an appropriate ingress class.
		if !ac.validIngress(ingResource) {
			continue
		}
		// Produce a new ALBIngress instance for every ingress found. If ALBIngress returns nil, there
		// was an issue with the ingress (e.g. bad annotations) and should not be added to the list.
		ALBIngress, err := NewALBIngressFromIngress(ingResource, ac)
		if ALBIngress == nil {
			continue
		}
		if err != nil {
			ALBIngress.tainted = true
		}
		// Add the new ALBIngress instance to the new ALBIngress list.
		ALBIngresses = append(ALBIngresses, ALBIngress)
	}

	// Capture any ingresses missing from the new list that qualify for deletion.
	deletable := ac.ingressToDelete(ALBIngresses)
	// If deletable ingresses were found, add them to the list so they'll be deleted when Reconcile()
	// is called.
	if len(deletable) > 0 {
		ALBIngresses = append(ALBIngresses, deletable...)
	}

	awsutil.ManagedIngresses.Set(float64(len(ALBIngresses)))
	// Update the list of ALBIngresses known to the ALBIngress controller to the newly generated list.
	ac.ALBIngresses = ALBIngresses

	// Sync the state, resulting in creation, modify, delete, or no action, for every ALBIngress
	// instance known to the ALBIngress controller.
	var wg sync.WaitGroup
	wg.Add(len(ac.ALBIngresses))
	for _, ingress := range ac.ALBIngresses {
		go func(wg *sync.WaitGroup, ingress *ALBIngress) {
			defer wg.Done()
			rOpts := alb.NewReconcileOptions().SetDisableRoute53(ac.disableRoute53).SetEventf(ingress.Eventf)
			ingress.Reconcile(rOpts)
		}(&wg, ingress)
	}
	wg.Wait()

	return nil
}

// validIngress checks whether the ingress controller has an IngressClass set. If it does, it will
// only return true if the ingress resource passed in has the same class specified via the
// kubernetes.io/ingress.class annotation.
func (ac ALBController) validIngress(i *extensions.Ingress) bool {
	if ac.IngressClass == "" {
		return true
	}
	if i.Annotations["kubernetes.io/ingress.class"] == ac.IngressClass {
		return true
	}
	return false
}

// OverrideFlags configures optional override flags for the ingress controller
func (ac *ALBController) OverrideFlags(flags *pflag.FlagSet) {
	flags.Set("update-status-on-shutdown", "false")
}

// SetConfig configures a configmap for the ingress controller
func (ac *ALBController) SetConfig(cfgMap *api.ConfigMap) {
}

// SetListers sets the configured store listers in the generic ingress controller
func (ac *ALBController) SetListers(lister ingress.StoreLister) {
	ac.storeLister = lister
}

// BackendDefaults returns default configurations for the backend
func (ac *ALBController) BackendDefaults() defaults.Backend {
	var backendDefaults defaults.Backend
	return backendDefaults
}

// Name returns the ingress controller name
func (ac *ALBController) Name() string {
	return "AWS Application Load Balancer Controller"
}

// Check tests the ingress controller configuration
func (ac *ALBController) Check(_ *http.Request) error {
	return nil
}

// DefaultIngressClass returns thed default ingress class
func (ac *ALBController) DefaultIngressClass() string {
	return "alb"
}

// Info returns information on the ingress contoller
func (ac *ALBController) Info() *ingress.BackendInfo {
	return &ingress.BackendInfo{
		Name:       "ALB Ingress Controller",
		Release:    "0.0.1",
		Build:      "git-00000000",
		Repository: "git://github.com/coreos/alb-ingress-controller",
	}
}

// ConfigureFlags
func (ac *ALBController) ConfigureFlags(pf *pflag.FlagSet) {
	pf.BoolVar(&ac.disableRoute53, "disable-route53", ac.disableRoute53, "Disable Route 53 management")
}

func (ac *ALBController) UpdateIngressStatus(ingress *extensions.Ingress) []api.LoadBalancerIngress {
	albIngress := NewALBIngress(ingress.ObjectMeta.Namespace, ingress.ObjectMeta.Name, *ac.clusterName)

	i := ac.ALBIngresses.find(albIngress)
	if i < 0 {
		log.Errorf("Unable to find ingress", *albIngress.id)
		return nil
	}

	var hostnames []api.LoadBalancerIngress
	for _, lb := range ac.ALBIngresses[i].LoadBalancers {
		if lb.CurrentLoadBalancer == nil || lb.CurrentLoadBalancer.DNSName == nil {
			continue
		}
		hostnames = append(hostnames, api.LoadBalancerIngress{Hostname: *lb.CurrentLoadBalancer.DNSName})
	}

	if len(hostnames) == 0 {
		log.Errorf("No ALB hostnames for ingress", *albIngress.id)
		return nil
	}

	return hostnames
}

// GetServiceNodePort returns the nodeport for a given Kubernetes service
func (ac *ALBController) GetServiceNodePort(serviceKey string, backendPort int32) (*int64, error) {
	// Verify the service (namespace/service-name) exists in Kubernetes.
	item, exists, _ := ac.storeLister.Service.GetByKey(serviceKey)
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
		// If assembling the ingress resource failed, don't attempt deletion
		if ingress.tainted {
			continue
		}
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

func (ac *ALBController) StateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ac.ALBIngresses)
}

// AssembleIngresses builds a list of existing ingresses from resources in AWS
func (ac *ALBController) AssembleIngresses() {
	log.Infof("Build up list of existing ingresses", "controller")
	ac.ALBIngresses = nil

	loadBalancers, err := awsutil.ALBsvc.DescribeLoadBalancers(ac.clusterName)
	if err != nil {
		log.Fatalf(err.Error(), "aws")
	}

	var wg sync.WaitGroup
	wg.Add(len(loadBalancers))

	for _, loadBalancer := range loadBalancers {
		go func(wg *sync.WaitGroup, loadBalancer *elbv2.LoadBalancer) {
			defer wg.Done()

			albIngress, ok := NewALBIngressFromLoadBalancer(loadBalancer, *ac.clusterName, ac.disableRoute53)
			if !ok {
				return
			}

			if i := ac.ALBIngresses.find(albIngress); i >= 0 {
				albIngress = ac.ALBIngresses[i]
				albIngress.LoadBalancers = append(albIngress.LoadBalancers, albIngress.LoadBalancers[0])
			} else {
				ac.ALBIngresses = append(ac.ALBIngresses, albIngress)
			}
		}(&wg, loadBalancer)
	}
	wg.Wait()

	log.Infof("Assembled %d ingresses from existing AWS resources", "controller", len(ac.ALBIngresses))
}
