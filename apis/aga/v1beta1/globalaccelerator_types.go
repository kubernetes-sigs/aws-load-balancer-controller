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

// +kubebuilder:validation:Enum=TCP;UDP
// GlobalAcceleratorProtocol defines the protocol for Global Accelerator listeners.
type GlobalAcceleratorProtocol string

const (
	GlobalAcceleratorProtocolTCP GlobalAcceleratorProtocol = "TCP"
	GlobalAcceleratorProtocolUDP GlobalAcceleratorProtocol = "UDP"
)

// +kubebuilder:validation:Enum=SOURCE_IP;NONE
// ClientAffinityType defines the client affinity for Global Accelerator listeners.
type ClientAffinityType string

const (
	ClientAffinitySourceIP ClientAffinityType = "SOURCE_IP"
	ClientAffinityNone     ClientAffinityType = "NONE"
)

// +kubebuilder:validation:Enum=IPV4;DUAL_STACK
// IPAddressType defines the IP address type for Global Accelerator.
type IPAddressType string

const (
	IPAddressTypeIPV4      IPAddressType = "IPV4"
	IPAddressTypeDualStack IPAddressType = "DUAL_STACK"
)

// PortRange defines the port range for Global Accelerator listeners.
// +kubebuilder:validation:XValidation:rule="self.fromPort <= self.toPort",message="FromPort must be less than or equal to ToPort"
type PortRange struct {
	// FromPort is the first port in the range of ports, inclusive.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	FromPort int32 `json:"fromPort"`

	// ToPort is the last port in the range of ports, inclusive.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ToPort int32 `json:"toPort"`
}

// GlobalAcceleratorListener defines a listener for the Global Accelerator.
type GlobalAcceleratorListener struct {
	// Protocol is the protocol for the connections from clients to the accelerator.
	// When not specified, the controller will automatically determine the protocol by inspecting
	// the referenced Kubernetes resources (Service, Ingress, or Gateway) in the endpoint groups.
	// +optional
	Protocol *GlobalAcceleratorProtocol `json:"protocol,omitempty"`

	// PortRanges is the list of port ranges for the connections from clients to the accelerator.
	// When not specified, the controller will automatically determine the port ranges by inspecting
	// the referenced Kubernetes resources (Service, Ingress, or Gateway) in the endpoint groups.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	// +optional
	PortRanges *[]PortRange `json:"portRanges,omitempty"`

	// ClientAffinity lets you direct all requests from a user to the same endpoint, if you have stateful applications, regardless of the port and protocol of the client request.
	// Client affinity gives you control over whether to always route each client to the same specific endpoint.
	// AWS Global Accelerator uses a consistent-flow hashing algorithm to choose the optimal endpoint for a connection.
	// If client affinity is NONE, Global Accelerator uses the "five-tuple" (5-tuple) properties—source IP address, source port, destination IP address, destination port, and protocol—to select the hash value, and then chooses the best endpoint.
	// However, with this setting, if someone uses different ports to connect to Global Accelerator, their connections might not be always routed to the same endpoint because the hash value changes.
	// If you want a given client to always be routed to the same endpoint, set client affinity to SOURCE_IP instead.
	// When you use the SOURCE_IP setting, Global Accelerator uses the "two-tuple" (2-tuple) properties— source (client) IP address and destination IP address—to select the hash value.
	// The default value is NONE.
	// +kubebuilder:default="NONE"
	// +optional
	ClientAffinity ClientAffinityType `json:"clientAffinity,omitempty"`

	// EndpointGroups defines a list of endpoint groups for a Global Accelerator listener.
	// +optional
	EndpointGroups *[]GlobalAcceleratorEndpointGroup `json:"endpointGroups,omitempty"`
}

// GlobalAcceleratorEndpointGroup defines an endpoint group for a Global Accelerator listener.
type GlobalAcceleratorEndpointGroup struct {
	// Region is the AWS Region where the endpoint group is located.
	// If unspecified, defaults to the current cluster region.
	// +kubebuilder:validation:MaxLength=255
	// +optional
	Region *string `json:"region,omitempty"`

	// TrafficDialPercentage is the percentage of traffic to send to an AWS Regions. Additional traffic is distributed to other endpoint groups for this listener
	// Use this action to increase (dial up) or decrease (dial down) traffic to a specific Region. The percentage is applied to the traffic that would otherwise have been routed to the Region based on optimal routing.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=100
	// +optional
	TrafficDialPercentage *int32 `json:"trafficDialPercentage,omitempty"`

	// PortOverrides is a list of endpoint port overrides. Allows you to override the destination ports used to route traffic to an endpoint. Using a port override lets you map a list of external destination ports (that your users send traffic to) to a list of internal destination ports that you want an application endpoint to receive traffic on.
	// +optional
	PortOverrides *[]PortOverride `json:"portOverrides,omitempty"`

	// Endpoints is the list of endpoint configurations for this endpoint group.
	// +optional
	Endpoints *[]GlobalAcceleratorEndpoint `json:"endpoints,omitempty"`
}

// PortOverride defines a port override for an endpoint group.
// Override specific listener ports used to route traffic to endpoints that are part of an endpoint group.
// For example, you can create a port override in which the listener receives user traffic on ports 80 and 443,
// but your accelerator routes that traffic to ports 1080 and 1443, respectively, on the endpoints.
//
// For more information, see Port overrides in the AWS Global Accelerator Developer Guide:
// https://docs.aws.amazon.com/global-accelerator/latest/dg/about-endpoint-groups-port-override.html
type PortOverride struct {
	// ListenerPort is the listener port that you want to map to a specific endpoint port.
	// This is the port that user traffic arrives to the Global Accelerator on.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ListenerPort int32 `json:"listenerPort"`

	// EndpointPort is the endpoint port that you want traffic to be routed to.
	// This is the port on the endpoint, such as the Application Load Balancer or Amazon EC2 instance.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	EndpointPort int32 `json:"endpointPort"`
}

// +kubebuilder:validation:Enum=EndpointID;Service;Ingress;Gateway
// GlobalAcceleratorEndpointType defines the type of endpoint for Global Accelerator.
type GlobalAcceleratorEndpointType string

const (
	GlobalAcceleratorEndpointTypeEndpointID GlobalAcceleratorEndpointType = "EndpointID"
	GlobalAcceleratorEndpointTypeService    GlobalAcceleratorEndpointType = "Service"
	GlobalAcceleratorEndpointTypeIngress    GlobalAcceleratorEndpointType = "Ingress"
	GlobalAcceleratorEndpointTypeGateway    GlobalAcceleratorEndpointType = "Gateway"
)

// GlobalAcceleratorEndpoint defines an endpoint for a Global Accelerator endpoint group.
// +kubebuilder:validation:XValidation:rule="self.type != 'EndpointID' || (has(self.endpointID) && !has(self.name))",message="endpointID is required and name must not be set when type is EndpointID"
// +kubebuilder:validation:XValidation:rule="self.type == 'EndpointID' || (has(self.name) && !has(self.endpointID))",message="name is required and endpointID must not be set when type is Service/Ingress/Gateway"
type GlobalAcceleratorEndpoint struct {
	// Type specifies the type of endpoint reference.
	Type GlobalAcceleratorEndpointType `json:"type"`

	// EndpointID is the ID of the endpoint when type is EndpointID.
	// If the endpoint is a Network Load Balancer or Application Load Balancer, this is the Amazon Resource Name (ARN) of the resource.
	// A resource must be valid and active when you add it as an endpoint.
	// Mandatory for remote regions.
	// +kubebuilder:validation:MaxLength=255
	// +optional
	EndpointID *string `json:"endpointID,omitempty"`

	// Name is the name of the Kubernetes resource when type is Service, Ingress, or Gateway.
	// +optional
	Name *string `json:"name,omitempty"`

	// Namespace is the namespace of the Kubernetes resource when type is Service, Ingress, or Gateway.
	// If not specified, defaults to the same namespace as the GlobalAccelerator resource.
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// Weight is the weight associated with the endpoint. When you add weights to endpoints, you configure Global Accelerator to route traffic based on proportions that you specify.
	// For example, you might specify endpoint weights of 4, 5, 5, and 6 (sum=20). The result is that 4/20 of your traffic, on average, is routed to the first endpoint,
	// 5/20 is routed both to the second and third endpoints, and 6/20 is routed to the last endpoint.
	// For more information, see Endpoint Weights in the AWS Global Accelerator Developer Guide:
	// https://docs.aws.amazon.com/global-accelerator/latest/dg/about-endpoints-endpoint-weights.html
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=255
	// +kubebuilder:default=128
	// +optional
	Weight *int32 `json:"weight,omitempty"`

	// ClientIPPreservationEnabled indicates whether client IP address preservation is enabled for an Application Load Balancer endpoint.
	// The value is true or false. The default value is true for new accelerators.
	// If the value is set to true, the client's IP address is preserved in the X-Forwarded-For request header as traffic travels to applications on the Application Load Balancer endpoint fronted by the accelerator.
	// For more information, see Preserve Client IP Addresses in the AWS Global Accelerator Developer Guide:
	// https://docs.aws.amazon.com/global-accelerator/latest/dg/preserve-client-ip-address.html
	// +kubebuilder:default=true
	// +optional
	ClientIPPreservationEnabled *bool `json:"clientIPPreservationEnabled,omitempty"`
}

// GlobalAcceleratorSpec defines the desired state of GlobalAccelerator
type GlobalAcceleratorSpec struct {
	// Name is the name of the Global Accelerator.
	// The name must contain only alphanumeric characters or hyphens (-), and must not begin or end with a hyphen.
	// +kubebuilder:validation:Pattern="^[a-zA-Z0-9_-]{1,64}$"
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	// +optional
	Name *string `json:"name,omitempty"`

	// IpAddresses optionally specifies the IP addresses from your own IP address pool (BYOIP) to use for the accelerator's static IP addresses.
	// You can specify one or two addresses. Do not include the /32 suffix.
	// If you bring your own IP address pool to Global Accelerator (BYOIP), you can choose an IPv4 address from your own pool to use for the accelerator's static IPv4 address.
	// After you bring an address range to AWS, it appears in your account as an address pool. When you create an accelerator, you can assign one IPv4 address from your range to it.
	// Global Accelerator assigns you a second static IPv4 address from an Amazon IP address range. If you bring two IPv4 address ranges to AWS, you can assign one IPv4 address from each range to your accelerator.
	// Note that you can't update IP addresses for an existing accelerator. To change them, you must create a new accelerator with the new addresses.
	// For more information, see Bring your own IP addresses (BYOIP) in the AWS Global Accelerator Developer Guide.
	// https://docs.aws.amazon.com/global-accelerator/latest/dg/using-byoip.html
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=2
	// +optional
	IpAddresses *[]string `json:"ipAddresses,omitempty"`

	// IPAddressType is the value for the address type.
	// +kubebuilder:default="IPV4"
	// +optional
	IPAddressType IPAddressType `json:"ipAddressType,omitempty"`

	// Tags defines list of Tags on the Global Accelerator.
	// +optional
	Tags *map[string]string `json:"tags,omitempty"`

	// Listeners defines the listeners for the Global Accelerator.
	// +optional
	Listeners *[]GlobalAcceleratorListener `json:"listeners,omitempty"`
}

// GlobalAcceleratorStatus defines the observed state of GlobalAccelerator
type GlobalAcceleratorStatus struct {
	// The generation observed by the GlobalAccelerator controller.
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`

	// AcceleratorARN is the Amazon Resource Name (ARN) of the accelerator.
	// +optional
	AcceleratorARN *string `json:"acceleratorARN,omitempty"`

	// DNSName The Domain Name System (DNS) name that Global Accelerator creates that points to an accelerator's static IPv4 addresses.
	// +optional
	DNSName *string `json:"dnsName,omitempty"`

	// DualStackDnsName is the Domain Name System (DNS) name that Global Accelerator creates that points to a dual-stack accelerator's four static IP addresses: two IPv4 addresses and two IPv6 addresses.
	// +optional
	DualStackDnsName *string `json:"dualStackDnsName,omitempty"`

	// IPSets is the static IP addresses that Global Accelerator associates with the accelerator.
	// +optional
	IPSets []IPSet `json:"ipSets,omitempty"`

	// Status is the current status of the accelerator.
	// +optional
	Status *string `json:"status,omitempty"`

	// Conditions represent the current conditions of the GlobalAccelerator.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// IPSet is the static IP addresses that Global Accelerator associates with the accelerator.
type IPSet struct {

	// IpAddresses is the array of IP addresses in the IP address set.
	// +optional
	IpAddresses *[]string `json:"ipAddresses,omitempty"`

	// IpAddressFamily is the types of IP addresses included in this IP set.
	// +optional
	IpAddressFamily *string `json:"ipAddressFamily,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="NAME",type="string",JSONPath=".spec.name",description="The Global Accelerator name"
// +kubebuilder:printcolumn:name="DNS-NAME",type="string",JSONPath=".status.dnsName",description="The Global Accelerator DNS name"
// +kubebuilder:printcolumn:name="TYPE",type="string",JSONPath=".spec.type",description="The Global Accelerator type"
// +kubebuilder:printcolumn:name="STATUS",type="string",JSONPath=".status.status",description="The Global Accelerator status"
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".status.acceleratorARN",description="The Global Accelerator ARN",priority=1
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// GlobalAccelerator is the Schema for the GlobalAccelerator API
type GlobalAccelerator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GlobalAcceleratorSpec   `json:"spec,omitempty"`
	Status GlobalAcceleratorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// GlobalAcceleratorList contains a list of GlobalAccelerator
type GlobalAcceleratorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlobalAccelerator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GlobalAccelerator{}, &GlobalAcceleratorList{})
}
