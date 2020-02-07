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
	"time"

	"k8s.io/klog"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"

	"github.com/spf13/pflag"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/net"
	apiv1 "k8s.io/api/core/v1"
)

const (
	defaultLeaderElection          = true
	defaultLeaderElectionID        = "ingress-controller-leader-alb"
	defaultLeaderElectionNamespace = ""
	defaultWatchNamespace          = apiv1.NamespaceAll
	defaultSyncPeriod              = 60 * time.Minute
	defaultHealthCheckPeriod       = 1 * time.Minute
	defaultHealthzPort             = 10254
	defaultProfilingEnabled        = true
	defaultEnableSdkCache          = false
	defaultSdkCacheDuration        = 5 * time.Minute
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
	ProfilingEnabled  bool

	// aws cloud specific configuration
	cloudConfig aws.CloudConfig

	// ingress controller specific configuration
	ingressCTLConfig config.Configuration

	// aws sdk cache options
	EnableSdkCache   bool
	SdkCacheDuration time.Duration
}

func (options *Options) BindFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&options.ShowVersion, "version", false,
		`Show release information about the AWS ALB Ingress controller and exit.`)
	fs.StringVar(&options.APIServerHost, "apiserver-host", "",
		`Address of the Kubernetes API server.
		Takes the form "protocol://address:port". If not specified, it is assumed the
		program runs inside a Kubernetes cluster and local discovery is attempted.`)
	fs.StringVar(&options.KubeConfigFile, "kubeconfig", "",
		`Path to a kubeconfig file containing authorization and API server information.`)
	fs.BoolVar(&options.LeaderElection, "election", defaultLeaderElection,
		`Whether we do leader election for ingress controller`)
	fs.StringVar(&options.LeaderElectionID, "election-id", defaultLeaderElectionID,
		`Namespace of leader-election configmap for ingress controller`)
	fs.StringVar(&options.LeaderElectionNamespace, "election-namespace", defaultLeaderElectionNamespace,
		`Namespace of leader-election configmap for ingress controller. If unspecified, the namespace of this controller pod will be used`)
	fs.StringVar(&options.WatchNamespace, "watch-namespace", defaultWatchNamespace,
		`Namespace the controller watches for updates to Kubernetes objects.
		This includes Ingresses, Services and all configuration resources. All
		namespaces are watched if this parameter is left empty.`)
	fs.DurationVar(&options.SyncPeriod, "sync-period", defaultSyncPeriod,
		`Period at which the controller forces the repopulation of its local object stores.`)
	fs.DurationVar(&options.HealthCheckPeriod, "health-check-period", defaultHealthCheckPeriod,
		`Period at which the controller executes AWS health checks for its healthz endpoint.`)
	fs.IntVar(&options.HealthzPort, "healthz-port", defaultHealthzPort,
		`Port to use for the healthz endpoint.`)
	fs.BoolVar(&options.ProfilingEnabled, "profiling", defaultProfilingEnabled,
		`Enable profiling via web interface host:port/debug/pprof/`)
	fs.BoolVar(&options.EnableSdkCache, "aws-cache-enable", defaultEnableSdkCache, "Enables AWS SDK Caching")
	fs.DurationVar(&options.SdkCacheDuration, "aws-cache-duration", defaultSdkCacheDuration, "Duration of AWS SDK Cache entries, default 5m")
	options.cloudConfig.BindFlags(fs)
	options.ingressCTLConfig.BindFlags(fs)

	_ = fs.MarkDeprecated("aws-sync-period", `No longer used, will be removed in next release`)
	_ = fs.MarkDeprecated("default-backend-service", `No longer used, will be removed in next release`)
}

func (options *Options) BindEnv() error {
	if err := options.cloudConfig.BindEnv(); err != nil {
		return err
	}
	if err := options.ingressCTLConfig.BindEnv(); err != nil {
		return err
	}
	return nil
}

func (options *Options) Validate() error {
	if !net.IsPortAvailable(options.HealthzPort) {
		return fmt.Errorf("port %v is already in use. Please check the flag --healthz-port", options.HealthzPort)
	}
	if err := options.ingressCTLConfig.Validate(); err != nil {
		return err
	}
	return nil
}

func getOptions() (*Options, error) {
	options := &Options{
		ingressCTLConfig: config.NewConfiguration(),
	}

	fs := pflag.NewFlagSet("", pflag.ExitOnError)
	options.BindFlags(fs)

	_ = flag.Set("logtostderr", "true")
	fs.AddGoFlagSet(flag.CommandLine)

	klogFs := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFs)
	fs.AddGoFlagSet(klogFs)

	_ = fs.Parse(os.Args)
	if err := options.BindEnv(); err != nil {
		return nil, err
	}
	if err := options.Validate(); err != nil {
		return nil, err
	}

	return options, nil
}
