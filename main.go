package main

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/glog"

	"git.tm.tmcs/kubernetes/alb-ingress/pkg/cmd/controller"
	ingresscontroller "k8s.io/ingress/core/pkg/ingress/controller"
)

func main() {
	ac := controller.NewALBController(&aws.Config{})
	ic := ingresscontroller.NewIngressController(ac)

	defer func() {
		glog.Infof("Shutting down ingress controller...")
		ic.Stop()
	}()
	ic.Start()
}
