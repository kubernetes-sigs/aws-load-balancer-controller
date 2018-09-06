package backend_test

import (
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/backend"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"

	api_v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestResolve(t *testing.T) {
	for _, tc := range []struct {
		ingress    *extensions.Ingress
		service    *api_v1.Service
		targetType string
	}{
		{
			ingress: &extensions.Ingress{
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8080),
					},
				},
			},
			service: &api_v1.Service{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "service",
					Namespace: api_v1.NamespaceDefault,
				},
				Spec: api_v1.ServiceSpec{
					Type: api_v1.ServiceTypeNodePort,
					Ports: []api_v1.ServicePort{
						{Port: 8080},
					},
				},
			},
			targetType: "instance",
		},
	} {
		store := store.NewDummy()
		store.GetServiceFunc = func(_ string) (*api_v1.Service, error) { return tc.service, nil }
		resolver := backend.NewEndpointResolver(store, tc.targetType)
		resolver.Resolve(tc.ingress, tc.ingress.Spec.Backend)
	}
}
