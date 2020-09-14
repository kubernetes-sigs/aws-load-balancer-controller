package ingress

import (
	"context"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/annotations"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/aws/services"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/networking"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	eventWarningConflictSettings = "ConflictSettings"
)

// ModelBuilder is responsible for build mode stack for a IngressGroup.
type ModelBuilder interface {
	// build mode stack for a IngressGroup.
	Build(ctx context.Context, ingGroup Group) (core.Stack, *elbv2model.LoadBalancer, error)
}

// NewDefaultModelBuilder constructs new defaultModelBuilder.
func NewDefaultModelBuilder(k8sClient client.Client, eventRecorder record.EventRecorder, ec2Client services.EC2, vpcID string, clusterName string,
	annotationParser annotations.Parser, subnetsResolver networking.SubnetsResolver,
	authConfigBuilder AuthConfigBuilder, enhancedBackendBuilder EnhancedBackendBuilder) *defaultModelBuilder {
	return &defaultModelBuilder{
		k8sClient:              k8sClient,
		eventRecorder:          eventRecorder,
		ec2Client:              ec2Client,
		vpcID:                  vpcID,
		clusterName:            clusterName,
		annotationParser:       annotationParser,
		subnetsResolver:        subnetsResolver,
		authConfigBuilder:      authConfigBuilder,
		enhancedBackendBuilder: enhancedBackendBuilder,

		defaultIPAddressType:                      elbv2model.IPAddressTypeIPV4,
		defaultScheme:                             elbv2model.LoadBalancerSchemeInternal,
		defaultSSLPolicy:                          "ELBSecurityPolicy-2016-08",
		defaultTargetType:                         elbv2model.TargetTypeInstance,
		defaultBackendProtocol:                    elbv2model.ProtocolHTTP,
		defaultHealthCheckPath:                    "/",
		defaultHealthCheckIntervalSeconds:         15,
		defaultHealthCheckTimeoutSeconds:          5,
		defaultHealthCheckHealthyThresholdCount:   2,
		defaultHealthCheckUnhealthyThresholdCount: 2,
		defaultHealthCheckMatcherHTTPCode:         "200",
	}
}

var _ ModelBuilder = &defaultModelBuilder{}

// default implementation for ModelBuilder
type defaultModelBuilder struct {
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	ec2Client     services.EC2

	vpcID       string
	clusterName string

	annotationParser       annotations.Parser
	subnetsResolver        networking.SubnetsResolver
	authConfigBuilder      AuthConfigBuilder
	enhancedBackendBuilder EnhancedBackendBuilder

	defaultIPAddressType                      elbv2model.IPAddressType
	defaultScheme                             elbv2model.LoadBalancerScheme
	defaultSSLPolicy                          string
	defaultTargetType                         elbv2model.TargetType
	defaultBackendProtocol                    elbv2model.Protocol
	defaultHealthCheckPath                    string
	defaultHealthCheckTimeoutSeconds          int64
	defaultHealthCheckIntervalSeconds         int64
	defaultHealthCheckHealthyThresholdCount   int64
	defaultHealthCheckUnhealthyThresholdCount int64
	defaultHealthCheckMatcherHTTPCode         string
}

// build mode stack for a IngressGroup.
func (b *defaultModelBuilder) Build(ctx context.Context, ingGroup Group) (core.Stack, *elbv2model.LoadBalancer, error) {
	stack := core.NewDefaultStack(ingGroup.ID.String())
	lb, err := b.buildLoadBalancer(ctx, stack, ingGroup)
	if err != nil {
		return nil, nil, err
	}
	if err := b.buildListenerAndListenerRules(ctx, stack, ingGroup, lb.LoadBalancerARN()); err != nil {
		return nil, nil, err
	}
	return stack, lb, nil
}
