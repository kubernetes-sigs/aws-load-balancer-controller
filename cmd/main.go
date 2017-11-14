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

	awsDebug, _ := strconv.ParseBool(os.Getenv("AWS_DEBUG"))

	conf := &config.Config{
		AWSDebug: awsDebug,
	}

	port := "8080"
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(fmt.Sprintf(":%s", port), nil)

	ac := controller.NewALBController(&aws.Config{MaxRetries: aws.Int(20)}, conf)
	ic := ingresscontroller.NewIngressController(ac)

	ac.Configure(ic)

	http.HandleFunc("/state", ac.StateHandler)
	http.HandleFunc("/healthz", ac.StatusHandler)

	defer func() {
		logger.Infof("Shutting down ingress controller...")
		ic.Stop()
	}()

	ic.Start()
}
