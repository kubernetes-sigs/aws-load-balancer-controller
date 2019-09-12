package generator

import (
	"fmt"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/lb"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/sg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
)

// Standard tag key names
const (
	TagKeyClusterName = "kubernetes.io/cluster-name"
	TagKeyNamespace   = "kubernetes.io/namespace"
	TagKeyIngressName = "kubernetes.io/ingress-name"
	TagKeyServiceName = "kubernetes.io/service-name"
	TagKeyServicePort = "kubernetes.io/service-port"
)

// Additional Tags used to be forward-compatible with V2 version.
const (
	V2TagKeyClusterID  = "ingress.k8s.aws/cluster"
	V2TagKeyStackID    = "ingress.k8s.aws/stack"
	V2TagKeyResourceID = "ingress.k8s.aws/resource"

	V2ResourceIDLoadBalancer           = "LoadBalancer"
	V2ResourceIDManagedLBSecurityGroup = "ManagedLBSecurityGroup"
)

var _ tg.TagGenerator = (*TagGenerator)(nil)
var _ lb.TagGenerator = (*TagGenerator)(nil)
var _ sg.TagGenerator = (*TagGenerator)(nil)

type TagGenerator struct {
	ClusterName string
	DefaultTags map[string]string
}

func (gen *TagGenerator) TagLB(namespace string, ingressName string) map[string]string {
	resTags := gen.tagIngressResources(namespace, ingressName)
	resTags[V2TagKeyResourceID] = V2ResourceIDLoadBalancer
	return resTags
}

func (gen *TagGenerator) TagTGGroup(namespace string, ingressName string) map[string]string {
	return gen.tagIngressResources(namespace, ingressName)
}

func (gen *TagGenerator) TagTG(namespace string, ingressName string, serviceName string, servicePort string) map[string]string {
	resTags := map[string]string{
		TagKeyServiceName: serviceName,
		TagKeyServicePort: servicePort,
	}
	resID := gen.buildV2TargetGroupID(namespace, ingressName, serviceName, servicePort)
	resTags[V2TagKeyResourceID] = resID
	return resTags
}

func (gen *TagGenerator) TagLBSG(namespace string, ingressName string) map[string]string {
	resTags := gen.tagSGs(namespace, ingressName)
	resTags[V2TagKeyResourceID] = V2ResourceIDManagedLBSecurityGroup
	return resTags
}

func (gen *TagGenerator) TagInstanceSG(namespace string, ingressName string) map[string]string {
	return gen.tagSGs(namespace, ingressName)
}

func (gen *TagGenerator) tagIngressResources(namespace string, ingressName string) map[string]string {
	m := make(map[string]string)
	for label, value := range gen.DefaultTags {
		m[label] = value
	}
	m["kubernetes.io/cluster/"+gen.ClusterName] = "owned"
	m[TagKeyNamespace] = namespace
	m[TagKeyIngressName] = ingressName

	v2Tags := gen.tagIngressResourcesV2(namespace, ingressName)
	for label, value := range v2Tags {
		m[label] = value
	}
	return m
}

// Tagging for sg is optional since ingress controller used name to resolve tags, but it will be required when
// * add support to clean up aws resources created by ingress controller
// * add support for sharing instance securityGroup among ingresses.

func (gen *TagGenerator) tagSGs(namespace string, ingressName string) map[string]string {
	m := make(map[string]string)
	for label, value := range gen.DefaultTags {
		m[label] = value
	}
	// To avoid conflict with core k8s, we don't tag SGs with `kubernetes.io/cluster/clusterName` since
	// core k8s currently used `kubernetes.io/cluster/clusterName` tag to identify tags for service with Type LoadBalancer.
	// see https://github.com/kubernetes/kubernetes/blob/e056703ea7474990f5d7c58813082065543187eb/pkg/cloudprovider/providers/aws/aws.go#L3768
	// A more sensible approach in the future should be change the out-of-tree cloud-provider-aws for more advanced SG discovery mechanism.
	// we can do it when out-of-tree cloud-provider-aws is stable.
	m[TagKeyClusterName] = gen.ClusterName

	m[TagKeyNamespace] = namespace
	m[TagKeyIngressName] = ingressName

	v2Tags := gen.tagIngressResourcesV2(namespace, ingressName)
	for label, value := range v2Tags {
		m[label] = value
	}

	return m
}

func (gen *TagGenerator) tagIngressResourcesV2(namespace string, ingressName string) map[string]string {
	m := make(map[string]string)
	stackID := gen.buildV2StackID(namespace, ingressName)
	m[V2TagKeyClusterID] = gen.ClusterName
	m[V2TagKeyStackID] = stackID
	return m
}

// buildV2StackID returns the stack ID that compatible with V2 version.
func (gen *TagGenerator) buildV2StackID(namespace string, ingressName string) string {
	return fmt.Sprintf("%s/%s", namespace, ingressName)
}

// buildV2TargetGroupID returns the targetGroup ResourceID that compatible with V2 version.
func (gen *TagGenerator) buildV2TargetGroupID(namespace string, ingressName string, serviceName string, servicePort string) string {
	return fmt.Sprintf("%s/%s-%s:%s", namespace, ingressName, serviceName, servicePort)
}
