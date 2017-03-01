package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"git.tmaws.io/kubernetes/alb-ingress/pkg/cmd/controller"
	ingresscontroller "k8s.io/ingress/core/pkg/ingress/controller"
)

func main() {
	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		glog.Exit("A CLUSTER_NAME environment variable must be defined")
	}

	if len(clusterName) > 11 {
		glog.Exit("CLUSTER_NAME must be 11 characters or less")
	}

	ac := controller.NewALBController(&aws.Config{}, clusterName)
	ic := ingresscontroller.NewIngressController(ac)
	http.Handle("/metrics", promhttp.Handler())

	port := os.Getenv("PROMETHEUS_PORT")
	if port == "" {
		port = "8080"
	}

	go http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
	defer func() {
		glog.Infof("Shutting down ingress controller...")
		ic.Stop()
	}()
	ic.Start()
}
