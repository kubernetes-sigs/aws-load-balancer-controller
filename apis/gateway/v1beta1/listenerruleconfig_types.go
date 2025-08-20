package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ListenerRuleConditionField defines the field in the HTTP request to match
// +kubebuilder:validation:Enum=source-ip
type ListenerRuleConditionField string

const (
	ListenerRuleConditionFieldSourceIP ListenerRuleConditionField = "source-ip"
)

// AuthenticateCognitoActionConditionalBehaviorEnum defines the behavior when a user is not authenticated
// +kubebuilder:validation:Enum=deny;allow;authenticate
type AuthenticateCognitoActionConditionalBehaviorEnum string

// Enum values for AuthenticateCognitoActionConditionalBehaviorEnum
const (
	AuthenticateCognitoActionConditionalBehaviorEnumDeny         AuthenticateCognitoActionConditionalBehaviorEnum = "deny"
	AuthenticateCognitoActionConditionalBehaviorEnumAllow        AuthenticateCognitoActionConditionalBehaviorEnum = "allow"
	AuthenticateCognitoActionConditionalBehaviorEnumAuthenticate AuthenticateCognitoActionConditionalBehaviorEnum = "authenticate"
)

// AuthenticateOidcActionConditionalBehaviorEnum defines the behavior when a user is not authenticated
// +kubebuilder:validation:Enum=deny;allow;authenticate
type AuthenticateOidcActionConditionalBehaviorEnum string

// Enum values for AuthenticateOidcActionConditionalBehaviorEnum
const (
	AuthenticateOidcActionConditionalBehaviorEnumDeny         AuthenticateOidcActionConditionalBehaviorEnum = "deny"
	AuthenticateOidcActionConditionalBehaviorEnumAllow        AuthenticateOidcActionConditionalBehaviorEnum = "allow"
	AuthenticateOidcActionConditionalBehaviorEnumAuthenticate AuthenticateOidcActionConditionalBehaviorEnum = "authenticate"
)

// Information about a source IP condition
type SourceIPConditionConfig struct {
	// One or more source IP addresses, in CIDR format
	// +kubebuilder:validation:MinItems=1
	Values []string `json:"values"`
}

// Information about a condition for a listener rule
// +kubebuilder:validation:XValidation:rule="has(self.field) && self.field == 'source-ip' ? has(self.sourceIPConfig) : !has(self.sourceIPConfig)",message="sourceIPConfig must be specified only when field is 'source-ip'"
type ListenerRuleCondition struct {
	// The field in the HTTP request
	Field ListenerRuleConditionField `json:"field"`

	// Information for a source IP condition
	// +optional
	SourceIPConfig *SourceIPConditionConfig `json:"sourceIPConfig,omitempty"`
}

// ActionType defines the type of action for the rule
// +kubebuilder:validation:Enum=forward;fixed-response;redirect;authenticate-cognito;authenticate-oidc
type ActionType string

const (
	ActionTypeForward             ActionType = "forward"
	ActionTypeFixedResponse       ActionType = "fixed-response"
	ActionTypeRedirect            ActionType = "redirect"
	ActionTypeAuthenticateCognito ActionType = "authenticate-cognito"
	ActionTypeAuthenticateOIDC    ActionType = "authenticate-oidc"
)

// Information about the target group stickiness for a listener rule.
type TargetGroupStickinessConfig struct {

	// The time period, in seconds, during which requests from a client should be
	// routed to the same target group. The range is 1-604800 seconds (7 days).
	// +kubebuilder:default=3600
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=604800
	DurationSeconds *int32 `json:"durationSeconds,omitempty"`

	// Indicates whether target group stickiness is enabled.
	// +kubebuilder:default=false
	Enabled *bool `json:"enabled,omitempty"`
}

// Information about a forward action.
type ForwardActionConfig struct {
	// +kubebuilder:default={}
	// The target group stickiness for the rule.
	// Note: ForwardActionConfig only supports target group stickiness configuration through CRD.
	// All other forward action fields must be set through the Gateway API native way.
	TargetGroupStickinessConfig *TargetGroupStickinessConfig `json:"targetGroupStickinessConfig,omitempty"`
}

// Information about a redirect action.
type RedirectActionConfig struct {
	// The query parameters, URL-encoded when necessary, but not percent-encoded. Do
	// not include the leading "?", as it is automatically added. You can specify any
	// of the reserved keywords.
	// Note: RedirectActionConfig only supports setting the query parameter through CRD.
	// All other redirect action fields must be set through the Gateway API native way.
	// +kubebuilder:default="#{query}"
	Query *string `json:"query,omitempty"`
}

// Information about an action that returns a custom HTTP response.
type FixedResponseActionConfig struct {
	// The HTTP response code (2XX, 4XX, or 5XX).
	// +kubebuilder:validation:XValidation:rule="(self >= 200 && self <= 299) || (self >= 400 && self <= 599)",message="StatusCode must be a valid HTTP status code in the 2XX, 4XX, or 5XX range"
	StatusCode int32 `json:"statusCode"`

	// The content type of the fixed response.
	// +optional
	// +kubebuilder:default="text/plain"
	// +kubebuilder:validation:Enum=text/plain;text/css;text/html;application/javascript;application/json
	ContentType *string `json:"contentType,omitempty"`

	// The message
	// +optional
	MessageBody *string `json:"messageBody,omitempty"`
}

// Secret holds OAuth 2.0 clientID and clientSecret. You need to create this secret and provide its name and namespace
type Secret struct {
	// Name is name of the secret
	Name string `json:"name"`
	// Namespace is namespace of secret. If empty it will be considered to be in same namespace as of the resource referring it
	Namespace *string `json:"namespace,omitempty"`
}

// Information about an authenticate-cognito action
type AuthenticateCognitoActionConfig struct {
	// The Amazon Resource Name (ARN) of the Amazon Cognito user pool.
	UserPoolArn string `json:"userPoolArn"`

	// The ID of the Amazon Cognito user pool client.
	UserPoolClientID string `json:"userPoolClientId"`

	// The domain prefix or fully-qualified domain name of the Amazon Cognito user
	// pool.
	UserPoolDomain string `json:"userPoolDomain"`

	// The set of user claims to be requested from the IdP. The default is openid .
	//
	// To verify which scope values your IdP supports and how to separate multiple
	// values, see the documentation for your IdP.
	// +optional
	// +kubebuilder:default="openid"
	Scope *string `json:"scope,omitempty"`

	// The query parameters (up to 10) to include in the redirect request to the
	// authorization endpoint.
	// +optional
	// +kubebuilder:validation:MaxProperties=10
	AuthenticationRequestExtraParams *map[string]string `json:"authenticationRequestExtraParams,omitempty"`

	// The behavior if the user is not authenticated. The following are possible
	// +kubebuilder:default="authenticate"
	OnUnauthenticatedRequest *AuthenticateCognitoActionConditionalBehaviorEnum `json:"onUnauthenticatedRequest,omitempty"`

	// The name of the cookie used to maintain session information. The default is
	// AWSELBAuthSessionCookie.
	// +optional
	// +kubebuilder:default="AWSELBAuthSessionCookie"
	SessionCookieName *string `json:"sessionCookieName,omitempty"`

	// The maximum duration of the authentication session, in seconds. The default is
	// 604800 seconds (7 days).
	// +optional
	// +kubebuilder:default=604800
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=604800
	SessionTimeout *int64 `json:"sessionTimeout,omitempty"`
}

// Information about an authenticate-oidc action
type AuthenticateOidcActionConfig struct {
	// The authorization endpoint of the IdP. This must be a full URL, including the
	// HTTPS protocol, the domain, and the path.
	AuthorizationEndpoint string `json:"authorizationEndpoint"`

	// Secret holds OAuth 2.0 clientID and clientSecret. You need to create this secret and provide its name and namespace
	Secret *Secret `json:"secret"`

	// The OIDC issuer identifier of the IdP. This must be a full URL, including the
	// HTTPS protocol, the domain, and the path.
	Issuer string `json:"issuer"`

	// The token endpoint of the IdP. This must be a full URL, including the HTTPS
	// protocol, the domain, and the path.
	TokenEndpoint string `json:"tokenEndpoint"`

	// The user info endpoint of the IdP. This must be a full URL, including the HTTPS
	// protocol, the domain, and the path.
	UserInfoEndpoint string `json:"userInfoEndpoint"`

	// The set of user claims to be requested from the IdP. The default is openid .
	//
	// To verify which scope values your IdP supports and how to separate multiple
	// values, see the documentation for your IdP.
	// +optional
	// +kubebuilder:default="openid"
	Scope *string `json:"scope,omitempty"`

	// The query parameters (up to 10) to include in the redirect request to the
	// authorization endpoint.
	// +optional
	// +kubebuilder:validation:MaxProperties=10
	AuthenticationRequestExtraParams *map[string]string `json:"authenticationRequestExtraParams,omitempty"`

	// The behavior if the user is not authenticated. The following are possible
	// +kubebuilder:default="authenticate"
	OnUnauthenticatedRequest *AuthenticateOidcActionConditionalBehaviorEnum `json:"onUnauthenticatedRequest,omitempty"`

	// The name of the cookie used to maintain session information. The default is
	// AWSELBAuthSessionCookie.
	// +optional
	// +kubebuilder:default="AWSELBAuthSessionCookie"
	SessionCookieName *string `json:"sessionCookieName,omitempty"`

	// The maximum duration of the authentication session, in seconds. The default is
	// 604800 seconds (7 days).
	// +optional
	// +kubebuilder:default=604800
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=604800
	SessionTimeout *int32 `json:"sessionTimeout,omitempty"`

	// Indicates whether to use the existing client secret when modifying a listener rule. If
	// you are creating a listener rule, you can omit this parameter or set it to false.
	// +optional
	UseExistingClientSecret *bool `json:"useExistingClientSecret,omitempty"`
}

// Action defines an action for a listener rule
// +kubebuilder:validation:XValidation:rule="has(self.type) && self.type == 'forward' ? has(self.forwardConfig) : !has(self.forwardConfig)",message="forwardConfig must be specified only when type is 'forward'"
// +kubebuilder:validation:XValidation:rule="has(self.type) && self.type == 'redirect' ? has(self.redirectConfig) : !has(self.redirectConfig)",message="redirectConfig must be specified only when type is 'redirect'"
// +kubebuilder:validation:XValidation:rule="has(self.type) && self.type == 'fixed-response' ? has(self.fixedResponseConfig) : !has(self.fixedResponseConfig)",message="fixedResponseConfig must be specified only when type is 'fixed-response'"
// +kubebuilder:validation:XValidation:rule="has(self.type) && self.type == 'authenticate-cognito' ? has(self.authenticateCognitoConfig) : !has(self.authenticateCognitoConfig)",message="authenticateCognitoConfig must be specified only when type is 'authenticate-cognito'"
// +kubebuilder:validation:XValidation:rule="has(self.type) && self.type == 'authenticate-oidc' ? has(self.authenticateOIDCConfig) : !has(self.authenticateOIDCConfig)",message="authenticateOIDCConfig must be specified only when type is 'authenticate-oidc'"
type Action struct {
	// The type of action
	Type ActionType `json:"type"`

	// Information for a forward action
	// +optional
	ForwardConfig *ForwardActionConfig `json:"forwardConfig,omitempty"`

	// Information for a redirect action
	// +optional
	RedirectConfig *RedirectActionConfig `json:"redirectConfig,omitempty"`

	// Information for a fixed-response action
	// +optional
	FixedResponseConfig *FixedResponseActionConfig `json:"fixedResponseConfig,omitempty"`

	// Information for an authenticate-cognito action
	// +optional
	AuthenticateCognitoConfig *AuthenticateCognitoActionConfig `json:"authenticateCognitoConfig,omitempty"`

	// Information for an authenticate-oidc action
	// +optional
	AuthenticateOIDCConfig *AuthenticateOidcActionConfig `json:"authenticateOIDCConfig,omitempty"`
}

// ListenerRuleConfigurationSpec defines the desired state of ListenerRuleConfiguration
// +kubebuilder:validation:XValidation:rule="!has(self.actions) || size(self.actions) > 0",message="At least one action must be specified if actions field is present"
// +kubebuilder:validation:XValidation:rule="!has(self.actions) || self.actions.all(a, a.type == 'authenticate-oidc' || a.type == 'authenticate-cognito' || a.type == 'fixed-response' || a.type == 'forward' || a.type == 'redirect')",message="Only forward, redirect, authenticate-oidc, authenticate-cognito, and fixed-response action types are supported"
// +kubebuilder:validation:XValidation:rule="!has(self.actions) || size(self.actions.filter(a, a.type == 'authenticate-oidc' || a.type == 'authenticate-cognito')) <= 1",message="At most one authentication action (either authenticate-oidc or authenticate-cognito) can be specified"
// +kubebuilder:validation:XValidation:rule="!has(self.actions) || size(self.actions.filter(a, a.type == 'fixed-response' || a.type == 'forward' || a.type == 'redirect')) <= 1",message="At most one routing action (fixed-response or forward or redirect) can be specified"
type ListenerRuleConfigurationSpec struct {
	// Actions defines the set of actions to be performed when conditions match.
	// This CRD implementation currently supports only  authenticate-oidc, authenticate-cognito, and fixed-response action types fully and forward and redirect actions partially
	//
	// For other fields in forward and redirect actions, please use the standard Gateway API HTTPRoute or other route resources, which provide
	// native support for those conditions through the Gateway API specification.
	//
	// At most one authentication action can be specified (either authenticate-oidc or authenticate-cognito).
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=2
	Actions []Action `json:"actions,omitempty"`

	// Conditions defines the circumstances under which the rule actions will be performed.
	// This CRD implementation currently supports only the source-ip condition type
	//
	// For other condition types (such as path-pattern, host-header, http-header, etc.),
	// please use the standard Gateway API HTTPRoute or other route resources, which provide
	// native support for those conditions through the Gateway API specification.
	// +optional
	// +kubebuilder:validation:MinItems=1
	Conditions []ListenerRuleCondition `json:"conditions,omitempty"`

	// Tags are the AWS resource tags to be applied to all AWS resources created for this rule.
	// +optional
	Tags *map[string]string `json:"tags,omitempty"`
}

// ListenerRuleConfigurationStatus defines the observed state of ListenerRuleConfiguration
type ListenerRuleConfigurationStatus struct {

	// The observed generation of the rule configuration
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// ListenerRuleConfiguration is the Schema for the ListenerRuleConfiguration API
type ListenerRuleConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ListenerRuleConfigurationSpec   `json:"spec,omitempty"`
	Status ListenerRuleConfigurationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// ListenerRuleConfigurationList contains a list of ListenerRuleConfiguration
type ListenerRuleConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ListenerRuleConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ListenerRuleConfiguration{}, &ListenerRuleConfigurationList{})
}
