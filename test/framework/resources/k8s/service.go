package k8s

import (
	"context"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ServiceManager is responsible for Service resources.
type ServiceManager interface {
	WaitUntilServiceActive(ctx context.Context, svc *corev1.Service) (*corev1.Service, error)
	WaitUntilServiceDeleted(ctx context.Context, svc *corev1.Service) error
}

// NewDefaultServiceManager constructs new ServiceManager.
func NewDefaultServiceManager(k8sClient client.Client, logger logr.Logger) *defaultServiceManager {
	return &defaultServiceManager{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

var _ ServiceManager = &defaultServiceManager{}

// default implementation for ServiceManager.
type defaultServiceManager struct {
	k8sClient client.Client
	logger    logr.Logger
}

func (m *defaultServiceManager) WaitUntilServiceActive(ctx context.Context, svc *corev1.Service) (*corev1.Service, error) {
	observedSvc := &corev1.Service{}
	return observedSvc, wait.PollImmediateUntil(utils.PollIntervalShort, func() (bool, error) {
		if err := m.k8sClient.Get(ctx, k8s.NamespacedName(svc), observedSvc); err != nil {
			return false, err
		}
		if observedSvc.Status.LoadBalancer.Ingress != nil {
			return true, nil
		}
		return false, nil
	}, ctx.Done())

}

func (m *defaultServiceManager) WaitUntilServiceDeleted(ctx context.Context, svc *corev1.Service) error {
	observedSVC := &corev1.Service{}
	return wait.PollImmediateUntil(utils.PollIntervalShort, func() (bool, error) {
		if err := m.k8sClient.Get(ctx, k8s.NamespacedName(svc), observedSVC); err != nil {
			if apierrs.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	}, ctx.Done())
}
