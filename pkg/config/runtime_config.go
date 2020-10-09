package config

import (
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"time"
)

const (
	flagMetricsAddr             = "metrics-addr"
	flagEnableLeaderElection    = "enable-leader-election"
	flagLeaderElectionID        = "leader-election-id"
	flagLeaderElectionNamespace = "leader-election-namespace"
	flagWatchNamespace          = "watch-namespace"
	flagSyncPeriod              = "sync-period"
	flagMaster                  = "master"
	flagKubeconfig              = "kubeconfig"

	defaultMaster                  = ""
	defaultKubeconfig              = ""
	defaultLeaderElectionID        = "aws-load-balancer-controller-leader"
	defaultLeaderElectionNamespace = ""
	defaultWatchNamespace          = corev1.NamespaceAll
	defaultControllerPort          = 9443
	defaultMetricsAddr             = ":8080"
	defaultSyncPeriod              = 60 * time.Minute
)

// NewRuntimeConfig constructs a new RuntimeConfig object
func NewRuntimeConfig(scheme *runtime.Scheme) RuntimeConfig {
	return RuntimeConfig{
		Scheme:         scheme,
		ControllerPort: defaultControllerPort,
	}
}

type RuntimeConfig struct {
	Scheme                  *runtime.Scheme
	APIServer               string
	KubeConfig              string
	ControllerPort          int
	MetricsBindAddress      string
	EnableLeaderElection    bool
	LeaderElectionID        string
	LeaderElectionNamespace string
	WatchNamespace          string
	SyncPeriod              time.Duration
}

func (c *RuntimeConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.APIServer, flagMaster, defaultMaster,
		"The address of the Kubernetes API server.")
	fs.StringVar(&c.KubeConfig, flagKubeconfig, defaultKubeconfig,
		"Path to the kubeconfig file containing authorization and API server information.")
	fs.StringVar(&c.MetricsBindAddress, flagMetricsAddr, defaultMetricsAddr,
		"The address the metric endpoint binds to.")
	fs.BoolVar(&c.EnableLeaderElection, flagEnableLeaderElection, true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	fs.StringVar(&c.LeaderElectionID, flagLeaderElectionID, defaultLeaderElectionID,
		"Name of the leader election ID to use for this controller")
	fs.StringVar(&c.LeaderElectionNamespace, flagLeaderElectionNamespace, defaultLeaderElectionNamespace,
		"Name of the leader election ID to use for this controller")
	fs.StringVar(&c.WatchNamespace, flagWatchNamespace, defaultWatchNamespace,
		"Namespace the controller watches for updates to Kubernetes objects, If empty, all namespaces are watched.")
	fs.DurationVar(&c.SyncPeriod, flagSyncPeriod, defaultSyncPeriod,
		"Period at which the controller forces the repopulation of its local object stores.")
}
