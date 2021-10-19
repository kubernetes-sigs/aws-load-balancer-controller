package elbv2

import (
	"context"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

var _ core.Resource = &TargetGroup{}

// TargetGroup represents a ELBV2 TargetGroup
type TargetGroup struct {
	core.ResourceMeta `json:"-"`

	// desired state of TargetGroup
	Spec TargetGroupSpec `json:"spec"`

	// observed state of TargetGroup
	// +optional
	Status *TargetGroupStatus `json:"status,omitempty"`
}

// NewTargetGroup constructs new TargetGroup resource.
func NewTargetGroup(stack core.Stack, id string, spec TargetGroupSpec) *TargetGroup {
	tg := &TargetGroup{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", id),
		Spec:         spec,
		Status:       nil,
	}
	stack.AddResource(tg)
	return tg
}

// SetStatus sets the TargetGroup's status
func (tg *TargetGroup) SetStatus(status TargetGroupStatus) {
	tg.Status = &status
}

// LoadBalancerARN returns The Amazon Resource Name (ARN) of the target group.
func (tg *TargetGroup) TargetGroupARN() core.StringToken {
	return core.NewResourceFieldStringToken(tg, "status/targetGroupARN",
		func(ctx context.Context, res core.Resource, fieldPath string) (s string, err error) {
			tg := res.(*TargetGroup)
			if tg.Status == nil {
				return "", errors.Errorf("TargetGroup is not fulfilled yet: %v", tg.ID())
			}
			return tg.Status.TargetGroupARN, nil
		},
	)
}

type TargetType string

const (
	TargetTypeInstance TargetType = "instance"
	TargetTypeIP       TargetType = "ip"
)

type TargetGroupIPAddressType string

const (
	TargetGroupIPAddressTypeIPv4 TargetGroupIPAddressType = "ipv4"
	TargetGroupIPAddressTypeIPv6 TargetGroupIPAddressType = "ipv6"
)

// Information to use when checking for a successful response from a target.
type HealthCheckMatcher struct {
	// The HTTP codes.
	HTTPCode *string `json:"httpCode,omitempty"`

	// The gRPC codes
	GRPCCode *string `json:"grpcCode,omitempty"`
}

// Configuration for TargetGroup's HealthCheck.
type TargetGroupHealthCheckConfig struct {
	// The port the load balancer uses when performing health checks on targets.
	// +optional
	Port *intstr.IntOrString `json:"port,omitempty"`

	// The protocol the load balancer uses when performing health checks on targets.
	// +optional
	Protocol *Protocol `json:"protocol,omitempty"`

	// [HTTP/HTTPS health checks] The ping path that is the destination on the targets for health checks.
	// +optional
	Path *string `json:"path,omitempty"`

	// [HTTP/HTTPS health checks] The HTTP codes to use when checking for a successful response from a target.
	// +optional
	Matcher *HealthCheckMatcher `json:"matcher,omitempty"`

	// The approximate amount of time, in seconds, between health checks of an individual target.
	// +optional
	IntervalSeconds *int64 `json:"intervalSeconds,omitempty"`

	// The amount of time, in seconds, during which no response from a target means a failed health check.
	// +optional
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty"`

	// The number of consecutive health checks successes required before considering an unhealthy target healthy.
	// +optional
	HealthyThresholdCount *int64 `json:"healthyThresholdCount,omitempty"`

	// The number of consecutive health check failures required before considering a target unhealthy.
	// +optional
	UnhealthyThresholdCount *int64 `json:"unhealthyThresholdCount,omitempty"`
}

// Specifies a target group attribute.
type TargetGroupAttribute struct {
	// The name of the attribute.
	Key string `json:"key"`

	// The value of the attribute.
	Value string `json:"value"`
}

// TargetGroupSpec defines the observed state of TargetGroup
type TargetGroupSpec struct {
	// The name of the target group.
	Name string `json:"name"`

	// The type of target that you must specify when registering targets with this target group.
	TargetType TargetType `json:"targetType"`

	// The port on which the targets receive traffic.
	Port int64 `json:"port"`

	// The protocol to use for routing traffic to the targets.
	Protocol Protocol `json:"protocol"`

	// The target group protocol version.
	// +optional
	ProtocolVersion *ProtocolVersion `json:"protocolVersion,omitempty"`

	// Target group IP address type IPv4 or IPv6
	// +optional
	IPAddressType *TargetGroupIPAddressType `json:"ipAddressType,omitempty"`

	// Configuration for TargetGroup's HealthCheck.
	// +optional
	HealthCheckConfig *TargetGroupHealthCheckConfig `json:"healthCheckConfig,omitempty"`

	// The target group attributes.
	// +optional
	TargetGroupAttributes []TargetGroupAttribute `json:"targetGroupAttributes,omitempty"`

	// The tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// TargetGroupStatus defines the observed state of TargetGroup
type TargetGroupStatus struct {
	// The Amazon Resource Name (ARN) of the target group.
	TargetGroupARN string `json:"targetGroupARN"`
}
