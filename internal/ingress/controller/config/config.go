package config

import (
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
)

const (
	defaultIngressClass            = ""
	defaultALBAnnotationPrefix     = "alb.ingress.kubernetes.io"
	defaultNLBAnnotationPrefix     = "nlb.service.kubernetes.io"
	defaultALBNamePrefix           = ""
	defaultNLBNamePrefix           = ""
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

	// IngressClass is the ingress class that this controller will monitor for
	IngressClass string
	// ServiceClass is the service class that this controller will monitor for
	ServiceClass string

	IngressALBAnnotationPrefix string
	ServiceNLBAnnotationPrefix string
	ALBNamePrefix              string
	NLBNamePrefix              string
	DefaultTags                map[string]string
	DefaultTargetType          string
	DefaultBackendProtocol     string

	SyncRateLimit float32

	RestrictScheme          bool
	RestrictSchemeNamespace string

	// InternetFacingIngresses is an dynamic setting that can be updated by configMaps
	InternetFacingIngresses map[string][]string
	InternetFacingServices map[string][]string

	FeatureGate FeatureGate
}

// NewConfiguration constructs new Configuration obj.
func NewConfiguration() Configuration {
	return Configuration{
		FeatureGate: NewFeatureGate(),
	}
}

// BindFlags will bind the commandline flags to fields in config
func (cfg *Configuration) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.ClusterName, "cluster-name", "", `Kubernetes cluster name (required)`)
	fs.StringVar(&cfg.IngressClass, "ingress-class", defaultIngressClass,
		`Name of the ingress class this controller satisfies.
		The class of an Ingress object is set using the annotation "kubernetes.io/ingress.class".
		All ingress classes are satisfied if this parameter is left empty.`)
	fs.StringVar(&cfg.IngressALBAnnotationPrefix, "annotations-prefix", defaultALBAnnotationPrefix,
		`Prefix of the Ingress annotations specific to the AWS ALB controller.`)
	fs.StringVar(&cfg.ServiceNLBAnnotationPrefix, "nlb-annotations-prefix", defaultNLBAnnotationPrefix,
		`Prefix of the Service annotations specific to the AWS NLB controller.`)

	fs.StringVar(&cfg.ALBNamePrefix, "alb-name-prefix", defaultALBNamePrefix,
		`Prefix to add to ALB resources (11 alphanumeric characters or less)`)
	fs.StringVar(&cfg.NLBNamePrefix, "nlb-name-prefix", defaultNLBNamePrefix,
		`Prefix to add to NLB resources (11 alphanumeric characters or less)`)
	fs.StringToStringVar(&cfg.DefaultTags, "default-tags", defaultDefaultTags,
		`Default tags to add to all ALBs`)
	fs.StringVar(&cfg.DefaultTargetType, "target-type", defaultTargetType,
		`Default target type to use for target groups, must be "instance" or "ip"`)
	fs.StringVar(&cfg.DefaultBackendProtocol, "backend-protocol", defaultBackendProtocol,
		`Default protocol to use for target groups, must be "HTTP" or "HTTPS"`)
	fs.Float32Var(&cfg.SyncRateLimit, "sync-rate-limit", defaultSyncRateLimit,
		`Define the sync frequency upper limit`)
	fs.BoolVar(&cfg.RestrictScheme, "restrict-scheme", defaultRestrictScheme,
		`Restrict the scheme to internal except for whitelisted namespaces`)
	fs.StringVar(&cfg.RestrictSchemeNamespace, "restrict-scheme-namespace", defaultRestrictSchemeNamespace,
		`The namespace with the ConfigMap containing the allowed ingresses. Only respected when restrict-scheme is true.`)

	cfg.FeatureGate.BindFlags(fs)
}

func (cfg *Configuration) BindEnv() error {
	if s, ok := os.LookupEnv("CLUSTER_NAME"); ok {
		glog.Warningf("Environment variable configuration is deprecated, switch to the --cluster-name flag.")
		cfg.ClusterName = s
	}
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

func (cfg *Configuration) Validate() error {
	if cfg.DefaultTargetType == "pod" {
		glog.Warningf("The target type parameter for 'pod' has changed to 'ip' to better match AWS APIs and documentation.")
		cfg.DefaultTargetType = elbv2.TargetTypeEnumIp
	}
	if len(cfg.ClusterName) == 0 {
		return fmt.Errorf("clusterName must be specified")
	}
	if len(cfg.ALBNamePrefix) > 12 {
		return fmt.Errorf("ALBNamePrefix must be 12 characters or less")
	}
	if len(cfg.ALBNamePrefix) == 0 {
		cfg.ALBNamePrefix = generateALBNamePrefix(cfg.ClusterName)
	}

	// TODO: I know, bad smell here:D
	parser.ALBAnnotationsPrefix = cfg.IngressALBAnnotationPrefix
	parser.NLBAnnotationsPrefix = cfg.ServiceNLBAnnotationPrefix
	return nil
}

func generateALBNamePrefix(clusterName string) string {
	hash := crc32.New(crc32.MakeTable(0xedb88320))
	_, _ = hash.Write([]byte(clusterName))
	return hex.EncodeToString(hash.Sum(nil))
}
