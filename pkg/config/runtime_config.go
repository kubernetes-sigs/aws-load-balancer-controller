package config

import (
	"crypto/tls"
	"crypto/x509"
	"github.com/pkg/errors"
	"os"
	"time"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
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
	flagWebhookCertDir          = "webhook-cert-dir"
	flagWebhookCertName         = "webhook-cert-file"
	flagWebhookKeyName          = "webhook-key-file"
	flagKubernetesCaPemFilepath = "kube-ca-pem-filepath"

	defaultKubeconfig              = ""
	defaultLeaderElectionID        = "aws-load-balancer-controller-leader"
	defaultLeaderElectionNamespace = ""
	defaultWatchNamespace          = corev1.NamespaceAll
	defaultMetricsAddr             = ":8080"
	defaultHealthProbeBindAddress  = ":61779"
	defaultSyncPeriod              = 10 * time.Hour
	defaultWebhookBindPort         = 9443
	// High enough QPS to fit all expected use cases. QPS=0 is not set here, because
	// client code is overriding it.
	defaultQPS = 1e6
	// High enough Burst to fit all expected use cases. Burst=0 is not set here, because
	// client code is overriding it.
	defaultBurst           = 1e6
	defaultWebhookCertDir  = ""
	defaultWebhookCertName = ""
	defaultWebhookKeyName  = ""
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
	WebhookCertDir          string
	WebhookCertName         string
	WebhookKeyName          string
	KubernetesCaPemFilePath string
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
	fs.StringVar(&c.WebhookCertDir, flagWebhookCertDir, defaultWebhookCertDir, "WebhookCertDir is the directory that contains the webhook server key and certificate.")
	fs.StringVar(&c.WebhookCertName, flagWebhookCertName, defaultWebhookCertName, "WebhookCertName is the webhook server certificate name.")
	fs.StringVar(&c.WebhookKeyName, flagWebhookKeyName, defaultWebhookKeyName, "WebhookKeyName is the webhook server key name.")
	fs.StringVar(&c.KubernetesCaPemFilePath, flagKubernetesCaPemFilepath, "", "Location of Kubernetes CA file on disk.")

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

	restCFG.AcceptContentTypes = "application/vnd.kubernetes.protobuf,application/json"
	restCFG.QPS = defaultQPS
	restCFG.Burst = defaultBurst
	return restCFG, nil
}

// BuildRuntimeOptions builds the options for the controller runtime based on config
func BuildRuntimeOptions(rtCfg RuntimeConfig, scheme *runtime.Scheme) (ctrl.Options, error) {
	baseOpts := []func(config *tls.Config){
		func(config *tls.Config) {
			config.MinVersion = tls.VersionTLS12
			config.CipherSuites = []uint16{
				// AEADs w/ ECDHE
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305, tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,

				// AEADs w/o ECDHE
				tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			}
		},
	}

	if rtCfg.KubernetesCaPemFilePath != "" {
		caCertPool := x509.NewCertPool()
		data, err := os.ReadFile(rtCfg.KubernetesCaPemFilePath)
		if err != nil {
			return ctrl.Options{}, err
		}
		if !caCertPool.AppendCertsFromPEM(data) {
			return ctrl.Options{}, errors.Errorf("Unable to append CA PEM to pool")
		}
		// This ensures that only the CA configured in the LBC options is allowed to invoke the webhook.
		baseOpts = append(baseOpts, func(config *tls.Config) {
			config.ClientCAs = caCertPool
			config.ClientAuth = tls.RequireAndVerifyClientCert
		})
	}

	opt := ctrl.Options{
		Scheme:                     scheme,
		HealthProbeBindAddress:     rtCfg.HealthProbeBindAddress,
		LeaderElection:             rtCfg.EnableLeaderElection,
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		LeaderElectionID:           rtCfg.LeaderElectionID,
		LeaderElectionNamespace:    rtCfg.LeaderElectionNamespace,
		Cache: cache.Options{
			SyncPeriod: &rtCfg.SyncPeriod,
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{&corev1.Secret{}},
			},
		},
		Metrics: server.Options{
			BindAddress: rtCfg.MetricsBindAddress,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:     rtCfg.WebhookBindPort,
			CertDir:  rtCfg.WebhookCertDir,
			CertName: rtCfg.WebhookCertName,
			KeyName:  rtCfg.WebhookKeyName,
			TLSOpts:  baseOpts,
		}),
	}

	// cannot set DefaultNamespaces = corev1.NamespaceAll
	// https://github.com/kubernetes-sigs/controller-runtime/issues/2628
	if rtCfg.WatchNamespace != corev1.NamespaceAll {
		opt.Cache.DefaultNamespaces = map[string]cache.Config{
			rtCfg.WatchNamespace: {},
		}
	}

	return opt, nil
}
