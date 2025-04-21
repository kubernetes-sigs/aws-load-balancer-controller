package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=internal;internet-facing
// LoadBalancerScheme is the scheme of your LB
//
// * with `internal` scheme, the LB is only accessible within the VPC.
// * with `internet-facing` scheme, the LB is accesible via the public internet.
type LoadBalancerScheme string

const (
	LoadBalancerSchemeInternal       LoadBalancerScheme = "internal"
	LoadBalancerSchemeInternetFacing LoadBalancerScheme = "internet-facing"
)

// +kubebuilder:validation:Enum=ipv4;dualstack;dualstack-without-public-ipv4
// LoadBalancerIpAddressType is the IP Address type of your LB.
type LoadBalancerIpAddressType string

const (
	LoadBalancerIpAddressTypeIPv4                       LoadBalancerIpAddressType = "ipv4"
	LoadBalancerIpAddressTypeDualstack                  LoadBalancerIpAddressType = "dualstack"
	LoadBalancerIpAddressTypeDualstackWithoutPublicIpv4 LoadBalancerIpAddressType = "dualstack-without-public-ipv4"
)

// LoadBalancerAttribute defines LB attribute.
type LoadBalancerAttribute struct {
	// The key of the attribute.
	Key string `json:"key"`

	// The value of the attribute.
	Value string `json:"value"`
}

// ListenerAttribute defines listener attribute.
type ListenerAttribute struct {
	// The key of the attribute.
	Key string `json:"key"`

	// The value of the attribute.
	Value string `json:"value"`
}

// AWSTag defines a AWS Tag on resources.
type AWSTag struct {
	// The key of the tag.
	Key string `json:"key"`

	// The value of the tag.
	Value string `json:"value"`
}

// SubnetConfiguration defines the subnet settings for a Load Balancer.
type SubnetConfiguration struct {
	// identifier [Application LoadBalancer / Network LoadBalancer] name or id for the subnet
	// +optional
	Identifier string `json:"identifier"`

	// eipAllocation [Network LoadBalancer] the EIP name for this subnet.
	// +optional
	EIPAllocation *string `json:"eipAllocation,omitempty"`

	// privateIPv4Allocation [Network LoadBalancer] the private ipv4 address to assign to this subnet.
	// +optional
	PrivateIPv4Allocation *string `json:"privateIPv4Allocation,omitempty"`

	// IPv6Allocation [Network LoadBalancer] the ipv6 address to assign to this subnet.
	// +optional
	IPv6Allocation *string `json:"ipv6Allocation,omitempty"`

	// SourceNatIPv6Prefix [Network LoadBalancer] The IPv6 prefix to use for source NAT. Specify an IPv6 prefix (/80 netmask) from the subnet CIDR block or auto_assigned to use an IPv6 prefix selected at random from the subnet CIDR block.
	// +optional
	SourceNatIPv6Prefix *string `json:"sourceNatIPv6Prefix,omitempty"`
}

// +kubebuilder:validation:Enum=HTTP1Only;HTTP2Only;HTTP2Optional;HTTP2Preferred;None
// ALPNPolicy defines the ALPN policy configuration for TLS listeners forwarding to TLS target groups
// HTTP1Only Negotiate only HTTP/1.*. The ALPN preference list is http/1.1, http/1.0.
// HTTP2Only Negotiate only HTTP/2. The ALPN preference list is h2.
// HTTP2Optional Prefer HTTP/1.* over HTTP/2 (which can be useful for HTTP/2 testing). The ALPN preference list is http/1.1, http/1.0, h2.
// HTTP2Preferred Prefer HTTP/2 over HTTP/1.*. The ALPN preference list is h2, http/1.1, http/1.0.
// None Do not negotiate ALPN. This is the default.
type ALPNPolicy string

// Supported ALPN policies
const (
	ALPNPolicyNone           ALPNPolicy = "None"
	ALPNPolicyHTTP1Only      ALPNPolicy = "HTTP1Only"
	ALPNPolicyHTTP2Only      ALPNPolicy = "HTTP2Only"
	ALPNPolicyHTTP2Optional  ALPNPolicy = "HTTP2Optional"
	ALPNPolicyHTTP2Preferred ALPNPolicy = "HTTP2Preferred"
)

// +kubebuilder:validation:Enum=on;off
type AdvertiseTrustStoreCaNamesEnum string

// Enum values for AdvertiseTrustStoreCaNamesEnum
const (
	AdvertiseTrustStoreCaNamesEnumOn  AdvertiseTrustStoreCaNamesEnum = "on"
	AdvertiseTrustStoreCaNamesEnumOff AdvertiseTrustStoreCaNamesEnum = "off"
)

// +kubebuilder:validation:Enum=off;passthrough;verify
// MutualAuthenticationMode mTLS mode for mutual TLS authentication config for listener
type MutualAuthenticationMode string

// Supported mTLS modes
const (
	MutualAuthenticationOffMode         MutualAuthenticationMode = "off"
	MutualAuthenticationPassthroughMode MutualAuthenticationMode = "passthrough"
	MutualAuthenticationVerifyMode      MutualAuthenticationMode = "verify"
)

// Information about the mutual authentication attributes of a listener.
type MutualAuthenticationAttributes struct {

	// Indicates whether trust store CA certificate names are advertised.
	// +optional
	AdvertiseTrustStoreCaNames *AdvertiseTrustStoreCaNamesEnum `json:"advertiseTrustStoreCaNames,omitempty"`

	// Indicates whether expired client certificates are ignored.
	// +optional
	IgnoreClientCertificateExpiry *bool `json:"ignoreClientCertificateExpiry,omitempty"`

	// The client certificate handling method. Options are off , passthrough or verify
	Mode MutualAuthenticationMode `json:"mode"`

	// The Name or ARN of the trust store.
	// +optional
	TrustStore *string `json:"trustStore,omitempty"`
}

// +kubebuilder:validation:Pattern="^(HTTP|HTTPS|TLS|TCP|UDP)?:(6553[0-5]|655[0-2]\\d|65[0-4]\\d{2}|6[0-4]\\d{3}|[1-5]\\d{4}|[1-9]\\d{0,3})?$"
type ProtocolPort string
type ListenerConfiguration struct {
	// protocolPort is identifier for the listener on load balancer. It should be of the form PROTOCOL:PORT
	ProtocolPort ProtocolPort `json:"protocolPort"`

	// TODO: Add validation in admission webhook to make it required for secure protocols
	// defaultCertificate the cert arn to be used by default.
	DefaultCertificate *string `json:"defaultCertificate,omitempty"`

	// certificates is the list of other certificates to add to the listener.
	// +optional
	Certificates []*string `json:"certificates,omitempty"`

	// sslPolicy is the security policy that defines which protocols and ciphers are supported for secure listeners [HTTPS or TLS listener].
	SslPolicy *string `json:"sslPolicy,omitempty"`

	// alpnPolicy an optional string that allows you to configure ALPN policies on your Load Balancer
	// +optional
	ALPNPolicy *ALPNPolicy `json:"alpnPolicy,omitempty"`

	// mutualAuthentication defines the mutual authentication configuration information.
	// +optional
	MutualAuthentication *MutualAuthenticationAttributes `json:"mutualAuthentication,omitempty"`

	// listenerAttributes defines the attributes for the listener
	// +optional
	ListenerAttributes []ListenerAttribute `json:"listenerAttributes,omitempty"`
}

// LoadBalancerConfigurationSpec defines the desired state of LoadBalancerConfiguration
type LoadBalancerConfigurationSpec struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	// loadBalancerName defines the name of the LB to provision. If unspecified, it will be automatically generated.
	// +optional
	LoadBalancerName *string `json:"loadBalancerName,omitempty"`

	// scheme defines the type of LB to provision. If unspecified, it will be automatically inferred.
	// +optional
	Scheme *LoadBalancerScheme `json:"scheme,omitempty"`

	// loadBalancerIPType defines what kind of load balancer to provision (ipv4, dual stack)
	// +optional
	IpAddressType *LoadBalancerIpAddressType `json:"ipAddressType,omitempty"`

	// enforceSecurityGroupInboundRulesOnPrivateLinkTraffic Indicates whether to evaluate inbound security group rules for traffic sent to a Network Load Balancer through Amazon Web Services PrivateLink.
	// +optional
	EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic *string `json:"enforceSecurityGroupInboundRulesOnPrivateLinkTraffic,omitempty"`

	// customerOwnedIpv4Pool [Application LoadBalancer]
	// is the ID of the customer-owned address for Application Load Balancers on Outposts pool.
	// +optional
	CustomerOwnedIpv4Pool *string `json:"customerOwnedIpv4Pool,omitempty"`

	// IPv4IPAMPoolId [Application LoadBalancer]
	// defines the IPAM pool ID used for IPv4 Addresses on the ALB.
	// +optional
	IPv4IPAMPoolId *string `json:"ipv4IPAMPoolId,omitempty"`

	// loadBalancerSubnets is an optional list of subnet configurations to be used in the LB
	// This value takes precedence over loadBalancerSubnetsSelector if both are selected.
	// +optional
	LoadBalancerSubnets *[]SubnetConfiguration `json:"loadBalancerSubnets,omitempty"`

	// LoadBalancerSubnetsSelector specifies subnets in the load balancer's VPC where each
	// tag specified in the map key contains one of the values in the corresponding
	// value list.
	// +optional
	LoadBalancerSubnetsSelector *map[string][]string `json:"loadBalancerSubnetsSelector,omitempty"`

	// listenerConfigurations is an optional list of configurations for each listener on LB
	// +optional
	ListenerConfigurations *[]ListenerConfiguration `json:"listenerConfigurations,omitempty"`

	// securityGroups an optional list of security group ids or names to apply to the LB
	// +optional
	SecurityGroups *[]string `json:"securityGroups,omitempty"`

	// securityGroupPrefixes an optional list of prefixes that are allowed to access the LB.
	// +optional
	SecurityGroupPrefixes *[]string `json:"securityGroupPrefixes,omitempty"`

	// sourceRanges an optional list of CIDRs that are allowed to access the LB.
	// +optional
	SourceRanges *[]string `json:"sourceRanges,omitempty"`

	// vpcId is the ID of the VPC for the load balancer.
	// +optional
	VpcId *string `json:"vpcId,omitempty"`

	// LoadBalancerAttributes defines the attribute of LB
	// +optional
	LoadBalancerAttributes []LoadBalancerAttribute `json:"loadBalancerAttributes,omitempty"`

	// Tags defines list of Tags on LB.
	// +optional
	Tags []AWSTag `json:"tags,omitempty"`

	// EnableICMP [Network LoadBalancer]
	// enables the creation of security group rules to the managed security group
	// to allow explicit ICMP traffic for Path MTU discovery for IPv4 and dual-stack VPCs
	// +optional
	EnableICMP bool `json:"enableICMP,omitempty"`

	// ManageBackendSecurityGroupRules [Application / Network LoadBalancer]
	// specifies whether you want the controller to configure security group rules on Node/Pod for traffic access
	// when you specify securityGroups
	// +optional
	ManageBackendSecurityGroupRules bool `json:"manageBackendSecurityGroupRules,omitempty"`
}

// TODO -- these can be used to set what generation the gateway is currently on to track progress on reconcile.

// LoadBalancerConfigurationStatus defines the observed state of TargetGroupBinding
type LoadBalancerConfigurationStatus struct {
	// The generation of the Gateway Configuration attached to the Gateway object.
	// +optional
	ObservedGatewayConfigurationGeneration *int64 `json:"observedGatewayConfigurationGeneration,omitempty"`
	// The generation of the Gateway Configuration attached to the GatewayClass object.
	// +optional
	ObservedGatewayClassConfigurationGeneration *int64 `json:"observedGatewayClassConfigurationGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// LoadBalancerConfiguration is the Schema for the LoadBalancerConfiguration API
type LoadBalancerConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoadBalancerConfigurationSpec   `json:"spec,omitempty"`
	Status LoadBalancerConfigurationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LoadBalancerConfigurationList contains a list of LoadBalancerConfiguration
type LoadBalancerConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoadBalancerConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LoadBalancerConfiguration{}, &LoadBalancerConfigurationList{})
}
