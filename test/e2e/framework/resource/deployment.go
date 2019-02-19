package resource

import (
	"context"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/test/e2e/framework/utils"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

type DeploymentManager struct {
	cs kubernetes.Interface
}

func NewDeploymentManager(cs kubernetes.Interface) *DeploymentManager {
	return &DeploymentManager{
		cs: cs,
	}
}

func (m *DeploymentManager) WaitDeploymentReady(ctx context.Context, dp *appsv1.Deployment) (*appsv1.Deployment, error) {
	var (
		observedDP *appsv1.Deployment
		err        error
	)
	return observedDP, wait.PollImmediateUntil(utils.PollIntervalShort, func() (bool, error) {
		observedDP, err = m.cs.AppsV1().Deployments(dp.Namespace).Get(dp.Name, metav1.GetOptions{})
		if err != nil {
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
