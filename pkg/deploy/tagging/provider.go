package tagging

import (
	"fmt"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"strings"
)

// an abstraction that can tag stack resources.
type Provider interface {
	// ResourceIDTagKey provide the tagKey for resourceID.
	ResourceIDTagKey() string

	// StackTags provide the tags for stack.
	StackTags(stack core.Stack) map[string]string

	// ResourceTags provide the tags for stack resources
	ResourceTags(stack core.Stack, res core.Resource, additionalTags map[string]string) map[string]string

	// StackLabels provide the suitable k8s labels for stack.
	StackLabels(stack core.Stack) map[string]string
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
	return p.prefixedTagKey("resource")
}

func (p *defaultProvider) StackTags(stack core.Stack) map[string]string {
	return map[string]string{
		p.prefixedTagKey("cluster"): p.clusterName,
		p.prefixedTagKey("stack"):   stack.StackID(),
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
	normalizedStackID := strings.ReplaceAll(stack.StackID(), "/", "_")
	return map[string]string{
		p.prefixedTagKey("stack"): normalizedStackID,
	}
}

func (p *defaultProvider) prefixedTagKey(tag string) string {
	return fmt.Sprintf("%v/%v", p.tagPrefix, tag)
}
