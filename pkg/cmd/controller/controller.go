package controller

import (
	"log"
	"net/http"
	"os/exec"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"

	"git.tm.tmcs/kubernetes/alb-ingress/pkg/config"
	"k8s.io/ingress/core/pkg/ingress"
	"k8s.io/ingress/core/pkg/ingress/defaults"
	"k8s.io/kubernetes/pkg/api"
)

type ALBController struct {
	route53svc        *route53.Route53
	elbv2svc          *elbv2.ELBV2
	lastUpdatePayload *ingress.Configuration
}

// NewALBController returns an ALBController
func NewALBController(awsconfig *aws.Config) ingress.Controller {
	alb := ALBController{
		route53svc: route53.New(session.New(awsconfig)),
		elbv2svc:   elbv2.New(session.New(awsconfig)),
	}
	return ingress.Controller(&alb)
}

func (ac *ALBController) SetConfig(cfgMap *api.ConfigMap) {
	log.Printf("Config map %+v", cfgMap)
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

func (ac *ALBController) OnUpdate(updatePayload ingress.Configuration) ([]byte, error) {
	log.Printf("Received OnUpdate notification")

	// We may want something smarter here, like iterate over a list of ingresses
	// and do a DeepEqual, just in case we dont want to hit the AWS APIs for
	// all ingresses every time one changes
	if reflect.DeepEqual(ac.lastUpdatePayload, &updatePayload) {
		log.Printf("Nothing new!!")
		return []byte(``), nil
	}

	ac.lastUpdatePayload = &updatePayload

	// Prints backends
	for _, b := range updatePayload.Backends {
		eps := []string{}
		for _, e := range b.Endpoints {
			eps = append(eps, e.Address)
		}
		log.Printf("%v: %v", b.Name, strings.Join(eps, ", "))
	}

	// Prints servers
	for _, b := range updatePayload.Servers {
		log.Printf("%v", b.Hostname)
	}

	// Really dont think we will ever use these bytes since we are not writing
	// configuration files. there are a lot of assumptions in the core ingress
	return []byte(`<string containing a configuration file>`), nil
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
