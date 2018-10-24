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
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"hash/crc32"
	"os"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/spf13/pflag"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	ing_net "github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/net"
	apiv1 "k8s.io/api/core/v1"
)

const (
	defaultLeaderElection          = true
	defaultLeaderElectionID        = "ingress-controller-leader-alb"
	defaultLeaderElectionNamespace = ""
	defaultWatchNamespace          = apiv1.NamespaceAll
	defaultSyncPeriod              = 30 * time.Second
	defaultHealthCheckPeriod       = 1 * time.Minute
	defaultHealthzPort             = 10254
	defaultAWSAPIMaxRetries        = 10
	defaultAWSAPIDebug             = false
	defaultProfilingEnabled        = true
)

// Options defines the commandline interface of this binary
type Options struct {
	ShowVersion bool

	APIServerHost  string
	KubeConfigFile string

	LeaderElection          bool
	LeaderElectionID        string
	LeaderElectionNamespace string

	WatchNamespace    string
	SyncPeriod        time.Duration
	HealthCheckPeriod time.Duration
	HealthzPort       int

	AWSAPIMaxRetries int
	AWSAPIDebug      bool
	ProfilingEnabled bool

	config config.Configuration
}

func getOptions() (*Options, error) {
	options := &Options{}

	flags := pflag.NewFlagSet("", pflag.ExitOnError)
	flags.BoolVar(&options.ShowVersion, "version", false,
		`Show release information about the AWS ALB Ingress controller and exit.`)
	flags.StringVar(&options.APIServerHost, "apiserver-host", "",
		`Address of the Kubernetes API server.
		Takes the form "protocol://address:port". If not specified, it is assumed the
		program runs inside a Kubernetes cluster and local discovery is attempted.`)
	flags.StringVar(&options.KubeConfigFile, "kubeconfig", "",
		`Path to a kubeconfig file containing authorization and API server information.`)
	flags.BoolVar(&options.LeaderElection, "election", defaultLeaderElection,
		`Whether we do leader election for ingress controller`)
	flags.StringVar(&options.LeaderElectionID, "election-id", defaultLeaderElectionID,
		`Namespace of leader-election configmap for ingress controller`)
	flags.StringVar(&options.LeaderElectionNamespace, "election-namespace", defaultLeaderElectionNamespace,
		`Namespace of leader-election configmap for ingress controller. If unspecified, the namespace of this controller pod will be used`)
	flags.StringVar(&options.WatchNamespace, "watch-namespace", defaultWatchNamespace,
		`Namespace the controller watches for updates to Kubernetes objects.
		This includes Ingresses, Services and all configuration resources. All
		namespaces are watched if this parameter is left empty.`)
	flags.DurationVar(&options.SyncPeriod, "sync-period", defaultSyncPeriod,
		`Period at which the controller forces the repopulation of its local object stores.`)
	flags.DurationVar(&options.HealthCheckPeriod, "health-check-period", defaultHealthCheckPeriod,
		`Period at which the controller executes AWS health checks for its healthz endpoint.`)
	flags.IntVar(&options.HealthzPort, "healthz-port", defaultHealthzPort,
		`Port to use for the healthz endpoint.`)
	flags.IntVar(&options.AWSAPIMaxRetries, "aws-max-retries", defaultAWSAPIMaxRetries,
		`Maximum number of times to retry the AWS API.`)
	flags.BoolVar(&options.AWSAPIDebug, "aws-api-debug", defaultAWSAPIDebug,
		`Enable debug logging of AWS API`)
	flags.BoolVar(&options.ProfilingEnabled, "profiling", defaultProfilingEnabled,
		`Enable profiling via web interface host:port/debug/pprof/`)
	options.config.BindFlags(flags)

	flags.MarkDeprecated("aws-sync-period", `No longer used, will be removed in next release`)
	flags.MarkDeprecated("default-backend-service", `No longer used, will be removed in next release`)

	flag.Set("logtostderr", "true")
	flags.AddGoFlagSet(flag.CommandLine)
	flags.Parse(os.Args)

	err := configOptionsByEnvironmentVariables(options)
	if err != nil {
		return options, err
	}
	err = validateOptions(options)

	// TODO: I know, bad smell here:D
	parser.AnnotationsPrefix = options.config.AnnotationPrefix
	return options, err
}

// configOptionsByEnvironmentVariables deals with the legacy way of configuration by environment variables
// TODO: completely remove this legacy configuration when we feels comfortable
func configOptionsByEnvironmentVariables(options *Options) error {
	if s, ok := os.LookupEnv("AWS_MAX_RETRIES"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --aws-max-retries flag.")
		v, err := strconv.ParseInt(s, 0, 32)
		if err != nil {
			return fmt.Errorf("AWS_MAX_RETRIES environment variable must be an integer. Value was: %s", s)
		}
		options.AWSAPIMaxRetries = int(v)
	}
	if s, ok := os.LookupEnv("CLUSTER_NAME"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --cluster-name flag.")
		options.config.ClusterName = s
	}
	if s, ok := os.LookupEnv("ALB_PREFIX"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --alb-name-prefix flag.")
		options.config.ALBNamePrefix = s
	}
	if s, ok := os.LookupEnv("ALB_CONTROLLER_RESTRICT_SCHEME"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --restrict-scheme flag.")
		v, err := strconv.ParseBool(s)
		if err != nil {
			return fmt.Errorf("ALB_CONTROLLER_RESTRICT_SCHEME environment variable must be either true or false. Value was: %s", s)
		}
		options.config.RestrictScheme = v
	}
	if s, ok := os.LookupEnv("ALB_CONTROLLER_RESTRICT_SCHEME_CONFIG_NAMESPACE"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --restrict-scheme-namespace flag.")
		options.config.RestrictSchemeNamespace = s
	}
	return nil
}

func validateOptions(options *Options) error {
	if options.config.DefaultTargetType == "pod" {
		glog.Warningf("The target type parameter for 'pod' has changed to 'ip' to better match AWS APIs and documentation.")
		options.config.DefaultTargetType = elbv2.TargetTypeEnumIp
	}
	if options.config.ClusterName == "" {
		return fmt.Errorf("clusterName must be specified")
	}
	if len(options.config.ALBNamePrefix) > 12 {
		return fmt.Errorf("ALBNamePrefix must be 12 characters or less")
	}
	if options.config.ALBNamePrefix == "" {
		options.config.ALBNamePrefix = generateALBNamePrefix(options.config.ClusterName)
	}

	// check port collisions
	if !ing_net.IsPortAvailable(options.HealthzPort) {
		return fmt.Errorf("port %v is alreadt in use. Please check the flag --healthz-port", options.HealthzPort)
	}
	return nil
}

func generateALBNamePrefix(clusterName string) string {
	hash := crc32.New(crc32.MakeTable(0xedb88320))
	hash.Write([]byte(clusterName))
	return hex.EncodeToString(hash.Sum(nil))
}
