package generator

import (
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/lb"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/sg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
)

// Standard tag key names
const (
	TagKeyNamespace   = "kubernetes.io/namespace"
	TagKeyIngressName = "kubernetes.io/ingress-name"
	TagKeyServiceName = "kubernetes.io/service-name"
	TagKeyServicePort = "kubernetes.io/service-port"
)

var _ tg.TagGenerator = (*TagGenerator)(nil)
var _ lb.TagGenerator = (*TagGenerator)(nil)
var _ sg.TagGenerator = (*TagGenerator)(nil)

type TagGenerator struct {
	ClusterName string
	DefaultTags map[string]string
}

func (gen *TagGenerator) TagLB(namespace string, ingressName string) map[string]string {
	return gen.tagIngressResources(namespace, ingressName)
}

func (gen *TagGenerator) TagTGGroup(namespace string, ingressName string) map[string]string {
	return gen.tagIngressResources(namespace, ingressName)
}

func (gen *TagGenerator) TagTG(serviceName string, servicePort string) map[string]string {
	return map[string]string{
		TagKeyServiceName: serviceName,
		TagKeyServicePort: servicePort,
	}
}

func (gen *TagGenerator) TagLBSG(namespace string, ingressName string) map[string]string {
	return gen.tagIngressResources(namespace, ingressName)
}

func (gen *TagGenerator) TagInstanceSG(namespace string, ingressName string) map[string]string {
	return gen.tagIngressResources(namespace, ingressName)
}

func (gen *TagGenerator) tagIngressResources(namespace string, ingressName string) map[string]string {
	m := make(map[string]string)
	for label, value := range gen.DefaultTags {
		m[label] = value
	}
	m["kubernetes.io/cluster/"+gen.ClusterName] = "owned"
	m[TagKeyNamespace] = namespace
	m[TagKeyIngressName] = ingressName
	return m
}
