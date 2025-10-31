package routeutils

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
)

type ServiceBackendConfig struct {
	service          *corev1.Service
	targetGroupProps *elbv2gw.TargetGroupProps
	servicePort      *corev1.ServicePort
}

func NewServiceBackendConfig(service *corev1.Service, targetGroupProps *elbv2gw.TargetGroupProps, servicePort *corev1.ServicePort) *ServiceBackendConfig {
	return &ServiceBackendConfig{
		service:          service,
		targetGroupProps: targetGroupProps,
		servicePort:      servicePort,
	}
}

func (s *ServiceBackendConfig) GetHealthCheckPort(targetType elbv2model.TargetType, isServiceExternalTrafficPolicyTypeLocal bool) (intstr.IntOrString, error) {
	portConfigNotExist := s.targetGroupProps == nil || s.targetGroupProps.HealthCheckConfig == nil || s.targetGroupProps.HealthCheckConfig.HealthCheckPort == nil

	if portConfigNotExist && isServiceExternalTrafficPolicyTypeLocal {
		return intstr.FromInt32(s.service.Spec.HealthCheckNodePort), nil
	}

	if portConfigNotExist || *s.targetGroupProps.HealthCheckConfig.HealthCheckPort == shared_constants.HealthCheckPortTrafficPort {
		return intstr.FromString(shared_constants.HealthCheckPortTrafficPort), nil
	}

	healthCheckPort := intstr.Parse(*s.targetGroupProps.HealthCheckConfig.HealthCheckPort)
	if healthCheckPort.Type == intstr.Int {
		return healthCheckPort, nil
	}
	hcSvcPort, err := k8s.LookupServicePort(s.service, healthCheckPort)
	if err != nil {
		return intstr.FromString(""), err
	}

	if targetType == elbv2model.TargetTypeInstance {
		return intstr.FromInt(int(hcSvcPort.NodePort)), nil
	}

	if hcSvcPort.TargetPort.Type == intstr.Int {
		return hcSvcPort.TargetPort, nil
	}
	return intstr.IntOrString{}, errors.New("cannot use named healthCheckPort for IP TargetType when service's targetPort is a named port")
}

// GetTargetGroupPort constructs the TargetGroup's port.
// Note: TargetGroup's port is not in the data path as we always register targets with port specified.
// so these settings don't really matter to our controller,
// and we do our best to use the most appropriate port as targetGroup's port to avoid UX confusing.

func (s *ServiceBackendConfig) GetTargetGroupPort(targetType elbv2model.TargetType) int32 {
	if targetType == elbv2model.TargetTypeInstance {
		return s.servicePort.NodePort
	}
	if s.servicePort.TargetPort.Type == intstr.Int {
		return int32(s.servicePort.TargetPort.IntValue())
	}
	// If all else fails, return 1 as alluded to above, this setting doesn't really matter.
	return 1
}

func (s *ServiceBackendConfig) GetIPAddressType() elbv2model.TargetGroupIPAddressType {
	var ipv6Configured bool
	for _, ipFamily := range s.service.Spec.IPFamilies {
		if ipFamily == corev1.IPv6Protocol {
			ipv6Configured = true
			break
		}
	}
	if ipv6Configured {
		return elbv2model.TargetGroupIPAddressTypeIPv6
	}
	return elbv2model.TargetGroupIPAddressTypeIPv4
}

func (s *ServiceBackendConfig) GetExternalTrafficPolicy() corev1.ServiceExternalTrafficPolicyType {
	return s.service.Spec.ExternalTrafficPolicy
}

func (s *ServiceBackendConfig) GetServicePort() *corev1.ServicePort {
	return s.servicePort
}

func (s *ServiceBackendConfig) GetIdentifierPort() intstr.IntOrString {
	return s.servicePort.TargetPort
}

func (s *ServiceBackendConfig) GetBackendNamespacedName() types.NamespacedName {
	return k8s.NamespacedName(s.service)
}

func (s *ServiceBackendConfig) GetTargetGroupProps() *elbv2gw.TargetGroupProps {
	return s.targetGroupProps
}

var _ TargetGroupConfigurator = &ServiceBackendConfig{}
