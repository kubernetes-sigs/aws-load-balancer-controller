package k8s

import (
	"context"
	"github.com/go-logr/logr"
	networking "k8s.io/api/networking/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IngressManager is responsible for Ingress resources.
type IngressManager interface {
	WaitUntilIngressReady(ctx context.Context, ing *networking.Ingress) (*networking.Ingress, error)
	WaitUntilIngressDeleted(ctx context.Context, ing *networking.Ingress) error
}

// NewDefaultIngressManager constructs new IngressManager.
func NewDefaultIngressManager(k8sClient client.Client, logger logr.Logger) *defaultIngressManager {
	return &defaultIngressManager{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

var _ IngressManager = &defaultIngressManager{}

// default implementation for IngressManager.
type defaultIngressManager struct {
	k8sClient client.Client
	logger    logr.Logger
}

func (m *defaultIngressManager) WaitUntilIngressReady(ctx context.Context, ing *networking.Ingress) (*networking.Ingress, error) {
	observedING := &networking.Ingress{}
	return observedING, wait.PollImmediateUntil(utils.PollIntervalShort, func() (bool, error) {
		if err := m.k8sClient.Get(ctx, k8s.NamespacedName(ing), observedING); err != nil {
			return false, err
		}
		for _, lbIngress := range observedING.Status.LoadBalancer.Ingress {
			if len(lbIngress.Hostname) != 0 || len(lbIngress.IP) != 0 {
				return true, nil
			}
		}
		return false, nil
	}, ctx.Done())
}

func (m *defaultIngressManager) WaitUntilIngressDeleted(ctx context.Context, ing *networking.Ingress) error {
	observedING := &networking.Ingress{}
	return wait.PollImmediateUntil(utils.PollIntervalShort, func() (bool, error) {
		if err := m.k8sClient.Get(ctx, k8s.NamespacedName(ing), observedING); err != nil {
			if apierrs.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	}, ctx.Done())
}
