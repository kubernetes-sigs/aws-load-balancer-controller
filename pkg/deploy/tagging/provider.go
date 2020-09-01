package tagging

import (
	"fmt"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
)

// an abstraction that can tag stack resources.
type Provider interface {
	// ResourceIDTagKey provide the tagKey for resourceID.
	ResourceIDTagKey() string

	// StackTags provide the tags for stack.
	StackTags(stack core.Stack) map[string]string
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

func (p *defaultProvider) prefixedTagKey(tag string) string {
	return fmt.Sprintf("%v/%v", p.tagPrefix, tag)
}
