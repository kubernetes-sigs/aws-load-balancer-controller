package elbv2

import (
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
)

var _ core.Resource = &TargetGroup{}

// TargetGroup represents a ELBV2 TargetGroup
type TargetGroup struct {
	// resource id
	id string

	// desired state of TargetGroup
	spec TargetGroupSpec `json:"spec"`

	// observed state of TargetGroup
	// +optional
	status *TargetGroupStatus `json:"status,omitempty"`
}

// NewLoadBalancer constructs new LoadBalancer resource.
func NewTargetGroup(stack core.Stack, id string, spec TargetGroupSpec) *TargetGroup {
	tg := &TargetGroup{
		id:     id,
		spec:   spec,
		status: nil,
	}
	stack.AddResource(tg)
	return tg
}

// ID returns resource's ID within stack.
func (tg *TargetGroup) ID() string {
	return tg.id
}

type TargetType string

const (
	TargetTypeInstance TargetType = "instance"
	TargetTypeIP                  = "ip"
)

// Information to use when checking for a successful response from a target.
type HealthCheckMatcher struct {
	// The HTTP codes.
	HTTPCode string `json:"httpCode"`
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
	// +optional
	TargetGroupARN *string `json:"targetGroupARN,omitempty"`
}
