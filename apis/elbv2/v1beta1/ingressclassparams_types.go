/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=ipv4;dualstack;dualstack-without-public-ipv4
// IPAddressType is the ip address type of load balancer.
type IPAddressType string

const (
	IPAddressTypeIPV4                       IPAddressType = "ipv4"
	IPAddressTypeDualStack                  IPAddressType = "dualstack"
	IPAddressTypeDualStackWithoutPublicIPV4 IPAddressType = "dualstack-without-public-ipv4"
)

// +kubebuilder:validation:Enum=internal;internet-facing
// Scheme is the scheme of load balancer.
//
// * the nodes of an internet-facing load balancer have public IP addresses.
// * the nodes of an internal load balancer have only private IP addresses.
type LoadBalancerScheme string

const (
	LoadBalancerSchemeInternal       LoadBalancerScheme = "internal"
	LoadBalancerSchemeInternetFacing LoadBalancerScheme = "internet-facing"
)

// SubnetID specifies a subnet ID.
// +kubebuilder:validation:Pattern=subnet-[0-9a-f]+
type SubnetID string

// SubnetSelector selects one or more existing subnets.
type SubnetSelector struct {
	// IDs specify the resource IDs of subnets. Exactly one of this or `tags` must be specified.
	// +kubebuilder:validation:MinItems=1
	// +optional
	IDs []SubnetID `json:"ids,omitempty"`

	// Tags specifies subnets in the load balancer's VPC where each
	// tag specified in the map key contains one of the values in the corresponding
	// value list.
	// Exactly one of this or `ids` must be specified.
	// +optional
	Tags map[string][]string `json:"tags,omitempty"`
}

// IngressGroup defines IngressGroup configuration.
type IngressGroup struct {
	// Name is the name of IngressGroup.
	Name string `json:"name"`
}

// Tag defines a AWS Tag on resources.
type Tag struct {
	// The key of the tag.
	Key string `json:"key"`

	// The value of the tag.
	Value string `json:"value"`
}

// Attributes defines custom attributes on resources.
type Attribute struct {
	// The key of the attribute.
	Key string `json:"key"`

	// The value of the attribute.
	Value string `json:"value"`
}

type ListenerProtocol string

const (
	ListenerProtocolHTTP  ListenerProtocol = "HTTP"
	ListenerProtocolHTTPS ListenerProtocol = "HTTPS"
)

type Listener struct {
	// The protocol of the listener
	Protocol ListenerProtocol `json:"protocol,omitempty"`
	// The port of the listener
	Port int32 `json:"port,omitempty"`
	// The attributes of the listener
	ListenerAttributes []Attribute `json:"listenerAttributes,omitempty"`
}

// Information about a load balancer capacity reservation.
type MinimumLoadBalancerCapacity struct {
	// The Capacity Units Value.
	CapacityUnits int32 `json:"capacityUnits"`
}

// IPAMConfiguration defines the IPAM configuration for an Ingress.
type IPAMConfiguration struct {
	// IPv4IPAMPoolId defines the IPAM pool ID used for IPv4 Addresses on the ALB.
	// +optional
	IPv4IPAMPoolId *string `json:"ipv4IPAMPoolId,omitempty"`
}

type AuthType string

const (
	AuthTypeNone    AuthType = "none"
	AuthTypeCognito AuthType = "cognito"
	AuthTypeOIDC    AuthType = "oidc"
)

// Amazon Cognito user pools configuration
type AuthIDPConfigCognito struct {
	// The Amazon Resource Name (ARN) of the Amazon Cognito user pool.
	UserPoolARN string `json:"userPoolARN"`

	// The ID of the Amazon Cognito user pool client.
	UserPoolClientID string `json:"userPoolClientID"`

	// The domain prefix or fully-qualified domain name of the Amazon Cognito user pool.
	// If you are using Amazon Cognito Domain, the userPoolDomain should be set to the domain prefix (my-domain) instead of full domain (https://my-domain.auth.us-west-2.amazoncognito.com).
	UserPoolDomain string `json:"userPoolDomain"`

	// The query parameters (up to 10) to include in the redirect request to the authorization endpoint.
	// +kubebuilder:validation:MinProperties=1
	// +kubebuilder:validation:MaxProperties=10
	// +optional
	AuthenticationRequestExtraParams map[string]string `json:"authenticationRequestExtraParams,omitempty"`
}

// OpenID Connect (OIDC) identity provider (IdP) configuration
type AuthIDPConfigOIDC struct {
	// The OIDC issuer identifier of the IdP.
	Issuer string `json:"issuer"`

	// The authorization endpoint of the IdP.
	AuthorizationEndpoint string `json:"authorizationEndpoint"`

	// The token endpoint of the IdP.
	TokenEndpoint string `json:"tokenEndpoint"`

	// The user info endpoint of the IdP.
	UserInfoEndpoint string `json:"userInfoEndpoint"`

	// The k8s secret name. The secret must be in the 'default' namespace.
	// Example format:
	//   apiVersion: v1
	//   kind: Secret
	//   metadata:
	//     namespace: default
	//     name: my-k8s-secret
	//   data:
	//     clientID: base64 of your plain text clientId
	//     clientSecret: base64 of your plain text clientSecret
	SecretName string `json:"secretName"`

	// The query parameters (up to 10) to include in the redirect request to the authorization endpoint.
	// +kubebuilder:validation:MinProperties=1
	// +kubebuilder:validation:MaxProperties=10
	// +optional
	AuthenticationRequestExtraParams map[string]string `json:"authenticationRequestExtraParams,omitempty"`
}

// Authentication configuration for Ingress
type AuthConfig struct {
	// The authentication type on targets.
	// +kubebuilder:validation:Enum=none;oidc;cognito
	Type AuthType `json:"type"`

	// The Cognito IdP configuration.
	// +optional
	IDPConfigCognito *AuthIDPConfigCognito `json:"idpCognitoConfiguration,omitempty"`

	// The OIDC IdP configuration.
	// +optional
	IDPConfigOIDC *AuthIDPConfigOIDC `json:"idpOidcConfiguration,omitempty"`

	// The behavior if the user is not authenticated.
	// +kubebuilder:validation:Enum=authenticate;deny;allow
	// +optional
	OnUnauthenticatedRequest string `json:"onUnauthenticatedRequest,omitempty"`

	// The set of user claims to be requested from the Cognito IdP or OIDC IdP, in a space-separated list.
	// * Options: phone, email, profile, openid, aws.cognito.signin.user.admin
	// * Ex. 'email openid'
	// +optional
	Scope string `json:"scope,omitempty"`

	// The name of the cookie used to maintain session information.
	// +optional
	SessionCookieName string `json:"sessionCookie,omitempty"`

	// The maximum duration of the authentication session, in seconds.
	// +optional
	SessionTimeout *int64 `json:"sessionTimeout,omitempty"`
}

// IngressClassParamsSpec defines the desired state of IngressClassParams
type IngressClassParamsSpec struct {
	// CertificateArn specifies the ARN of the certificates for all Ingresses that belong to IngressClass with this IngressClassParams.
	// +optional
	CertificateArn []string `json:"certificateArn,omitempty"`

	// NamespaceSelector restrict the namespaces of Ingresses that are allowed to specify the IngressClass with this IngressClassParams.
	// * If absent or present but empty, it selects all namespaces.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// Group defines the IngressGroup for all Ingresses that belong to IngressClass with this IngressClassParams.
	// +optional
	Group *IngressGroup `json:"group,omitempty"`

	// Scheme defines the scheme for all Ingresses that belong to IngressClass with this IngressClassParams.
	// +optional
	Scheme *LoadBalancerScheme `json:"scheme,omitempty"`

	// InboundCIDRs specifies the CIDRs that are allowed to access the Ingresses that belong to IngressClass with this IngressClassParams.
	// +optional
	InboundCIDRs []string `json:"inboundCIDRs,omitempty"`

	// SSLPolicy specifies the SSL Policy for all Ingresses that belong to IngressClass with this IngressClassParams.
	// +optional
	SSLPolicy string `json:"sslPolicy,omitEmpty"`

	// Subnets defines the subnets for all Ingresses that belong to IngressClass with this IngressClassParams.
	// +optional
	Subnets *SubnetSelector `json:"subnets,omitempty"`

	// IPAddressType defines the IP address type for all Ingresses that belong to IngressClass with this IngressClassParams.
	// +optional
	IPAddressType *IPAddressType `json:"ipAddressType,omitempty"`

	// Tags defines list of Tags on AWS resources provisioned for Ingresses that belong to IngressClass with this IngressClassParams.
	// +optional
	Tags []Tag `json:"tags,omitempty"`

	// LoadBalancerAttributes define the custom attributes to LoadBalancers for all Ingress that that belong to IngressClass with this IngressClassParams.
	// +optional
	LoadBalancerAttributes []Attribute `json:"loadBalancerAttributes,omitempty"`

	// Listeners define a list of listeners with their protocol, port and attributes.
	// +optional
	Listeners []Listener `json:"listeners,omitempty"`

	// MinimumLoadBalancerCapacity define the capacity reservation for LoadBalancers for all Ingress that belong to IngressClass with this IngressClassParams.
	// +optional
	MinimumLoadBalancerCapacity *MinimumLoadBalancerCapacity `json:"minimumLoadBalancerCapacity,omitempty"`

	// IPAMConfiguration defines the IPAM settings for a Load Balancer.
	// +optional
	IPAMConfiguration *IPAMConfiguration `json:"ipamConfiguration,omitempty"`

	// PrefixListsIDs defines the security group prefix lists for all Ingresses that belong to IngressClass with this IngressClassParams.
	// +optional
	PrefixListsIDs []string `json:"PrefixListsIDs,omitempty"`

	// AuthenticationConfiguration defines the authentication configuration for a Load Balancer. Application Load Balancer (ALB) supports authentication with Cognito or OIDC.
	// +optional
	AuthConfig *AuthConfig `json:"authenticationConfiguration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="GROUP-NAME",type="string",JSONPath=".spec.group.name",description="The Ingress Group name"
// +kubebuilder:printcolumn:name="SCHEME",type="string",JSONPath=".spec.scheme",description="The AWS Load Balancer scheme"
// +kubebuilder:printcolumn:name="IP-ADDRESS-TYPE",type="string",JSONPath=".spec.ipAddressType",description="The AWS Load Balancer ipAddressType"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// IngressClassParams is the Schema for the IngressClassParams API
type IngressClassParams struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec IngressClassParamsSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// IngressClassParamsList contains a list of IngressClassParams
type IngressClassParamsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IngressClassParams `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IngressClassParams{}, &IngressClassParamsList{})
}
