package main

import (
	"log"

	"github.com/aws/aws-sdk-go/aws"

	"git.tm.tmcs/kubernetes/alb-ingress/pkg/cmd/controller"
	ingresscontroller "k8s.io/ingress/core/pkg/ingress/controller"
)

func main() {
	// FIX: hard coded us-east-1, read it from a configmap?
	ac := controller.NewALBController(&aws.Config{Region: aws.String(`us-east-1`)})
	ic := ingresscontroller.NewIngressController(ac)

	defer func() {
		log.Printf("Shutting down ingress controller...")
		ic.Stop()
	}()
	ic.Start()
}
