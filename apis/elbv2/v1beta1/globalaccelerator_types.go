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
	"k8s.io/apimachinery/pkg/util/intstr"
)

// +kubebuilder:validation:Enum=STANDARD;CUSTOM_ROUTING
// AcceleratorType is the type of Global Accelerator.
type AcceleratorType string

const (
	AcceleratorTypeStandard      AcceleratorType = "STANDARD"
	AcceleratorTypeCustomRouting AcceleratorType = "CUSTOM_ROUTING"
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

// PortRange defines the port range for Global Accelerator listeners.
type PortRange struct {
	// FromPort is the first port in the range of ports.
	FromPort int32 `json:"fromPort"`

	// ToPort is the last port in the range of ports.
	ToPort int32 `json:"toPort"`
}

// GlobalAcceleratorListener defines a listener for the Global Accelerator.
type GlobalAcceleratorListener struct {
	// Protocol is the protocol for the connections from clients to the accelerator.
	Protocol GlobalAcceleratorProtocol `json:"protocol"`

	// PortRanges is the list of port ranges to support for connections from clients to the accelerator.
	PortRanges []PortRange `json:"portRanges"`

	// ClientAffinity controls whether traffic from the same client IP is routed to the same endpoint.
	// +optional
	ClientAffinity *ClientAffinityType `json:"clientAffinity,omitempty"`
}

// EndpointGroup defines an endpoint group for a Global Accelerator listener.
type EndpointGroup struct {
	// Region is the AWS Region where the endpoint group is located.
	Region string `json:"region"`

	// TrafficDialPercentage is the percentage of traffic to send to this endpoint group.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +optional
	TrafficDialPercentage *int32 `json:"trafficDialPercentage,omitempty"`

	// HealthCheckIntervalSeconds is the interval in seconds between health checks.
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=30
	// +optional
	HealthCheckIntervalSeconds *int32 `json:"healthCheckIntervalSeconds,omitempty"`

	// HealthCheckPath is the path that you want to use for health checks.
	// +optional
	HealthCheckPath *string `json:"healthCheckPath,omitempty"`

	// ThresholdCount is the number of consecutive health check failures required before considering the endpoint unhealthy.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	// +optional
	ThresholdCount *int32 `json:"thresholdCount,omitempty"`

	// PortOverrides is a list of endpoint port overrides.
	// +optional
	PortOverrides []PortOverride `json:"portOverrides,omitempty"`

	// Endpoints is the list of endpoint configurations for this endpoint group.
	Endpoints []GlobalAcceleratorEndpoint `json:"endpoints"`
}

// PortOverride defines a port override for an endpoint group.
type PortOverride struct {
	// ListenerPort is the listener port that you want to map to a specific endpoint port.
	ListenerPort int32 `json:"listenerPort"`

	// EndpointPort is the endpoint port that you want traffic to be routed to.
	EndpointPort int32 `json:"endpointPort"`
}

// GlobalAcceleratorEndpoint defines an endpoint for a Global Accelerator endpoint group.
type GlobalAcceleratorEndpoint struct {
	// EndpointID is the ID of the endpoint.
	// For Application Load Balancers, this is the ARN.
	// For Network Load Balancers, this is the ARN.
	// For EC2 instances, this is the instance ID.
	// For Elastic IP addresses, this is the allocation ID.
	EndpointID string `json:"endpointID"`

	// Weight is used to determine the proportion of traffic that is directed to an endpoint.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=255
	// +optional
	Weight *int32 `json:"weight,omitempty"`

	// ClientIPPreservationEnabled indicates whether client IP address preservation is enabled.
	// +optional
	ClientIPPreservationEnabled *bool `json:"clientIPPreservationEnabled,omitempty"`
}

// ServiceEndpointReference defines a reference to a Kubernetes Service that should be used as an endpoint.
type ServiceEndpointReference struct {
	// Name is the name of the Service.
	Name string `json:"name"`

	// Namespace is the namespace of the Service.
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// Port is the port of the ServicePort.
	Port intstr.IntOrString `json:"port"`

	// Weight is used to determine the proportion of traffic that is directed to this service endpoint.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=255
	// +optional
	Weight *int32 `json:"weight,omitempty"`
}

// GlobalAcceleratorSpec defines the desired state of GlobalAccelerator
type GlobalAcceleratorSpec struct {
	// Name is the name of the Global Accelerator.
	// +optional
	Name *string `json:"name,omitempty"`

	// Type is the type of accelerator.
	// +optional
	Type *AcceleratorType `json:"type,omitempty"`

	// IPAddressType is the value for the address type.
	// +kubebuilder:validation:Enum=IPV4;DUAL_STACK
	// +optional
	IPAddressType *string `json:"ipAddressType,omitempty"`

	// Enabled indicates whether the accelerator is enabled.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Attributes define the custom attributes to Global Accelerator.
	// +optional
	Attributes []Attribute `json:"attributes,omitempty"`

	// Tags defines list of Tags on the Global Accelerator.
	// +optional
	Tags []Tag `json:"tags,omitempty"`

	// Listeners defines the listeners for the Global Accelerator.
	Listeners []GlobalAcceleratorListener `json:"listeners"`

	// EndpointGroups defines the endpoint groups for the Global Accelerator listeners.
	EndpointGroups []EndpointGroup `json:"endpointGroups"`

	// ServiceEndpoints defines Kubernetes services that should be automatically configured as endpoints.
	// +optional
	ServiceEndpoints []ServiceEndpointReference `json:"serviceEndpoints,omitempty"`

	// IAM Role ARN to assume when calling AWS APIs.
	// +optional
	IamRoleArnToAssume *string `json:"iamRoleArnToAssume,omitempty"`

	// AssumeRoleExternalId is the external ID for assume role operations.
	// +optional
	AssumeRoleExternalId *string `json:"assumeRoleExternalId,omitempty"`
}

// GlobalAcceleratorStatus defines the observed state of GlobalAccelerator
type GlobalAcceleratorStatus struct {
	// The generation observed by the GlobalAccelerator controller.
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`

	// AcceleratorARN is the Amazon Resource Name (ARN) of the accelerator.
	// +optional
	AcceleratorARN *string `json:"acceleratorARN,omitempty"`

	// DNSName is the Domain Name System (DNS) name that Global Accelerator creates that points to your accelerator's static IP addresses.
	// +optional
	DNSName *string `json:"dnsName,omitempty"`

	// IPSets is information about the IP address type.
	// +optional
	IPSets []IPSet `json:"ipSets,omitempty"`

	// Status is the current status of the accelerator.
	// +optional
	Status *string `json:"status,omitempty"`

	// Conditions represent the current conditions of the GlobalAccelerator.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// IPSet contains information about the IP address type.
type IPSet struct {
	// IpFamily is the IP address version.
	// +optional
	IpFamily *string `json:"ipFamily,omitempty"`

	// IpAddresses is the array of IP addresses in the IP address set.
	// +optional
	IpAddresses []string `json:"ipAddresses,omitempty"`

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
