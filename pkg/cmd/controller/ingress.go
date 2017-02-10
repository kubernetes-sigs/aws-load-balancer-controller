package controller

import (
	"log"

	"k8s.io/ingress/core/pkg/ingress"
)

// albIngress contains all information needed to assemble an ALB
type albIngress struct {
	server  *ingress.Server
	targets *ingress.Endpoint
	alb     *ALB
}

func (a *albIngress) albHostname() string {
	return "alb.hostname"
}

func (a *albIngress) Build() error {
	log.Printf("Creating an ALB for %v", a.server.Hostname)
	log.Printf("Creating a Route53 record for %v", a.albHostname())

	return nil
}
