package config

import (
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"
)

const (
	flagMetricsBindAddr         = "metrics-bind-addr"
	flagHealthProbeBindAddr     = "health-probe-bind-addr"
	flagWebhookBindPort         = "webhook-bind-port"
	flagEnableLeaderElection    = "enable-leader-election"
	flagLeaderElectionID        = "leader-election-id"
	flagLeaderElectionNamespace = "leader-election-namespace"
	flagWatchNamespace          = "watch-namespace"
	flagSyncPeriod              = "sync-period"
	flagKubeconfig              = "kubeconfig"

	defaultKubeconfig              = ""
	defaultLeaderElectionID        = "aws-load-balancer-controller-leader"
	defaultLeaderElectionNamespace = ""
	defaultWatchNamespace          = corev1.NamespaceAll
	defaultMetricsAddr             = ":8080"
	defaultHealthProbeBindAddress  = ":61779"
	defaultSyncPeriod              = 60 * time.Minute
	defaultWebhookBindPort         = 9443
	// High enough QPS to fit all expected use cases. QPS=0 is not set here, because
	// client code is overriding it.
	defaultQPS = 1e6
	// High enough Burst to fit all expected use cases. Burst=0 is not set here, because
	// client code is overriding it.
	defaultBurst = 1e6
)

// RuntimeConfig stores the configuration for the controller-runtime
type RuntimeConfig struct {
	APIServer               string
	KubeConfig              string
	WebhookBindPort         int
	MetricsBindAddress      string
	HealthProbeBindAddress  string
	EnableLeaderElection    bool
	LeaderElectionID        string
	LeaderElectionNamespace string
	WatchNamespace          string
	SyncPeriod              time.Duration
}

// BindFlags binds the command line flags to the fields in the config object
func (c *RuntimeConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.KubeConfig, flagKubeconfig, defaultKubeconfig,
		"Path to the kubeconfig file containing authorization and API server information.")
	fs.StringVar(&c.MetricsBindAddress, flagMetricsBindAddr, defaultMetricsAddr,
		"The address the metric endpoint binds to.")
	fs.StringVar(&c.HealthProbeBindAddress, flagHealthProbeBindAddr, defaultHealthProbeBindAddress,
		"The address the health probes binds to.")
	fs.IntVar(&c.WebhookBindPort, flagWebhookBindPort, defaultWebhookBindPort,
		"The TCP port the Webhook server binds to.")
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

// BuildRestConfig builds the REST config for the controller runtime
func BuildRestConfig(rtCfg RuntimeConfig) (*rest.Config, error) {
	var restCFG *rest.Config
	var err error
	if rtCfg.KubeConfig == "" {
		restCFG, err = rest.InClusterConfig()
	} else {
		restCFG, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: rtCfg.KubeConfig}, &clientcmd.ConfigOverrides{}).ClientConfig()
	}
	if err != nil {
		return nil, err
	}

	restCFG.QPS = defaultQPS
	restCFG.Burst = defaultBurst
	return restCFG, nil
}

// BuildRuntimeOptions builds the options for the controller runtime based on config
func BuildRuntimeOptions(rtCfg RuntimeConfig, scheme *runtime.Scheme) ctrl.Options {
	return ctrl.Options{
		Scheme:                  scheme,
		Port:                    rtCfg.WebhookBindPort,
		MetricsBindAddress:      rtCfg.MetricsBindAddress,
		HealthProbeBindAddress:  rtCfg.HealthProbeBindAddress,
		LeaderElection:          rtCfg.EnableLeaderElection,
		LeaderElectionID:        rtCfg.LeaderElectionID,
		LeaderElectionNamespace: rtCfg.LeaderElectionNamespace,
		Namespace:               rtCfg.WatchNamespace,
		SyncPeriod:              &rtCfg.SyncPeriod,
	}
}
