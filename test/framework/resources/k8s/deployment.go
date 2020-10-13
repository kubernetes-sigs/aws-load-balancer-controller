package k8s

import (
	"context"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeploymentManager is responsible for deployment resources.
type DeploymentManager interface {
	WaitUntilDeploymentReady(ctx context.Context, dp *appsv1.Deployment) (*appsv1.Deployment, error)
	WaitUntilDeploymentDeleted(ctx context.Context, dp *appsv1.Deployment) error
}

// NewDefaultDeploymentManager constructs new DeploymentManager
func NewDefaultDeploymentManager(k8sClient client.Client, logger logr.Logger) *defaultDeploymentManager {
	return &defaultDeploymentManager{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

var _ DeploymentManager = &defaultDeploymentManager{}

// default implementation for DeploymentManager.
type defaultDeploymentManager struct {
	k8sClient client.Client
	logger    logr.Logger
}

func (m *defaultDeploymentManager) WaitUntilDeploymentReady(ctx context.Context, dp *appsv1.Deployment) (*appsv1.Deployment, error) {
	observedDP := &appsv1.Deployment{}
	return observedDP, wait.PollImmediateUntil(utils.PollIntervalShort, func() (bool, error) {
		if err := m.k8sClient.Get(ctx, k8s.NamespacedName(dp), observedDP); err != nil {
			return false, err
		}
		if observedDP.Status.UpdatedReplicas == (*dp.Spec.Replicas) &&
			observedDP.Status.Replicas == (*dp.Spec.Replicas) &&
			observedDP.Status.AvailableReplicas == (*dp.Spec.Replicas) &&
			observedDP.Status.ObservedGeneration >= dp.Generation {
			return true, nil
		}
		return false, nil
	}, ctx.Done())
}

func (m *defaultDeploymentManager) WaitUntilDeploymentDeleted(ctx context.Context, dp *appsv1.Deployment) error {
	observedDP := &appsv1.Deployment{}
	return wait.PollImmediateUntil(utils.PollIntervalShort, func() (bool, error) {
		if err := m.k8sClient.Get(ctx, k8s.NamespacedName(dp), observedDP); err != nil {
			if apierrs.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	}, ctx.Done())
}
