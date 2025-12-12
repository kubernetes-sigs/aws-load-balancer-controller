package routeutils

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ServiceBackendConfig struct {
	service          *corev1.Service
	targetGroupProps *elbv2gw.TargetGroupProps
	servicePort      *corev1.ServicePort
}

var _ TargetGroupConfigurator = &ServiceBackendConfig{}

func NewServiceBackendConfig(service *corev1.Service, targetGroupProps *elbv2gw.TargetGroupProps, servicePort *corev1.ServicePort) *ServiceBackendConfig {
	return &ServiceBackendConfig{
		service:          service,
		targetGroupProps: targetGroupProps,
		servicePort:      servicePort,
	}
}

func (s *ServiceBackendConfig) GetTargetType(defaultTargetType elbv2model.TargetType) elbv2model.TargetType {
	if s.targetGroupProps == nil || s.targetGroupProps.TargetType == nil {
		return defaultTargetType
	}

	return elbv2model.TargetType(*s.targetGroupProps.TargetType)
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

var (
	http2 = elbv2model.ProtocolVersionHTTP2
	http1 = elbv2model.ProtocolVersionHTTP1
	grpc  = elbv2model.ProtocolVersionHTTP1
)

func (s *ServiceBackendConfig) GetProtocolVersion() *elbv2model.ProtocolVersion {
	if s.servicePort.AppProtocol == nil {
		return nil
	}

	switch *s.servicePort.AppProtocol {
	case "kubernetes.io/h2c":
		return &http2
	default:
		return nil
	}
}

func serviceLoader(ctx context.Context, k8sClient client.Client, routeIdentifier types.NamespacedName, routeKind RouteKind, backendRef gwv1.BackendRef) (*ServiceBackendConfig, error, error) {
	if backendRef.Port == nil {
		initialErrorMessage := "Port is required"
		wrappedGatewayErrorMessage := generateInvalidMessageWithRouteDetails(initialErrorMessage, routeKind, routeIdentifier)
		return nil, wrapError(errors.Errorf("%s", initialErrorMessage), gwv1.GatewayReasonListenersNotValid, gwv1.RouteReasonUnsupportedValue, &wrappedGatewayErrorMessage, nil), nil
	}

	var svcNamespace string
	if backendRef.Namespace == nil {
		svcNamespace = routeIdentifier.Namespace
	} else {
		svcNamespace = string(*backendRef.Namespace)
	}

	svcIdentifier := types.NamespacedName{
		Namespace: svcNamespace,
		Name:      string(backendRef.Name),
	}

	// Check for reference grant when performing cross namespace gateway -> route attachment
	if svcNamespace != routeIdentifier.Namespace {
		allowed, err := referenceGrantCheck(ctx, k8sClient, serviceKind, coreAPIGroup, svcIdentifier, routeIdentifier, routeKind, gatewayAPIGroup)
		if err != nil {
			// Currently, this API only fails for a k8s related error message, hence no status update + make the error fatal.
			return nil, nil, errors.Wrapf(err, "Unable to perform reference grant check")
		}

		// We should not give any hints about the existence of this resource, therefore, we return nil.
		// That way, users can't infer if the route is missing because of a misconfigured service reference
		// or the sentence grant is not allowing the connection.
		if !allowed {
			wrappedGatewayErrorMessage := generateInvalidMessageWithRouteDetails(referenceGrantNotExists, routeKind, routeIdentifier)
			return nil, wrapError(errors.Errorf("%s", referenceGrantNotExists), gwv1.GatewayReasonListenersNotValid, gwv1.RouteReasonRefNotPermitted, &wrappedGatewayErrorMessage, nil), nil
		}
	}

	svc := &corev1.Service{}
	err := k8sClient.Get(ctx, svcIdentifier, svc)
	if err != nil {

		convertToNotFoundError := client.IgnoreNotFound(err)

		if convertToNotFoundError == nil {
			// Svc not found, post an updated status.
			initialErrorMessage := fmt.Sprintf("Service (%s:%s) not found)", svcIdentifier.Namespace, svcIdentifier.Name)
			wrappedGatewayErrorMessage := generateInvalidMessageWithRouteDetails(initialErrorMessage, routeKind, routeIdentifier)
			return nil, wrapError(errors.Errorf("%s", initialErrorMessage), gwv1.GatewayReasonListenersNotValid, gwv1.RouteReasonBackendNotFound, &wrappedGatewayErrorMessage, nil), nil
		}
		// Otherwise, general error. No need for status update.
		return nil, nil, errors.Wrap(err, fmt.Sprintf("Unable to fetch svc object %+v", svcIdentifier))
	}

	// We just take 1, we don't care about the underlying protocol
	// This works for singular protocols [TCP] or dual protocols [TCP_UDP].
	var servicePort *corev1.ServicePort

	for _, svcPort := range svc.Spec.Ports {
		if svcPort.Port == int32(*backendRef.Port) {
			servicePort = &svcPort
			break
		}
	}

	if servicePort == nil {
		initialErrorMessage := fmt.Sprintf("Unable to find service port for port %d", *backendRef.Port)
		wrappedGatewayErrorMessage := generateInvalidMessageWithRouteDetails(initialErrorMessage, routeKind, routeIdentifier)
		return nil, wrapError(errors.Errorf("%s", initialErrorMessage), gwv1.GatewayReasonListenersNotValid, gwv1.RouteReasonBackendNotFound, &wrappedGatewayErrorMessage, nil), nil
	}

	tgConfig, err := LookUpTargetGroupConfiguration(ctx, k8sClient, serviceKind, k8s.NamespacedName(svc))

	if err != nil {
		// As of right now, this error can only be thrown because of a k8s api error hence no status update.
		return nil, nil, errors.Wrap(err, fmt.Sprintf("Unable to fetch tg config object"))
	}

	var tgProps *elbv2gw.TargetGroupProps

	if tgConfig != nil {
		tgProps = tgConfigConstructor.ConstructTargetGroupConfigForRoute(tgConfig, routeIdentifier.Name, routeIdentifier.Namespace, string(routeKind))
	}

	return NewServiceBackendConfig(svc, tgProps, servicePort), nil, nil
}
