package config

import (
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
	flagDefaultTargetType                            = "default-target-type"
	flagDefaultLoadBalancerScheme                    = "default-load-balancer-scheme"
	flagExternalManagedTags                          = "external-managed-tags"
	flagServiceTargetENISGTags                       = "service-target-eni-security-group-tags"
	flagServiceMaxConcurrentReconciles               = "service-max-concurrent-reconciles"
	flagTargetGroupBindingMaxConcurrentReconciles    = "targetgroupbinding-max-concurrent-reconciles"
	flagTargetGroupBindingMaxExponentialBackoffDelay = "targetgroupbinding-max-exponential-backoff-delay"
	flagLbStabilizationMonitorInterval               = "lb-stabilization-monitor-interval"
	flagDefaultSSLPolicy                             = "default-ssl-policy"
	flagEnableBackendSG                              = "enable-backend-security-group"
	flagBackendSecurityGroup                         = "backend-security-group"
	flagEnableEndpointSlices                         = "enable-endpoint-slices"
	flagDisableRestrictedSGRules                     = "disable-restricted-sg-rules"
	defaultLogLevel                                  = "info"
	defaultMaxConcurrentReconciles                   = 3
	defaultMaxExponentialBackoffDelay                = time.Second * 1000
	defaultSSLPolicy                                 = "ELBSecurityPolicy-2016-08"
	defaultEnableBackendSG                           = true
	defaultEnableEndpointSlices                      = false
	defaultDisableRestrictedSGRules                  = false
	defaultLbStabilizationMonitorInterval            = time.Second * 120
)

var (
	trackingTagKeys = sets.NewString(
		"elbv2.k8s.aws/cluster",
		"elbv2.k8s.aws/resource",
		"ingress.k8s.aws/stack",
		"ingress.k8s.aws/resource",
		"service.k8s.aws/stack",
		"service.k8s.aws/resource",
	)
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

	// Default target type for Ingress and Service objects
	DefaultTargetType string

	// Default scheme for ELB
	DefaultLoadBalancerScheme string

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

	// LBStabilizationMonitorInterval specifies the duration of interval to monitor the load balancer state for stabilization
	LBStabilizationMonitorInterval time.Duration

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
	fs.StringVar(&cfg.DefaultLoadBalancerScheme, flagDefaultLoadBalancerScheme, string(elbv2.LoadBalancerSchemeInternal),
		"Default scheme for ELBs")
	fs.StringSliceVar(&cfg.ExternalManagedTags, flagExternalManagedTags, nil,
		"List of Tag keys on AWS resources that will be managed externally")
	fs.IntVar(&cfg.ServiceMaxConcurrentReconciles, flagServiceMaxConcurrentReconciles, defaultMaxConcurrentReconciles,
		"Maximum number of concurrently running reconcile loops for service")
	fs.IntVar(&cfg.TargetGroupBindingMaxConcurrentReconciles, flagTargetGroupBindingMaxConcurrentReconciles, defaultMaxConcurrentReconciles,
		"Maximum number of concurrently running reconcile loops for targetGroupBinding")
	fs.DurationVar(&cfg.TargetGroupBindingMaxExponentialBackoffDelay, flagTargetGroupBindingMaxExponentialBackoffDelay, defaultMaxExponentialBackoffDelay,
		"Maximum duration of exponential backoff for targetGroupBinding reconcile failures")
	fs.DurationVar(&cfg.LBStabilizationMonitorInterval, flagLbStabilizationMonitorInterval, defaultLbStabilizationMonitorInterval,
		"Duration of interval to monitor the load balancer state for stabilization")
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

	if err := cfg.validateDefaultTagsCollisionWithTrackingTags(); err != nil {
		return err
	}
	if err := cfg.validateExternalManagedTagsCollisionWithTrackingTags(); err != nil {
		return err
	}
	if err := cfg.validateExternalManagedTagsCollisionWithDefaultTags(); err != nil {
		return err
	}
	if err := cfg.validateDefaultTargetType(); err != nil {
		return err
	}
	if err := cfg.validateDefaultLoadBalancerScheme(); err != nil {
		return err
	}
	if err := cfg.validateBackendSecurityGroupConfiguration(); err != nil {
		return err
	}
	return nil
}

func (cfg *ControllerConfig) validateDefaultTagsCollisionWithTrackingTags() error {
	for tagKey := range cfg.DefaultTags {
		if trackingTagKeys.Has(tagKey) {
			return errors.Errorf("tag key %v cannot be specified in %v flag", tagKey, flagDefaultTags)
		}
	}
	return nil
}

func (cfg *ControllerConfig) validateExternalManagedTagsCollisionWithTrackingTags() error {
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

func (cfg *ControllerConfig) validateDefaultLoadBalancerScheme() error {
	switch cfg.DefaultLoadBalancerScheme {
	case string(elbv2.LoadBalancerSchemeInternal), string(elbv2.LoadBalancerSchemeInternetFacing):
		return nil
	default:
		return errors.Errorf("invalid value %v for default scheme", cfg.DefaultLoadBalancerScheme)
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
