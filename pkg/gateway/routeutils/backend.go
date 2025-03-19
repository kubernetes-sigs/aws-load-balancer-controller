package routeutils

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Backend struct {
	Service     *corev1.Service
	ServicePort *corev1.ServicePort
	Weight      int
	// Add TG config here //
}

func commonBackendLoader(ctx context.Context, k8sClient client.Client, backendRef gwv1.BackendRef, routeIdentifier types.NamespacedName, routeKind string) (*Backend, error) {
	if backendRef.Port == nil {
		return nil, errors.Errorf("Missing port in backend reference")
	}
	var namespace string
	if backendRef.Namespace == nil {
		namespace = routeIdentifier.Namespace
	} else {
		namespace = string(*backendRef.Namespace)
	}

	svcName := types.NamespacedName{
		Namespace: namespace,
		Name:      string(backendRef.Name),
	}
	svc := &corev1.Service{}
	err := k8sClient.Get(ctx, svcName, svc)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Unable to fetch svc object %+v", svcName))
	}

	// TODO -- Is this correct?
	var servicePort *corev1.ServicePort

	for _, svcPort := range svc.Spec.Ports {
		if svcPort.Port == int32(*backendRef.Port) {
			servicePort = &svcPort
			break
		}
	}

	if servicePort == nil {
		return nil, errors.Errorf("Unable to find service port for port %d", *backendRef.Port)
	}

	// look up target group config here

	// Weight specifies the proportion of requests forwarded to the referenced
	// backend. This is computed as weight/(sum of all weights in this
	// BackendRefs list). For non-zero values, there may be some epsilon from
	// the exact proportion defined here depending on the precision an
	// implementation supports. Weight is not a percentage and the sum of
	// weights does not need to equal 100.
	//
	// If only one backend is specified and it has a weight greater than 0, 100%
	// of the traffic is forwarded to that backend. If weight is set to 0, no
	// traffic should be forwarded for this entry. If unspecified, weight
	// defaults to 1.
	weight := 1
	if backendRef.Weight != nil {
		weight = int(*backendRef.Weight)
	}
	return &Backend{
		Service:     svc,
		ServicePort: servicePort,
		Weight:      weight,
	}, nil
}
