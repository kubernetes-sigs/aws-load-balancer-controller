package albtargetcontrol

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
)

//+kubebuilder:rbac:groups=elbv2.k8s.aws,resources=albtargetcontrolconfigs,verbs=get

const (
	// defaultAlbTargetControlAgentConfigName is the default name for the AlbTargetControlAgentConfig
	defaultAlbTargetControlAgentConfigName = "aws-load-balancer-controller-alb-target-control-agent-config"

	// Logger and controller constants
	LoggerName                 = "alb-target-control-agent-injector"
	DefaultControllerNamespace = "kube-system"

	// Container and environment variable names
	SidecarContainerName  = "alb-target-control-agent"
	EnvDataAddress        = "TARGET_CONTROL_DATA_ADDRESS"
	EnvControlAddress     = "TARGET_CONTROL_CONTROL_ADDRESS"
	EnvDestinationAddress = "TARGET_CONTROL_DESTINATION_ADDRESS"
	EnvMaxConcurrency     = "TARGET_CONTROL_MAX_CONCURRENCY"
	EnvTLSCertPath        = "TARGET_CONTROL_TLS_CERT_PATH"
	EnvTLSKeyPath         = "TARGET_CONTROL_TLS_KEY_PATH"
	EnvTLSSecurityPolicy  = "TARGET_CONTROL_TLS_SECURITY_POLICY"
	EnvProtocolVersion    = "TARGET_CONTROL_PROTOCOL_VERSION"
	EnvRustLog            = "RUST_LOG"

	// Container port names
	PortNameData    = "data"
	PortNameControl = "control"

	// Label values
	NamespaceInjectionEnabled = "enabled"

	// ALB target control agent injection labels and annotations
	InjectLabel        = "elbv2.k8s.aws/alb-target-control-agent-inject"
	InjectedAnnotation = "elbv2.k8s.aws/alb-target-control-agent-injected"
	NamespaceLabel     = "elbv2.k8s.aws/alb-target-control-agent-injection"

	// Pod annotation keys for configuration overrides
	AnnotationImage              = "elbv2.k8s.aws/alb-target-control-agent-image"
	AnnotationDataAddress        = "elbv2.k8s.aws/alb-target-control-agent-data-address"
	AnnotationControlAddress     = "elbv2.k8s.aws/alb-target-control-agent-control-address"
	AnnotationDestinationAddress = "elbv2.k8s.aws/alb-target-control-agent-destination-address"
	AnnotationConcurrency        = "elbv2.k8s.aws/alb-target-control-agent-max-concurrency"
	AnnotationTLSCertPath        = "elbv2.k8s.aws/alb-target-control-agent-tls-cert-path"
	AnnotationTLSKeyPath         = "elbv2.k8s.aws/alb-target-control-agent-tls-key-path"
	AnnotationTLSSecurityPolicy  = "elbv2.k8s.aws/alb-target-control-agent-tls-security-policy"
	AnnotationProtocolVersion    = "elbv2.k8s.aws/alb-target-control-agent-protocol-version"
	AnnotationRustLog            = "elbv2.k8s.aws/alb-target-control-agent-rust-log"
	AnnotationCPURequest         = "elbv2.k8s.aws/alb-target-control-agent-cpu-request"
	AnnotationCPULimit           = "elbv2.k8s.aws/alb-target-control-agent-cpu-limit"
	AnnotationMemoryRequest      = "elbv2.k8s.aws/alb-target-control-agent-memory-request"
	AnnotationMemoryLimit        = "elbv2.k8s.aws/alb-target-control-agent-memory-limit"
)

// ALBTargetControlAgentInjector is a pod mutator that inject ALB Target Control Agent to pod that has enable label.
type ALBTargetControlAgentInjector interface {
	Mutate(ctx context.Context, pod *corev1.Pod) error
}

// ALBTargetControlAgentInjectorImpl is the implementation of ALBTargetControlAgentInjector
type ALBTargetControlAgentInjectorImpl struct {
	// k8sClient is the Kubernetes client for API operations
	k8sClient client.Client
	// apiReader is the API reader for configuration retrieval
	apiReader client.Reader
	// logger is the structured logger instance
	logger logr.Logger
	// controllerNamespace is the namespace where the controller is running
	controllerNamespace string
	// configCache caches ALBTargetControlConfigSpec by namespace
	configCache map[string]*elbv2api.ALBTargetControlConfigSpec
	// cacheMutex protects configCache
	cacheMutex sync.RWMutex
}

// NewALBTargetControlAgentInjector constructs new ALBTargetControlAgentInjector
func NewALBTargetControlAgentInjector(k8sClient client.Client, apiReader client.Reader, logger logr.Logger, controllerNamespace string) ALBTargetControlAgentInjector {
	return &ALBTargetControlAgentInjectorImpl{
		k8sClient:           k8sClient,
		apiReader:           apiReader,
		logger:              logger,
		controllerNamespace: controllerNamespace,
		configCache:         make(map[string]*elbv2api.ALBTargetControlConfigSpec),
	}
}

// Mutate injects ALB target control agent sidecar based on pod and namespace labels
func (r *ALBTargetControlAgentInjectorImpl) Mutate(ctx context.Context, pod *corev1.Pod) error {
	r.logger.V(1).Info("Processing pod for sidecar injection", "pod", pod.Name, "namespace", pod.Namespace)

	if !r.isInjectionEnabled(ctx, pod) {
		r.logger.V(1).Info("Sidecar injection not enabled", "pod", pod.Name, "namespace", pod.Namespace)
		return nil
	}

	r.logger.Info("Starting ALB target control agent injection", "pod", pod.Name, "namespace", pod.Namespace)
	return r.injectALBTargetControlAgent(ctx, pod)
}

// isInjectionEnabled determines if sidecar should be injected based on pod labels and namespace labels
func (r *ALBTargetControlAgentInjectorImpl) isInjectionEnabled(ctx context.Context, pod *corev1.Pod) bool {

	// Check pod-level injection label (explicit opt-out takes precedence)
	if pod.Labels != nil {
		if value, exists := pod.Labels[InjectLabel]; exists {
			if value == "false" {
				r.logger.V(1).Info("Pod injection explicitly disabled by label", "pod", pod.Name, "label", InjectLabel)
				return false
			}
			if value == "true" {
				r.logger.V(1).Info("Pod injection explicitly enabled by label", "pod", pod.Name, "label", InjectLabel)
				return true
			}
		}
	}

	// Check namespace-level ALB target control agent injection label
	ns := &corev1.Namespace{}
	if err := r.k8sClient.Get(ctx, client.ObjectKey{Name: pod.Namespace}, ns); err != nil {
		r.logger.Error(err, "failed to get namespace", "namespace", pod.Namespace)
		return false
	}

	if ns.Labels != nil {
		if value, exists := ns.Labels[NamespaceLabel]; exists {
			if value != NamespaceInjectionEnabled {
				r.logger.V(1).Info("Namespace injection not enabled", "namespace", pod.Namespace, "label", NamespaceLabel, "value", value)
				return false
			}
			r.logger.V(1).Info("Namespace injection enabled", "namespace", pod.Namespace, "label", NamespaceLabel)
			return true
		}
	}

	r.logger.V(1).Info("No injection annotations/labels set, defaulting to disabled", "pod", pod.Name, "namespace", pod.Namespace)
	return false
}

// getSidecarConfig retrieves ALB target control agent configuration
// First tries pod namespace, then falls back to controller namespace
func (r *ALBTargetControlAgentInjectorImpl) getSidecarConfig(ctx context.Context, podNamespace string) (*elbv2api.ALBTargetControlConfigSpec, error) {
	// Try pod namespace first
	if config := r.getConfigFromNamespace(ctx, podNamespace); config != nil {
		return config, nil
	}

	// Fallback to controller namespace
	if r.controllerNamespace != "" && r.controllerNamespace != podNamespace {
		if config := r.getConfigFromNamespace(ctx, r.controllerNamespace); config != nil {
			return config, nil
		}
	}

	return nil, apierrors.NewNotFound(elbv2api.GroupVersion.WithResource("albtargetcontrolconfigs").GroupResource(), defaultAlbTargetControlAgentConfigName)
}

// getConfigFromNamespace gets config from namespace with caching
func (r *ALBTargetControlAgentInjectorImpl) getConfigFromNamespace(ctx context.Context, namespace string) *elbv2api.ALBTargetControlConfigSpec {
	r.cacheMutex.RLock()
	cached, exists := r.configCache[namespace]
	r.cacheMutex.RUnlock()
	if exists {
		return cached
	}

	agentConfig := &elbv2api.ALBTargetControlConfig{}
	err := r.apiReader.Get(ctx, client.ObjectKey{Name: defaultAlbTargetControlAgentConfigName, Namespace: namespace}, agentConfig)

	r.cacheMutex.Lock()
	defer r.cacheMutex.Unlock()

	if err != nil {
		r.configCache[namespace] = nil
		return nil
	}

	spec := &agentConfig.Spec
	r.configCache[namespace] = spec
	r.logger.V(1).Info("Found and cached ALBTargetControlConfig", "namespace", namespace)
	return spec
}

// injectALBTargetControlAgent adds the ALB target control agent sidecar container to the pod
func (r *ALBTargetControlAgentInjectorImpl) injectALBTargetControlAgent(ctx context.Context, pod *corev1.Pod) error {
	r.logger.V(1).Info("Starting sidecar injection", "pod", pod.Name, "existingContainers", len(pod.Spec.Containers))

	// Check if sidecar already exists
	for _, container := range pod.Spec.Containers {
		r.logger.V(1).Info("Checking existing container", "containerName", container.Name)
		if container.Name == SidecarContainerName {
			r.logger.V(1).Info("ALB target control agent sidecar already exists, skipping injection", "pod", pod.Name)
			return nil
		}
	}

	r.logger.V(1).Info("Creating ALB target control agent sidecar container")
	sidecarConfig, err := r.getSidecarConfig(ctx, pod.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.logger.V(1).Info("ALBTargetControlAgentConfig not found in pod or controller namespace, skipping injection",
				"configName", defaultAlbTargetControlAgentConfigName,
				"podNamespace", pod.Namespace,
				"controllerNamespace", r.controllerNamespace)
			return nil
		}
		r.logger.Error(err, "failed to get sidecar config")
		return err
	}

	// Create a copy of the config to override with pod-specific annotations
	effectiveConfig := *sidecarConfig

	// Initialize values from CRD configuration
	image := effectiveConfig.Image
	maxConcurrency := effectiveConfig.MaxConcurrency
	dataAddress := effectiveConfig.DataAddress
	controlAddress := effectiveConfig.ControlAddress
	destinationAddress := effectiveConfig.DestinationAddress
	var tlsCertPath, tlsKeyPath, tlsSecurityPolicy, protocolVersion, rustLog string
	if effectiveConfig.TLSCertPath != nil {
		tlsCertPath = *effectiveConfig.TLSCertPath
	}
	if effectiveConfig.TLSKeyPath != nil {
		tlsKeyPath = *effectiveConfig.TLSKeyPath
	}
	if effectiveConfig.TLSSecurityPolicy != nil {
		tlsSecurityPolicy = *effectiveConfig.TLSSecurityPolicy
	}
	if effectiveConfig.ProtocolVersion != nil {
		protocolVersion = *effectiveConfig.ProtocolVersion
	}
	if effectiveConfig.RustLog != nil {
		rustLog = *effectiveConfig.RustLog
	}

	// Extract ports from addresses for container port configuration
	dataPort, _ := extractPortFromAddress(dataAddress)
	controlPort, _ := extractPortFromAddress(controlAddress)

	// Override config with pod-specific annotations
	if pod.Annotations != nil {
		// Container configuration
		if customImage, exists := pod.Annotations[AnnotationImage]; exists {
			image = customImage
		}
		if concurrency, exists := pod.Annotations[AnnotationConcurrency]; exists {
			if isValidConcurrency(concurrency) {
				if c, err := strconv.ParseInt(concurrency, 10, 32); err == nil {
					maxConcurrency = int32(c)
				}
			} else {
				return fmt.Errorf("invalid concurrency value: %s (must be 0-1000)", concurrency)
			}
		}
		// Address overrides with validation
		if customDataAddress, exists := pod.Annotations[AnnotationDataAddress]; exists {
			if isValidAddress(customDataAddress) {
				dataAddress = customDataAddress
				dataPort, _ = extractPortFromAddress(dataAddress)
			} else {
				return fmt.Errorf("invalid data address format: %s (expected host:port format, e.g., 0.0.0.0:80)", customDataAddress)
			}
		}
		if customControlAddress, exists := pod.Annotations[AnnotationControlAddress]; exists {
			if isValidAddress(customControlAddress) {
				controlAddress = customControlAddress
				controlPort, _ = extractPortFromAddress(controlAddress)
			} else {
				return fmt.Errorf("invalid control address format: %s (expected host:port format, e.g., 0.0.0.0:3000)", customControlAddress)
			}
		}
		// Destination address override with validation
		if customDestinationAddress, exists := pod.Annotations[AnnotationDestinationAddress]; exists {
			if isValidAddress(customDestinationAddress) {
				destinationAddress = customDestinationAddress
			} else {
				return fmt.Errorf("invalid destination address format: %s (expected host:port format, e.g., 127.0.0.1:8080)", customDestinationAddress)
			}
		}
		// TLS path overrides (let agent handle validation)
		if certPath, exists := pod.Annotations[AnnotationTLSCertPath]; exists {
			tlsCertPath = certPath
		}
		if keyPath, exists := pod.Annotations[AnnotationTLSKeyPath]; exists {
			tlsKeyPath = keyPath
		}
		// TLS security policy override (let agent handle validation)
		if policy, exists := pod.Annotations[AnnotationTLSSecurityPolicy]; exists {
			tlsSecurityPolicy = policy
		}
		// Protocol version override with validation
		if version, exists := pod.Annotations[AnnotationProtocolVersion]; exists {
			if isValidProtocolVersion(version) {
				protocolVersion = version
			} else {
				return fmt.Errorf("invalid protocol version: %s (must be HTTP1, HTTP2, or GRPC)", version)
			}
		}
		// Rust log level override with validation
		if logLevel, exists := pod.Annotations[AnnotationRustLog]; exists {
			if isValidRustLogLevel(logLevel) {
				rustLog = logLevel
			} else {
				return fmt.Errorf("invalid rust log level: %s (must be debug, info, or error)", logLevel)
			}
		}
		// Resource limits
		if effectiveConfig.Resources == nil {
			effectiveConfig.Resources = &corev1.ResourceRequirements{}
		}
		if effectiveConfig.Resources.Requests == nil {
			effectiveConfig.Resources.Requests = make(corev1.ResourceList)
		}
		if effectiveConfig.Resources.Limits == nil {
			effectiveConfig.Resources.Limits = make(corev1.ResourceList)
		}
		if cpuRequest, exists := pod.Annotations[AnnotationCPURequest]; exists {
			if qty, err := resource.ParseQuantity(cpuRequest); err == nil {
				effectiveConfig.Resources.Requests[corev1.ResourceCPU] = qty
			}
		}
		if cpuLimit, exists := pod.Annotations[AnnotationCPULimit]; exists {
			if qty, err := resource.ParseQuantity(cpuLimit); err == nil {
				effectiveConfig.Resources.Limits[corev1.ResourceCPU] = qty
			}
		}
		if memoryRequest, exists := pod.Annotations[AnnotationMemoryRequest]; exists {
			if qty, err := resource.ParseQuantity(memoryRequest); err == nil {
				effectiveConfig.Resources.Requests[corev1.ResourceMemory] = qty
			}
		}
		if memoryLimit, exists := pod.Annotations[AnnotationMemoryLimit]; exists {
			if qty, err := resource.ParseQuantity(memoryLimit); err == nil {
				effectiveConfig.Resources.Limits[corev1.ResourceMemory] = qty
			}
		}
	}

	// Build environment variables
	envVars := []corev1.EnvVar{
		{
			Name:  EnvDataAddress,
			Value: dataAddress,
		},
		{
			Name:  EnvControlAddress,
			Value: controlAddress,
		},
		{
			Name:  EnvDestinationAddress,
			Value: destinationAddress,
		},
		{
			Name:  EnvMaxConcurrency,
			Value: fmt.Sprintf("%d", maxConcurrency),
		},
	}
	if tlsCertPath != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  EnvTLSCertPath,
			Value: tlsCertPath,
		})
	}
	if tlsKeyPath != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  EnvTLSKeyPath,
			Value: tlsKeyPath,
		})
	}
	if tlsSecurityPolicy != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  EnvTLSSecurityPolicy,
			Value: tlsSecurityPolicy,
		})
	}
	if protocolVersion != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  EnvProtocolVersion,
			Value: protocolVersion,
		})
	}
	if rustLog != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  EnvRustLog,
			Value: rustLog,
		})
	}

	// Create alb target control agent sidecar container
	sidecarContainer := corev1.Container{
		Name:  SidecarContainerName,
		Image: image,
		Env:   envVars,
		Ports: []corev1.ContainerPort{
			{
				Name:          PortNameData,
				ContainerPort: dataPort,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          PortNameControl,
				ContainerPort: controlPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
	}

	// Set resources if configured
	if effectiveConfig.Resources != nil {
		sidecarContainer.Resources = *effectiveConfig.Resources
	}

	// Add sidecar to pod
	pod.Spec.Containers = append(pod.Spec.Containers, sidecarContainer)

	// Add injected annotation
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[InjectedAnnotation] = "true"

	r.logger.Info("Successfully injected ALB target control agent sidecar", "pod", pod.Name, "dataAddress", dataAddress, "controlAddress", controlAddress)
	return nil
}

// isValidAddress validates address format (IP:port) matching CRD pattern
func isValidAddress(address string) bool {
	if address == "" {
		return false
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}

	// Validate port
	if _, err := strconv.ParseInt(port, 10, 32); err != nil {
		return false
	}

	return net.ParseIP(host) != nil
}

// isValidConcurrency validates max concurrency value (0-1000)
func isValidConcurrency(concurrency string) bool {
	if concurrency == "" {
		return false
	}
	c, err := strconv.ParseInt(concurrency, 10, 32)
	return err == nil && c >= 0 && c <= 1000
}

// isValidProtocolVersion validates protocol version enum values
func isValidProtocolVersion(version string) bool {
	return version == "HTTP1" || version == "HTTP2" || version == "GRPC"
}

// isValidRustLogLevel validates rust log level enum values
func isValidRustLogLevel(level string) bool {
	return level == "debug" || level == "info" || level == "error"
}

// extractPortFromAddress extracts port number from address string (host:port)
func extractPortFromAddress(address string) (int32, bool) {
	if address == "" {
		return 0, false
	}

	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return 0, false
	}

	if p, err := strconv.ParseInt(port, 10, 32); err == nil {
		return int32(p), true
	}

	return 0, false
}
