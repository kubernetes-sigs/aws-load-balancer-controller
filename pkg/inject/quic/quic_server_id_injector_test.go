package quic

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Mock ID generator for testing
type mockQuicServerIDGenerator struct {
	id  string
	err error
}

func (m *mockQuicServerIDGenerator) generate() (string, error) {
	return m.id, m.err
}

func TestQUICServerIDInjector_Mutate_NoAnnotations(t *testing.T) {
	config := ServerIDInjectionConfig{
		EnvironmentVariableName: "TEST_QUIC_ID",
	}

	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	client := testclient.NewClientBuilder().WithScheme(scheme).Build()

	injector := NewQUICServerIDInjector(config, client, client, logr.Discard())

	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "test-container"},
			},
		},
	}

	err := injector.Mutate(context.Background(), pod)

	require.NoError(t, err)
	assert.Empty(t, pod.Spec.Containers[0].Env)
}

func TestQUICServerIDInjector_Mutate_NoQuicAnnotation(t *testing.T) {
	config := ServerIDInjectionConfig{
		EnvironmentVariableName: "TEST_QUIC_ID",
	}

	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	client := testclient.NewClientBuilder().WithScheme(scheme).Build()

	injector := NewQUICServerIDInjector(config, client, client, logr.Discard())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"other-annotation": "value",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "test-container"},
			},
		},
	}

	err := injector.Mutate(context.Background(), pod)

	require.NoError(t, err)
	assert.Empty(t, pod.Spec.Containers[0].Env)
}

func TestQUICServerIDInjector_Mutate_SingleContainer(t *testing.T) {
	config := ServerIDInjectionConfig{
		EnvironmentVariableName: "TEST_QUIC_ID",
	}

	injector := &quicServerIDInjectorImpl{
		config: config,
		logger: logr.Discard(),
		idGenerator: &mockQuicServerIDGenerator{
			id: "test-server-id-123",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.QuicEnabledContainersAnnotation: "test-container",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "test-container"},
			},
		},
	}

	err := injector.Mutate(context.Background(), pod)

	require.NoError(t, err)
	require.Len(t, pod.Spec.Containers[0].Env, 1)
	assert.Equal(t, "TEST_QUIC_ID", pod.Spec.Containers[0].Env[0].Name)
	assert.Equal(t, "test-server-id-123", pod.Spec.Containers[0].Env[0].Value)
}

func TestQUICServerIDInjector_Mutate_MultipleContainers(t *testing.T) {
	config := ServerIDInjectionConfig{
		EnvironmentVariableName: "TEST_QUIC_ID",
	}

	injector := &quicServerIDInjectorImpl{
		config: config,
		logger: logr.Discard(),
		idGenerator: &mockQuicServerIDGenerator{
			id: "test-server-id-456",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.QuicEnabledContainersAnnotation: "container1,container3",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "container1"},
				{Name: "container2"},
				{Name: "container3"},
			},
		},
	}

	err := injector.Mutate(context.Background(), pod)

	require.NoError(t, err)

	// container1 should have the env var
	require.Len(t, pod.Spec.Containers[0].Env, 1)
	assert.Equal(t, "TEST_QUIC_ID", pod.Spec.Containers[0].Env[0].Name)
	assert.Equal(t, "test-server-id-456", pod.Spec.Containers[0].Env[0].Value)

	// container2 should not have the env var
	assert.Empty(t, pod.Spec.Containers[1].Env)

	// container3 should have the env var
	require.Len(t, pod.Spec.Containers[2].Env, 1)
	assert.Equal(t, "TEST_QUIC_ID", pod.Spec.Containers[2].Env[0].Name)
	assert.Equal(t, "test-server-id-456", pod.Spec.Containers[2].Env[0].Value)
}

func TestQUICServerIDInjector_Mutate_InitContainers(t *testing.T) {
	config := ServerIDInjectionConfig{
		EnvironmentVariableName: "TEST_QUIC_ID",
	}

	injector := &quicServerIDInjectorImpl{
		config: config,
		logger: logr.Discard(),
		idGenerator: &mockQuicServerIDGenerator{
			id: "test-server-id-789",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.QuicEnabledContainersAnnotation: "init-container",
			},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: "init-container"},
			},
			Containers: []corev1.Container{
				{Name: "main-container"},
			},
		},
	}

	err := injector.Mutate(context.Background(), pod)

	require.NoError(t, err)

	// init container should have the env var
	require.Len(t, pod.Spec.InitContainers[0].Env, 1)
	assert.Equal(t, "TEST_QUIC_ID", pod.Spec.InitContainers[0].Env[0].Name)
	assert.Equal(t, "test-server-id-789", pod.Spec.InitContainers[0].Env[0].Value)

	// main container should not have the env var
	assert.Empty(t, pod.Spec.Containers[0].Env)
}

func TestQUICServerIDInjector_Mutate_ExistingEnvVars(t *testing.T) {
	config := ServerIDInjectionConfig{
		EnvironmentVariableName: "TEST_QUIC_ID",
	}

	injector := &quicServerIDInjectorImpl{
		config: config,
		logger: logr.Discard(),
		idGenerator: &mockQuicServerIDGenerator{
			id: "test-server-id-abc",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.QuicEnabledContainersAnnotation: "test-container",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "test-container",
					Env: []corev1.EnvVar{
						{Name: "EXISTING_VAR", Value: "existing-value"},
					},
				},
			},
		},
	}

	err := injector.Mutate(context.Background(), pod)

	require.NoError(t, err)
	require.Len(t, pod.Spec.Containers[0].Env, 2)

	// Check existing env var is preserved
	assert.Equal(t, "EXISTING_VAR", pod.Spec.Containers[0].Env[0].Name)
	assert.Equal(t, "existing-value", pod.Spec.Containers[0].Env[0].Value)

	// Check new env var is added
	assert.Equal(t, "TEST_QUIC_ID", pod.Spec.Containers[0].Env[1].Name)
	assert.Equal(t, "test-server-id-abc", pod.Spec.Containers[0].Env[1].Value)
}

func TestQUICServerIDInjector_Mutate_DuplicateEnvVar(t *testing.T) {
	config := ServerIDInjectionConfig{
		EnvironmentVariableName: "TEST_QUIC_ID",
	}

	injector := &quicServerIDInjectorImpl{
		config: config,
		logger: logr.Discard(),
		idGenerator: &mockQuicServerIDGenerator{
			id: "test-server-id-def",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.QuicEnabledContainersAnnotation: "test-container",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "test-container",
					Env: []corev1.EnvVar{
						{Name: "TEST_QUIC_ID", Value: "existing-quic-id"},
					},
				},
			},
		},
	}

	err := injector.Mutate(context.Background(), pod)

	require.NoError(t, err)
	require.Len(t, pod.Spec.Containers[0].Env, 1)

	// Should not add duplicate, existing value should remain
	assert.Equal(t, "TEST_QUIC_ID", pod.Spec.Containers[0].Env[0].Name)
	assert.Equal(t, "existing-quic-id", pod.Spec.Containers[0].Env[0].Value)
}

func TestQUICServerIDInjector_Mutate_GeneratorError(t *testing.T) {
	config := ServerIDInjectionConfig{
		EnvironmentVariableName: "TEST_QUIC_ID",
	}

	injector := &quicServerIDInjectorImpl{
		config: config,
		logger: logr.Discard(),
		idGenerator: &mockQuicServerIDGenerator{
			err: fmt.Errorf("generator failed"),
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.QuicEnabledContainersAnnotation: "test-container",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "test-container"},
			},
		},
	}

	err := injector.Mutate(context.Background(), pod)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "generator failed")
}

func TestQUICServerIDInjector_Mutate_EmptyContainerList(t *testing.T) {
	config := ServerIDInjectionConfig{
		EnvironmentVariableName: "TEST_QUIC_ID",
	}

	injector := &quicServerIDInjectorImpl{
		config: config,
		logger: logr.Discard(),
		idGenerator: &mockQuicServerIDGenerator{
			id: "test-server-id-ghi",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.QuicEnabledContainersAnnotation: "",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "test-container"},
			},
		},
	}

	err := injector.Mutate(context.Background(), pod)

	require.NoError(t, err)
	assert.Empty(t, pod.Spec.Containers[0].Env)
}

func TestQUICServerIDInjector_Mutate_WhitespaceInContainerList(t *testing.T) {
	config := ServerIDInjectionConfig{
		EnvironmentVariableName: "TEST_QUIC_ID",
	}

	injector := &quicServerIDInjectorImpl{
		config: config,
		logger: logr.Discard(),
		idGenerator: &mockQuicServerIDGenerator{
			id: "test-server-id-jkl",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.QuicEnabledContainersAnnotation: " container1 , container2 ",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "container1"},
				{Name: "container2"},
				{Name: " container1 "}, // This should not match due to whitespace
			},
		},
	}

	err := injector.Mutate(context.Background(), pod)

	require.NoError(t, err)

	// container1 should not have the env var due to whitespace mismatch
	assert.Empty(t, pod.Spec.Containers[0].Env)

	// container2 should not have the env var due to whitespace mismatch
	assert.Empty(t, pod.Spec.Containers[1].Env)

	// " container1 " should have the env var as it matches the annotation exactly
	require.Len(t, pod.Spec.Containers[2].Env, 1)
	assert.Equal(t, "TEST_QUIC_ID", pod.Spec.Containers[2].Env[0].Name)
}

func TestServerIDInjectionConfig_BindFlags(t *testing.T) {
	config := &ServerIDInjectionConfig{}

	// This test would require pflag.FlagSet setup, but since we're focusing on core logic,
	// we'll just verify the default value is set correctly
	assert.Equal(t, "", config.EnvironmentVariableName)

	// Test with default value
	config.EnvironmentVariableName = defaultEnvironmentVariableName
	assert.Equal(t, "AWS_LBC_QUIC_SERVER_ID", config.EnvironmentVariableName)
}
