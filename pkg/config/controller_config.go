package config

import (
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/inject"
)

const (
	flagLogLevel                                  = "log-level"
	flagK8sClusterName                            = "cluster-name"
	flagDefaultTags                               = "default-tags"
	flagServiceMaxConcurrentReconciles            = "service-max-concurrent-reconciles"
	flagTargetGroupBindingMaxConcurrentReconciles = "targetgroupbinding-max-concurrent-reconciles"
	defaultLogLevel                               = "info"
	defaultMaxConcurrentReconciles                = 3
)

// ControllerConfig contains the controller configuration
type ControllerConfig struct {
	// Log level for the controller logs
	LogLevel string
	// Name of the Kubernetes cluster
	ClusterName string
	// Configurations for AWS.
	AWSConfig aws.CloudConfig
	// Configurations for the Controller Runtime
	RuntimeConfig RuntimeConfig
	// Configurations for Pod inject webhook
	PodWebhookConfig inject.Config
	// Configurations for the Ingress controller
	IngressConfig IngressConfig
	// Configurations for Addons feature
	AddonsConfig AddonsConfig

	// Default AWS Tags that will be applied to all AWS resources managed by this controller.
	DefaultTags map[string]string

	// Max concurrent reconcile loops for Service objects
	ServiceMaxConcurrentReconciles int
	// Max concurrent reconcile loops for TargetGroupBinding objects
	TargetGroupBindingMaxConcurrentReconciles int
}

// BindFlags binds the command line flags to the fields in the config object
func (cfg *ControllerConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.LogLevel, flagLogLevel, defaultLogLevel,
		"Set the controller log level - info(default), debug")
	fs.StringVar(&cfg.ClusterName, flagK8sClusterName, "", "Kubernetes cluster name")
	fs.StringToStringVar(&cfg.DefaultTags, flagDefaultTags, nil,
		"Default AWS Tags that will be applied to all AWS resources managed by this controller")
	fs.IntVar(&cfg.ServiceMaxConcurrentReconciles, flagServiceMaxConcurrentReconciles, defaultMaxConcurrentReconciles,
		"Maximum number of concurrently running reconcile loops for service")
	fs.IntVar(&cfg.TargetGroupBindingMaxConcurrentReconciles, flagTargetGroupBindingMaxConcurrentReconciles, defaultMaxConcurrentReconciles,
		"Maximum number of concurrently running reconcile loops for targetGroupBinding")

	cfg.AWSConfig.BindFlags(fs)
	cfg.RuntimeConfig.BindFlags(fs)

	cfg.PodWebhookConfig.BindFlags(fs)
	cfg.IngressConfig.BindFlags(fs)
	cfg.AddonsConfig.BindFlags(fs)
}

// Validate the controller configuration
func (cfg *ControllerConfig) Validate() error {
	if len(cfg.ClusterName) == 0 {
		return errors.New("kubernetes cluster name must be specified")
	}
	return nil
}
