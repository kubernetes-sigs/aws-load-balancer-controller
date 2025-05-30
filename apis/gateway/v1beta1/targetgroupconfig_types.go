package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

// Reference defines how to look up the Target Group configuration for a service.
type Reference struct {
	// Group is the group of the referent. For example, "gateway.networking.k8s.io".
	// When unspecified or empty string, core API group is inferred.
	//
	// +optional
	// +kubebuilder:default=""
	Group *string `json:"group,omitempty"`

	// Kind is the Kubernetes resource kind of the referent. For example
	// "Service".
	//
	// Defaults to "Service" when not specified.
	// +optional
	// +kubebuilder:default=Service
	Kind *string `json:"kind,omitempty"`

	// Name is the name of the referent.
	Name string `json:"name"`
}

// TODO: Add a validation in the admission webhook to check if only one of HTTPCode or GRPCCode is set.
// Information to use when checking for a successful response from a target.
type HealthCheckMatcher struct {
	// The HTTP codes.
	HTTPCode *string `json:"httpCode,omitempty"`

	// The gRPC codes
	GRPCCode *string `json:"grpcCode,omitempty"`
}

// HealthCheckConfiguration defines the Health Check configuration for a Target Group.
type HealthCheckConfiguration struct {
	// healthyThresholdCount The number of consecutive health checks successes required before considering an unhealthy target healthy.
	// +optional
	HealthyThresholdCount *int32 `json:"healthyThresholdCount,omitempty"`

	// healthCheckInterval The approximate amount of time, in seconds, between health checks of an individual target.
	// +optional
	HealthCheckInterval *int32 `json:"healthCheckInterval,omitempty"`

	// healthCheckPath The destination for health checks on the targets.
	// +optional
	HealthCheckPath *string `json:"healthCheckPath,omitempty"`

	// healthCheckPort The port the load balancer uses when performing health checks on targets.
	// The default is to use the port on which each target receives traffic from the load balancer.
	// +optional
	HealthCheckPort *string `json:"healthCheckPort,omitempty"`

	// healthCheckProtocol The protocol to use to connect with the target. The GENEVE, TLS, UDP, and TCP_UDP protocols are not supported for health checks.
	// +optional
	HealthCheckProtocol *TargetGroupHealthCheckProtocol `json:"healthCheckProtocol,omitempty"`

	// healthCheckTimeout The amount of time, in seconds, during which no response means a failed health check
	// +optional
	HealthCheckTimeout *int32 `json:"healthCheckTimeout,omitempty"`

	// unhealthyThresholdCount The number of consecutive health check failures required before considering the target unhealthy.
	// +optional
	UnhealthyThresholdCount *int32 `json:"unhealthyThresholdCount,omitempty"`

	// healthCheckCodes The HTTP or gRPC codes to use when checking for a successful response from a target
	// +optional
	Matcher *HealthCheckMatcher `json:"matcher,omitempty"`
}

// +kubebuilder:validation:Enum=ipv4;ipv6
// TargetGroupIPAddressType is the IP Address type of your ELBV2 TargetGroup.
type TargetGroupIPAddressType string

const (
	TargetGroupIPAddressTypeIPv4 TargetGroupIPAddressType = "ipv4"
	TargetGroupIPAddressTypeIPv6 TargetGroupIPAddressType = "ipv6"
)

// +kubebuilder:validation:Enum=instance;ip
// TargetType is the targetType of your ELBV2 TargetGroup.
//
// * with `instance` TargetType, nodes with nodePort for your service will be registered as targets
// * with `ip` TargetType, Pods with containerPort for your service will be registered as targets
type TargetType string

const (
	TargetTypeInstance TargetType = "instance"
	TargetTypeIP       TargetType = "ip"
)

// +kubebuilder:validation:Enum=http;https;tcp
type TargetGroupHealthCheckProtocol string

const (
	TargetGroupHealthCheckProtocolHTTP  TargetGroupHealthCheckProtocol = "HTTP"
	TargetGroupHealthCheckProtocolHTTPS TargetGroupHealthCheckProtocol = "HTTPS"
	TargetGroupHealthCheckProtocolTCP   TargetGroupHealthCheckProtocol = "TCP"
)

// +kubebuilder:validation:Enum=HTTP;HTTPS;TCP;TLS;UDP;TCP_UDP
type Protocol string

const (
	ProtocolHTTP    Protocol = "HTTP"
	ProtocolHTTPS   Protocol = "HTTPS"
	ProtocolTCP     Protocol = "TCP"
	ProtocolTLS     Protocol = "TLS"
	ProtocolUDP     Protocol = "UDP"
	ProtocolTCP_UDP Protocol = "TCP_UDP"
)

// +kubebuilder:validation:Enum=HTTP1;HTTP2;GRPC
type ProtocolVersion string

const (
	ProtocolVersionHTTP1 ProtocolVersion = "HTTP1"
	ProtocolVersionHTTP2 ProtocolVersion = "HTTP2"
	ProtocolVersionGRPC  ProtocolVersion = "GRPC"
)

// +kubebuilder:validation:Enum=none;prefer-route-specific;prefer-default
type TargetGroupConfigMergeMode string

const (
	MergeModeNone                TargetGroupConfigMergeMode = "none"
	MergeModePreferRouteSpecific TargetGroupConfigMergeMode = "prefer-route-specific"
	MergeModePreferDefault       TargetGroupConfigMergeMode = "prefer-default"
)

// TargetGroupConfigurationSpec defines the TargetGroup properties for a route.
type TargetGroupConfigurationSpec struct {

	// targetReference the kubernetes object to attach the Target Group settings to.
	TargetReference Reference `json:"targetReference"`

	// mergeMode the mode to use for merging the identified per-route configuration and default configuration.

	// defaultRouteConfiguration fallback configuration applied to all routes, unless overridden by route-specific configurations.
	// +optional
	DefaultConfiguration TargetGroupProps `json:"defaultConfiguration,omitempty"`

	// routeConfigurations the route configuration for specific routes
	// +optional
	RouteConfigurations []RouteConfiguration `json:"routeConfigurations,omitempty"`
}

// +kubebuilder:validation:Pattern="^(HTTPRoute|TLSRoute|TCPRoute|UDPRoute|GRPCRoute)?:([^:]+)?:([^:]+)?$"
type RouteIdentifier string

// RouteConfiguration defines the per route configuration
type RouteConfiguration struct {
	// name the identifier of the route, it should be in the form of ROUTE:NAMESPACE:NAME
	Identifier RouteIdentifier `json:"identifier"`

	// targetGroupProps the target group specific properties
	TargetGroupProps TargetGroupProps `json:"targetGroupProps"`
}

// TargetGroupProps defines the target group properties
type TargetGroupProps struct {
	// targetGroupName specifies the name to assign to the Target Group. If not defined, then one is generated.
	// +optional
	TargetGroupName string `json:"targetGroupName,omitempty"`

	// ipAddressType specifies whether the target group is of type IPv4 or IPv6. If unspecified, it will be automatically inferred.
	// +optional
	IPAddressType *TargetGroupIPAddressType `json:"ipAddressType,omitempty"`

	// healthCheckConfig The Health Check configuration for this backend.
	// +optional
	HealthCheckConfig *HealthCheckConfiguration `json:"healthCheckConfig,omitempty"`

	// node selector for instance type target groups to only register certain nodes
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`

	// targetGroupAttributes defines the attribute of target group
	// +optional
	TargetGroupAttributes []TargetGroupAttribute `json:"targetGroupAttributes,omitempty"`

	// targetType is the TargetType of TargetGroup. If unspecified, it will be automatically inferred as instance.
	// +optional
	TargetType *TargetType `json:"targetType,omitempty"`

	// Protocol [Application / Network Load Balancer] the protocol for the target group.
	// If unspecified, it will be automatically inferred.
	// +optional
	Protocol *Protocol `json:"protocol,omitempty"`

	// protocolVersion [HTTP/HTTPS protocol] The protocol version. The possible values are GRPC , HTTP1 and HTTP2
	// +optional
	ProtocolVersion *ProtocolVersion `json:"protocolVersion,omitempty"`

	// EnableMultiCluster [Application / Network LoadBalancer]
	// Allows for multiple Clusters / Services to use the generated TargetGroup ARN
	// +optional
	EnableMultiCluster bool `json:"enableMultiCluster,omitempty"`

	// vpcID is the VPC of the TargetGroup. If unspecified, it will be automatically inferred.
	// +optional
	VpcID *string `json:"vpcID,omitempty"`

	// Tags defines list of Tags on target group.
	// +optional
	Tags []Tag `json:"tags,omitempty"`
}

// TargetGroupAttribute defines target group attribute.
type TargetGroupAttribute struct {
	// The key of the attribute.
	Key string `json:"key"`

	// The value of the attribute.
	Value string `json:"value"`
}

// Tag defines a AWS Tag on resources.
type Tag struct {
	// The key of the tag.
	Key string `json:"key"`

	// The value of the tag.
	Value string `json:"value"`
}

// TODO -- these can be used to set what generation the gateway is currently on to track progress on reconcile.

// TargetGroupConfigurationStatus defines the observed state of TargetGroupConfiguration
type TargetGroupConfigurationStatus struct {
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
// +kubebuilder:printcolumn:name="SERVICE-NAME",type="string",JSONPath=".spec.targetReference.name",description="The Kubernetes Service's name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// TargetGroupConfiguration is the Schema for defining TargetGroups with an AWS ELB Gateway
type TargetGroupConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TargetGroupConfigurationSpec   `json:"spec,omitempty"`
	Status TargetGroupConfigurationStatus `json:"status,omitempty"`
}

func (tgConfig *TargetGroupConfiguration) GetTargetGroupConfigForRoute(name, namespace, kind string) *TargetGroupProps {
	return &tgConfig.Spec.DefaultConfiguration
}

// +kubebuilder:object:root=true

// TargetGroupConfigurationList contains a list of TargetGroupConfiguration
type TargetGroupConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TargetGroupConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TargetGroupConfiguration{}, &TargetGroupConfigurationList{})
}
