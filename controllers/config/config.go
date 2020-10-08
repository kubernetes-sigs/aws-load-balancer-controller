package config

import (
	"fmt"
	"github.com/spf13/pflag"
)

const (
	flagK8sClusterName                            = "cluster-name"
	flagIngressClass                              = "ingress-class"
	flagIngressMaxConcurrentReconciles            = "ingress-max-concurrent-reconciles"
	flagServiceMaxConcurrentReconciles            = "service-max-concurrent-reconciles"
	flagTargetgroupBindingMaxConcurrentReconciles = "targetgroupbinding-max-concurrent-reconciles"

	defaultIngressClass            = "alb"
	defaultMaxConcurrentReconciles = 3
)

// ControllerConfig contains the controller configuration
type ControllerConfig struct {
	// Name of the Kubernetes cluster
	ClusterName string
	// Class of the Ingress objects
	IngressClass string
	// Feature gates configuration
	Features FeatureGate
	// Max concurrent reconcile loops for Ingress objects
	IngressMaxConcurrentReconciles int
	// Max concurrent reconcile loops for Service objects
	ServiceMaxConcurrentReconciles int
	// Max concurrent reconcile loops for TargetGroupBinding objects
	TargetgroupBindingMaxConcurrentReconciles int
}

// NewControllerConfig constructs a new ControllerConfig object
func NewControllerConfig() ControllerConfig {
	return ControllerConfig{
		Features: NewFeatureGate(),
	}
}

// BindFlags binds the command line flags to the fields in the config object
func (cfg *ControllerConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.ClusterName, flagK8sClusterName, "", "Kubernetes cluster name")
	fs.StringVar(&cfg.IngressClass, flagIngressClass, defaultIngressClass, "Name of the ingress class this controller satisfies")
	fs.IntVar(&cfg.IngressMaxConcurrentReconciles, flagIngressMaxConcurrentReconciles, defaultMaxConcurrentReconciles,
		"Maximum number of concurrently running reconcile loops for ingress")
	fs.IntVar(&cfg.ServiceMaxConcurrentReconciles, flagServiceMaxConcurrentReconciles, defaultMaxConcurrentReconciles,
		"Maximum number of concurrently running reconcile loops for service")
	fs.IntVar(&cfg.TargetgroupBindingMaxConcurrentReconciles, flagTargetgroupBindingMaxConcurrentReconciles, defaultMaxConcurrentReconciles,
		"Maximum number of concurrently running reconcile loops for targetgroup binding")
	cfg.Features.BindFlags(fs)
}

// Validate the controller configuration
func (cfg *ControllerConfig) Validate() error {
	if len(cfg.ClusterName) == 0 {
		return fmt.Errorf("Kubernetes cluster name must be specified")
	}
	return nil
}
