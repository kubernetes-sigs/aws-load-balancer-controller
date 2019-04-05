package k8s

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func LookupServicePort(svc *corev1.Service, port intstr.IntOrString) (corev1.ServicePort, error) {
	if port.Type == intstr.String {
		for _, p := range svc.Spec.Ports {
			if p.Name == port.StrVal {
				return p, nil
			}
		}
	} else {
		for _, p := range svc.Spec.Ports {
			if p.Port == port.IntVal {
				return p, nil
			}
		}
	}

	return corev1.ServicePort{}, errors.Errorf("Unable to find %s port on service %s", port.String(), svc.Name)
}
