package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/inject"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

const (
	flagLogLevel                                     = "log-level"
	flagK8sClusterName                               = "cluster-name"
	flagDefaultTags                                  = "default-tags"
	flagResourceTrackingConfiguration                = "resource-tracking-configuration"
	flagDefaultTargetType                            = "default-target-type"
	flagExternalManagedTags                          = "external-managed-tags"
	flagServiceTargetENISGTags                       = "service-target-eni-security-group-tags"
	flagServiceMaxConcurrentReconciles               = "service-max-concurrent-reconciles"
	flagTargetGroupBindingMaxConcurrentReconciles    = "targetgroupbinding-max-concurrent-reconciles"
	flagTargetGroupBindingMaxExponentialBackoffDelay = "targetgroupbinding-max-exponential-backoff-delay"
	flagDefaultSSLPolicy                             = "default-ssl-policy"
	flagEnableBackendSG                              = "enable-backend-security-group"
	flagBackendSecurityGroup                         = "backend-security-group"
	flagEnableEndpointSlices                         = "enable-endpoint-slices"
	flagDisableRestrictedSGRules                     = "disable-restricted-sg-rules"

	ClusterTagPrefixKey         = "clusterTagPrefix"
	IngressTagPrefixKey         = "ingressTagPrefix"
	ServiceTagPrefixKey         = "serviceTagPrefix"
	BackendSGNamePrefixKey      = "backendSGNamePrefix"
	ClusterSgRuleLabelPrefixKey = "clusterSgRuleLabelPrefix"

	defaultClusterTagPrefix           = "elbv2.k8s.aws"
	defaultIngressTagPrefix           = "ingress.k8s.aws"
	defaultServiceTagPrefix           = "service.k8s.aws"
	defaultBackendSGNamePrefix        = "k8s-traffic"
	defaultClusterSgRuleLabelPrefix   = "elbv2.k8s.aws"
	defaultLogLevel                   = "info"
	defaultMaxConcurrentReconciles    = 3
	defaultMaxExponentialBackoffDelay = time.Second * 1000
	defaultSSLPolicy                  = "ELBSecurityPolicy-2016-08"
	defaultEnableBackendSG            = true
	defaultEnableEndpointSlices       = false
	defaultDisableRestrictedSGRules   = false
)

var (
	validPrefixKeys = sets.NewString(
		ClusterTagPrefixKey,
		IngressTagPrefixKey,
		ServiceTagPrefixKey,
		BackendSGNamePrefixKey,
		ClusterSgRuleLabelPrefixKey,
	)
	defaultResourcePrefix = map[string]string{
		ClusterTagPrefixKey:         defaultClusterTagPrefix,
		IngressTagPrefixKey:         defaultIngressTagPrefix,
		ServiceTagPrefixKey:         defaultServiceTagPrefix,
		BackendSGNamePrefixKey:      defaultBackendSGNamePrefix,
		ClusterSgRuleLabelPrefixKey: defaultClusterSgRuleLabelPrefix,
	}
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
	// Configurations for the Service controller
	ServiceConfig ServiceConfig

	// Default AWS Tags that will be applied to all AWS resources managed by this controller.
	DefaultTags map[string]string

	// ResourceTrackingConfiguration provides tracking prefix for resource tags, backend SG name and worker node SG rules label.
	ResourceTrackingConfiguration map[string]string

	// Default target type for Ingress and Service objects
	DefaultTargetType string

	// List of Tag keys on AWS resources that will be managed externally.
	ExternalManagedTags []string

	// ServiceTargetENISGTags are AWS tags, in addition to the cluster tags, for finding the target ENI security group to which to add inbound rules from NLBs.
	ServiceTargetENISGTags map[string]string

	// Default SSL Policy that will be applied to all ingresses or services that do not have
	// the SSL Policy annotation.
	DefaultSSLPolicy string

	// Enable EndpointSlices for IP targets instead of Endpoints
	EnableEndpointSlices bool

	// Max concurrent reconcile loops for Service objects
	ServiceMaxConcurrentReconciles int
	// Max concurrent reconcile loops for TargetGroupBinding objects
	TargetGroupBindingMaxConcurrentReconciles int
	// Max exponential backoff delay for reconcile failures of TargetGroupBinding
	TargetGroupBindingMaxExponentialBackoffDelay time.Duration

	// EnableBackendSecurityGroup specifies whether to use optimized security group rules
	EnableBackendSecurityGroup bool

	// BackendSecurityGroups specifies the configured backend security group to use
	// for optimized security group rules
	BackendSecurityGroup string

	// DisableRestrictedSGRules specifies whether to use restricted security group rules
	DisableRestrictedSGRules bool

	FeatureGates FeatureGates
}

// BindFlags binds the command line flags to the fields in the config object
func (cfg *ControllerConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.LogLevel, flagLogLevel, defaultLogLevel,
		"Set the controller log level - info(default), debug")
	fs.StringVar(&cfg.ClusterName, flagK8sClusterName, "", "Kubernetes cluster name")
	fs.StringToStringVar(&cfg.DefaultTags, flagDefaultTags, nil,
		"Default AWS Tags that will be applied to all AWS resources managed by this controller")
	fs.StringVar(&cfg.DefaultTargetType, flagDefaultTargetType, string(elbv2.TargetTypeInstance),
		"Default target type for Ingresses and Services - ip, instance")
	fs.StringSliceVar(&cfg.ExternalManagedTags, flagExternalManagedTags, nil,
		"List of Tag keys on AWS resources that will be managed externally")
	fs.IntVar(&cfg.ServiceMaxConcurrentReconciles, flagServiceMaxConcurrentReconciles, defaultMaxConcurrentReconciles,
		"Maximum number of concurrently running reconcile loops for service")
	fs.IntVar(&cfg.TargetGroupBindingMaxConcurrentReconciles, flagTargetGroupBindingMaxConcurrentReconciles, defaultMaxConcurrentReconciles,
		"Maximum number of concurrently running reconcile loops for targetGroupBinding")
	fs.DurationVar(&cfg.TargetGroupBindingMaxExponentialBackoffDelay, flagTargetGroupBindingMaxExponentialBackoffDelay, defaultMaxExponentialBackoffDelay,
		"Maximum duration of exponential backoff for targetGroupBinding reconcile failures")
	fs.StringVar(&cfg.DefaultSSLPolicy, flagDefaultSSLPolicy, defaultSSLPolicy,
		"Default SSL policy for load balancers listeners")
	fs.BoolVar(&cfg.EnableBackendSecurityGroup, flagEnableBackendSG, defaultEnableBackendSG,
		"Enable sharing of security groups for backend traffic")
	fs.StringVar(&cfg.BackendSecurityGroup, flagBackendSecurityGroup, "",
		"Backend security group id to use for the ingress rules on the worker node SG")
	fs.BoolVar(&cfg.EnableEndpointSlices, flagEnableEndpointSlices, defaultEnableEndpointSlices,
		"Enable EndpointSlices for IP targets instead of Endpoints")
	fs.BoolVar(&cfg.DisableRestrictedSGRules, flagDisableRestrictedSGRules, defaultDisableRestrictedSGRules,
		"Disable the usage of restricted security group rules")
	fs.StringToStringVar(&cfg.ServiceTargetENISGTags, flagServiceTargetENISGTags, nil,
		"AWS Tags, in addition to cluster tags, for finding the target ENI security group to which to add inbound rules from NLBs")
	fs.StringToStringVar(&cfg.ResourceTrackingConfiguration, flagResourceTrackingConfiguration, defaultResourcePrefix,
		"the prefixes for resource tags, backend SG name and worker node SG rules label.")

	cfg.FeatureGates.BindFlags(fs)
	cfg.AWSConfig.BindFlags(fs)
	cfg.RuntimeConfig.BindFlags(fs)
	cfg.PodWebhookConfig.BindFlags(fs)
	cfg.IngressConfig.BindFlags(fs)
	cfg.AddonsConfig.BindFlags(fs)
	cfg.ServiceConfig.BindFlags(fs)
}

// Validate the controller configuration
func (cfg *ControllerConfig) Validate() error {
	if len(cfg.ClusterName) == 0 {
		return errors.New("kubernetes cluster name must be specified")
	}

	if err := cfg.validateResourcePrefixKeys(); err != nil {
		return err
	}

	trackingTagKeys := sets.New[string](
		cfg.ResourceTrackingConfiguration[ClusterTagPrefixKey]+"/cluster",
		cfg.ResourceTrackingConfiguration[ClusterTagPrefixKey]+"/resource",
		cfg.ResourceTrackingConfiguration[IngressTagPrefixKey]+"/stack",
		cfg.ResourceTrackingConfiguration[IngressTagPrefixKey]+"/resource",
		cfg.ResourceTrackingConfiguration[ServiceTagPrefixKey]+"/stack",
		cfg.ResourceTrackingConfiguration[ServiceTagPrefixKey]+"/resource",
	)

	if err := cfg.validateDefaultTagsCollisionWithTrackingTags(trackingTagKeys); err != nil {
		return err
	}
	if err := cfg.validateExternalManagedTagsCollisionWithTrackingTags(trackingTagKeys); err != nil {
		return err
	}
	if err := cfg.validateExternalManagedTagsCollisionWithDefaultTags(); err != nil {
		return err
	}
	if err := cfg.validateDefaultTargetType(); err != nil {
		return err
	}
	if err := cfg.validateBackendSecurityGroupConfiguration(); err != nil {
		return err
	}
	return nil
}

func (cfg *ControllerConfig) validateDefaultTagsCollisionWithTrackingTags(trackingTagKeys sets.Set[string]) error {
	for tagKey := range cfg.DefaultTags {
		if trackingTagKeys.Has(tagKey) {
			return errors.Errorf("tag key %v cannot be specified in %v flag", tagKey, flagDefaultTags)
		}
	}
	return nil
}

func (cfg *ControllerConfig) validateExternalManagedTagsCollisionWithTrackingTags(trackingTagKeys sets.Set[string]) error {
	for _, tagKey := range cfg.ExternalManagedTags {
		if trackingTagKeys.Has(tagKey) {
			return errors.Errorf("tag key %v cannot be specified in %v flag", tagKey, flagExternalManagedTags)
		}
	}
	return nil
}

func (cfg *ControllerConfig) validateExternalManagedTagsCollisionWithDefaultTags() error {
	for _, tagKey := range cfg.ExternalManagedTags {
		if _, ok := cfg.DefaultTags[tagKey]; ok {
			return errors.Errorf("tag key %v cannot be specified in both %v and %v flag",
				tagKey, flagDefaultTags, flagExternalManagedTags)
		}
	}
	return nil
}

func (cfg *ControllerConfig) validateDefaultTargetType() error {
	switch cfg.DefaultTargetType {
	case string(elbv2.TargetTypeInstance), string(elbv2.TargetTypeIP):
		return nil
	default:
		return errors.Errorf("invalid value %v for default target type", cfg.DefaultTargetType)
	}
}

func (cfg *ControllerConfig) validateBackendSecurityGroupConfiguration() error {
	if len(cfg.BackendSecurityGroup) == 0 {
		return nil
	}
	if !strings.HasPrefix(cfg.BackendSecurityGroup, "sg-") {
		return errors.Errorf("invalid value %v for backend security group id", cfg.BackendSecurityGroup)
	}
	return nil
}

func (cfg *ControllerConfig) validateResourcePrefixKeys() error {
	keys := make([]string, 0, len(cfg.ResourceTrackingConfiguration))
	for key := range cfg.ResourceTrackingConfiguration {
		if !validPrefixKeys.Has(key) {
			return fmt.Errorf("invalid key: %s. Valid keys are: %v", key, validPrefixKeys.List())
		}
		keys = append(keys, key)
	}
	if len(keys) != len(validPrefixKeys.List()) {
		return fmt.Errorf("invalid number of keys. Expected %d keys, but got %d keys",
			len(validPrefixKeys.List()), len(keys))
	}
	return nil
}
