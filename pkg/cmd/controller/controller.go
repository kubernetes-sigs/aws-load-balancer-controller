package controller

import (
	"log"
	"net/http"
	"os/exec"
	"reflect"

	"github.com/aws/aws-sdk-go/aws"

	"git.tm.tmcs/kubernetes/alb-ingress/pkg/config"
	"k8s.io/ingress/core/pkg/ingress"
	"k8s.io/ingress/core/pkg/ingress/defaults"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
)

type ALBController struct {
	route53svc       *Route53
	elbv2svc         *ALB
	storeLister      ingress.StoreLister
	lastAlbIngresses []*albIngress
}

// NewALBController returns an ALBController
func NewALBController(awsconfig *aws.Config) ingress.Controller {
	alb := ALBController{
		route53svc: newRoute53(awsconfig),
		elbv2svc:   newALB(awsconfig),
	}
	return ingress.Controller(&alb)
}

func (ac *ALBController) OnUpdate(ingressConfiguration ingress.Configuration) ([]byte, error) {
	log.Printf("Received OnUpdate notification")
	var albIngresses []*albIngress

	// TODO: if ac.lastAlbIngresses is empty, try to build it up from AWS resources

	for _, ingress := range ac.storeLister.Ingress.List() {

		// first assemble the albIngress objects
	NEWINGRESSES:
		for _, albIngress := range newAlbIngressesFromIngress(ingress.(*extensions.Ingress), ac) {

			// search for albIngress in ac.lastAlbIngresses, if found and
			// unchanged, continue
			for _, lastIngress := range ac.lastAlbIngresses {
				if reflect.DeepEqual(albIngress, lastIngress) {
					log.Printf("Nothing new with %v", albIngress.ServiceKey())
					continue NEWINGRESSES
				}
			}

			// new/modified ingress, add to albIngresses, execute .Build
			albIngresses = append(albIngresses, albIngress)
			albIngress.Build()
		}
	}

	// TODO: compare albIngresses to ac.lastAlbIngresses, execute .Destroy on
	// any that were removed

	ac.lastAlbIngresses = albIngresses
	return []byte(""), nil
}

func (ac *ALBController) SetConfig(cfgMap *api.ConfigMap) {
	log.Printf("Config map %+v", cfgMap)
}

// SetListers sets the configured store listers in the generic ingress controller
func (ac *ALBController) SetListers(lister ingress.StoreLister) {
	ac.storeLister = lister
}

func (ac *ALBController) Reload(data []byte) ([]byte, bool, error) {
	log.Printf("Reload()")
	out, err := exec.Command("echo", string(data)).CombinedOutput()
	if err != nil {
		log.Printf("Reloaded new config %s", out)
	} else {
		return out, false, err
	}
	return out, true, err
}

func (ac *ALBController) Test(file string) *exec.Cmd {
	log.Printf("Test()")
	return exec.Command("echo", file)
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
