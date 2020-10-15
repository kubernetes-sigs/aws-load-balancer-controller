package ingress

import (
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NOTE: these types are user-facing data structures.
// They are different from these `pkg/model` structures(which are implementation details)
// Changes to this file **must be** backwards-compatible.

// Information about an action that returns a custom HTTP response.
type FixedResponseActionConfig struct {
	// The content type.
	// +optional
	ContentType *string `json:"contentType,omitempty"`

	// The message.
	// +optional
	MessageBody *string `json:"messageBody,omitempty"`

	// The HTTP response code.
	StatusCode string `json:"statusCode"`
}

func (c *FixedResponseActionConfig) validate() error {
	if len(c.StatusCode) == 0 {
		return errors.New("statusCode is required")
	}
	return nil
}

// Information about a redirect action.
type RedirectActionConfig struct {
	// The hostname.
	// +optional
	Host *string `json:"host,omitempty"`

	// The absolute path.
	// +optional
	Path *string `json:"path,omitempty"`

	// The port.
	// +optional
	Port *string `json:"port,omitempty"`

	// The protocol.
	// +optional
	Protocol *string `json:"protocol,omitempty"`

	// The query parameters
	// +optional
	Query *string `json:"query,omitempty"`

	// The HTTP redirect code.
	StatusCode string `json:"statusCode"`
}

func (c *RedirectActionConfig) validate() error {
	if len(c.StatusCode) == 0 {
		return errors.New("statusCode is required")
	}
	return nil
}

// Information about how traffic will be distributed between multiple target groups in a forward rule.
type TargetGroupTuple struct {
	// The Amazon Resource Name (ARN) of the target group.
	TargetGroupARN *string `json:"targetGroupARN"`

	// the K8s service Name
	ServiceName *string `json:"serviceName"`

	// the K8s service port
	ServicePort *intstr.IntOrString `json:"servicePort"`

	// The weight.
	// +optional
	Weight *int64 `json:"weight,omitempty"`
}

func (t *TargetGroupTuple) validate() error {
	if (t.TargetGroupARN != nil) == (t.ServiceName != nil) {
		return errors.New("precisely one of targetGroupARN and serviceName can be specified")
	}

	if t.ServiceName != nil && t.ServicePort == nil {
		return errors.New("missing servicePort")
	}
	return nil
}

// Information about the target group stickiness for a rule.
type TargetGroupStickinessConfig struct {
	// Indicates whether target group stickiness is enabled.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// The time period, in seconds, during which requests from a client should be routed to the same target group.
	// +optional
	DurationSeconds *int64 `json:"durationSeconds,omitempty"`
}

// Information about a forward action.
type ForwardActionConfig struct {
	// One or more target groups.
	// [Network Load Balancers] you can specify a single target group.
	TargetGroups []TargetGroupTuple `json:"targetGroups"`

	// The target group stickiness for the rule.
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
				return errors.New("weight must be set when route to multiple target groups")
			}
		}
	}
	return nil
}

// The type of action.
type ActionType string

const (
	ActionTypeFixedResponse ActionType = "fixed-response"
	ActionTypeForward       ActionType = "forward"
	ActionTypeRedirect      ActionType = "redirect"
)

type Action struct {
	// The type of action.
	Type ActionType `json:"type"`

	// The Amazon Resource Name (ARN) of the target group. Specify only when Type
	// is forward and you want to route to a single target group. To route to one
	// or more target groups, use ForwardConfig instead.
	TargetGroupARN *string `json:"targetGroupARN"`

	// [Application Load Balancer] Information for creating an action that returns a custom HTTP response.
	// +optional
	FixedResponseConfig *FixedResponseActionConfig `json:"fixedResponseConfig,omitempty"`

	// [Application Load Balancer] Information for creating a redirect action.
	// +optional
	RedirectConfig *RedirectActionConfig `json:"redirectConfig,omitempty"`

	// Information for creating an action that distributes requests among one or more target groups.
	// +optional
	ForwardConfig *ForwardActionConfig `json:"forwardConfig,omitempty"`
}

func (a *Action) validate() error {
	switch a.Type {
	case elbv2.ActionTypeEnumFixedResponse:
		if a.FixedResponseConfig == nil {
			return errors.New("missing FixedResponseConfig")
		}
		if err := a.FixedResponseConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid FixedResponseConfig")
		}
	case elbv2.ActionTypeEnumRedirect:
		if a.RedirectConfig == nil {
			return errors.New("missing RedirectConfig")
		}
		if err := a.RedirectConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid RedirectConfig")
		}
	case elbv2.ActionTypeEnumForward:
		if (a.TargetGroupARN != nil) == (a.ForwardConfig != nil) {
			return errors.New("precisely one of TargetGroupArn and ForwardConfig can be specified")
		}
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

type RuleConditionField string

const (
	RuleConditionFieldHTTPHeader        RuleConditionField = "http-header"
	RuleConditionFieldHTTPRequestMethod RuleConditionField = "http-request-method"
	RuleConditionFieldHostHeader        RuleConditionField = "host-header"
	RuleConditionFieldPathPattern       RuleConditionField = "path-pattern"
	RuleConditionFieldQueryString       RuleConditionField = "query-string"
	RuleConditionFieldSourceIP          RuleConditionField = "source-ip"
)

// Information for a host header condition.
type HostHeaderConditionConfig struct {
	// One or more host names.
	Values []string `json:"values"`
}

func (c *HostHeaderConditionConfig) validate() error {
	if len(c.Values) == 0 {
		return errors.New("values cannot be empty")
	}
	return nil
}

// Information for an HTTP header condition.
type HTTPHeaderConditionConfig struct {
	// The name of the HTTP header field.
	HTTPHeaderName string `json:"httpHeaderName"`
	// One or more strings to compare against the value of the HTTP header.
	Values []string `json:"values"`
}

func (c *HTTPHeaderConditionConfig) validate() error {
	if len(c.Values) == 0 {
		return errors.New("values cannot be empty")
	}
	return nil
}

// Information for an HTTP method condition.
type HTTPRequestMethodConditionConfig struct {
	// The name of the request method.
	Values []string `json:"values"`
}

func (c *HTTPRequestMethodConditionConfig) validate() error {
	if len(c.Values) == 0 {
		return errors.New("values cannot be empty")
	}
	return nil
}

// Information about a path pattern condition.
type PathPatternConditionConfig struct {
	// One or more path patterns to compare against the request URL.
	Values []string `json:"values"`
}

func (c *PathPatternConditionConfig) validate() error {
	if len(c.Values) == 0 {
		return errors.New("values cannot be empty")
	}
	return nil
}

// Information about a key/value pair.
type QueryStringKeyValuePair struct {
	// The key.
	// +optional
	Key *string `json:"key,omitempty"`

	// The value.
	Value string `json:"value"`
}

func (c *QueryStringKeyValuePair) validate() error {
	if len(c.Value) == 0 {
		return errors.New("value cannot be empty")
	}
	return nil
}

// Information about a query string condition.
type QueryStringConditionConfig struct {
	// One or more key/value pairs or values to find in the query string.
	Values []QueryStringKeyValuePair `json:"values"`
}

func (c *QueryStringConditionConfig) validate() error {
	if len(c.Values) == 0 {
		return errors.New("Values cannot be empty")
	}
	for _, pair := range c.Values {
		if err := pair.validate(); err != nil {
			return err
		}
	}
	return nil
}

// Information about a source IP condition.
type SourceIPConditionConfig struct {
	// One or more source IP addresses, in CIDR format.
	Values []string `json:"values"`
}

func (c *SourceIPConditionConfig) validate() error {
	if len(c.Values) == 0 {
		return errors.New("values cannot be empty")
	}
	return nil
}

// Information about a condition for a rule.
type RuleCondition struct {
	// The field in the HTTP request.
	Field RuleConditionField `json:"field"`
	// Information for a host header condition.
	HostHeaderConfig *HostHeaderConditionConfig `json:"hostHeaderConfig"`
	// Information for an HTTP header condition.
	HTTPHeaderConfig *HTTPHeaderConditionConfig `json:"httpHeaderConfig"`
	// Information for an HTTP method condition.
	HTTPRequestMethodConfig *HTTPRequestMethodConditionConfig `json:"httpRequestMethodConfig"`
	// Information for a path pattern condition.
	PathPatternConfig *PathPatternConditionConfig `json:"pathPatternConfig"`
	// Information for a query string condition.
	QueryStringConfig *QueryStringConditionConfig `json:"queryStringConfig"`
	// Information for a source IP condition.
	SourceIPConfig *SourceIPConditionConfig `json:"sourceIPConfig"`
}

func (c *RuleCondition) validate() error {
	switch c.Field {
	case RuleConditionFieldHostHeader:
		if c.HostHeaderConfig == nil {
			return errors.New("missing hostHeaderConfig")
		}
		if err := c.HostHeaderConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid hostHeaderConfig")
		}
	case RuleConditionFieldPathPattern:
		if c.PathPatternConfig == nil {
			return errors.New("missing pathPatternConfig")
		}
		if err := c.PathPatternConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid pathPatternConfig")
		}
	case RuleConditionFieldHTTPHeader:
		if c.HTTPHeaderConfig == nil {
			return errors.New("missing httpHeaderConfig")
		}
		if err := c.HTTPHeaderConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid httpHeaderConfig")
		}
	case RuleConditionFieldHTTPRequestMethod:
		if c.HTTPRequestMethodConfig == nil {
			return errors.New("missing httpRequestMethodConfig")
		}
		if err := c.HTTPRequestMethodConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid httpRequestMethodConfig")
		}
	case RuleConditionFieldQueryString:
		if c.QueryStringConfig == nil {
			return errors.New("missing queryStringConfig")
		}
		if err := c.QueryStringConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid queryStringConfig")
		}
	case RuleConditionFieldSourceIP:
		if c.SourceIPConfig == nil {
			return errors.New("missing sourceIPConfig")
		}
		if err := c.SourceIPConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid sourceIPConfig")
		}
	}
	return nil
}

type AuthType string

const (
	AuthTypeNone    AuthType = "none"
	AuthTypeCognito AuthType = "cognito"
	AuthTypeOIDC    AuthType = "oidc"
)

type AuthIDPConfigCognito struct {
	// The Amazon Resource Name (ARN) of the Amazon Cognito user pool.
	UserPoolARN string `json:"userPoolARN"`

	// The ID of the Amazon Cognito user pool client.
	UserPoolClientID string `json:"userPoolClientID"`

	// The domain prefix or fully-qualified domain name of the Amazon Cognito user pool.
	UserPoolDomain string `json:"userPoolDomain"`

	// The query parameters (up to 10) to include in the redirect request to the authorization endpoint.
	// +optional
	AuthenticationRequestExtraParams map[string]string `json:"authenticationRequestExtraParams,omitempty"`
}

// configuration for IDP of OIDC
type AuthIDPConfigOIDC struct {
	// The OIDC issuer identifier of the IdP.
	Issuer string `json:"issuer"`

	// The authorization endpoint of the IdP.
	AuthorizationEndpoint string `json:"authorizationEndpoint"`

	// The token endpoint of the IdP.
	TokenEndpoint string `json:"tokenEndpoint"`

	// The user info endpoint of the IdP.
	UserInfoEndpoint string `json:"userInfoEndpoint"`

	// The k8s secretName.
	SecretName string `json:"secretName"`

	// The query parameters (up to 10) to include in the redirect request to the authorization endpoint.
	// +optional
	AuthenticationRequestExtraParams map[string]string `json:"authenticationRequestExtraParams,omitempty"`
}
