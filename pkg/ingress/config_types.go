package ingress

import (
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

	// The Name of the target group.
	// +optional
	TargetGroupName *string `json:"targetGroupName,omitempty"`

	// the K8s service Name
	ServiceName *string `json:"serviceName"`

	// the K8s service port
	ServicePort *intstr.IntOrString `json:"servicePort"`

	// The weight.
	// +optional
	Weight *int32 `json:"weight,omitempty"`
}

func (t *TargetGroupTuple) validate() error {
	if t.TargetGroupARN == nil && t.TargetGroupName == nil && t.ServiceName == nil {
		return errors.New("missing serviceName or targetGroupARN/targetGroupName")
	}

	if (t.TargetGroupARN != nil || t.TargetGroupName != nil) && t.ServiceName != nil {
		return errors.New("either serviceName or targetGroupARN/targetGroupName can be specified")
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
	DurationSeconds *int32 `json:"durationSeconds,omitempty"`
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

	// Name of the target group. Can be specified instead of TargetGroupARN.
	TargetGroupName *string `json:"targetGroupName,omitempty"`

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
	case ActionTypeFixedResponse:
		if a.FixedResponseConfig == nil {
			return errors.New("missing FixedResponseConfig")
		}
		if err := a.FixedResponseConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid FixedResponseConfig")
		}
	case ActionTypeRedirect:
		if a.RedirectConfig == nil {
			return errors.New("missing RedirectConfig")
		}
		if err := a.RedirectConfig.validate(); err != nil {
			return errors.Wrap(err, "invalid RedirectConfig")
		}
	case ActionTypeForward:
		if a.TargetGroupARN == nil && a.TargetGroupName == nil && a.ForwardConfig == nil {
			return errors.New("missing ForwardConfig or TargetGroupARN/TargetGroupName")
		}
		if (a.TargetGroupARN != nil || a.TargetGroupName != nil) && (a.ForwardConfig != nil) {
			return errors.New("either ForwardConfig or TargetGroupARN/TargetGroupName can be specified")
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
	// One or more regex expressions for request host header.
	// +optional
	RegexValues []string `json:"regexValues,omitempty"`
	// One or more value expressions for request host header.
	// +optional
	Values []string `json:"values,omitempty"`
}

func (c *HostHeaderConditionConfig) validate() error {
	if len(c.Values) == 0 && len(c.RegexValues) == 0 {
		return errors.New("values or regexValues must be specified")
	}
	if len(c.Values) != 0 && len(c.RegexValues) != 0 {
		return errors.New("precisely one of values and regexValues can be specified")
	}
	return nil
}

// Information for an HTTP header condition.
type HTTPHeaderConditionConfig struct {
	// The name of the HTTP header field.
	HTTPHeaderName string `json:"httpHeaderName"`
	// One or more regex matches for request HTTP headers.
	// +optional
	RegexValues []string `json:"regexValues,omitempty"`
	// One or more value matches for request HTTP headers.
	// +optional
	Values []string `json:"values,omitempty"`
}

func (c *HTTPHeaderConditionConfig) validate() error {
	if len(c.Values) == 0 && len(c.RegexValues) == 0 {
		return errors.New("values or regexValues must be specified")
	}
	if len(c.Values) != 0 && len(c.RegexValues) != 0 {
		return errors.New("precisely one of values and regexValues can be specified")
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
	// One or more regex matches for request URL path.
	// +optional
	RegexValues []string `json:"regexValues,omitempty"`
	// One or more value matches for request URL path.
	// +optional
	Values []string `json:"values,omitempty"`
}

func (c *PathPatternConditionConfig) validate() error {
	if len(c.Values) == 0 && len(c.RegexValues) == 0 {
		return errors.New("values or regexValues must be specified")
	}
	if len(c.Values) != 0 && len(c.RegexValues) != 0 {
		return errors.New("precisely one of values and regexValues can be specified")
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

func (c *RuleCondition) Validate() error {
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

type TransformType string

const (
	TransformTypeUrlRewrite        TransformType = "url-rewrite"
	TransformTypeHostHeaderRewrite TransformType = "host-header-rewrite"
)

type RewriteConfig struct {
	// Regex expression
	Regex string `json:"regex"`
	// Replacement expression
	Replace string `json:"replace"`
}

type RewriteConfigObject struct {
	// Rewrites for the transform
	Rewrites []RewriteConfig `json:"rewrites"`
}

type Transform struct {
	// The type of transform
	Type TransformType `json:"type"`
	// Information for a host header rewrite.
	// +optional
	HostHeaderRewriteConfig *RewriteConfigObject `json:"hostHeaderRewriteConfig,omitempty"`
	// Information for a URL rewrite.
	// +optional
	UrlRewriteConfig *RewriteConfigObject `json:"urlRewriteConfig,omitempty"`
}

func (t *Transform) Validate() error {
	switch t.Type {
	case TransformTypeHostHeaderRewrite:
		if t.HostHeaderRewriteConfig == nil {
			return errors.New("missing hostHeaderRewriteConfig")
		}
		if len(t.HostHeaderRewriteConfig.Rewrites) == 0 {
			return errors.New("hostHeaderRewriteConfig.rewrites cannot be empty")
		}
	case TransformTypeUrlRewrite:
		if t.UrlRewriteConfig == nil {
			return errors.New("missing urlRewriteConfig")
		}
		if len(t.UrlRewriteConfig.Rewrites) == 0 {
			return errors.New("urlRewriteConfig.rewrites cannot be empty")
		}
	default:
		return errors.Errorf("unknown transform type: %v", t.Type)
	}
	return nil
}

// The format of an additional claim's value(s) used in JWT validation.
type jwtAdditionalClaimFormat string

const (
	FormatSingleString         jwtAdditionalClaimFormat = "single-string"
	FormatStringArray          jwtAdditionalClaimFormat = "string-array"
	FormatSpaceSeparatedValues jwtAdditionalClaimFormat = "space-separated-values"
)

// An additional claim to validate during JWT validation.
type JwtAdditionalClaim struct {
	// The format of the claim value(s).
	Format jwtAdditionalClaimFormat `json:"format"`
	// The claim name.
	Name string `json:"name"`
	// The claim values.
	Values []string `json:"values"`
}

// Information about an action that performs JSON Web Token (JWT) validation prior to the routing action.
type JwtValidationConfig struct {
	// The JSON Web Key Set (JWKS) endpoint containing the public keys used to verify the JWT.
	JwksEndpoint string `json:"jwksEndpoint"`
	// The issuer of the JWT.
	Issuer string `json:"issuer"`
	// Any additional claims in the JWT that should be validated.
	// +optional
	AdditionalClaims []JwtAdditionalClaim `json:"additionalClaims,omitempty"`
}
