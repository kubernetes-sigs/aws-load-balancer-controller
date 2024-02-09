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

// +kubebuilder:validation:Enum=ipv4;ipv6
// TargetGroupIPAddressType is the IP Address type of your ELBV2 TargetGroup.
type TargetGroupIPAddressType string

const (
	TargetGroupIPAddressTypeIPv4 TargetGroupIPAddressType = "ipv4"
	TargetGroupIPAddressTypeIPv6 TargetGroupIPAddressType = "ipv6"
)

// ServiceReference defines reference to a Kubernetes Service and its ServicePort.
type ServiceReference struct {
	// Name is the name of the Service.
	Name string `json:"name"`

	// Port is the port of the ServicePort.
	Port intstr.IntOrString `json:"port"`
}

// IPBlock defines source/destination IPBlock in networking rules.
type IPBlock struct {
	// CIDR is the network CIDR.
	// Both IPV4 or IPV6 CIDR are accepted.
	CIDR string `json:"cidr"`
}

// SecurityGroup defines reference to an AWS EC2 SecurityGroup.
type SecurityGroup struct {
	// GroupID is the EC2 SecurityGroupID.
	GroupID string `json:"groupID"`
}

// NetworkingPeer defines the source/destination peer for networking rules.
type NetworkingPeer struct {
	// IPBlock defines an IPBlock peer.
	// If specified, none of the other fields can be set.
	// +optional
	IPBlock *IPBlock `json:"ipBlock,omitempty"`

	// SecurityGroup defines a SecurityGroup peer.
	// If specified, none of the other fields can be set.
	// +optional
	SecurityGroup *SecurityGroup `json:"securityGroup,omitempty"`
}

// +kubebuilder:validation:Enum=TCP;UDP
// NetworkingProtocol defines the protocol for networking rules.
type NetworkingProtocol string

const (
	// NetworkingProtocolTCP is the TCP protocol.
	NetworkingProtocolTCP NetworkingProtocol = "TCP"

	// NetworkingProtocolUDP is the UDP protocol.
	NetworkingProtocolUDP NetworkingProtocol = "UDP"
)

// NetworkingPort defines the port and protocol for networking rules.
type NetworkingPort struct {
	// The protocol which traffic must match.
	// If protocol is unspecified, it defaults to TCP.
	Protocol *NetworkingProtocol `json:"protocol,omitempty"`

	// The port which traffic must match.
	// When NodePort endpoints(instance TargetType) is used, this must be a numerical port.
	// When Port endpoints(ip TargetType) is used, this can be either numerical or named port on pods.
	// if port is unspecified, it defaults to all ports.
	// +optional
	Port *intstr.IntOrString `json:"port,omitempty"`
}

// NetworkingIngressRule defines a particular set of traffic that is allowed to access TargetGroup's targets.
type NetworkingIngressRule struct {
	// List of peers which should be able to access the targets in TargetGroup.
	// At least one NetworkingPeer should be specified.
	From []NetworkingPeer `json:"from"`

	// List of ports which should be made accessible on the targets in TargetGroup.
	// If ports is empty or unspecified, it defaults to all ports with TCP.
	Ports []NetworkingPort `json:"ports"`
}

// TargetGroupBindingNetworking defines the networking rules to allow ELBV2 LoadBalancer to access targets in TargetGroup.
type TargetGroupBindingNetworking struct {
	// List of ingress rules to allow ELBV2 LoadBalancer to access targets in TargetGroup.
	// +optional
	Ingress []NetworkingIngressRule `json:"ingress,omitempty"`
}

// TargetGroupBindingSpec defines the desired state of TargetGroupBinding
type TargetGroupBindingSpec struct {
	// targetGroupARN is the Amazon Resource Name (ARN) for the TargetGroup.
	// +kubebuilder:validation:MinLength=1
	TargetGroupARN string `json:"targetGroupARN"`

	// targetType is the TargetType of TargetGroup. If unspecified, it will be automatically inferred.
	// +optional
	TargetType *TargetType `json:"targetType,omitempty"`

	// serviceRef is a reference to a Kubernetes Service and ServicePort.
	ServiceRef ServiceReference `json:"serviceRef"`

	// networking defines the networking rules to allow ELBV2 LoadBalancer to access targets in TargetGroup.
	// +optional
	Networking *TargetGroupBindingNetworking `json:"networking,omitempty"`

	// node selector for instance type target groups to only register certain nodes
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`

	// ipAddressType specifies whether the target group is of type IPv4 or IPv6. If unspecified, it will be automatically inferred.
	// +optional
	IPAddressType *TargetGroupIPAddressType `json:"ipAddressType,omitempty"`
}

// TargetGroupBindingStatus defines the observed state of TargetGroupBinding
type TargetGroupBindingStatus struct {
	// The generation observed by the TargetGroupBinding controller.
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="SERVICE-NAME",type="string",JSONPath=".spec.serviceRef.name",description="The Kubernetes Service's name"
// +kubebuilder:printcolumn:name="SERVICE-PORT",type="string",JSONPath=".spec.serviceRef.port",description="The Kubernetes Service's port"
// +kubebuilder:printcolumn:name="TARGET-TYPE",type="string",JSONPath=".spec.targetType",description="The AWS TargetGroup's TargetType"
// +kubebuilder:printcolumn:name="ARN",type="string",JSONPath=".spec.targetGroupARN",description="The AWS TargetGroup's Amazon Resource Name",priority=1
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// TargetGroupBinding is the Schema for the TargetGroupBinding API
type TargetGroupBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TargetGroupBindingSpec   `json:"spec,omitempty"`
	Status TargetGroupBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TargetGroupBindingList contains a list of TargetGroupBinding
type TargetGroupBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TargetGroupBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TargetGroupBinding{}, &TargetGroupBindingList{})
}
