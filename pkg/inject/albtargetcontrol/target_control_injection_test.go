package albtargetcontrol

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_AlbTargetControlAgent_Mutate(t *testing.T) {
	tests := []struct {
		name           string
		pod            *corev1.Pod
		namespace      *corev1.Namespace
		agentConfig    *elbv2api.ALBTargetControlConfig
		wantContainers int
		wantAnnotation bool
		wantError      bool
	}{
		{
			name: "successful injection",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels: map[string]string{
						InjectLabel: "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app:latest"},
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "default"},
			},
			agentConfig: &elbv2api.ALBTargetControlConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-load-balancer-controller-alb-target-control-agent-config",
					Namespace: "default",
				},
				Spec: elbv2api.ALBTargetControlConfigSpec{
					Image:              "public.ecr.aws/aws-elb/target-optimizer/target-control-agent:latest",
					DataAddress:        "0.0.0.0:80",
					ControlAddress:     "0.0.0.0:3000",
					DestinationAddress: "127.0.0.1:8080",
					MaxConcurrency:     1000,
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
					},
				},
			},
			wantContainers: 2,
			wantAnnotation: true,
			wantError:      false,
		},
		{
			name: "ALBTargetControlConfig missing - injection skipped",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels: map[string]string{
						InjectLabel: "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app:latest"},
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "default"},
			},
			agentConfig:    nil,
			wantContainers: 1,
			wantAnnotation: false,
			wantError:      false,
		},
		{
			name: "annotation overrides work correctly",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels: map[string]string{
						InjectLabel: "true",
					},
					Annotations: map[string]string{
						AnnotationImage:              "123456789012.dkr.ecr.us-west-2.amazonaws.com/custom-agent:v2.0",
						AnnotationDataAddress:        "0.0.0.0:8080",
						AnnotationControlAddress:     "0.0.0.0:9000",
						AnnotationDestinationAddress: "127.0.0.1:3000",
						AnnotationConcurrency:        "500",
						AnnotationTLSCertPath:        "/etc/ssl/certs/tls.crt",
						AnnotationTLSKeyPath:         "/etc/ssl/private/tls.key",
						AnnotationCPURequest:         "200m",
						AnnotationCPULimit:           "1000m",
						AnnotationMemoryRequest:      "256Mi",
						AnnotationMemoryLimit:        "1Gi",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app:latest"},
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "default"},
			},
			agentConfig: &elbv2api.ALBTargetControlConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-load-balancer-controller-alb-target-control-agent-config",
					Namespace: "default",
				},
				Spec: elbv2api.ALBTargetControlConfigSpec{
					Image:              "public.ecr.aws/aws-elb/target-optimizer/target-control-agent:latest",
					DataAddress:        "0.0.0.0:80",
					ControlAddress:     "0.0.0.0:3000",
					DestinationAddress: "127.0.0.1:8080",
					MaxConcurrency:     1000,
				},
			},
			wantContainers: 2,
			wantAnnotation: true,
			wantError:      false,
		},
		{
			name: "invalid concurrency annotation - injection fails",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels: map[string]string{
						InjectLabel: "true",
					},
					Annotations: map[string]string{
						AnnotationConcurrency: "2000",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app:latest"},
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "default"},
			},
			agentConfig: &elbv2api.ALBTargetControlConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-load-balancer-controller-alb-target-control-agent-config",
					Namespace: "default",
				},
				Spec: elbv2api.ALBTargetControlConfigSpec{
					Image:              "public.ecr.aws/aws-elb/target-optimizer/target-control-agent:latest",
					DataAddress:        "0.0.0.0:80",
					ControlAddress:     "0.0.0.0:3000",
					DestinationAddress: "127.0.0.1:8080",
					MaxConcurrency:     1000,
				},
			},
			wantContainers: 1,
			wantAnnotation: false,
			wantError:      true,
		},
		{
			name: "sidecar already exists - skip injection",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels: map[string]string{
						InjectLabel: "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app:latest"},
						{Name: SidecarContainerName, Image: "existing:latest"},
					},
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "default"},
			},
			agentConfig: &elbv2api.ALBTargetControlConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-load-balancer-controller-alb-target-control-agent-config",
					Namespace: "default",
				},
				Spec: elbv2api.ALBTargetControlConfigSpec{
					Image:              "public.ecr.aws/aws-elb/target-optimizer/target-control-agent:latest",
					DataAddress:        "0.0.0.0:80",
					ControlAddress:     "0.0.0.0:3000",
					DestinationAddress: "127.0.0.1:8080",
					MaxConcurrency:     1000,
				},
			},
			wantContainers: 2,
			wantAnnotation: false,
			wantError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()

			assert.NoError(t, k8sClient.Create(ctx, tt.namespace.DeepCopy()))

			if tt.agentConfig != nil {
				assert.NoError(t, k8sClient.Create(ctx, tt.agentConfig.DeepCopy()))
			}

			agent := NewALBTargetControlAgentInjector(k8sClient, k8sClient, logr.New(&log.NullLogSink{}), DefaultControllerNamespace)
			err := agent.Mutate(ctx, tt.pod)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantContainers, len(tt.pod.Spec.Containers))

			if tt.wantAnnotation {
				assert.Equal(t, "true", tt.pod.Annotations[InjectedAnnotation])

				// Verify sidecar container exists
				var sidecarContainer *corev1.Container
				for i := range tt.pod.Spec.Containers {
					if tt.pod.Spec.Containers[i].Name == SidecarContainerName {
						sidecarContainer = &tt.pod.Spec.Containers[i]
						break
					}
				}
				assert.NotNil(t, sidecarContainer, "sidecar container should exist")

				// For annotation override test, verify overrides are applied
				if tt.name == "annotation overrides work correctly" {
					// Verify image override
					assert.Equal(t, "123456789012.dkr.ecr.us-west-2.amazonaws.com/custom-agent:v2.0", sidecarContainer.Image)

					// Verify port overrides
					assert.Equal(t, int32(8080), sidecarContainer.Ports[0].ContainerPort) // data port
					assert.Equal(t, int32(9000), sidecarContainer.Ports[1].ContainerPort) // control port

					// Verify environment variables
					envVars := make(map[string]string)
					for _, env := range sidecarContainer.Env {
						envVars[env.Name] = env.Value
					}
					assert.Equal(t, "0.0.0.0:8080", envVars[EnvDataAddress])
					assert.Equal(t, "0.0.0.0:9000", envVars[EnvControlAddress])
					assert.Equal(t, "127.0.0.1:3000", envVars[EnvDestinationAddress])
					assert.Equal(t, "500", envVars[EnvMaxConcurrency])
					assert.Equal(t, "/etc/ssl/certs/tls.crt", envVars[EnvTLSCertPath])
					assert.Equal(t, "/etc/ssl/private/tls.key", envVars[EnvTLSKeyPath])

					// Verify resource overrides
					assert.Equal(t, resource.MustParse("200m"), sidecarContainer.Resources.Requests[corev1.ResourceCPU])
					assert.Equal(t, resource.MustParse("1000m"), sidecarContainer.Resources.Limits[corev1.ResourceCPU])
					assert.Equal(t, resource.MustParse("256Mi"), sidecarContainer.Resources.Requests[corev1.ResourceMemory])
					assert.Equal(t, resource.MustParse("1Gi"), sidecarContainer.Resources.Limits[corev1.ResourceMemory])
				}
			} else {
				if tt.pod.Annotations != nil {
					assert.NotEqual(t, "true", tt.pod.Annotations[InjectedAnnotation])
				}
			}
		})
	}
}

func Test_isValidAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    bool
	}{
		{
			name:    "valid IP address with port",
			address: "0.0.0.0:8080",
			want:    true,
		},
		{
			name:    "hostname with port",
			address: "localhost:3000",
			want:    false,
		},
		{
			name:    "invalid address without port",
			address: "127.0.0.1",
			want:    false,
		},
		{
			name:    "empty address",
			address: "",
			want:    false,
		},
		{
			name:    "multiple colons",
			address: "127.0.0.1:80:80",
			want:    false,
		},
		{
			name:    "port only",
			address: "3000",
			want:    false,
		},
		{
			name:    "valid IPv6 with port",
			address: "[::1]:8080",
			want:    true,
		},
		{
			name:    "valid IPv6 full address with port",
			address: "[2001:db8::1]:3000",
			want:    true,
		},
		{
			name:    "invalid IPv6 without brackets",
			address: "::1:8080",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidAddress(tt.address)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_extractPortFromAddress(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		wantPort int32
		wantOk   bool
	}{
		{
			name:     "valid address with port",
			address:  "0.0.0.0:8080",
			wantPort: 8080,
			wantOk:   true,
		},
		{
			name:     "invalid format",
			address:  "invalid",
			wantPort: 0,
			wantOk:   false,
		},
		{
			name:     "empty string",
			address:  "",
			wantPort: 0,
			wantOk:   false,
		},
		{
			name:     "IPv6 with port",
			address:  "[::1]:8080",
			wantPort: 8080,
			wantOk:   true,
		},
		{
			name:     "IPv6 full address with port",
			address:  "[2001:db8::1]:3000",
			wantPort: 3000,
			wantOk:   true,
		},
		{
			name:     "IPv6 without brackets",
			address:  "::1:8080",
			wantPort: 0,
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPort, gotOk := extractPortFromAddress(tt.address)
			assert.Equal(t, tt.wantPort, gotPort)
			assert.Equal(t, tt.wantOk, gotOk)
		})
	}
}

func Test_isValidConcurrency(t *testing.T) {
	tests := []struct {
		name        string
		concurrency string
		want        bool
	}{
		{
			name:        "valid concurrency 1",
			concurrency: "1",
			want:        true,
		},
		{
			name:        "valid concurrency 500",
			concurrency: "500",
			want:        true,
		},
		{
			name:        "valid concurrency 1000",
			concurrency: "1000",
			want:        true,
		},
		{
			name:        "zero concurrency",
			concurrency: "0",
			want:        true,
		},
		{
			name:        "negative concurrency",
			concurrency: "-1",
			want:        false,
		},
		{
			name:        "too high concurrency",
			concurrency: "1001",
			want:        false,
		},
		{
			name:        "invalid format",
			concurrency: "abc",
			want:        false,
		},
		{
			name:        "empty string",
			concurrency: "",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidConcurrency(tt.concurrency)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_isValidProtocolVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{
			name:    "valid HTTP1",
			version: "HTTP1",
			want:    true,
		},
		{
			name:    "valid HTTP2",
			version: "HTTP2",
			want:    true,
		},
		{
			name:    "valid GRPC",
			version: "GRPC",
			want:    true,
		},
		{
			name:    "invalid lowercase http1",
			version: "http1",
			want:    false,
		},
		{
			name:    "invalid HTTP3",
			version: "HTTP3",
			want:    false,
		},
		{
			name:    "empty string",
			version: "",
			want:    false,
		},
		{
			name:    "invalid random string",
			version: "invalid",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidProtocolVersion(tt.version)
			assert.Equal(t, tt.want, got)
		})
	}
}
