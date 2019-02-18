package resource

import (
	"context"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/test/e2e/framework/utils"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

type ServiceManager struct {
	cs kubernetes.Interface
}

func NewServiceManager(cs kubernetes.Interface) *ServiceManager {
	return &ServiceManager{
		cs: cs,
	}
}

func (m *ServiceManager) WaitServiceHasEndpointsNum(ctx context.Context, svc *corev1.Service, epCounts int) (*corev1.Service, error) {
	if err := wait.PollImmediateUntil(utils.PollIntervalShort, func() (bool, error) {
		ep, err := m.cs.CoreV1().Endpoints(svc.Namespace).Get(svc.Name, metav1.GetOptions{})
		if err != nil {
			if apierrs.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		observedEpCount := 0
		for _, sub := range ep.Subsets {
			observedEpCount += len(sub.Addresses)
		}
		if observedEpCount == epCounts {
			return true, nil
		}
		return false, nil
	}, ctx.Done()); err != nil {
		return nil, err
	}
	return m.cs.CoreV1().Services(svc.Namespace).Get(svc.Name, metav1.GetOptions{})
}
