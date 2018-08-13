package config

import (
	"time"

	"github.com/aws/aws-sdk-go/service/elbv2"

	clientset "k8s.io/client-go/kubernetes"
)

const (
	healthCheckPeriod = 1 * time.Minute
	resyncPeriod      = 30 * time.Second

	targetType = elbv2.TargetTypeEnumInstance

	backendProtocol = elbv2.ProtocolEnumHttp
	healthzPort     = 10254

	albNamePrefix           = "alb"
	restrictSchemeNamespace = "default"
	awsSyncPeriod           = 60 * time.Minute
	awsAPIMaxRetries        = 10
)

// Configuration contains all the settings required by an Ingress controller
type Configuration struct {
	APIServerHost  string
	KubeConfigFile string
	Client         clientset.Interface

	HealthCheckPeriod time.Duration
	ResyncPeriod      time.Duration

	ConfigMapName string

	Namespace string

	DefaultTargetType string

	DefaultBackendProtocol string

	ElectionID string

	HealthzPort int

	ClusterName             string
	ALBNamePrefix           string
	RestrictScheme          bool
	RestrictSchemeNamespace string
	AWSSyncPeriod           time.Duration
	AWSAPIMaxRetries        int
	AWSAPIDebug             bool

	EnableProfiling bool

	SyncRateLimit float32
}

// NewDefault returns a default controller configuration
func NewDefault() *Configuration {
	return &Configuration{
		HealthCheckPeriod: healthCheckPeriod,
		ResyncPeriod:      resyncPeriod,

		// ConfigMapName string

		// Namespace string

		DefaultTargetType: targetType,

		DefaultBackendProtocol: backendProtocol,
		// ElectionID string

		HealthzPort: healthzPort,

		// ClusterName             string
		ALBNamePrefix: albNamePrefix,
		// RestrictScheme          bool
		RestrictSchemeNamespace: restrictSchemeNamespace,
		AWSSyncPeriod:           awsSyncPeriod,
		AWSAPIMaxRetries:        awsAPIMaxRetries,
		// AWSAPIDebug             bool

		// EnableProfiling bool

		// SyncRateLimit float32

	}
}
