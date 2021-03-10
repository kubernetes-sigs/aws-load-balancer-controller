package config

import "github.com/spf13/pflag"

const (
	flagIngressClass                         = "ingress-class"
	flagDisableIngressClassAnnotation        = "disable-ingress-class-annotation"
	flagDisableIngressGroupNameAnnotation    = "disable-ingress-group-name-annotation"
	flagIngressMaxConcurrentReconciles       = "ingress-max-concurrent-reconciles"
	defaultIngressClass                      = "alb"
	defaultDisableIngressClassAnnotation     = false
	defaultDisableIngressGroupNameAnnotation = false
	defaultMaxIngressConcurrentReconciles    = 3
)

// IngressConfig contains the configurations for the Ingress controller
type IngressConfig struct {
	// Name of the Ingress class this controller satisfies
	// If empty, all Ingresses without ingress.class annotation, or ingress.class==alb get considered
	IngressClass string

	// DisableIngressClassAnnotation specifies whether to disable new usage of kubernetes.io/ingress.class annotation.
	DisableIngressClassAnnotation bool

	// DisableIngressGroupNameAnnotation specifies whether to disable new usage of alb.ingress.kubernetes.io/group.name annotation.
	DisableIngressGroupNameAnnotation bool

	// Max concurrent reconcile loops for Ingress objects
	MaxConcurrentReconciles int
}

// BindFlags binds the command line flags to the fields in the config object
func (cfg *IngressConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.IngressClass, flagIngressClass, defaultIngressClass,
		"Name of the ingress class this controller satisfies")
	fs.BoolVar(&cfg.DisableIngressClassAnnotation, flagDisableIngressClassAnnotation, defaultDisableIngressClassAnnotation,
		"Disable new usage of kubernetes.io/ingress.class annotation")
	fs.BoolVar(&cfg.DisableIngressGroupNameAnnotation, flagDisableIngressGroupNameAnnotation, defaultDisableIngressGroupNameAnnotation,
		"Disable new usage of alb.ingress.kubernetes.io/group.name annotation")
	fs.IntVar(&cfg.MaxConcurrentReconciles, flagIngressMaxConcurrentReconciles, defaultMaxIngressConcurrentReconciles,
		"Maximum number of concurrently running reconcile loops for ingress")
}
