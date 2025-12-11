package service

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// TargetGroupTuple Information about how traffic will be distributed between multiple target groups in a forward rule.
// Target group protocol defaults to listener protocol.
type TargetGroupTuple struct {
	// The Amazon Resource Name (ARN) of the target group.
	TargetGroupARN *string `json:"targetGroupARN"`

	// The K8s service Name.
	ServiceName *string `json:"serviceName"`

	// The K8s service port.
	ServicePort *intstr.IntOrString `json:"servicePort"`

	// Whether the traffic should be decrypted and forwarded to the target unencrypted. For a TLS listener, its target groups default to TLS protocol. To set the target group to TCP protocol. Set Decrypt to true.
	// +optional
	Decrypt *bool `json:"decrypt,omitempty"`

	// The weight.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=999
	// +optional
	Weight *int32 `json:"weight,omitempty"`
}

func (t *TargetGroupTuple) validate() error {
	if (t.TargetGroupARN != nil) == (t.ServiceName != nil) {
		return errors.New("precisely one of targetGroupARN and serviceName can be specified")
	}

	if t.ServiceName != nil && t.ServicePort == nil {
		return errors.New("missing servicePort")
	}

	if t.Weight != nil && (*t.Weight < 0 || *t.Weight > 999) {
		return errors.New("target group weight must be between 0 and 999")
	}
	return nil
}

// TargetGroupStickinessConfig Information about the target group stickiness.
type TargetGroupStickinessConfig struct {
	// Whether target group stickiness is enabled.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// ForwardActionConfig Information about a forward action.
type ForwardActionConfig struct {
	// The weight of the base service.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=999
	BaseServiceWeight *int32 `json:"baseServiceWeight"`

	// One or more target groups.
	// +kubebuilder:validation:MaxProperties=4
	TargetGroups []TargetGroupTuple `json:"targetGroups"`

	// The target group stickiness.
	// +optional
	TargetGroupStickinessConfig *TargetGroupStickinessConfig `json:"targetGroupStickinessConfig,omitempty"`
}

func (c *ForwardActionConfig) validate() error {
	for _, t := range c.TargetGroups {
		if err := t.validate(); err != nil {
			return errors.Wrap(err, "invalid TargetGroupTuple")
		}
	}
	if len(c.TargetGroups) > 1 {
		for _, t := range c.TargetGroups {
			if t.Weight == nil {
				return errors.New("weight must be set when routing to multiple target groups")
			}
		}
	}
	return nil
}

// ActionType The type of action.
type ActionType string

const (
	ActionTypeForward ActionType = "forward"
)

type Action struct {
	// The type of action.
	Type ActionType `json:"type"`

	// Information for creating an action that distributes requests among one or more target groups.
	ForwardConfig *ForwardActionConfig `json:"forwardConfig,omitempty"`
}

func (a *Action) validate() error {
	switch a.Type {
	case ActionTypeForward:
		if a.ForwardConfig != nil {
			if err := a.ForwardConfig.validate(); err != nil {
				return errors.Wrap(err, "invalid ForwardConfig")
			}
		}
	default:
		return errors.Errorf("unknown action type: %v", a.Type)
	}
	return nil
}
