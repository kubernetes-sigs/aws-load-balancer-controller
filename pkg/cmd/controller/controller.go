package controller

import (
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/glog"

	"git.tm.tmcs/kubernetes/alb-ingress/pkg/config"
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
}

type albIngressesT []*albIngress

// NewALBController returns an ALBController
func NewALBController(awsconfig *aws.Config) ingress.Controller {
	alb := ALBController{
		route53svc: newRoute53(awsconfig),
		elbv2svc:   newELBV2(awsconfig),
	}

	alb.route53svc.sanityTest()

	return ingress.Controller(&alb)
}

func (ac *ALBController) OnUpdate(ingressConfiguration ingress.Configuration) ([]byte, error) {
	glog.Infof("Received OnUpdate notification")
	var albIngresses albIngressesT

	// TODO: if ac.lastAlbIngresses is empty, try to build it up from AWS resources

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
				glog.Infof("Comparing %v to %v", albIngress.ServiceKey(), lastIngress.ServiceKey())
				if albIngress.Equals(lastIngress) {
					glog.Infof("Nothing new with %v", albIngress.ServiceKey())
					continue NEWINGRESSES
				}
			}

			// new/modified ingress, execute .Create
			if err := albIngress.Create(); err != nil {
				glog.Errorf("Error creating ingress!: %s", err)
			}
		}
	}

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

func (ac *ALBController) SetConfig(cfgMap *api.ConfigMap) {
	glog.Infof("Config map %+v", cfgMap)
}

// SetListers sets the configured store listers in the generic ingress controller
func (ac *ALBController) SetListers(lister ingress.StoreLister) {
	ac.storeLister = lister
}

func (ac *ALBController) Reload(data []byte) ([]byte, bool, error) {
	glog.Infof("Reload()")
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
		Repository: "git://git.tm.tmcs/kubernetes/alb-ingress-controller",
	}
}

func (a albIngressesT) find(b *albIngress) int {
	for p, v := range a {
		if v.Equals(b) {
			return p
		}
	}
	return -1
}
