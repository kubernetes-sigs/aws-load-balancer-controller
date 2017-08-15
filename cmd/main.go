package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/coreos/alb-ingress-controller/pkg/config"
	"github.com/coreos/alb-ingress-controller/pkg/controller"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	ingresscontroller "k8s.io/ingress/core/pkg/ingress/controller"
)

func main() {
	flag.Set("logtostderr", "true")
	flag.CommandLine.Parse([]string{})

	logLevel := os.Getenv("LOG_LEVEL")
	log.SetLogLevel(logLevel)

	logger := log.New("main")

	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		logger.Exitf("A CLUSTER_NAME environment variable must be defined")
	}

	awsDebug, _ := strconv.ParseBool(os.Getenv("AWS_DEBUG"))

	disableRoute53, _ := strconv.ParseBool(os.Getenv("DISABLE_ROUTE53"))

	conf := &config.Config{
		ClusterName:    clusterName,
		AWSDebug:       awsDebug,
		DisableRoute53: disableRoute53,
	}

	if len(clusterName) > 11 {
		logger.Exitf("CLUSTER_NAME must be 11 characters or less")
	}

	port := "8080"
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(fmt.Sprintf(":%s", port), nil)

	ac := controller.NewALBController(&aws.Config{MaxRetries: aws.Int(20)}, conf)
	ic := ingresscontroller.NewIngressController(ac)

	ac.Configure(ic)

	http.HandleFunc("/state", ac.StateHandler)

	defer func() {
		logger.Infof("Shutting down ingress controller...")
		ic.Stop()
	}()

	ac.AssembleIngresses()
	ic.Start()
}
