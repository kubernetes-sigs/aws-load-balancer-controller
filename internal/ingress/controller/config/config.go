package config

import (
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
)

const (
	defaultIngressClass            = ""
	defaultAnnotationPrefix        = "alb.ingress.kubernetes.io"
	defaultALBNamePrefix           = ""
	defaultTargetType              = elbv2.TargetTypeEnumInstance
	defaultBackendProtocol         = elbv2.ProtocolEnumHttp
	defaultRestrictScheme          = false
	defaultRestrictSchemeNamespace = corev1.NamespaceDefault
	defaultSyncRateLimit           = 0.3
)

var (
	defaultDefaultTags = map[string]string{}
)

// Configuration contains all the settings required by an Ingress controller
type Configuration struct {
	ClusterName string

	// VpcID is the ID of worker node's VPC
	VpcID string

	// IngressClass is the ingress class that this controller will monitor for
	IngressClass string

	AnnotationPrefix       string
	ALBNamePrefix          string
	DefaultTags            map[string]string
	DefaultTargetType      string
	DefaultBackendProtocol string

	SyncRateLimit float32

	RestrictScheme          bool
	RestrictSchemeNamespace string

	// InternetFacingIngresses is an dynamic setting that can be updated by configMaps
	InternetFacingIngresses map[string][]string
}

// BindFlags will bind the commandline flags to fields in config
func (config *Configuration) BindFlags(flags *pflag.FlagSet) {
	flags.StringVar(&config.ClusterName, "cluster-name", "", `Kubernetes cluster name (required)`)
	flags.StringVar(&config.IngressClass, "ingress-class", defaultIngressClass,
		`Name of the ingress class this controller satisfies.
		The class of an Ingress object is set using the annotation "kubernetes.io/ingress.class".
		All ingress classes are satisfied if this parameter is left empty.`)
	flags.StringVar(&config.AnnotationPrefix, "annotations-prefix", defaultAnnotationPrefix,
		`Prefix of the Ingress annotations specific to the AWS ALB controller.`)

	flags.StringVar(&config.ALBNamePrefix, "alb-name-prefix", defaultALBNamePrefix,
		`Prefix to add to ALB resources (11 alphanumeric characters or less)`)
	flags.StringToStringVar(&config.DefaultTags, "default-tags", defaultDefaultTags,
		`Default tags to add to all ALBs`)
	flags.StringVar(&config.DefaultTargetType, "target-type", defaultTargetType,
		`Default target type to use for target groups, must be "instance" or "ip"`)
	flags.StringVar(&config.DefaultBackendProtocol, "backend-protocol", defaultBackendProtocol,
		`Default target type to use for target groups, must be "instance" or "ip"`)
	flags.Float32Var(&config.SyncRateLimit, "sync-rate-limit", defaultSyncRateLimit,
		`Define the sync frequency upper limit`)
	flags.BoolVar(&config.RestrictScheme, "restrict-scheme", defaultRestrictScheme,
		`Restrict the scheme to internal except for whitelisted namespaces`)
	flags.StringVar(&config.RestrictSchemeNamespace, "restrict-scheme-namespace", defaultRestrictSchemeNamespace,
		`The namespace with the ConfigMap containing the allowed ingresses. Only respected when restrict-scheme is true.`)
}
