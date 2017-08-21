package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

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
		AWSDebug: awsDebug}

	disableRoute53, _ := strconv.ParseBool(os.Getenv("DISABLE_ROUTE53"))

	albSyncParam := os.Getenv("ALB_SYNC_INTERVAL")
	if albSyncParam == "" {
		albSyncParam = "3m"
	}
	albSyncInterval, err := time.ParseDuration(albSyncParam)
	if err != nil {
		log.Exitf("Failed to parse duration from ALB_SYNC_INTERVAL value of '%s'", albSyncParam)
	}

	conf := &config.Config{
		ClusterName:     clusterName,
		AWSDebug:        awsDebug,
		DisableRoute53:  disableRoute53,
		ALBSyncInterval: albSyncInterval,
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

	ic.Start()
}
