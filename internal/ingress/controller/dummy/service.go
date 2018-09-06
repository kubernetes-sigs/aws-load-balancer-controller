package dummy

import (
	api "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewService creates a dummy service which is corresponding to dummy.NewIngress
func NewService() *api.Service {
	return &api.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "service1",
			Namespace: api.NamespaceDefault,
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeNodePort,
			Ports: []api.ServicePort{
				{Port: 80},
				{Port: 30001},
				{Port: 30002},
				{Port: 30003},
			},
		},
	}
}
