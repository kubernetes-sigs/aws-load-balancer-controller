package config

import "github.com/spf13/pflag"

const (
	flagIngressClass                         = "ingress-class"
	flagDisableIngressClassAnnotation        = "disable-ingress-class-annotation"
	flagDisableIngressGroupNameAnnotation    = "disable-ingress-group-name-annotation"
	flagIngressMaxConcurrentReconciles       = "ingress-max-concurrent-reconciles"
	flagTolerateNonExistentBackendService    = "tolerate-non-existent-backend-service"
	flagTolerateNonExistentBackendAction     = "tolerate-non-existent-backend-action"
	flagAllowedCAArns                        = "allowed-certificate-authority-arns"
	defaultIngressClass                      = "alb"
	defaultDisableIngressClassAnnotation     = false
	defaultDisableIngressGroupNameAnnotation = false
	defaultMaxIngressConcurrentReconciles    = 3
	defaultTolerateNonExistentBackendService = true
	defaultTolerateNonExistentBackendAction  = true
)

// IngressConfig contains the configurations for the Ingress controller
type IngressConfig struct {
	// Name of the Ingress class this controller satisfies.
	// If empty, all with a `kubernetes.io/ingress.class`
	// annotation of `alb` get considered.
	// Also, if empty, all ingresses without either a `kubernetes.io/ingress.class` annotation or
	// an IngressClassName get considered.
	IngressClass string

	// DisableIngressClassAnnotation specifies whether to disable new use of the `kubernetes.io/ingress.class` annotation.
	DisableIngressClassAnnotation bool

	// DisableIngressGroupNameAnnotation specifies whether to disable new use of the `alb.ingress.kubernetes.io/group.name` annotation.
	DisableIngressGroupNameAnnotation bool

	// Max concurrent reconcile loops for Ingress objects
	MaxConcurrentReconciles int

	// TolerateNonExistentBackendService specifies whether to allow rules that reference a backend service that does not
	// exist. In this case, requests to that rule will result in a 503 error.
	TolerateNonExistentBackendService bool

	// TolerateNonExistentBackendAction specifies whether to allow rules that reference a backend action that does not
	// exist. In this case, requests to that rule will result in a 503 error.
	TolerateNonExistentBackendAction bool

	// AllowedCertificateAuthoritiyARNs contains a list of all CAs to consider when discovering certificates for ingress resources
	AllowedCertificateAuthorityARNs []string
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
	fs.BoolVar(&cfg.TolerateNonExistentBackendService, flagTolerateNonExistentBackendService, defaultTolerateNonExistentBackendService,
		"Tolerate rules that specify a non-existent backend service")
	fs.BoolVar(&cfg.TolerateNonExistentBackendAction, flagTolerateNonExistentBackendAction, defaultTolerateNonExistentBackendAction,
		"Tolerate rules that specify a non-existent backend action")
	fs.StringSliceVar(&cfg.AllowedCertificateAuthorityARNs, flagAllowedCAArns, []string{}, "Specify an optional list of CA ARNs to filter on in cert discovery")
}
