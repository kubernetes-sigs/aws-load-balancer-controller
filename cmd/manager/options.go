package main

import (
	"flag"
	"github.com/spf13/pflag"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/klog"
	"os"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/ingress"
	"time"
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
	cloudConfig cloud.Config

	// ingress specific configuration
	ingressConfig ingress.Config
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
	options.cloudConfig.BindFlags(fs)
	options.ingressConfig.BindFlags(fs)

	_ = fs.MarkDeprecated("aws-sync-period", `No longer used, will be removed in next release`)
	_ = fs.MarkDeprecated("default-backend-service", `No longer used, will be removed in next release`)
}

func (options *Options) BindEnv() error {
	if err := options.cloudConfig.BindEnv(); err != nil {
		return err
	}
	if err := options.ingressConfig.BindEnv(); err != nil {
		return err
	}
	return nil
}

func (options *Options) Validate() error {
	if err := options.cloudConfig.Validate(); err != nil {
		return err
	}
	if err := options.ingressConfig.Validate(); err != nil {
		return err
	}
	return nil
}

var shit string

func getOptions() (*Options, error) {
	options := &Options{
		ingressConfig: ingress.NewConfig(),
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
