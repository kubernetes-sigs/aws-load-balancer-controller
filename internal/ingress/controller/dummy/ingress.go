package dummy

import (
	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func NewIngress() *extensions.Ingress {
	ports := []int64{
		int64(80),
		int64(443),
		int64(8080),
	}
	hosts := []string{
		"1.test.domain",
		"2.test.domain",
		"3.test.domain",
	}
	paths := []string{
		"/",
		"/store",
		"/store/dev",
	}
	svcs := []string{
		"1service",
		"2service",
		"3service",
	}
	svcPorts := []int{
		30001,
		30002,
		30003,
	}

	ing := &extensions.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "ingress1",
			Namespace: api.NamespaceDefault,
		},
		Spec: extensions.IngressSpec{
			Backend: &extensions.IngressBackend{
				ServiceName: "default-backend",
				ServicePort: intstr.FromInt(80),
			},
		},
	}
	for i := range ports {
		extRules := extensions.IngressRule{
			Host: hosts[i],
			IngressRuleValue: extensions.IngressRuleValue{
				HTTP: &extensions.HTTPIngressRuleValue{
					Paths: []extensions.HTTPIngressPath{{
						Path: paths[i],
						Backend: extensions.IngressBackend{
							ServiceName: svcs[i],
							ServicePort: intstr.FromInt(svcPorts[i]),
						},
					},
					},
				},
			},
		}
		ing.Spec.Rules = append(ing.Spec.Rules, extRules)
	}
	return ing
}
