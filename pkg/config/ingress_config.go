package config

import "github.com/spf13/pflag"

const (
	flagIngressClass                      = "ingress-class"
	flagIngressMaxConcurrentReconciles    = "ingress-max-concurrent-reconciles"
	defaultIngressClass                   = ""
	defaultMaxIngressConcurrentReconciles = 3
)

// IngressConfig contains the configurations for the Ingress controller
type IngressConfig struct {
	// Name of the Ingress class this controller satisfies
	// If empty, all Ingresses without ingress.class annotation, or ingress.class==alb get considered
	IngressClass string

	// Max concurrent reconcile loops for Ingress objects
	MaxConcurrentReconciles int
}

// BindFlags binds the command line flags to the fields in the config object
func (cfg *IngressConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.IngressClass, flagIngressClass, defaultIngressClass,
		"Name of the ingress class this controller satisfies")
	fs.IntVar(&cfg.MaxConcurrentReconciles, flagIngressMaxConcurrentReconciles, defaultMaxIngressConcurrentReconciles,
		"Maximum number of concurrently running reconcile loops for ingress")
}
