package manifest

import (
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewIngressBuilder constructs a builder that capable to build manifest for an Ingress.
func NewIngressBuilder() *ingressBuilder {
	return &ingressBuilder{
		ingClassName:   nil,
		ingAnnotations: nil,
		ingRules:       nil,
		httpRoutes:     make(map[string][]networking.HTTPIngressPath),
	}
}

type ingressBuilder struct {
	ingClassName   *string
	ingAnnotations map[string]string

	ingRules   []networking.IngressRule
	httpRoutes map[string][]networking.HTTPIngressPath
}

func (b *ingressBuilder) WithIngressClassName(ingClassName string) *ingressBuilder {
	b.ingClassName = &ingClassName
	return b
}

func (b *ingressBuilder) WithAnnotations(ingAnnotations map[string]string) *ingressBuilder {
	b.ingAnnotations = ingAnnotations
	return b
}

func (b *ingressBuilder) WithIngressRules(rules []networking.IngressRule) *ingressBuilder {
	b.ingRules = rules
	return b
}

func (b *ingressBuilder) AddHTTPRoute(host string, route networking.HTTPIngressPath) *ingressBuilder {
	b.httpRoutes[host] = append(b.httpRoutes[host], route)
	return b
}

func (b *ingressBuilder) Build(namespace string, name string) *networking.Ingress {
	ing := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			Annotations: b.ingAnnotations,
		},
		Spec: networking.IngressSpec{
			IngressClassName: b.ingClassName,
			Rules:            b.buildIngressRules(),
		},
	}
	return ing
}

func (b *ingressBuilder) buildIngressRules() []networking.IngressRule {
	if len(b.ingRules) != 0 {
		return b.ingRules
	}
	for host, routes := range b.httpRoutes {
		ingRule := networking.IngressRule{
			Host: host,
			IngressRuleValue: networking.IngressRuleValue{
				HTTP: &networking.HTTPIngressRuleValue{},
			},
		}
		for _, route := range routes {
			ingRule.HTTP.Paths = append(ingRule.HTTP.Paths, route)
		}
		b.ingRules = append(b.ingRules, ingRule)
	}
	return b.ingRules
}
