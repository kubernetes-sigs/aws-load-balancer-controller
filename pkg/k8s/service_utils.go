package k8s

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// LookupServicePort returns the ServicePort structure for specific port on service.
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
		// Synthesize ServicePort if missing from ExternalName and port is number
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			return corev1.ServicePort{
				Port:       port.IntVal,
				TargetPort: port,
			}, nil
		}
	}

	return corev1.ServicePort{}, errors.Errorf("unable to find port %s on service %s", port.String(), NamespacedName(svc))
}
