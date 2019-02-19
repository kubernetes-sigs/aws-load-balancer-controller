package resource

import (
	"context"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/test/e2e/framework/utils"
	extensionsv1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

type IngressManager struct {
	cs kubernetes.Interface
}

func NewIngressManager(cs kubernetes.Interface) *IngressManager {
	return &IngressManager{
		cs: cs,
	}
}

func (m *IngressManager) WaitIngressReady(ctx context.Context, ing *extensionsv1.Ingress) (*extensionsv1.Ingress, error) {
	var (
		observedIng *extensionsv1.Ingress
		err         error
	)
	return observedIng, wait.PollImmediateUntil(utils.PollIntervalShort, func() (bool, error) {
		observedIng, err = m.cs.ExtensionsV1beta1().Ingresses(ing.Namespace).Get(ing.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, sts := range observedIng.Status.LoadBalancer.Ingress {
			if len(sts.Hostname) != 0 || len(sts.IP) != 0 {
				return true, nil
			}
		}
		return false, nil
	}, ctx.Done())
}
