package service

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EnhancedBackend struct{}

// EnhancedBackendBuilder is capable of build EnhancedBackend for Service backend.
type EnhancedBackendBuilder interface {
	Build(ctx context.Context, svc *corev1.Service, action Action, backendServices map[types.NamespacedName]*corev1.Service) error
}

// NewDefaultEnhancedBackendBuilder constructs new defaultEnhancedBackendBuilder.
func NewDefaultEnhancedBackendBuilder(k8sClient client.Client, annotationParser annotations.Parser, logger logr.Logger) *defaultEnhancedBackendBuilder {
	return &defaultEnhancedBackendBuilder{
		k8sClient:        k8sClient,
		annotationParser: annotationParser,
		logger:           logger,
	}
}

type defaultEnhancedBackendBuilder struct {
	k8sClient        client.Client
	annotationParser annotations.Parser
	logger           logr.Logger
}

func (b *defaultEnhancedBackendBuilder) Build(ctx context.Context, svc *corev1.Service, action Action, backendServices map[types.NamespacedName]*corev1.Service) error {
	if err := b.loadBackendServices(ctx, &action, svc.Namespace, backendServices); err != nil {
		return err
	}

	return nil
}

// loadBackendServices loads referenced backend services into backendServices.
func (b *defaultEnhancedBackendBuilder) loadBackendServices(ctx context.Context, action *Action, namespace string,
	backendServices map[types.NamespacedName]*corev1.Service) error {
	svcNames := sets.NewString()
	for _, tgt := range action.ForwardConfig.TargetGroups {
		if tgt.ServiceName != nil {
			svcNames.Insert(awssdk.ToString(tgt.ServiceName))
		}
	}

	for svcName := range svcNames {
		svcKey := types.NamespacedName{Namespace: namespace, Name: svcName}
		if _, ok := backendServices[svcKey]; ok {
			continue
		}

		// Fetch the Service from the API
		svc := &corev1.Service{}
		err := b.k8sClient.Get(ctx, svcKey, svc)
		if err != nil {
			return err
		}

		backendServices[svcKey] = svc
	}

	return nil
}
