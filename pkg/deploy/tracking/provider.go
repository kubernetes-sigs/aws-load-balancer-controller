package tracking

import (
	"fmt"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

//we use AWS tags and K8s labels to track resources we have created.
//
//For AWS resources created by this controller, the tagging strategy is as follows:
//  * `elbv2.k8s.aws/cluster: cluster-name` will be applied on all AWS resources.
//  * `ingress.k8s.aws/stack: stack-id` will be applied on all AWS resources provisioned for Ingress resources:
//    * For explicit IngressGroup, `stack-id` will be `groupName`
//    * For implicit IngressGroup, `stack-id` will be `namespace/ingressName`
//  * `ingress.k8s.aws/resource: resource-id` will be applied on all AWS resources provisioned for Ingress resources:
//    * For LoadBalancer, `resource-id` will be `LoadBalancer`
//    * For Managed LB SecurityGroup, `resource-id` will be `ManagedLBSecurityGroup`
//    * For TargetGroup, `resource-id` will be `namespace/ingressName-serviceName:servicePort`
//  * `service.k8s.aws/stack: stack-id` will be applied on all AWS resources provisioned for Service resources:
//    * `stack-id` will be `namespace/serviceName`
//  * `service.k8s.aws/resource: resource-id` will be applied on all AWS resources provisioned for Service resources:
//    * For LoadBalancer, `resource-id` will be `LoadBalancer`
//    * For TargetGroup, `resource-id` will be `namespace/serviceName:servicePort`
//For K8s resources created by this controller, the labelling strategy is as follows:
//  * For explicit IngressGroup, the following tags will be applied on all K8s resources:
//    * `ingress.k8s.aws/stack: groupName`
//  * For implicit IngressGroup, the following tags will be applied on all K8s resources:
//    * `ingress.k8s.aws/stack-namespace: namespace`
//    * `ingress.k8s.aws/stack-name: ingressName`
//  * For Service, the following tags will be applied on all K8s resources:
//    * `service.k8s.aws/stack-namespace: namespace`
//    * `service.k8s.aws/stack-name: serviceName`

// AWS TagKey for cluster resources.
const clusterNameTagKey = "elbv2.k8s.aws/cluster"

// Legacy AWS TagKey for cluster resources, which is used by AWSALBIngressController(v1.1.3+)
const clusterNameTagKeyLegacy = "ingress.k8s.aws/cluster"

// an abstraction that generates metadata to track actual resources provisioned for stack.
type Provider interface {
	// ResourceIDTagKey provide the tagKey for resourceID.
	ResourceIDTagKey() string

	// StackTags provide the tags for stack.
	StackTags(stack core.Stack) map[string]string

	// ResourceTags provide the tags for stack resources
	ResourceTags(stack core.Stack, res core.Resource, additionalTags map[string]string) map[string]string

	// StackLabels provide the suitable k8s labels for stack.
	StackLabels(stack core.Stack) map[string]string

	// StackTagsLegacy provides the tags for stack with legacy clusterName.
	// this is for backwards compatibility with AWSALBIngressController(v1.1.3+)
	StackTagsLegacy(stack core.Stack) map[string]string

	// LegacyTagKeys returns AWS tag keys added to AWS resources provisioned by AWSALBIngressController(v1.1.3+).
	// These tag keys is required for AWSALBIngressController(v1.1.3+) to identify resources.
	// To be able to downgrade AWSLoadBalancerController to AWSALBIngressController(v1.1.3+), we shouldn't remove these tag keys.
	LegacyTagKeys() []string
}

// NewDefaultProvider constructs defaultProvider
func NewDefaultProvider(tagPrefix string, clusterName string) *defaultProvider {
	return &defaultProvider{
		tagPrefix:   tagPrefix,
		clusterName: clusterName,
	}
}

var _ Provider = &defaultProvider{}

// defaultImplementation for Provider
type defaultProvider struct {
	tagPrefix   string
	clusterName string
}

func (p *defaultProvider) ResourceIDTagKey() string {
	return p.prefixedTrackingKey("resource")
}

func (p *defaultProvider) StackTags(stack core.Stack) map[string]string {
	stackID := stack.StackID()
	return map[string]string{
		clusterNameTagKey:              p.clusterName,
		p.prefixedTrackingKey("stack"): stackID.String(),
	}
}

func (p *defaultProvider) ResourceTags(stack core.Stack, res core.Resource, additionalTags map[string]string) map[string]string {
	stackTags := p.StackTags(stack)
	resourceIDTags := map[string]string{
		p.ResourceIDTagKey(): res.ID(),
	}
	return algorithm.MergeStringMap(stackTags, resourceIDTags, additionalTags)
}

func (p *defaultProvider) StackLabels(stack core.Stack) map[string]string {
	stackID := stack.StackID()
	if stackID.Namespace == "" {
		return map[string]string{
			p.prefixedTrackingKey("stack"): stackID.Name,
		}
	}
	return map[string]string{
		p.prefixedTrackingKey("stack-namespace"): stackID.Namespace,
		p.prefixedTrackingKey("stack-name"):      stackID.Name,
	}
}

func (p *defaultProvider) StackTagsLegacy(stack core.Stack) map[string]string {
	stackID := stack.StackID()
	return map[string]string{
		clusterNameTagKeyLegacy:        p.clusterName,
		p.prefixedTrackingKey("stack"): stackID.String(),
	}
}

func (p *defaultProvider) LegacyTagKeys() []string {
	return []string{
		fmt.Sprintf("kubernetes.io/cluster/%s", p.clusterName),
		"kubernetes.io/cluster-name",
		"kubernetes.io/namespace",
		"kubernetes.io/ingress-name",
		"kubernetes.io/service-name",
		"kubernetes.io/service-port",
		clusterNameTagKeyLegacy,
	}
}

func (p *defaultProvider) prefixedTrackingKey(tag string) string {
	return fmt.Sprintf("%v/%v", p.tagPrefix, tag)
}
