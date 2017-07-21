package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/coreos/alb-ingress-controller/controller"
	"github.com/coreos/alb-ingress-controller/controller/config"
	"github.com/coreos/alb-ingress-controller/log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ingresscontroller "k8s.io/ingress/core/pkg/ingress/controller"
)

func main() {
	flag.Set("logtostderr", "true")
	flag.CommandLine.Parse([]string{})

	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		glog.Exit("A CLUSTER_NAME environment variable must be defined")
	}

	logLevel := os.Getenv("LOG_LEVEL")
	log.SetLogLevel(logLevel)

	awsDebug, _ := strconv.ParseBool(os.Getenv("AWS_DEBUG"))

	disableRoute53, _ := strconv.ParseBool(os.Getenv("DISABLE_ROUTE53"))

	conf := &config.Config{
		ClusterName:    clusterName,
		AWSDebug:       awsDebug,
		DisableRoute53: disableRoute53,
	}

	if len(clusterName) > 11 {
		glog.Exit("CLUSTER_NAME must be 11 characters or less")
	}

	port := "8080"
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(fmt.Sprintf(":%s", port), nil)

	apiServerHost := os.Getenv("KUBERNETES_APISERVER_HOST")
	var config *rest.Config
	var err error
	// Create kubeclient
	if len(apiServerHost) == 0 {
		if config, err = rest.InClusterConfig(); err != nil {
			glog.Fatalf("error creating client configuration: %v", err)
		}
	} else {
		kubeConfigFile := os.Getenv("KUBECONFIG_FILE")
		if len(kubeConfigFile) == 0 {
			glog.Fatalf("env 'KUBECONFIG_FILE' should be specified with 'KUBERNETES_APISERVER_HOST'")
		}
		config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfigFile},
			&clientcmd.ConfigOverrides{
				ClusterInfo: clientcmdapi.Cluster{
					Server: apiServerHost,
				},
			}).ClientConfig()
		if err != nil {
			glog.Fatalf("error creating client configuration: %v", err)
		}
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Failed to create client: %v.", err)
	}

	ac := controller.NewALBController(&aws.Config{MaxRetries: aws.Int(5)}, conf, kubeClient)
	ic := ingresscontroller.NewIngressController(ac)

	ac.IngressClass = ic.IngressClass()
	if ac.IngressClass != "" {
		log.Infof("Ingress class set to %s", "controller", ac.IngressClass)
	}

	http.HandleFunc("/state", ac.StateHandler)

	defer func() {
		glog.Infof("Shutting down ingress controller...")
		ic.Stop()
	}()
	ic.Start()
}
