/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/spf13/pflag"

	apiv1 "k8s.io/api/core/v1"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/class"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller"
	ing_net "github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/net"
)

func parseFlags() (bool, *controller.Configuration, error) {
	var (
		flags = pflag.NewFlagSet("", pflag.ExitOnError)

		apiserverHost = flags.String("apiserver-host", "",
			`Address of the Kubernetes API server.
Takes the form "protocol://address:port". If not specified, it is assumed the
program runs inside a Kubernetes cluster and local discovery is attempted.`)

		kubeConfigFile = flags.String("kubeconfig", "",
			`Path to a kubeconfig file containing authorization and API server information.`)

		ingressClass = flags.String("ingress-class", "",
			`Name of the ingress class this controller satisfies.
The class of an Ingress object is set using the annotation "kubernetes.io/ingress.class".
All ingress classes are satisfied if this parameter is left empty.`)

		// configMap = flags.String("configmap", "",
		// 	`Name of the ConfigMap containing custom global configurations for the controller.`)

		resyncPeriod = flags.Duration("sync-period", 30*time.Second,
			`Period at which the controller forces the repopulation of its local object stores.`)

		watchNamespace = flags.String("watch-namespace", apiv1.NamespaceAll,
			`Namespace the controller watches for updates to Kubernetes objects.
This includes Ingresses, Services and all configuration resources. All
namespaces are watched if this parameter is left empty.`)

		profiling = flags.Bool("profiling", true,
			`Enable profiling via web interface host:port/debug/pprof/`)

		defHealthzURL = flags.String("health-check-path", "/healthz",
			`URL path of the health check endpoint.
Configured inside the NGINX status server. All requests received on the port
defined by the healthz-port parameter are forwarded internally to this path.`)

		electionID = flags.String("election-id", "ingress-controller-leader",
			`Election id to use for Ingress status updates.`)

		showVersion = flags.Bool("version", false,
			`Show release information about the AWS ALB Ingress controller and exit.`)

		annotationsPrefix = flags.String("annotations-prefix", "alb.ingress.kubernetes.io",
			`Prefix of the Ingress annotations specific to the AWS ALB controller.`)

		syncRateLimit = flags.Float32("sync-rate-limit", 0.3,
			`Define the sync frequency upper limit`)

		clusterName = flags.String("cluster-name", "",
			`Kubernetes cluster name (required)`)

		albNamePrefix = flags.String("alb-name-prefix", "",
			`Prefix to add to ALB resources (11 alphanumeric characters or less)`)

		healthcheckPeriod = flags.Duration("health-check-period", 1*time.Minute,
			`Period at which the controller executes AWS health checks for its healthz endpoint.`)

		restrictScheme = flags.Bool("restrict-scheme", false,
			`Restrict the scheme to internal except for whitelisted namespaces`)

		restrictSchemeNamespace = flags.String("restrict-scheme-namespace", "default",
			`The namespace with the ConfigMap containing the allowed ingresses. Only respected when restrict-scheme is true.`)

		awsSyncPeriod = flags.Duration("aws-sync-period", 60*time.Minute,
			`Period at which the controller refreshes the state from AWS.`)

		awsAPIMaxRetries = flags.Int("aws-max-retries", 10,
			`Maximum number of times to retry the AWS API.`)

		awsAPIDebug = flags.Bool("aws-api-debug", false,
			`Enable debug logging of AWS API`)
		healthzPort = flags.Int("healthz-port", 10254, "Port to use for the healthz endpoint.")

		_ = flags.String("default-backend-service", "", `No longer used, will be removed in next release`)
	)

	flag.Set("logtostderr", "true")

	flags.AddGoFlagSet(flag.CommandLine)
	flags.Parse(os.Args)

	// Workaround for this issue:
	// https://github.com/kubernetes/kubernetes/issues/17162
	flag.CommandLine.Parse([]string{})

	pflag.VisitAll(func(flag *pflag.Flag) {
		glog.V(2).Infof("FLAG: --%s=%q", flag.Name, flag.Value)
	})

	if *showVersion {
		return true, nil, nil
	}

	if *ingressClass != "" {
		glog.Infof("Watching for Ingress class: %s", *ingressClass)

		if *ingressClass != class.DefaultClass {
			glog.Warningf("Only Ingresses with class %q will be processed by this Ingress controller", *ingressClass)
		}

		class.IngressClass = *ingressClass
	}

	parser.AnnotationsPrefix = *annotationsPrefix

	// check port collisions
	if !ing_net.IsPortAvailable(*healthzPort) {
		return false, nil, fmt.Errorf("Port %v is already in use. Please check the flag --healthz-port", *healthzPort)
	}

	// Deal with legacy environment variable configuration options
	if s, ok := os.LookupEnv("CLUSTER_NAME"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --cluster-name flag.")
		clusterName = &s
	}
	if s, ok := os.LookupEnv("ALB_PREFIX"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --alb-name-prefix flag.")
		albNamePrefix = &s
	}
	if s, ok := os.LookupEnv("ALB_CONTROLLER_RESTRICT_SCHEME"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --restrict-scheme flag.")
		v, err := strconv.ParseBool(s)
		if err != nil {
			return false, nil, fmt.Errorf("ALB_CONTROLLER_RESTRICT_SCHEME environment variable must be either true or false. Value was: %s", s)
		}
		restrictScheme = &v
	}
	if s, ok := os.LookupEnv("ALB_CONTROLLER_RESTRICT_SCHEME_CONFIG_NAMESPACE"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --restrict-scheme-namespace flag.")
		restrictSchemeNamespace = &s
	}
	if s, ok := os.LookupEnv("ALB_SYNC_INTERVAL"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --aws-resync-period flag.")
		v, err := time.ParseDuration(s)
		if err != nil {
			return false, nil, fmt.Errorf("Failed to parse duration from ALB_SYNC_INTERVAL value of '%s'", s)
		}
		awsSyncPeriod = &v
	}
	if s, ok := os.LookupEnv("AWS_MAX_RETRIES"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --aws-max-retries flag.")
		v, err := strconv.ParseInt(s, 0, 32)
		if err != nil {
			return false, nil, fmt.Errorf("AWS_MAX_RETRIES environment variable must be an integer. Value was: %s", s)
		}
		i := int(v)
		awsAPIMaxRetries = &i
	}

	config := &controller.Configuration{
		ClusterName:             *clusterName,
		ALBNamePrefix:           *albNamePrefix,
		RestrictScheme:          *restrictScheme,
		RestrictSchemeNamespace: *restrictSchemeNamespace,
		AWSSyncPeriod:           *awsSyncPeriod,
		AWSAPIMaxRetries:        *awsAPIMaxRetries,
		AWSAPIDebug:             *awsAPIDebug,
		HealthCheckPeriod:       *healthcheckPeriod,

		APIServerHost:   *apiserverHost,
		KubeConfigFile:  *kubeConfigFile,
		ElectionID:      *electionID,
		EnableProfiling: *profiling,
		ResyncPeriod:    *resyncPeriod,
		Namespace:       *watchNamespace,
		// ConfigMapName:           *configMap,
		DefaultHealthzURL: *defHealthzURL,
		SyncRateLimit:     *syncRateLimit,
		HealthzPort:       *healthzPort,
	}

	return false, config, nil
}
