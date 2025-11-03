package routeutils

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ TargetGroupConfigurator = &GatewayBackendConfig{}

type GatewayBackendConfig struct {
	gateway          *gwv1.Gateway
	targetGroupProps *elbv2gw.TargetGroupProps
	arn              string
	port             int32
}

func NewGatewayBackendConfig(gateway *gwv1.Gateway, targetGroupProps *elbv2gw.TargetGroupProps, arn string, port int32) *GatewayBackendConfig {
	return &GatewayBackendConfig{
		gateway:          gateway,
		targetGroupProps: targetGroupProps,
		arn:              arn,
		port:             port,
	}
}

func (g *GatewayBackendConfig) GetALBARN() string {
	return g.arn
}

func (g *GatewayBackendConfig) GetTargetType(_ elbv2model.TargetType) elbv2model.TargetType {
	return elbv2model.TargetTypeALB
}

func (g *GatewayBackendConfig) GetTargetGroupProps() *elbv2gw.TargetGroupProps {
	return g.targetGroupProps
}

func (g *GatewayBackendConfig) GetBackendNamespacedName() types.NamespacedName {
	return k8s.NamespacedName(g.gateway)
}

func (g *GatewayBackendConfig) GetIdentifierPort() intstr.IntOrString {
	return intstr.FromInt32(g.port)
}

// GetExternalTrafficPolicy doesn't really apply to this backend type, so we return the most permissive type.
func (g *GatewayBackendConfig) GetExternalTrafficPolicy() corev1.ServiceExternalTrafficPolicyType {
	return corev1.ServiceExternalTrafficPolicyTypeCluster
}

// GetIPAddressType Gateway based backends always communicate over IPv4.
func (g *GatewayBackendConfig) GetIPAddressType() elbv2model.TargetGroupIPAddressType {
	return elbv2model.TargetGroupIPAddressTypeIPv4
}

// GetTargetGroupPort Gateway based backends always forward traffic to the Gateway listener.
func (g *GatewayBackendConfig) GetTargetGroupPort(_ elbv2model.TargetType) int32 {
	return g.port
}

func (g *GatewayBackendConfig) GetHealthCheckPort(_ elbv2model.TargetType, _ bool) (intstr.IntOrString, error) {
	portConfigNotExist := g.targetGroupProps == nil || g.targetGroupProps.HealthCheckConfig == nil || g.targetGroupProps.HealthCheckConfig.HealthCheckPort == nil

	if portConfigNotExist || *g.targetGroupProps.HealthCheckConfig.HealthCheckPort == shared_constants.HealthCheckPortTrafficPort {
		return intstr.FromString(shared_constants.HealthCheckPortTrafficPort), nil
	}

	return intstr.FromInt32(g.port), nil
}

func gatewayLoader(ctx context.Context, k8sClient client.Client, routeIdentifier types.NamespacedName, routeKind RouteKind, backendRef gwv1.BackendRef) (*GatewayBackendConfig, error, error) {
	if backendRef.Port == nil {
		initialErrorMessage := "Port is required"
		wrappedGatewayErrorMessage := generateInvalidMessageWithRouteDetails(initialErrorMessage, routeKind, routeIdentifier)
		return nil, wrapError(errors.Errorf("%s", initialErrorMessage), gwv1.GatewayReasonListenersNotValid, gwv1.RouteReasonUnsupportedValue, &wrappedGatewayErrorMessage, nil), nil
	}

	var gwNamespace string
	if backendRef.Namespace == nil {
		gwNamespace = routeIdentifier.Namespace
	} else {
		gwNamespace = string(*backendRef.Namespace)
	}

	gwIdentifier := types.NamespacedName{
		Namespace: gwNamespace,
		Name:      string(backendRef.Name),
	}

	// Check for reference grant when performing cross namespace gateway -> route attachment
	if gwIdentifier.Namespace != routeIdentifier.Namespace {
		allowed, err := referenceGrantCheck(ctx, k8sClient, gatewayKind, gwIdentifier, routeIdentifier, routeKind)
		if err != nil {
			// Currently, this API only fails for a k8s related error message, hence no status update + make the error fatal.
			return nil, nil, errors.Wrapf(err, "Unable to perform reference grant check")
		}

		// We should not give any hints about the existence of this resource, therefore, we return nil.
		// That way, users can't infer if the route is missing because of a misconfigured gateway reference
		// or the sentence grant is not allowing the connection.
		if !allowed {
			wrappedGatewayErrorMessage := generateInvalidMessageWithRouteDetails(referenceGrantNotExists, routeKind, routeIdentifier)
			return nil, wrapError(errors.Errorf("%s", referenceGrantNotExists), gwv1.GatewayReasonListenersNotValid, gwv1.RouteReasonRefNotPermitted, &wrappedGatewayErrorMessage, nil), nil
		}
	}

	gw := &gwv1.Gateway{}
	err := k8sClient.Get(ctx, gwIdentifier, gw)
	if err != nil {

		convertToNotFoundError := client.IgnoreNotFound(err)

		if convertToNotFoundError == nil {
			// Svc not found, post an updated status.
			initialErrorMessage := fmt.Sprintf("Gateway (%s:%s) not found)", gwIdentifier.Namespace, gwIdentifier.Name)
			wrappedGatewayErrorMessage := generateInvalidMessageWithRouteDetails(initialErrorMessage, routeKind, routeIdentifier)
			return nil, wrapError(errors.Errorf("%s", initialErrorMessage), gwv1.GatewayReasonListenersNotValid, gwv1.RouteReasonBackendNotFound, &wrappedGatewayErrorMessage, nil), nil
		}
		// Otherwise, general error. No need for status update.
		return nil, nil, errors.Wrap(err, fmt.Sprintf("Unable to fetch gw object %+v", gw))
	}

	tgConfig, err := LookUpTargetGroupConfiguration(ctx, k8sClient, gatewayKind, k8s.NamespacedName(gw))

	if err != nil {
		// As of right now, this error can only be thrown because of a k8s api error hence no status update.
		return nil, nil, errors.Wrap(err, fmt.Sprintf("Unable to fetch tg config object"))
	}

	var tgProps *elbv2gw.TargetGroupProps

	if tgConfig != nil {
		tgProps = tgConfigConstructor.ConstructTargetGroupConfigForRoute(tgConfig, routeIdentifier.Name, routeIdentifier.Namespace, string(routeKind))
	}

	var arn string

	// Find the ALB ARN within the Gateway Programmed Condition, the controller will always embed the ARN there.
	for _, cond := range gw.Status.Conditions {
		if cond.Type == string(gwv1.GatewayConditionProgrammed) {
			if cond.Status == metav1.ConditionTrue {
				arn = cond.Message
				break
			}
		}
	}

	if arn == "" {
		// If the ARN is not available, then the backend is not yet usable.
		initialErrorMessage := fmt.Sprintf("Gateway (%s:%s) is not usable yet, LB ARN is not provisioned)", gwIdentifier.Namespace, gwIdentifier.Name)
		wrappedGatewayErrorMessage := generateInvalidMessageWithRouteDetails(initialErrorMessage, routeKind, routeIdentifier)
		return nil, wrapError(errors.Errorf("%s", initialErrorMessage), gwv1.GatewayReasonListenersNotValid, gwv1.RouteReasonBackendNotFound, &wrappedGatewayErrorMessage, nil), nil
	}

	return NewGatewayBackendConfig(gw, tgProps, arn, int32(*backendRef.Port)), nil, nil
}
