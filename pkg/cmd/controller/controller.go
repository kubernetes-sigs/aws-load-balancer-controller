package controller

import (
	"log"
	"net/http"
	"os/exec"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/davecgh/go-spew/spew"

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
	lastAlbIngresses []albIngress
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
	var albIngresses []albIngress

	for _, ingress := range ac.storeLister.Ingress.List() {
		// first assemble the albIngress object

		// search for albIngress in ac.lastAlbIngresses, if found and
		// unchanged, continue on

		// new/modified ingress, add to albIngresses, execute .Build

		// compare albIngresses to ac.lastAlbIngresses, execute .Destroy on
		// any that were removed
		spew.Dump(ingress.(*extensions.Ingress).Spec)
	}

	// // Build up list of albIngress's
	// for _, b := range ingressConfiguration.Servers {
	// 	// default backend shows up with a _ hostname, even when there isn't
	// 	// an ingress defined
	// 	if b.Hostname == "_" {
	// 		continue
	// 	}

	// 	// AFAIK ALB only supports one backend pool or something
	// 	item, exists, _ := ac.storeLister.Service.Indexer.GetByKey(b.Locations[0].Backend)
	// 	if !exists {
	// 		log.Printf("Unable to find %v in services", b.Locations[0].Backend)
	// 		continue
	// 	}

	// 	service := item.(*api.Service)

	// 	// should be able to get the nodes here and slam together with nodeport
	// 	// to build up albIngress.targets
	// 	for _, port := range service.Spec.Ports {
	// 		log.Printf("%v: %v", b.Hostname, b.Locations[0].Backend, port.NodePort)

	// 	}

	// 	ingresses = append(ingresses, albIngress{server: b})
	// }

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
