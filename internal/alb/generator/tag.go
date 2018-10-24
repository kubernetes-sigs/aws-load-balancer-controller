package generator

import (
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/lb"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
)

var _ tg.TagGenerator = (*TagGenerator)(nil)
var _ lb.TagGenerator = (*TagGenerator)(nil)

type TagGenerator struct {
	ClusterName string
}

func (gen *TagGenerator) TagLB(namespace string, ingressName string) map[string]string {
	return gen.tagIngressResources(namespace, ingressName)
}

func (gen *TagGenerator) TagTGGroup(namespace string, ingressName string) map[string]string {
	return gen.tagIngressResources(namespace, ingressName)
}

func (gen *TagGenerator) TagTG(serviceName string, servicePort string) map[string]string {
	return map[string]string{
		tags.ServiceName: serviceName,
		tags.ServicePort: servicePort,
	}
}

func (gen *TagGenerator) tagIngressResources(namespace string, ingressName string) map[string]string {
	m := make(map[string]string)
	m["kubernetes.io/cluster/"+gen.ClusterName] = "owned"
	m[tags.Namespace] = namespace
	m[tags.IngressName] = ingressName
	return m
}
