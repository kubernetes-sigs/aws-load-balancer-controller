package routeutils

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwbeta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	serviceKind = "Service"
)

// Backend an abstraction on the Gateway Backend, meant to hide the underlying backend type from consumers (unless they really want to see it :))
type Backend struct {
	Service                *corev1.Service
	ELBv2TargetGroupConfig *elbv2gw.TargetGroupConfiguration
	ServicePort            *corev1.ServicePort
	TypeSpecificBackend    interface{}
	Weight                 int
}

// commonBackendLoader this function will load the services and target group configurations associated with this gateway backend.
func commonBackendLoader(ctx context.Context, k8sClient client.Client, typeSpecificBackend interface{}, backendRef gwv1.BackendRef, routeIdentifier types.NamespacedName, routeKind RouteKind) (*Backend, error) {

	// We only support references of type service.
	if backendRef.Kind != nil && *backendRef.Kind != "Service" {
		return nil, nil
	}

	if backendRef.Weight != nil && *backendRef.Weight == 0 {
		return nil, nil
	}

	if backendRef.Port == nil {
		return nil, errors.Errorf("Missing port in backend reference")
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

	// Check for reference grant when performing crossname gateway -> route attachment
	if svcNamespace != routeIdentifier.Namespace {
		allowed, err := referenceGrantCheck(ctx, k8sClient, svcIdentifier, routeIdentifier, routeKind)
		if err != nil {
			return nil, errors.Wrapf(err, "Unable to perform reference grant check")
		}

		// We should not give any hints about the existence of this resource, therefore, we return nil.
		// That way, users can't infer if the route is missing because of a misconfigured service reference
		// or the sentence grant is not allowing the connection.
		if !allowed {
			return nil, nil
		}
	}

	svc := &corev1.Service{}
	err := k8sClient.Get(ctx, svcIdentifier, svc)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Unable to fetch svc object %+v", svcIdentifier))
	}

	var servicePort *corev1.ServicePort

	for _, svcPort := range svc.Spec.Ports {
		if svcPort.Port == int32(*backendRef.Port) {
			servicePort = &svcPort
			break
		}
	}

	tgConfig, err := lookUpTargetGroupConfiguration(ctx, k8sClient, k8s.NamespacedName(svc))

	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Unable to fetch tg config object"))
	}

	if servicePort == nil {
		return nil, errors.Errorf("Unable to find service port for port %d", *backendRef.Port)
	}

	// Weight specifies the proportion of requests forwarded to the referenced
	// backend. This is computed as weight/(sum of all weights in this
	// BackendRefs list). For non-zero values, there may be some epsilon from
	// the exact proportion defined here depending on the precision an
	// implementation supports. Weight is not a percentage and the sum of
	// weights does not need to equal 100.
	//
	// If only one backend is specified, and it has a weight greater than 0, 100%
	// of the traffic is forwarded to that backend. If weight is set to 0, no
	// traffic should be forwarded for this entry. If unspecified, weight
	// defaults to 1.
	weight := 1
	if backendRef.Weight != nil {
		weight = int(*backendRef.Weight)
	}
	return &Backend{
		Service:                svc,
		ServicePort:            servicePort,
		Weight:                 weight,
		TypeSpecificBackend:    typeSpecificBackend,
		ELBv2TargetGroupConfig: tgConfig,
	}, nil
}

// lookUpTargetGroupConfiguration given a service, lookup the target group configuration associated with the service.
// recall that target group configuration always lives within the same namespace as the service.
func lookUpTargetGroupConfiguration(ctx context.Context, k8sClient client.Client, serviceMetadata types.NamespacedName) (*elbv2gw.TargetGroupConfiguration, error) {
	tgConfigList := &elbv2gw.TargetGroupConfigurationList{}

	// TODO - Add index
	if err := k8sClient.List(ctx, tgConfigList, client.InNamespace(serviceMetadata.Namespace)); err != nil {
		return nil, err
	}

	for _, tgConfig := range tgConfigList.Items {
		if tgConfig.Spec.TargetReference.Kind != nil && *tgConfig.Spec.TargetReference.Kind != serviceKind {
			continue
		}

		// TODO - Add a webhook to validate that only one target group config references this service.
		// TODO - Add an index for this
		if tgConfig.Spec.TargetReference.Name == serviceMetadata.Name {
			return &tgConfig, nil
		}
	}
	return nil, nil
}

// Implements the reference grant API
// https://gateway-api.sigs.k8s.io/api-types/referencegrant/
func referenceGrantCheck(ctx context.Context, k8sClient client.Client, svcIdentifier types.NamespacedName, routeIdentifier types.NamespacedName, routeKind RouteKind) (bool, error) {
	referenceGrantList := &gwbeta1.ReferenceGrantList{}
	if err := k8sClient.List(ctx, referenceGrantList, client.InNamespace(svcIdentifier.Namespace)); err != nil {
		return false, err
	}

	for _, grant := range referenceGrantList.Items {
		var routeAllowed bool

		for _, from := range grant.Spec.From {
			// Kind check maybe?
			if string(from.Kind) == string(routeKind) && string(from.Namespace) == routeIdentifier.Namespace {
				routeAllowed = true
				break
			}
		}

		if routeAllowed {
			for _, to := range grant.Spec.To {
				// As this is a backend reference, we only care about the "Service" Kind.
				if to.Kind != serviceKind {
					continue
				}

				// If name is specified, we need to ensure that svc name matches the "to" name.
				if to.Name != nil && string(*to.Name) != svcIdentifier.Name {
					continue
				}

				return true, nil
			}

		}
	}

	return false, nil
}
