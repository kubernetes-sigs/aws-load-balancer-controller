package config

import (
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
)

const (
	flagIngressClass                            = "ingress-class"
	flagDisableIngressClassAnnotation           = "disable-ingress-class-annotation"
	flagDisableIngressGroupNameAnnotation       = "disable-ingress-group-name-annotation"
	flagIngressMaxConcurrentReconciles          = "ingress-max-concurrent-reconciles"
	flagTolerateNonExistentBackendService       = "tolerate-non-existent-backend-service"
	flagTolerateNonExistentBackendAction        = "tolerate-non-existent-backend-action"
	flagAllowedCAArns                           = "allowed-certificate-authority-arns"
	flagEnableACMCertificates                   = "enable-acm-certificates"
	flagDefaultPCAARN                           = "default-pca-arn"
	flagRoute53ValidationRecordRoutingPolicy    = "route53-validation-record-routing-policy"
	flagRoute53ValidationRecordWeight           = "route53-validation-record-weight"
	defaultIngressClass                         = "alb"
	defaultDisableIngressClassAnnotation        = false
	defaultDisableIngressGroupNameAnnotation    = false
	defaultMaxIngressConcurrentReconciles       = 3
	defaultTolerateNonExistentBackendService    = true
	defaultTolerateNonExistentBackendAction     = true
	defaultDefaultPCAArn                        = ""
	defaultRoute53ValidationRecordRoutingPolicy = Route53RoutingPolicySimple
	defaultRoute53ValidationRecordWeight        = int64(100)

	// Route53RoutingPolicySimple is the legacy behavior: one Simple record per validation CNAME.
	// Only one controller can own the record for a given name at a time; a second controller
	// creating a validation record for the same domain will fail.
	Route53RoutingPolicySimple = "simple"

	// Route53RoutingPolicyWeighted creates validation records using Route53's Weighted routing
	// policy instead, identified by this controller's cluster name. This lets multiple independent
	// LBC deployments (e.g. blue/green clusters) each own a validation record for the same domain
	// without conflicting.
	Route53RoutingPolicyWeighted = "weighted"
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

	// ACM Certificates Management feature
	DefaultPCAArn string

	// Route53ValidationRecordRoutingPolicy controls the Route53 routing policy used for the DNS
	// validation records this controller creates.
	Route53ValidationRecordRoutingPolicy string

	// Route53ValidationRecordWeight is the weight assigned to this controller's validation records
	// when Route53ValidationRecordRoutingPolicy is "weighted".
	Route53ValidationRecordWeight int64
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
	fs.StringVar(&cfg.DefaultPCAArn, flagDefaultPCAARN, defaultDefaultPCAArn, "Default PCA ARN to use for creating ACM certificates")
	fs.StringVar(&cfg.Route53ValidationRecordRoutingPolicy, flagRoute53ValidationRecordRoutingPolicy, defaultRoute53ValidationRecordRoutingPolicy,
		"Route53 routing policy to use for ACM DNS validation records this controller creates: simple(default), weighted")
	fs.Int64Var(&cfg.Route53ValidationRecordWeight, flagRoute53ValidationRecordWeight, defaultRoute53ValidationRecordWeight,
		"Weight to use for this controller's ACM DNS validation records when route53-validation-record-routing-policy is weighted")
}

// Validate validates the IngressConfigs routing options
func (cfg *IngressConfig) Validate() error {
	switch cfg.Route53ValidationRecordRoutingPolicy {
	case Route53RoutingPolicySimple, Route53RoutingPolicyWeighted:
	default:
		return errors.Errorf("invalid value %v for %v, must be one of: %v, %v",
			cfg.Route53ValidationRecordRoutingPolicy, flagRoute53ValidationRecordRoutingPolicy, Route53RoutingPolicySimple, Route53RoutingPolicyWeighted)
	}
	if cfg.Route53ValidationRecordRoutingPolicy == Route53RoutingPolicyWeighted && cfg.Route53ValidationRecordWeight <= 0 {
		return errors.Errorf("%v must be a positive integer when %v is %v, got %v",
			flagRoute53ValidationRecordWeight, flagRoute53ValidationRecordRoutingPolicy, Route53RoutingPolicyWeighted, cfg.Route53ValidationRecordWeight)
	}
	return nil
}
