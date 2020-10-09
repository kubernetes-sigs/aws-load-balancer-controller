package runtime

import (
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"
)

const (
	flagMetricsAddr             = "metrics-addr"
	flagEnableLeaderElection    = "enable-leader-election"
	flagLeaderElectionID        = "leader-election-id"
	flagLeaderElectionNamespace = "leader-election-namespace"
	flagWatchNamespace          = "watch-namespace"
	flagSyncPeriod              = "sync-period"

	defaultLeaderElectionID        = "aws-load-balancer-controller-leader"
	defaultLeaderElectionNamespace = ""
	defaultWatchNamespace          = corev1.NamespaceAll
	defaultControllerPort          = 9443
	defaultMetricsAddr             = ":8080"
	defaultSyncPeriod              = 60 * time.Minute
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = elbv2api.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

// Config interface for controller runtime configuration
type Config interface {
	// BindFlags to bind to the command line flags
	BindFlags(fs *pflag.FlagSet)
	// Return the rest config
	GetRestConfig() *rest.Config
	// Return controller-runtine Options
	GetRuntimeOptions() ctrl.Options
}

// NewConfig constructs a new Config object
func NewConfig() Config {
	return &defaultConfig{
		scheme:         scheme,
		controllerPort: defaultControllerPort,
	}
}

type defaultConfig struct {
	scheme                  *runtime.Scheme
	controllerPort          int
	metricsBindAddress      string
	enableLeaderElection    bool
	leaderElectionID        string
	leaderElectionNamespace string
	watchNamespace          string
	syncPeriod              time.Duration
}

func (c *defaultConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.metricsBindAddress, flagMetricsAddr, defaultMetricsAddr,
		"The address the metric endpoint binds to.")
	fs.BoolVar(&c.enableLeaderElection, flagEnableLeaderElection, true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	fs.StringVar(&c.leaderElectionID, flagLeaderElectionID, defaultLeaderElectionID,
		"Name of the leader election ID to use for this controller")
	fs.StringVar(&c.leaderElectionNamespace, flagLeaderElectionNamespace, defaultLeaderElectionNamespace,
		"Name of the leader election ID to use for this controller")
	fs.StringVar(&c.watchNamespace, flagWatchNamespace, defaultWatchNamespace,
		`Namespace the controller watches for updates to Kubernetes objects.
		This includes Ingresses, Services and all configuration resources. All
		namespaces are watched if this parameter is left empty.`)
	fs.DurationVar(&c.syncPeriod, flagSyncPeriod, defaultSyncPeriod,
		"Period at which the controller forces the repopulation of its local object stores.")
}

func (c *defaultConfig) GetRestConfig() *rest.Config {
	return ctrl.GetConfigOrDie()
}

func (c *defaultConfig) GetRuntimeOptions() ctrl.Options {
	return ctrl.Options{
		Scheme:                  c.scheme,
		Port:                    c.controllerPort,
		MetricsBindAddress:      c.metricsBindAddress,
		LeaderElection:          c.enableLeaderElection,
		LeaderElectionID:        c.leaderElectionID,
		LeaderElectionNamespace: c.leaderElectionNamespace,
		Namespace:               c.watchNamespace,
		SyncPeriod:              &c.syncPeriod,
	}
}
