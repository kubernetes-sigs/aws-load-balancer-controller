package action

import (
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/pkg/errors"
)

// Information about an action that returns a custom HTTP response.
type FixedResponseActionConfig struct {
	// The content type.
	//
	// Valid Values: text/plain | text/css | text/html | application/javascript
	// | application/json
	ContentType *string

	// The message.
	MessageBody *string

	// The HTTP response code (2XX, 4XX, or 5XX).
	//
	// StatusCode is a required field
	StatusCode *string
}

// Information about an redirect action
type RedirectActionConfig struct {
	// The hostname. This component is not percent-encoded. The hostname can contain
	// #{host}.
	Host *string

	// The absolute path, starting with the leading "/". This component is not percent-encoded.
	// The path can contain #{host}, #{path}, and #{port}.
	Path *string

	// The port. You can specify a value from 1 to 65535 or #{port}.
	Port *string

	// The protocol. You can specify HTTP, HTTPS, or #{protocol}. You can redirect
	// HTTP to HTTP, HTTP to HTTPS, and HTTPS to HTTPS. You cannot redirect HTTPS
	// to HTTP.
	Protocol *string

	// The query parameters, URL-encoded when necessary, but not percent-encoded.
	// Do not include the leading "?", as it is automatically added. You can specify
	// any of the reserved keywords.
	Query *string

	// The HTTP redirect code. The redirect is either permanent (HTTP 301) or temporary
	// (HTTP 302).
	//
	// StatusCode is a required field
	StatusCode *string
}

func (c *RedirectActionConfig) validate() error {
	if c.StatusCode == nil {
		return errors.New("StatusCode is required")
	}
	return nil
}

func (c *RedirectActionConfig) setDefaults() {
	if c.Host == nil {
		c.Host = aws.String("#{host}")
	}
	if c.Path == nil {
		c.Path = aws.String("/#{path}")
	}
	if c.Port == nil {
		c.Port = aws.String("#{port}")
	}
	if c.Protocol == nil {
		c.Protocol = aws.String("#{protocol}")
	}
	if c.Query == nil {
		c.Query = aws.String("#{query}")
	}
}

// Information about the target group stickiness for a rule.
type TargetGroupStickinessConfig struct {
	// The time period, in seconds, during which requests from a client should be
	// routed to the same target group. The range is 1-604800 seconds (7 days).
	DurationSeconds *int64

	// Indicates whether target group stickiness is enabled.
	Enabled *bool
}

// Information about how traffic will be distributed between multiple target
// groups in a forward rule.
type TargetGroupTuple struct {
	// The Amazon Resource Name (ARN) of the target group.
	TargetGroupArn *string

	// the K8s service Name
	ServiceName *string

	// the K8s service port
	ServicePort *string

	// The weight. The range is 0 to 999.
	Weight *int64
}

func (t *TargetGroupTuple) validate() error {
	if (t.TargetGroupArn != nil) == (t.ServiceName != nil) {
		return errors.New("precisely one of TargetGroupArn and ServiceName can be specified")
	}

	if t.ServiceName != nil && t.ServicePort == nil {
		return errors.New("missing ServicePort")
	}
	return nil
}

// Information about a forward action.
type ForwardActionConfig struct {
	// The target group stickiness for the rule.
	TargetGroupStickinessConfig *TargetGroupStickinessConfig

	// One or more target groups. For Network Load Balancers, you can specify a
	// single target group.
	TargetGroups []*TargetGroupTuple
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
				return errors.New("weight must be set when route to multiple target groups")
			}
		}
	}
	return nil
}

// Information about an action.
type Action struct {
	// The type of action.
	//
	// Type is a required field
	Type *string

	// The Amazon Resource Name (ARN) of the target group. Specify only when Type
	// is forward and you want to route to a single target group. To route to one
	// or more target groups, use ForwardConfig instead.
	TargetGroupArn *string

	// [Application Load Balancer] Information for creating an action that returns
	// a custom HTTP response. Specify only when Type is fixed-response.
	FixedResponseConfig *FixedResponseActionConfig

	// Information for creating an action that distributes requests among one or
	// more target groups. For Network Load Balancers, you can specify a single
	// target group. Specify only when Type is forward. If you specify both ForwardConfig
	// and TargetGroupArn, you can specify only one target group using ForwardConfig
	// and it must be the same target group specified in TargetGroupArn.
	ForwardConfig *ForwardActionConfig

	// [Application Load Balancer] Information for creating a redirect action. Specify
	// only when Type is redirect.
	RedirectConfig *RedirectActionConfig
}

func (a *Action) validate() error {
	switch aws.StringValue(a.Type) {
	case elbv2.ActionTypeEnumFixedResponse:
		if a.FixedResponseConfig == nil {
			return errors.New("missing FixedResponseConfig")
		}
	case elbv2.ActionTypeEnumRedirect:
		if a.RedirectConfig == nil {
			return errors.New("missing RedirectConfig")
		}
		if err := a.RedirectConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid RedirectConfig")
		}
	case elbv2.ActionTypeEnumForward:
		if (a.TargetGroupArn != nil) == (a.ForwardConfig != nil) {
			return errors.New("precisely one of TargetGroupArn and ForwardConfig can be specified")
		}
		if a.ForwardConfig != nil {
			if err := a.ForwardConfig.validate(); err != nil {
				return errors.Wrap(err, "invalid ForwardConfig")
			}
		}
	default:
		return errors.Errorf("unknown action type: %v", *a.Type)
	}
	return nil
}

func (a *Action) setDefaults() {
	if aws.StringValue(a.Type) == elbv2.ActionTypeEnumForward && a.TargetGroupArn != nil {
		a.ForwardConfig = &ForwardActionConfig{
			TargetGroups: []*TargetGroupTuple{
				{
					TargetGroupArn: a.TargetGroupArn,
				},
			},
		}
	}

	if a.RedirectConfig != nil {
		a.RedirectConfig.setDefaults()
	}
}
