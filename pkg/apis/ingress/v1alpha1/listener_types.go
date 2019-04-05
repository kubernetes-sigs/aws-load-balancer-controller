package v1alpha1

import corev1 "k8s.io/api/core/v1"

type OnUnauthenticatedRequestAction string

const (
	OnUnauthenticatedRequestActionDeny         OnUnauthenticatedRequestAction = "deny"
	OnUnauthenticatedRequestActionAllow                                       = "allow"
	OnUnauthenticatedRequestActionAuthenticate                                = "authenticate"
)

func (action OnUnauthenticatedRequestAction) String() string {
	return string(action)
}

type AuthenticateConfig struct {
	// +optional
	AuthenticationRequestExtraParams map[string]string `json:"authenticationRequestExtraParams,omitempty"`

	// +kubebuilder:validation:Enum=deny,allow,authenticate
	// +optional
	OnUnauthenticatedRequest OnUnauthenticatedRequestAction `json:"onUnauthenticatedRequest,omitempty"`

	// +optional
	Scope string `json:"scope,omitempty"`

	// +optional
	SessionCookieName string `json:"sessionCookieName,omitempty"`

	// +optional
	SessionTimeout int64 `json:"sessionTimeout,omitempty"`
}

type AuthenticateCognitoConfig struct {
	AuthenticateConfig `json:",inline"`

	UserPoolARN      string `json:"userPoolARN"`
	UserPoolClientID string `json:"userPoolClientID"`
	UserPoolDomain   string `json:"userPoolDomain"`
}

type AuthenticateOIDCConfig struct {
	AuthenticateConfig `json:",inline"`

	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorizationEndpoint"`
	TokenEndpoint         string `json:"tokenEndpoint"`
	UserInfoEndpoint      string `json:"userInfoEndpoint"`
	ClientID              string `json:"clientID"`
	ClientSecret          string `json:"clientSecret"`
}

type FixedResponseConfig struct {
	// +kubebuilder:validation:Enum=text/plain,text/css,text/html,application/javascript,application/json
	// +optional
	ContentType string `json:"contentType,omitempty"`

	// +kubebuilder:validation:MinLength=0
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	MessageBody string `json:"messageBody,omitempty"`

	// +kubebuilder:validation:Pattern=^(2|4|5)\d\d$
	// +optional
	StatusCode string `json:"statusCode"`
}

type RedirectConfig struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +optional
	Host string `json:"host,omitempty"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:Pattern=^/.*
	// +optional
	Path string `json:"path,omitempty"`

	// +optional
	Port string `json:"port,omitempty"`

	// +kubebuilder:validation:Pattern=^(HTTPS?|#\{protocol\})$
	// +optional
	Protocol string `json:"protocol,omitempty"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +optional
	Query string `json:"query,omitempty"`

	// +kubebuilder:validation:Enum=HTTP_301,HTTP_302
	// +optional
	StatusCode string `json:"statusCode,omitempty"`
}

type TargetGroupReference struct {
	TargetGroupRef corev1.LocalObjectReference `json:"targetGroupRef,omitempty"`
	TargetGroupARN string                      `json:"targetGroupARN,omitempty"`
}

type ForwardConfig struct {
	TargetGroup TargetGroupReference `json:"targetGroup"`
}

type ListenerActionType string

const (
	ListenerActionTypeAuthenticateCognito ListenerActionType = "authenticate-cognito"
	ListenerActionTypeAuthenticateOIDC                       = "authenticate-oidc"
	ListenerActionTypeFixedResponse                          = "fixed-response"
	ListenerActionTypeForward                                = "forward"
	ListenerActionTypeRedirect                               = "redirect"
)

type ListenerAction struct {
	// +kubebuilder:validation:Enum=authenticate-cognito,authenticate-oidc,forward,redirect,fixed-response
	Type ListenerActionType `json:"type"`

	// +optional
	AuthenticateCognito *AuthenticateCognitoConfig `json:"authenticateCognitoConfig,omitempty"`

	// +optional
	AuthenticateOIDC *AuthenticateOIDCConfig `json:"authenticateOidcConfig,omitempty"`

	// +optional
	FixedResponse *FixedResponseConfig `json:"fixedResponseConfig,omitempty"`

	// +optional
	Redirect *RedirectConfig `json:"redirectConfig,omitempty"`

	// +optional
	Forward *ForwardConfig `json:"forwardConfig,omitempty"`
}

type HostHeaderConditionConfig struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1
	Values []string `json:"values"`
}

type PathPatternConditionConfig struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1
	Values []string `json:"values"`
}

type RuleConditionField string

const (
	RuleConditionFieldHostHeader  RuleConditionField = "host-header"
	RuleConditionFieldPathPattern RuleConditionField = "path-pattern"
)

func (field RuleConditionField) String() string {
	return string(field)
}

type ListenerRuleCondition struct {
	Field RuleConditionField `json:"field"`

	// +optional
	HostHeader *HostHeaderConditionConfig `json:"hostHeaderConfig,omitempty"`

	// +optional
	PathPattern *PathPatternConditionConfig `json:"pathPatternConfig,omitempty"`
}

// ListenerRule defines the desired state of ListenerRule
type ListenerRule struct {
	// +kubebuilder:validation:MinItems=1
	Conditions []ListenerRuleCondition `json:"conditions"`

	// +kubebuilder:validation:MinItems=1
	Actions []ListenerAction `json:"actions"`
}

// Listener defines the desired state of Listener
type Listener struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int64 `json:"port"`

	// +kubebuilder:validation:Enum=HTTP,HTTPS
	Protocol Protocol `json:"protocol"`

	// +optional
	SSLPolicy string `json:"sslPolicy,omitempty"`

	// +optional
	Certificates []string `json:"certificates,omitempty"`

	// +kubebuilder:validation:MinItems=1
	DefaultActions []ListenerAction `json:"defaultActions"`

	// +optional
	Rules []ListenerRule `json:"rules,omitempty"`
}
