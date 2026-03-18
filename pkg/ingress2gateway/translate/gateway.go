package translate

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	gwconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	sharedconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// buildGatewayClass returns the static ALB GatewayClass.
func buildGatewayClass() gwv1.GatewayClass {
	return gwv1.GatewayClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwconstants.GatewayResourceGroupVersion,
			Kind:       utils.GatewayClassKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.GatewayClassName,
		},
		Spec: gwv1.GatewayClassSpec{
			ControllerName: gwv1.GatewayController(gwconstants.ALBGatewayController),
		},
	}
}

// buildGateway builds a Gateway resource from listen-ports.
// If lbConfig is non-nil, the Gateway's infrastructure.parametersRef points to it.
func buildGateway(name, namespace string, lbConfig *gatewayv1beta1.LoadBalancerConfiguration, listenPorts []listenPortEntry) gwv1.Gateway {
	var listeners []gwv1.Listener
	for _, lp := range listenPorts {
		listeners = append(listeners, gwv1.Listener{
			Name:     gwv1.SectionName(utils.GetListenerName(lp.Protocol, lp.Port)),
			Port:     gwv1.PortNumber(lp.Port),
			Protocol: toALBProtocol(lp.Protocol),
		})
	}

	gw := gwv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwconstants.GatewayResourceGroupVersion,
			Kind:       sharedconstants.GatewayApiKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: gwv1.ObjectName(utils.GatewayClassName),
			Listeners:        listeners,
		},
	}

	if lbConfig != nil {
		group := gwv1.Group(gwconstants.ControllerCRDGroupVersion)
		kind := gwv1.Kind(gwconstants.LoadBalancerConfiguration)
		gw.Spec.Infrastructure = &gwv1.GatewayInfrastructure{
			ParametersRef: &gwv1.LocalParametersReference{
				Group: group,
				Kind:  kind,
				Name:  lbConfig.Name,
			},
		}
	}

	return gw
}

// toALBProtocol converts an Ingress listen-ports protocol string to a Gateway API ProtocolType.
// For ALB, only HTTP and HTTPS are valid.
func toALBProtocol(proto string) gwv1.ProtocolType {
	switch proto {
	case "HTTPS":
		return gwv1.HTTPSProtocolType
	case "HTTP":
		return gwv1.HTTPProtocolType
	default:
		return gwv1.HTTPProtocolType
	}
}
