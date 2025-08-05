package inject

import (
	"context"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"strings"
)

// QUICServerIDInjector is a pod mutator that adds server IDs to containers that a match a certain criteria.
type QUICServerIDInjector interface {
	Mutate(ctx context.Context, pod *corev1.Pod) error
}

// quicServerIDInjectorImpl concrete implementation of QUICServerIDInjector
type quicServerIDInjectorImpl struct {
	config      QUICServerIDInjectionConfig
	idGenerator quicServerIDGenerator
	logger      logr.Logger
}

// NewQUICServerIDInjector constructs a new injector to generate QUIC server IDs for containers.
func NewQUICServerIDInjector(config QUICServerIDInjectionConfig, logger logr.Logger) QUICServerIDInjector {
	return &quicServerIDInjectorImpl{
		config:      config,
		logger:      logger,
		idGenerator: newQuicServerIDGenerator(),
	}
}

// Mutate injects a server ID into each container specified.
func (m *quicServerIDInjectorImpl) Mutate(ctx context.Context, pod *corev1.Pod) error {
	// see https://github.com/kubernetes/kubernetes/issues/88282 and https://github.com/kubernetes/kubernetes/issues/76680

	if pod.Annotations == nil {
		return nil
	}

	containerNameList, ok := pod.Annotations[quicEnabledContainers]
	if !ok {
		return nil
	}

	containerNameSet := sets.New(strings.Split(containerNameList, ",")...)
	for i := range pod.Spec.Containers {
		m.mutateContainerSpec(&pod.Spec.Containers[i], containerNameSet)
	}

	for i := range pod.Spec.InitContainers {
		m.mutateContainerSpec(&pod.Spec.InitContainers[i], containerNameSet)
	}

	return nil
}

func (m *quicServerIDInjectorImpl) mutateContainerSpec(cont *corev1.Container, containerNameSet sets.Set[string]) {
	if containerNameSet.Has(cont.Name) {
		if cont.Env == nil {
			cont.Env = make([]corev1.EnvVar, 0)
		}

		var duplicateFound bool
		for _, env := range cont.Env {
			if env.Name == m.config.EnvironmentVariableName {
				duplicateFound = true
				break
			}
		}

		if !duplicateFound {
			cont.Env = append(cont.Env, corev1.EnvVar{
				Name:  m.config.EnvironmentVariableName,
				Value: m.idGenerator.generate(),
			})
		}
	}
}
