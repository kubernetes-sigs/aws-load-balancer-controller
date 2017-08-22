package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/spf13/pflag"

	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/record"
	"k8s.io/ingress/core/pkg/ingress"
	"k8s.io/ingress/core/pkg/ingress/controller"
	"k8s.io/ingress/core/pkg/ingress/defaults"

	"github.com/coreos/alb-ingress-controller/pkg/albingress"
	"github.com/coreos/alb-ingress-controller/pkg/albingresses"
	"github.com/coreos/alb-ingress-controller/pkg/aws/acm"
	"github.com/coreos/alb-ingress-controller/pkg/aws/ec2"
	"github.com/coreos/alb-ingress-controller/pkg/aws/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/aws/iam"
	"github.com/coreos/alb-ingress-controller/pkg/aws/session"
	"github.com/coreos/alb-ingress-controller/pkg/config"
	albprom "github.com/coreos/alb-ingress-controller/pkg/prometheus"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
)

// ALBController is our main controller
type ALBController struct {
	storeLister  ingress.StoreLister
	recorder     record.EventRecorder
	ALBIngresses albingresses.ALBIngresses
	clusterName  string
	IngressClass string
}

var logger *log.Logger

func init() {
	logger = log.New("controller")
}

// NewALBController returns an ALBController
func NewALBController(awsconfig *aws.Config, conf *config.Config) *ALBController {
	ac := new(ALBController)
	sess := session.NewSession(awsconfig, conf.AWSDebug)
	elbv2.NewELBV2(sess)
	ec2.NewEC2(sess)
	acm.NewACM(sess)
	iam.NewIAM(sess)

	return ingress.Controller(ac).(*ALBController)
}

func (ac *ALBController) Configure(ic *controller.GenericController) {
	ac.IngressClass = ic.IngressClass()
	if ac.IngressClass != "" {
		logger.Infof("Ingress class set to %s", ac.IngressClass)
	}

	if len(ac.clusterName) > 11 {
		logger.Exitf("Cluster name must be 11 characters or less")
	}

	if ac.clusterName == "" {
		logger.Exitf("A cluster name must be defined")
	}

	if strings.Contains(ac.clusterName, "-") {
		logger.Exitf("Cluster name cannot contain '-'")
	}

	ac.recorder = ic.GetRecoder()

	ac.ALBIngresses = albingresses.AssembleIngressesFromAWS(&albingresses.AssembleIngressesFromAWSOptions{
		Recorder:    ac.recorder,
		ClusterName: ac.clusterName,
	})
}

// OnUpdate is a callback invoked from the sync queue when ingress resources, or resources ingress
// resources touch, change. On each new event a new list of ALBIngresses are created and evaluated
// against the existing ALBIngress list known to the ALBController. Eventually the state of this
// list is synced resulting in new ingresses causing resource creation, modified ingresses having
// resources modified (when appropriate) and ingresses missing from the new list deleted from AWS.
func (ac *ALBController) OnUpdate(_ ingress.Configuration) error {
	albprom.OnUpdateCount.Add(float64(1))

	logger.Debugf("OnUpdate event seen by ALB ingress controller.")

	newIngresses := albingresses.NewALBIngressesFromIngresses(&albingresses.NewALBIngressesFromIngressesOptions{
		Recorder:            ac.recorder,
		ClusterName:         ac.clusterName,
		Ingresses:           ac.storeLister.Ingress.List(),
		ALBIngresses:        ac.ALBIngresses,
		IngressClass:        ac.IngressClass,
		DefaultIngressClass: ac.DefaultIngressClass(),
		GetServiceNodePort:  ac.GetServiceNodePort,
		GetNodes:            ac.GetNodes,
	})

	// Append any removed ingresses to newIngresses, their desired state will have been stripped.
	newIngresses = append(newIngresses, ac.ALBIngresses.RemovedIngresses(newIngresses)...)

	// Update the prometheus gauge
	albprom.ManagedIngresses.Set(float64(len(newIngresses)))

	// Update the list of ALBIngresses known to the ALBIngress controller to the newly generated list.
	ac.ALBIngresses = newIngresses

	// Sync the state, resulting in creation, modify, delete, or no action, for every ALBIngress
	// instance known to the ALBIngress controller.
	var wg sync.WaitGroup
	wg.Add(len(ac.ALBIngresses))
	for _, ingress := range ac.ALBIngresses {
		go func(wg *sync.WaitGroup, ingress *albingress.ALBIngress) {
			defer wg.Done()
			ingress.Reconcile(&albingress.ReconcileOptions{Eventf: ingress.Eventf})
		}(&wg, ingress)
	}
	wg.Wait()

	return nil
}

// OverrideFlags configures optional override flags for the ingress controller
func (ac *ALBController) OverrideFlags(flags *pflag.FlagSet) {
	flags.Set("update-status-on-shutdown", "false")
	flags.Set("sync-period", "30s")
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
		Release:    "1.0.0",
		Build:      "git-00000000",
		Repository: "git://github.com/coreos/alb-ingress-controller",
	}
}

// ConfigureFlags adds command line parameters to the ingress cmd.
func (ac *ALBController) ConfigureFlags(pf *pflag.FlagSet) {
	pf.StringVar(&ac.clusterName, "clusterName", os.Getenv("CLUSTER_NAME"), "Cluster Name (required)")
}

// StateHandler JSON encodes the ALBIngresses and writes to the HTTP ResponseWriter.
func (ac *ALBController) StateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ac.ALBIngresses)
}

// UpdateIngressStatus returns the hostnames for the ALB.
func (ac *ALBController) UpdateIngressStatus(ing *extensions.Ingress) []api.LoadBalancerIngress {
	ingress := albingress.NewALBIngress(&albingress.NewALBIngressOptions{
		Namespace:   ing.ObjectMeta.Namespace,
		Name:        ing.ObjectMeta.Name,
		ClusterName: ac.clusterName,
		Recorder:    ac.recorder,
	})

	i := ac.ALBIngresses.Find(ingress)
	if i < 0 {
		logger.Errorf("Unable to find ingress %s", ingress.Name())
		return nil
	}

	hostnames, err := ac.ALBIngresses[i].Hostnames()
	if err != nil {
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

// GetNodes returns a list of the cluster node external ids
func (ac *ALBController) GetNodes() util.AWSStringSlice {
	var result util.AWSStringSlice
	nodes := ac.storeLister.Node.List()
	for _, node := range nodes {
		result = append(result, aws.String(node.(*api.Node).Spec.ExternalID))
	}
	sort.Sort(result)
	return result
}
