package controller

import (
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/glog"
	"github.com/spf13/pflag"

	"git.tmaws.io/kubernetes/alb-ingress/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/ingress/core/pkg/ingress"
	"k8s.io/ingress/core/pkg/ingress/defaults"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
)

// ALBController is our main controller
type ALBController struct {
	route53svc       *Route53
	elbv2svc         *ELBV2
	storeLister      ingress.StoreLister
	lastAlbIngresses albIngressesT
	clusterName      string
}

type albIngressesT []*albIngress

var (
	OnUpdateCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "albingress_updates",
		Help: "Number of times OnUpdate has been called.",
	},
	)
	AWSErrorCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "albingress_aws_errors",
		Help: "Number of errors from the AWS API",
	},
		[]string{"service", "request"},
	)
	ManagedIngresses = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "albingress_managed_ingresses",
		Help: "Number of ingresses being managed",
	})
)

func init() {
	prometheus.MustRegister(OnUpdateCount)
	prometheus.MustRegister(AWSErrorCount)
	prometheus.MustRegister(ManagedIngresses)
}

// NewALBController returns an ALBController
func NewALBController(awsconfig *aws.Config, clusterName string) ingress.Controller {
	alb := ALBController{
		route53svc:  newRoute53(awsconfig),
		elbv2svc:    newELBV2(awsconfig),
		clusterName: clusterName,
	}

	alb.route53svc.sanityTest()

	return ingress.Controller(&alb)
}

func (ac *ALBController) OnUpdate(ingressConfiguration ingress.Configuration) ([]byte, error) {
	OnUpdateCount.Add(float64(1))

	var albIngresses albIngressesT

	if len(ac.lastAlbIngresses) == 0 {
		ac.lastAlbIngresses = ac.assembleIngresses()
	}

	for _, ingress := range ac.storeLister.Ingress.List() {
		// first assemble the albIngress objects
	NEWINGRESSES:
		for _, albIngress := range newAlbIngressesFromIngress(ingress.(*extensions.Ingress), ac) {
			albIngress.route53 = ac.route53svc
			albIngress.elbv2 = ac.elbv2svc
			albIngresses = append(albIngresses, albIngress)

			// search for albIngress in ac.lastAlbIngresses, if found and
			// unchanged, continue
			for _, lastIngress := range ac.lastAlbIngresses {
				if albIngress.Equals(lastIngress) {
					continue NEWINGRESSES
				}
			}

			// new/modified ingress, execute .Create
			if err := albIngress.Create(); err != nil {
				glog.Errorf("Error creating ingress!: %s", err)
			}
		}
	}

	ManagedIngresses.Set(float64(len(albIngresses)))

	// compare albIngresses to ac.lastAlbIngresses
	// execute .Destroy on any that were removed
	for _, albIngress := range ac.lastAlbIngresses {
		if albIngresses.find(albIngress) < 0 {
			albIngress.Destroy()
		}
	}

	ac.lastAlbIngresses = albIngresses
	return []byte(""), nil
}

// OverrideFlags
func (ac *ALBController) OverrideFlags(flags *pflag.FlagSet) {
	// TODO: use this?
}

func (ac *ALBController) SetConfig(cfgMap *api.ConfigMap) {
	glog.Infof("Config map %+v", cfgMap)
}

// SetListers sets the configured store listers in the generic ingress controller
func (ac *ALBController) SetListers(lister ingress.StoreLister) {
	ac.storeLister = lister
}

func (ac *ALBController) Reload(data []byte) ([]byte, bool, error) {
	return []byte(""), true, nil
}

func (ac *ALBController) BackendDefaults() defaults.Backend {
	return config.NewDefault().Backend
}

func (ac *ALBController) Name() string {
	return "AWS Application Load Balancer Controller"
}

func (ac *ALBController) Check(_ *http.Request) error {
	return nil
}

func (ac *ALBController) Info() *ingress.BackendInfo {
	return &ingress.BackendInfo{
		Name:       "ALB Controller",
		Release:    "0.0.1",
		Build:      "git-00000000",
		Repository: "git://git.tmaws.io/kubernetes/alb-ingress",
	}
}

func (ac *ALBController) assembleIngresses() albIngressesT {
	// TODO:
	// First, search out ALBs that have our tag sets
	// Build up albIngress's out of these
	// Populate with targets/configs/route53/etc
	// I think we can ignore Route53 for this
	glog.Info("Build up list of existing ingresses")
	return albIngressesT{}
}

func (a albIngressesT) find(b *albIngress) int {
	for p, v := range a {
		if v.Equals(b) {
			return p
		}
	}
	return -1
}
