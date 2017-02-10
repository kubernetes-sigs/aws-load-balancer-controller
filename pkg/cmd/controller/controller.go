package controller

import (
	"log"
	"net/http"
	"os/exec"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/davecgh/go-spew/spew"

	"git.tm.tmcs/kubernetes/alb-ingress/pkg/config"
	"k8s.io/ingress/core/pkg/ingress"
	"k8s.io/ingress/core/pkg/ingress/defaults"
	"k8s.io/kubernetes/pkg/api"
)

type ALBController struct {
	route53svc               *Route53
	elbv2svc                 *ALB
	storeLister              ingress.StoreLister
	lastIngressConfiguration *ingress.Configuration
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

	item, exists, _ := ac.storeLister.Service.Indexer.GetByKey("2048-game/service-2048")
	if exists {
		spew.Dump(item.(*api.Service).Spec.Ports)
	}

	// if ac.lastIngressConfiguration == nil, we should do some init
	// like looking for existing ALB & R53 with our tag

	// We may want something smarter here, like iterate over a list of ingresses
	// and do a DeepEqual, just in case we dont want to hit the AWS APIs for
	// all ingresses every time one changes
	if reflect.DeepEqual(ac.lastIngressConfiguration, &ingressConfiguration) {
		log.Printf("Nothing new!!")
		return []byte(``), nil
	}

	ac.lastIngressConfiguration = &ingressConfiguration

	// spew.Dump(ingressConfiguration)
	// Prints backends
	for _, b := range ingressConfiguration.Backends {
		eps := []string{}
		for _, e := range b.Endpoints {
			eps = append(eps, e.Address)
		}
		log.Printf("%v: %v", b.Name, strings.Join(eps, ", "))
	}

	// Prints servers
	for _, b := range ingressConfiguration.Servers {
		log.Printf("%v", b.Hostname)
	}

	//  ac.addR53Record()
	// Really dont think we will ever use these bytes since we are not writing
	// configuration files. there are a lot of assumptions in the core ingress
	return []byte(`<string containing a configuration file>`), nil
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
