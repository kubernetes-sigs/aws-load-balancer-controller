package ingress

import (
	"fmt"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"os"
	"strconv"
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

// Configuration for Ingress Objects.
type Config struct {
	// IngressClass is the ingress class that this controller will monitor for
	IngressClass string

	AnnotationPrefix       string
	ALBNamePrefix          string
	DefaultTags            map[string]string
	DefaultTargetType      string
	DefaultBackendProtocol string

	RestrictScheme          bool
	RestrictSchemeNamespace string

	// InternetFacingIngresses is an dynamic setting that can be updated by configMaps
	InternetFacingIngresses map[string][]string

	FeatureGate FeatureGate
}

// NewConfig constructs new NewConfig obj.
func NewConfig() Config {
	return Config{
		FeatureGate: NewFeatureGate(),
	}
}

// BindFlags will bind the commandline flags to fields in config
func (cfg *Config) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.IngressClass, "ingress-class", defaultIngressClass,
		`Name of the ingress class this controller satisfies.
		The class of an Ingress object is set using the annotation "kubernetes.io/ingress.class".
		All ingress classes are satisfied if this parameter is left empty.`)
	fs.StringVar(&cfg.AnnotationPrefix, "annotations-prefix", defaultAnnotationPrefix,
		`Prefix of the Ingress annotations specific to the AWS ALB controller.`)

	fs.StringVar(&cfg.ALBNamePrefix, "alb-name-prefix", defaultALBNamePrefix,
		`Prefix to add to ALB resources (11 alphanumeric characters or less)`)
	fs.StringToStringVar(&cfg.DefaultTags, "default-tags", defaultDefaultTags,
		`Default tags to add to all ALBs`)
	fs.StringVar(&cfg.DefaultTargetType, "target-type", defaultTargetType,
		`Default target type to use for target groups, must be "instance" or "ip"`)
	fs.StringVar(&cfg.DefaultBackendProtocol, "backend-protocol", defaultBackendProtocol,
		`Default protocol to use for target groups, must be "HTTP" or "HTTPS"`)
	fs.BoolVar(&cfg.RestrictScheme, "restrict-scheme", defaultRestrictScheme,
		`Restrict the scheme to internal except for whitelisted namespaces`)
	fs.StringVar(&cfg.RestrictSchemeNamespace, "restrict-scheme-namespace", defaultRestrictSchemeNamespace,
		`The namespace with the ConfigMap containing the allowed ingresses. Only respected when restrict-scheme is true.`)

	cfg.FeatureGate.BindFlags(fs)

	_ = fs.MarkDeprecated("sync-rate-limit", `No longer used, will be removed in next release`)
}

func (cfg *Config) BindEnv() error {
	if s, ok := os.LookupEnv("ALB_PREFIX"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --alb-name-prefix flag.")
		cfg.ALBNamePrefix = s
	}
	if s, ok := os.LookupEnv("ALB_CONTROLLER_RESTRICT_SCHEME"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --restrict-scheme flag.")
		v, err := strconv.ParseBool(s)
		if err != nil {
			return fmt.Errorf("ALB_CONTROLLER_RESTRICT_SCHEME environment variable must be either true or false. Value was: %s", s)
		}
		cfg.RestrictScheme = v
	}
	if s, ok := os.LookupEnv("ALB_CONTROLLER_RESTRICT_SCHEME_CONFIG_NAMESPACE"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --restrict-scheme-namespace flag.")
		cfg.RestrictSchemeNamespace = s
	}
	return nil
}

func (cfg *Config) Validate() error {
	if cfg.DefaultTargetType == "pod" {
		glog.Warningf("The target type parameter for 'pod' has changed to 'ip' to better match AWS APIs and documentation.")
		cfg.DefaultTargetType = elbv2.TargetTypeEnumIp
	}
	if len(cfg.ALBNamePrefix) > 12 {
		return fmt.Errorf("ALBNamePrefix must be 12 characters or less")
	}

	return nil
}
