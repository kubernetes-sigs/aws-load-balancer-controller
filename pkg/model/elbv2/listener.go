package elbv2

import (
	"context"
	"encoding/json"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

var _ core.Resource = &Listener{}

// Listener represents a ELBV2 Listener
type Listener struct {
	core.ResourceMeta `json:"-"`

	// desired state of LoadBalancer
	Spec ListenerSpec `json:"spec"`

	// observed state of LoadBalancer
	// +optional
	Status *ListenerStatus `json:"status,omitempty"`
}

// NewListener constructs new Listener resource.
func NewListener(stack core.Stack, id string, spec ListenerSpec) *Listener {
	ls := &Listener{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::Listener", id),
		Spec:         spec,
		Status:       nil,
	}
	stack.AddResource(ls)
	ls.registerDependencies(stack)
	return ls
}

// SetStatus sets the Listener's status
func (ls *Listener) SetStatus(status ListenerStatus) {
	ls.Status = &status
}

// ListenerARN returns The Amazon Resource Name (ARN) of the Listener
func (ls *Listener) ListenerARN() core.StringToken {
	return core.NewResourceFieldStringToken(ls, "status/listenerARN",
		func(ctx context.Context, res core.Resource, fieldPath string) (s string, err error) {
			ls := res.(*Listener)
			if ls.Status == nil {
				return "", errors.Errorf("Listener is not fulfilled yet: %v", ls.ID())
			}
			return ls.Status.ListenerARN, nil
		},
	)
}

// register dependencies for Listener.
func (ls *Listener) registerDependencies(stack core.Stack) {
	for _, dep := range ls.Spec.LoadBalancerARN.Dependencies() {
		stack.AddDependency(dep, ls)
	}
}

type Protocol string

const (
	ProtocolHTTP    Protocol = "HTTP"
	ProtocolHTTPS   Protocol = "HTTPS"
	ProtocolTCP     Protocol = "TCP"
	ProtocolTLS     Protocol = "TLS"
	ProtocolUDP     Protocol = "UDP"
	ProtocolTCP_UDP Protocol = "TCP_UDP"
)

type ProtocolVersion string

const (
	ProtocolVersionHTTP1 ProtocolVersion = "HTTP1"
	ProtocolVersionHTTP2 ProtocolVersion = "HTTP2"
	ProtocolVersionGRPC  ProtocolVersion = "GRPC"
)

// The type of action.
type ActionType string

const (
	ActionTypeAuthenticateCognito ActionType = "authenticate-cognito"
	ActionTypeAuthenticateOIDC    ActionType = "authenticate-oidc"
	ActionTypeFixedResponse       ActionType = "fixed-response"
	ActionTypeForward             ActionType = "forward"
	ActionTypeRedirect            ActionType = "redirect"
)

type AuthenticateCognitoActionConditionalBehavior string

const (
	AuthenticateCognitoActionConditionalBehaviorDeny         AuthenticateCognitoActionConditionalBehavior = "deny"
	AuthenticateCognitoActionConditionalBehaviorAllow        AuthenticateCognitoActionConditionalBehavior = "allow"
	AuthenticateCognitoActionConditionalBehaviorAuthenticate AuthenticateCognitoActionConditionalBehavior = "authenticate"
)

// Request parameters to use when integrating with Amazon Cognito to authenticate users.
type AuthenticateCognitoActionConfig struct {
	// The query parameters (up to 10) to include in the redirect request to the authorization endpoint.
	// +optional
	AuthenticationRequestExtraParams map[string]string `json:"authenticationRequestExtraParams,omitempty"`

	// The behavior if the user is not authenticated.
	// +optional
	OnUnauthenticatedRequest *AuthenticateCognitoActionConditionalBehavior `json:"onUnauthenticatedRequest,omitempty"`

	// The set of user claims to be requested from the IdP.
	// +optional
	Scope *string `json:"scope,omitempty"`

	// The name of the cookie used to maintain session information.
	// +optional
	SessionCookieName *string `json:"sessionCookieName,omitempty"`

	// The maximum duration of the authentication session in seconds.
	// +optional
	SessionTimeout *int64 `json:"sessionTimeout,omitempty"`

	// The Amazon Resource Name (ARN) of the Amazon Cognito user pool.
	UserPoolARN string `json:"userPoolARN"`

	// The ID of the Amazon Cognito user pool client.
	UserPoolClientID string `json:"userPoolClientID"`

	// The domain prefix or fully-qualified domain name of the Amazon Cognito user pool.
	UserPoolDomain string `json:"userPoolDomain"`
}

type AuthenticateOIDCActionConditionalBehavior string

const (
	AuthenticateOIDCActionConditionalBehaviorDeny         AuthenticateOIDCActionConditionalBehavior = "deny"
	AuthenticateOIDCActionConditionalBehaviorAllow        AuthenticateOIDCActionConditionalBehavior = "allow"
	AuthenticateOIDCActionConditionalBehaviorAuthenticate AuthenticateOIDCActionConditionalBehavior = "authenticate"
)

// Request parameters when using an identity provider (IdP) that is compliant with OpenID Connect (OIDC) to authenticate users.
type AuthenticateOIDCActionConfig struct {
	// The query parameters (up to 10) to include in the redirect request to the authorization endpoint.
	// +optional
	AuthenticationRequestExtraParams map[string]string `json:"authenticationRequestExtraParams,omitempty"`

	// The behavior if the user is not authenticated.
	// +optional
	OnUnauthenticatedRequest *AuthenticateOIDCActionConditionalBehavior `json:"onUnauthenticatedRequest,omitempty"`

	// The set of user claims to be requested from the IdP.
	// +optional
	Scope *string `json:"scope,omitempty"`

	// The name of the cookie used to maintain session information.
	// +optional
	SessionCookieName *string `json:"sessionCookieName,omitempty"`

	// The maximum duration of the authentication session in seconds.
	// +optional
	SessionTimeout *int64 `json:"sessionTimeout,omitempty"`

	// The OIDC issuer identifier of the IdP.
	Issuer string `json:"issuer"`

	// The authorization endpoint of the IdP.
	AuthorizationEndpoint string `json:"authorizationEndpoint"`

	// The token endpoint of the IdP.
	TokenEndpoint string `json:"tokenEndpoint"`

	// The user info endpoint of the IdP.
	UserInfoEndpoint string `json:"userInfoEndpoint"`

	// The OAuth 2.0 client identifier.
	ClientID string `json:"clientID"`

	// The OAuth 2.0 client secret.
	ClientSecret string `json:"clientSecret"`
}

func (cfg AuthenticateOIDCActionConfig) MarshalJSON() ([]byte, error) {
	type redactedCFG AuthenticateOIDCActionConfig
	redactedCfg := redactedCFG(cfg)
	redactedCfg.ClientID = "[REDACTED]"
	redactedCfg.ClientSecret = "[REDACTED]"
	return json.Marshal(redactedCfg)
}

// Information about an action that returns a custom HTTP response.
type FixedResponseActionConfig struct {
	// The content type.
	// +optional
	ContentType *string `json:"contentType,omitempty"`

	// The message.
	// +optional
	MessageBody *string `json:"messageBody,omitempty"`

	// The HTTP response code.
	// +optional
	StatusCode string `json:"statusCode"`
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
	// +optional
	StatusCode string `json:"statusCode"`
}

// Information about how traffic will be distributed between multiple target groups in a forward rule.
type TargetGroupTuple struct {
	// The Amazon Resource Name (ARN) of the target group.
	TargetGroupARN core.StringToken `json:"targetGroupARN"`

	// The weight.
	// +optional
	Weight *int64 `json:"weight,omitempty"`
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

// Information about an action.
type Action struct {
	// The type of action.
	Type ActionType `json:"type"`

	// Information for using Amazon Cognito to authenticate users.
	// +optional
	AuthenticateCognitoConfig *AuthenticateCognitoActionConfig `json:"authenticateCognitoConfig,omitempty"`

	// Information about an identity provider that is compliant with OpenID Connect (OIDC).
	// +optional
	AuthenticateOIDCConfig *AuthenticateOIDCActionConfig `json:"authenticateOIDCConfig,omitempty"`

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

// Information about an SSL server certificate.
type Certificate struct {
	// The Amazon Resource Name (ARN) of the certificate.
	// +optional
	CertificateARN *string `json:"certificateARN,omitempty"`
}

// ALPNPolicy ALPN policy configuration for TLS listeners forwarding to TLS target groups
type ALPNPolicy string

// Supported ALPN policies
const (
	ALPNPolicyNone           ALPNPolicy = "None"
	ALPNPolicyHTTP1Only      ALPNPolicy = "HTTP1Only"
	ALPNPolicyHTTP2Only      ALPNPolicy = "HTTP2Only"
	ALPNPolicyHTTP2Optional  ALPNPolicy = "HTTP2Optional"
	ALPNPolicyHTTP2Preferred ALPNPolicy = "HTTP2Preferred"
)

// ListenerSpec defines the desired state of Listener
type ListenerSpec struct {
	// The Amazon Resource Name (ARN) of the load balancer.
	LoadBalancerARN core.StringToken `json:"loadBalancerARN"`

	// The port on which the load balancer is listening.
	Port int64 `json:"port"`

	// The protocol for connections from clients to the load balancer.
	Protocol Protocol `json:"protocol"`

	// The actions for the default rule.
	// +optional
	DefaultActions []Action `json:"defaultActions,omitempty"`

	// The SSL server certificate for a secure listener.
	// The first certificate is the default certificate.
	// +optional
	Certificates []Certificate `json:"certificates,omitempty"`

	// [HTTPS and TLS listeners] The security policy that defines which protocols and ciphers are supported.
	// +optional
	SSLPolicy *string `json:"sslPolicy,omitempty"`

	// [TLS listener] The name of the Application-Layer Protocol Negotiation (ALPN) policy.
	// +optional
	ALPNPolicy []string `json:"alpnPolicy,omitempty"`

	// The tags.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ListenerStatus defines the observed state of Listener
type ListenerStatus struct {
	// The Amazon Resource Name (ARN) of the listener.
	ListenerARN string `json:"listenerARN"`
}
