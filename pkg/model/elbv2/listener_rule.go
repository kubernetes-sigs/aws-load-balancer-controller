package elbv2

import (
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

var _ core.Resource = &ListenerRule{}

// ListenerRule represents a ELBV2 ListenerRule
type ListenerRule struct {
	core.ResourceMeta `json:"-"`

	// desired state of ListenerRule
	Spec ListenerRuleSpec `json:"spec"`

	// observed state of ListenerRule
	// +optional
	Status *ListenerRuleStatus `json:"status,omitempty"`
}

// NewListenerRule constructs new ListenerRule resource.
func NewListenerRule(stack core.Stack, id string, spec ListenerRuleSpec) *ListenerRule {
	lr := &ListenerRule{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::ListenerRule", id),
		Spec:         spec,
		Status:       nil,
	}
	stack.AddResource(lr)
	lr.registerDependencies(stack)
	return lr
}

// SetStatus sets the ListenerRule's status
func (lr *ListenerRule) SetStatus(status ListenerRuleStatus) {
	lr.Status = &status
}

// register dependencies for ListenerRule.
func (lr *ListenerRule) registerDependencies(stack core.Stack) {
	for _, dep := range lr.Spec.ListenerARN.Dependencies() {
		stack.AddDependency(dep, lr)
	}
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

// Information for an HTTP header condition.
type HTTPHeaderConditionConfig struct {
	// The name of the HTTP header field.
	HTTPHeaderName string `json:"httpHeaderName"`
	// One or more strings to compare against the value of the HTTP header.
	Values []string `json:"values"`
}

// Information for an HTTP method condition.
type HTTPRequestMethodConditionConfig struct {
	// The name of the request method.
	Values []string `json:"values"`
}

// Information about a path pattern condition.
type PathPatternConditionConfig struct {
	// One or more path patterns to compare against the request URL.
	Values []string `json:"values"`
}

// Information about a key/value pair.
type QueryStringKeyValuePair struct {
	// The key.
	// +optional
	Key *string `json:"key,omitempty"`

	// The value.
	Value string `json:"value"`
}

// Information about a query string condition.
type QueryStringConditionConfig struct {
	// One or more key/value pairs or values to find in the query string.
	Values []QueryStringKeyValuePair `json:"values"`
}

// Information about a source IP condition.
type SourceIPConditionConfig struct {
	// One or more source IP addresses, in CIDR format.
	Values []string `json:"values"`
}

// Information about a condition for a rule.
type RuleCondition struct {
	// The field in the HTTP request.
	Field RuleConditionField `json:"field"`
	// Information for a host header condition.
	// +optional
	HostHeaderConfig *HostHeaderConditionConfig `json:"hostHeaderConfig,omitempty"`
	// Information for an HTTP header condition.
	// +optional
	HTTPHeaderConfig *HTTPHeaderConditionConfig `json:"httpHeaderConfig,omitempty"`
	// Information for an HTTP method condition.
	// +optional
	HTTPRequestMethodConfig *HTTPRequestMethodConditionConfig `json:"httpRequestMethodConfig,omitempty"`
	// Information for a path pattern condition.
	// +optional
	PathPatternConfig *PathPatternConditionConfig `json:"pathPatternConfig,omitempty"`
	// Information for a query string condition.
	// +optional
	QueryStringConfig *QueryStringConditionConfig `json:"queryStringConfig,omitempty"`
	// Information for a source IP condition.
	// +optional
	SourceIPConfig *SourceIPConditionConfig `json:"sourceIPConfig,omitempty"`
}

// ListenerRuleSpec defines the desired state of ListenerRule
type ListenerRuleSpec struct {
	// The Amazon Resource Name (ARN) of the listener.
	ListenerARN core.StringToken `json:"listenerARN"`
	// The rule priority.
	Priority int64 `json:"priority"`
	// The actions.
	Actions []Action `json:"actions"`
	// The conditions.
	Conditions []RuleCondition `json:"conditions"`
	// The tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ListenerRuleStatus defines the observed state of ListenerRule
type ListenerRuleStatus struct {
	// The Amazon Resource Name (ARN) of the rule.
	RuleARN string `json:"ruleARN"`
}
