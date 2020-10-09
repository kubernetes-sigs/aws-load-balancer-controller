package config

import (
	"fmt"
	"github.com/spf13/pflag"
)

const (
	flagLogLevel                                  = "log-level"
	flagK8sClusterName                            = "cluster-name"
	flagServiceMaxConcurrentReconciles            = "service-max-concurrent-reconciles"
	flagTargetgroupBindingMaxConcurrentReconciles = "targetgroupbinding-max-concurrent-reconciles"
	defaultLogLevel                               = "info"
	defaultMaxConcurrentReconciles                = 3
)

// ControllerConfig contains the controller configuration
type ControllerConfig struct {
	// Log level for the controller logs
	LogLevel string
	// Name of the Kubernetes cluster
	ClusterName string
	// Configurations for the Ingress controller
	IngressConfig IngressConfig
	// Configurations for Addons feature
	AddonsConfig AddonsConfig
	// Configurations for the Controller Runtime
	RuntimeConfig RuntimeConfig
	// Max concurrent reconcile loops for Service objects
	ServiceMaxConcurrentReconciles int
	// Max concurrent reconcile loops for TargetGroupBinding objects
	TargetgroupBindingMaxConcurrentReconciles int
}

// BindFlags binds the command line flags to the fields in the config object
func (cfg *ControllerConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.LogLevel, flagLogLevel, defaultLogLevel,
		"Set the controller log level - info(default), debug")
	fs.StringVar(&cfg.ClusterName, flagK8sClusterName, "", "Kubernetes cluster name")
	fs.IntVar(&cfg.ServiceMaxConcurrentReconciles, flagServiceMaxConcurrentReconciles, defaultMaxConcurrentReconciles,
		"Maximum number of concurrently running reconcile loops for service")
	fs.IntVar(&cfg.TargetgroupBindingMaxConcurrentReconciles, flagTargetgroupBindingMaxConcurrentReconciles, defaultMaxConcurrentReconciles,
		"Maximum number of concurrently running reconcile loops for targetgroup binding")

	cfg.IngressConfig.BindFlags(fs)
	cfg.AddonsConfig.BindFlags(fs)
	cfg.RuntimeConfig.BindFlags(fs)
}

// Validate the controller configuration
func (cfg *ControllerConfig) Validate() error {
	if len(cfg.ClusterName) == 0 {
		return fmt.Errorf("Kubernetes cluster name must be specified")
	}
	return nil
}
