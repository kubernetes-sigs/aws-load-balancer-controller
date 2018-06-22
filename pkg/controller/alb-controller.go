package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/spf13/pflag"

	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/record"
	"k8s.io/ingress/core/pkg/ingress"
	"k8s.io/ingress/core/pkg/ingress/controller"
	"k8s.io/ingress/core/pkg/ingress/defaults"

	"strings"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/albingress"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/albingresses"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/acm"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/ec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/iam"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/session"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/waf"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/config"
	albprom "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/prometheus"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	albsync "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/sync"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
	"github.com/prometheus/client_golang/prometheus"
)

// albController is our main controller
type albController struct {
	storeLister       ingress.StoreLister
	recorder          record.EventRecorder
	ALBIngresses      albingresses.ALBIngresses
	clusterName       string
	albNamePrefix     string
	IngressClass      string
	lastUpdate        time.Time
	albSyncInterval   time.Duration
	mutex             albsync.RWMutex
	awsChecks         map[string]func() error
	poller            func(*albController)
	initialSync       func(*albController)
	syncer            func(*albController)
	classNameGetter   func(*controller.GenericController) string
	recorderGetter    func(*controller.GenericController) record.EventRecorder
	annotationFactory annotations.AnnotationFactory
}

var logger *log.Logger

var Release = "1.0.0"
var Build = "git-00000000"

func init() {
	logger = log.New("controller")
}

// NewALBController returns an albController
func NewALBController(awsconfig *aws.Config, conf *config.Config) *albController {
	ac := &albController{
		awsChecks: make(map[string]func() error),
	}
	sess := session.NewSession(awsconfig, conf.AWSDebug)
	elbv2.NewELBV2(sess)
	ec2.NewEC2(sess)
	ec2.NewEC2Metadata(sess)
	acm.NewACM(sess)
	iam.NewIAM(sess)
	waf.NewWAFRegional(sess)

	ac.awsChecks["acm"] = acm.ACMsvc.Status()
	ac.awsChecks["ec2"] = ec2.EC2svc.Status()
	ac.awsChecks["elbv2"] = elbv2.ELBV2svc.Status()
	ac.awsChecks["iam"] = iam.IAMsvc.Status()

	ac.initialSync = syncALBsWithAWS
	ac.poller = startPolling
	ac.syncer = syncALBs
	ac.recorderGetter = recorderGetter
	ac.classNameGetter = classNameGetter
	ac.annotationFactory = annotations.NewValidatingAnnotationFactory(annotations.NewConcreteValidator())

	return ingress.Controller(ac).(*albController)
}

// Configure sets up the ingress controller based on the configuration provided in the manifest.
// Additionally, it calls the ingress assembly from AWS.
func (ac *albController) Configure(ic *controller.GenericController) error {
	var err error
	ac.IngressClass = ac.classNameGetter(ic)
	ac.albNamePrefix, err = cleanClusterName(ac.clusterName)
	if err != nil {
		return errors.New(fmt.Sprintf("Failed to generate an ALB prefix for naming. Error: %s", err.Error()))
	}

	if ac.IngressClass != "" {
		logger.Infof("Ingress class set to %s", ac.IngressClass)
	}

	if len(ac.albNamePrefix) > 11 {
		return errors.New("ALB name prefix must be 11 characters or less")
	}

	if ac.clusterName == "" {
		return errors.New("A cluster name must be defined")
	}

	if strings.Contains(ac.albNamePrefix, "-") {
		return errors.New("ALB name prefix cannot contain '-'")
	}
	ac.recorder = ac.recorderGetter(ic)

	ac.initialSync(ac)

	go ac.syncer(ac)
	go ac.poller(ac)
	return nil
}

func classNameGetter(ic *controller.GenericController) string {
	return ic.IngressClass()
}

func recorderGetter(ic *controller.GenericController) record.EventRecorder {
	return ic.GetRecorder()
}

func startPolling(ac *albController) {
	for {
		time.Sleep(10 * time.Second)
		if ac.lastUpdate.Add(60 * time.Second).Before(time.Now()) {
			logger.Infof("Ingress update being attempted. (Forced from no event seen in 60 seconds).")
			ac.update()
		}
	}
}

func syncALBs(ac *albController) {
	for {
		time.Sleep(ac.albSyncInterval)
		logger.Debugf("ALB sync interval %s elapsed; Assembly will be reattempted once lock is available..", ac.albSyncInterval)
		syncALBsWithAWS(ac)
	}
}

func syncALBsWithAWS(ac *albController) {
	ac.mutex.Lock()
	defer ac.mutex.Unlock()
	logger.Debugf("Lock was available. Attempting sync")
	ac.ALBIngresses = albingresses.AssembleIngressesFromAWS(&albingresses.AssembleIngressesFromAWSOptions{
		Recorder:      ac.recorder,
		ALBNamePrefix: ac.albNamePrefix,
	})
}

// OnUpdate is a callback invoked from the sync queue when ingress resources, or resources ingress
// resources touch, change. On each new event a new list of ALBIngresses are created and evaluated
// against the existing ALBIngress list known to the albController. Eventually the state of this
// list is synced resulting in new ingresses causing resource creation, modified ingresses having
// resources modified (when appropriate) and ingresses missing from the new list deleted from AWS.
func (ac *albController) OnUpdate(ingress.Configuration) error {
	ac.update()
	return nil
}

func (ac *albController) update() {
	ac.mutex.Lock()
	defer ac.mutex.Unlock()

	ac.lastUpdate = time.Now()
	albprom.OnUpdateCount.Add(float64(1))

	newIngresses := albingresses.NewALBIngressesFromIngresses(&albingresses.NewALBIngressesFromIngressesOptions{
		Recorder:            ac.recorder,
		ClusterName:         ac.clusterName,
		ALBNamePrefix:       ac.albNamePrefix,
		Ingresses:           ac.storeLister.Ingress.List(),
		ALBIngresses:        ac.ALBIngresses,
		IngressClass:        ac.IngressClass,
		DefaultIngressClass: ac.DefaultIngressClass(),
		GetServiceNodePort:  ac.GetServiceNodePort,
		GetNodes:            ac.GetNodes,
	}, ac.annotationFactory)

	// Append any removed ingresses to newIngresses, their desired state will have been stripped.
	newIngresses = append(newIngresses, ac.ALBIngresses.RemovedIngresses(newIngresses)...)

	// Update the prometheus gauge
	ingressesByNamespace := map[string]int{}
	logger.Debugf("Ingress count: %d", len(newIngresses))
	for _, ingress := range newIngresses {
		ingressesByNamespace[ingress.Namespace()]++
	}

	for ns, count := range ingressesByNamespace {
		albprom.ManagedIngresses.With(
			prometheus.Labels{"namespace": ns}).Set(float64(count))
	}

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

	// clean up all deleted ingresses from the list
	for _, ingress := range ac.ALBIngresses {
		if ingress.LoadBalancer != nil && ingress.LoadBalancer.IsDeleted() {
			i, _ := ac.ALBIngresses.FindByID(ingress.ID)
			ac.ALBIngresses = append(ac.ALBIngresses[:i], ac.ALBIngresses[i+1:]...)
			albprom.ManagedIngresses.With(
				prometheus.Labels{"namespace": ingress.Namespace()}).Dec()
		}
	}
}

// OverrideFlags configures optional override flags for the ingress controller
func (ac *albController) OverrideFlags(flags *pflag.FlagSet) {
	flags.Set("update-status-on-shutdown", "false")
	flags.Set("sync-period", "30s")
}

// SetConfig configures a configmap for the ingress controller
func (ac *albController) SetConfig(cfgMap *api.ConfigMap) {
}

func (ac *albController) DefaultEndpoint() ingress.Endpoint {
	return ingress.Endpoint{}
}

// SetListers sets the configured store listers in the generic ingress controller
func (ac *albController) SetListers(lister ingress.StoreLister) {
	ac.storeLister = lister
}

// BackendDefaults returns default configurations for the backend
func (ac *albController) BackendDefaults() defaults.Backend {
	var backendDefaults defaults.Backend
	return backendDefaults
}

// Name returns the ingress controller name
func (ac *albController) Name() string {
	return "AWS Application Load Balancer Controller"
}

// Check tests the ingress controller configuration
func (ac *albController) Check(*http.Request) error {
	return nil
}

// DefaultIngressClass returns thed default ingress class
func (ac *albController) DefaultIngressClass() string {
	return "alb"
}

// Info returns information on the ingress contoller
func (ac *albController) Info() *ingress.BackendInfo {
	return &ingress.BackendInfo{
		Name:       "ALB Ingress Controller",
		Release:    Release,
		Build:      Build,
		Repository: "git://github.com/kubernetes-sigs/aws-alb-ingress-controller",
	}
}

// ConfigureFlags adds command line parameters to the ingress cmd.
func (ac *albController) ConfigureFlags(pf *pflag.FlagSet) {
	pf.StringVar(&ac.clusterName, "clusterName", os.Getenv("CLUSTER_NAME"), "Cluster Name (required)")

	rawrs := os.Getenv("ALB_CONTROLLER_RESTRICT_SCHEME")
	// Default ALB_CONTROLLER_RESTRICT_SCHEME to false
	if rawrs == "" {
		rawrs = "false"
	}
	rs, err := strconv.ParseBool(rawrs)
	if err != nil {
		logger.Fatalf("ALB_CONTROLLER_RESTRICT_SCHEME environment variable must be either true or false. Value was: %s", rawrs)
	}
	pf.BoolVar(&config.RestrictScheme, "restrict-scheme", rs, "Restrict the scheme to internal except for whitelisted namespaces (defaults to false)")

	ns := os.Getenv("ALB_CONTROLLER_RESTRICT_SCHEME_CONFIG_NAMESPACE")
	if ns == "" {
		ns = "default"
	}
	pf.StringVar(&config.RestrictSchemeNamespace, "restrict-scheme-namespace", ns, "The namespace that holds the configmap with the allowed ingresses. Only respected when restrict-scheme is true.")

	albSyncParam := os.Getenv("ALB_SYNC_INTERVAL")
	if albSyncParam == "" {
		albSyncParam = "60m"
	}
	albSyncInterval, err := time.ParseDuration(albSyncParam)
	if err != nil {
		logger.Exitf("Failed to parse duration from ALB_SYNC_INTERVAL value of '%s'", albSyncParam)
	}
	pf.DurationVar(&ac.albSyncInterval, "alb-sync-interval", albSyncInterval, "Frequency with which to sync ALBs for external changes")
}

// StateHandler JSON encodes the ALBIngresses and writes to the HTTP ResponseWriter.
func (ac *albController) StateHandler(w http.ResponseWriter, r *http.Request) {
	ac.mutex.RLock()
	defer ac.mutex.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ac.ALBIngresses)
}

func (ac *albController) collectChecks() (map[string]string, int) {
	ac.mutex.RLock()
	defer ac.mutex.RUnlock()
	results := make(map[string]string)
	status := http.StatusOK
	for name, check := range ac.awsChecks {
		if err := check(); err != nil {
			status = http.StatusServiceUnavailable
			results[name] = err.Error()
		} else {
			results[name] = "OK"
		}
	}
	return results, status
}

// StatusHandler validates basic connectivity to the AWS APIs.
func (ac *albController) StatusHandler(w http.ResponseWriter, r *http.Request) {
	checkResults, status := ac.collectChecks()

	// write out the response code and content type header
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	// unless ?full=1, return an empty body. Kubernetes only cares about the
	// HTTP status code, so we won't waste bytes on the full body.
	if r.URL.Query().Get("full") != "1" {
		w.Write([]byte("{}\n"))
		return
	}

	// otherwise, write the JSON body ignoring any encoding errors (which
	// shouldn't really be possible since we're encoding a map[string]string).
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "    ")
	encoder.Encode(checkResults)
}

// UpdateIngressStatus returns the hostnames for the ALB.
func (ac *albController) UpdateIngressStatus(ing *extensions.Ingress) []api.LoadBalancerIngress {
	//ac.mutex.RLock()
	//defer ac.mutex.RUnlock()
	ingress := albingress.NewALBIngress(&albingress.NewALBIngressOptions{
		Namespace:   ing.ObjectMeta.Namespace,
		Name:        ing.ObjectMeta.Name,
		ClusterName: ac.clusterName,
		Recorder:    ac.recorder,
	})

	if _, i := ac.ALBIngresses.FindByID(ingress.ID); i != nil {
		hostnames, err := i.Hostnames()
		// ensures the hostname exists and that the ALBIngress succesfully reconciled before returning
		// hostnames for updating the ingress status.
		if err == nil && i.Reconciled {
			return hostnames
		}
	}

	return []api.LoadBalancerIngress{}
}

// GetServiceNodePort returns the nodeport for a given Kubernetes service
func (ac *albController) GetServiceNodePort(serviceKey string, backendPort int32) (*int64, error) {
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
func (ac *albController) GetNodes() util.AWSStringSlice {
	var result util.AWSStringSlice
	nodes := ac.storeLister.Node.List()
	for _, node := range nodes {
		n := node.(*api.Node)
		// excludes all master nodes from the list of nodes returned.
		// specifically, this looks for the presence of the label
		// 'node-role.kubernetes.io/master' as of this writing, this is the way to indicate
		// the nodes is a 'master node' xref: https://github.com/kubernetes/kubernetes/pull/41835
		if _, ok := n.ObjectMeta.Labels["node-role.kubernetes.io/master"]; ok {
			continue
		}
		if s, ok := n.ObjectMeta.Labels["alpha.service-controller.kubernetes.io/exclude-balancer"]; ok {
			if strings.ToUpper(s) == "TRUE" {
				continue
			}
		}
		result = append(result, aws.String(n.Spec.ExternalID))
	}
	sort.Sort(result)
	return result
}

func cleanClusterName(cn string) (string, error) {
	reg, err := regexp.Compile("[^a-zA-Z0-9]+")
	if err != nil {
		return "", err
	}
	n := strings.ToLower(reg.ReplaceAllString(cn, ""))
	if len(n) > 11 {
		n = n[:11]
	}
	return n, nil
}
