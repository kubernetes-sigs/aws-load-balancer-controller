package ingress

import (
	"context"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/annotations"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/aws/services"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/networking"
)

// ModelBuilder is responsible for build mode stack for a IngressGroup.
type ModelBuilder interface {
	// build mode stack for a IngressGroup.
	Build(ctx context.Context, ingGroup Group) (core.Stack, *elbv2model.LoadBalancer, error)
}

var _ ModelBuilder = &defaultModelBuilder{}

// default implementation for ModelBuilder
type defaultModelBuilder struct {
	annotationParser annotations.Parser
	subnetsResolver  networking.SubnetsResolver
	ec2Client        services.EC2
	vpcID            string
	clusterName      string

	defaultIPAddressType elbv2model.IPAddressType
	defaultScheme        elbv2model.LoadBalancerScheme
}

// build mode stack for a IngressGroup.
func (b *defaultModelBuilder) Build(ctx context.Context, ingGroup Group) (core.Stack, *elbv2model.LoadBalancer, error) {
	stack := core.NewDefaultStack(ingGroup.ID.String())
	lb, err := b.buildLoadBalancer(ctx, stack, ingGroup)
	if err != nil {
		return nil, nil, err
	}
	return stack, lb, nil
}
