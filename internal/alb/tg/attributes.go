package tg

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	api "k8s.io/api/core/v1"
)

const (
	DeregistrationDelayTimeoutSecondsKey = "deregistration_delay.timeout_seconds"
	SlowStartDurationSecondsKey          = "slow_start.duration_seconds"
	StickinessEnabledKey                 = "stickiness.enabled"
	StickinessTypeKey                    = "stickiness.type"
	StickinessLbCookieDurationSecondsKey = "stickiness.lb_cookie.duration_seconds"

	DeregistrationDelayTimeoutSeconds = 300
	SlowStartDurationSeconds          = 0
	StickinessEnabled                 = false
	StickinessType                    = "lb_cookie"
	StickinessLbCookieDurationSeconds = 86400
)

// Attributes represents the desired state of attributes for a target group.
type Attributes struct {
	// DeregistrationDelayTimeoutSeconds: deregistration_delay.timeout_seconds - The amount of time, in seconds,
	// for Elastic Load Balancing to wait before changing the state of a deregistering
	// target from draining to unused. The range is 0-3600 seconds. The default
	// value is 300 seconds.
	DeregistrationDelayTimeoutSeconds int64

	// SlowStartDurationSeconds: slow_start.duration_seconds - The time period, in seconds, during which
	// a newly registered target receives a linearly increasing share of the
	// traffic to the target group. After this time period ends, the target receives
	// its full share of traffic. The range is 30-900 seconds (15 minutes). Slow
	// start mode is disabled by default.
	SlowStartDurationSeconds int64

	// StickinessEnabled: stickiness.enabled - Indicates whether sticky sessions are enabled.
	// The value is true or false. The default is false.
	StickinessEnabled bool

	// StickinessType: stickiness.type - The type of sticky sessions. The possible value is
	// lb_cookie.
	StickinessType string

	// StickinessLbCookieDurationSeconds: stickiness.lb_cookie.duration_seconds - The time period, in seconds,
	// during which requests from a client should be routed to the same target.
	// After this time period expires, the load balancer-generated cookie is
	// considered stale. The range is 1 second to 1 week (604800 seconds). The
	// default value is 1 day (86400 seconds).
	StickinessLbCookieDurationSeconds int64
}

func NewAttributes(attrs []*elbv2.TargetGroupAttribute) (a *Attributes, err error) {
	a = &Attributes{
		DeregistrationDelayTimeoutSeconds: DeregistrationDelayTimeoutSeconds,
		SlowStartDurationSeconds:          SlowStartDurationSeconds,
		StickinessEnabled:                 StickinessEnabled,
		StickinessType:                    StickinessType,
		StickinessLbCookieDurationSeconds: StickinessLbCookieDurationSeconds,
	}
	var e error
	for _, attr := range attrs {
		attrValue := aws.StringValue(attr.Value)
		switch attrKey := aws.StringValue(attr.Key); attrKey {
		case DeregistrationDelayTimeoutSecondsKey:
			a.DeregistrationDelayTimeoutSeconds, err = strconv.ParseInt(attrValue, 10, 64)
			if err != nil {
				return a, fmt.Errorf("invalid target group attribute value %s=%s", attrKey, attrValue)
			}
			if a.DeregistrationDelayTimeoutSeconds < 0 || a.DeregistrationDelayTimeoutSeconds > 3600 {
				return a, fmt.Errorf("%s must be within 0-3600 seconds, not %v", attrKey, attrValue)
			}
		case SlowStartDurationSecondsKey:
			a.SlowStartDurationSeconds, err = strconv.ParseInt(attrValue, 10, 64)
			if err != nil {
				return a, fmt.Errorf("invalid target group attribute value %s=%s", attrKey, attrValue)
			}
			if (a.SlowStartDurationSeconds < 30 || a.SlowStartDurationSeconds > 900) && a.SlowStartDurationSeconds != 0 {
				return a, fmt.Errorf("%s must be within 30-900 seconds, not %v", attrKey, attrValue)
			}
		case StickinessEnabledKey:
			a.StickinessEnabled, err = strconv.ParseBool(attrValue)
			if err != nil {
				return a, fmt.Errorf("invalid target group attribute value %s=%s", attrKey, attrValue)
			}
		case StickinessTypeKey:
			a.StickinessType = attrValue
			if attrValue != "lb_cookie" {
				return a, fmt.Errorf("invalid target group attribute value %s=%s", attrKey, attrValue)
			}
		case StickinessLbCookieDurationSecondsKey:
			a.StickinessLbCookieDurationSeconds, err = strconv.ParseInt(attrValue, 10, 64)
			if err != nil {
				return a, fmt.Errorf("invalid target group attribute value %s=%s", attrKey, attrValue)
			}
			if a.StickinessLbCookieDurationSeconds < 1 || a.StickinessLbCookieDurationSeconds > 604800 {
				return a, fmt.Errorf("%s must be within 1-604800 seconds, not %v", attrKey, attrValue)
			}
		default:
			e = NewInvalidAttribute(attrKey)
		}
	}
	return a, e
}

// AttributesController provides functionality to manage Attributes
type AttributesController interface {
	// Reconcile ensures the target group attributes in AWS matches the state specified by the ingress configuration.
	Reconcile(ctx context.Context, tgArn string, attributes []*elbv2.TargetGroupAttribute) error
}

// NewAttributesController constructs a new attributes controller
func NewAttributesController(cloud aws.CloudAPI) AttributesController {
	return &attributesController{
		cloud: cloud,
	}
}

type attributesController struct {
	cloud aws.CloudAPI
}

func (c *attributesController) Reconcile(ctx context.Context, tgArn string, attributes []*elbv2.TargetGroupAttribute) error {
	desired, err := NewAttributes(attributes)
	if err != nil {
		return fmt.Errorf("invalid attributes due to %v", err)
	}
	raw, err := c.cloud.DescribeTargetGroupAttributesWithContext(ctx, &elbv2.DescribeTargetGroupAttributesInput{
		TargetGroupArn: aws.String(tgArn),
	})
	if err != nil {
		return fmt.Errorf("failed to retrieve attributes from TargetGroup in AWS: %s", err.Error())
	}
	current, err := NewAttributes(raw.Attributes)
	if err != nil && !IsInvalidAttribute(err) {
		return fmt.Errorf("failed parsing attributes: %v", err)
	}

	changeSet := attributesChangeSet(current, desired)
	if len(changeSet) > 0 {
		albctx.GetLogger(ctx).Infof("Modifying TargetGroup %v attributes to %v.", tgArn, log.Prettify(changeSet))
		_, err = c.cloud.ModifyTargetGroupAttributesWithContext(ctx, &elbv2.ModifyTargetGroupAttributesInput{
			TargetGroupArn: aws.String(tgArn),
			Attributes:     changeSet,
		})
		if err != nil {
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", "%s attributes modification failed: %s", tgArn, err.Error())
			return err
		}
	}
	return nil
}

// attributesChangeSet returns a list of elbv2.TargetGroupAttribute required to change a into b
func attributesChangeSet(a, b *Attributes) (changeSet []*elbv2.TargetGroupAttribute) {
	if a.DeregistrationDelayTimeoutSeconds != b.DeregistrationDelayTimeoutSeconds {
		changeSet = append(changeSet, tgAttribute(DeregistrationDelayTimeoutSecondsKey, fmt.Sprintf("%v", b.DeregistrationDelayTimeoutSeconds)))
	}

	if a.SlowStartDurationSeconds != b.SlowStartDurationSeconds {
		changeSet = append(changeSet, tgAttribute(SlowStartDurationSecondsKey, fmt.Sprintf("%v", b.SlowStartDurationSeconds)))
	}

	if a.StickinessEnabled != b.StickinessEnabled {
		changeSet = append(changeSet, tgAttribute(StickinessEnabledKey, fmt.Sprintf("%v", b.StickinessEnabled)))
	}

	if a.StickinessType != b.StickinessType {
		changeSet = append(changeSet, tgAttribute(StickinessTypeKey, b.StickinessType))
	}

	if a.StickinessLbCookieDurationSeconds != b.StickinessLbCookieDurationSeconds {
		changeSet = append(changeSet, tgAttribute(StickinessLbCookieDurationSecondsKey, fmt.Sprintf("%v", b.StickinessLbCookieDurationSeconds)))
	}

	return
}

func tgAttribute(k, v string) *elbv2.TargetGroupAttribute {
	return &elbv2.TargetGroupAttribute{Key: aws.String(k), Value: aws.String(v)}
}

// NewInvalidAttribute returns a new InvalidAttribute  error
func NewInvalidAttribute(name string) error {
	return InvalidAttribute{
		Name: fmt.Sprintf("the target group attribute %v is not valid", name),
	}
}

// InvalidAttribute error
type InvalidAttribute struct {
	Name string
}

func (e InvalidAttribute) Error() string {
	return e.Name
}

// IsInvalidAttribute checks if the err is from an invalid attribute
func IsInvalidAttribute(e error) bool {
	_, ok := e.(InvalidAttribute)
	return ok
}
