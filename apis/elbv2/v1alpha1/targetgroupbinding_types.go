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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
type NetworkingProtocol string

const (
	// NetworkingProtocolTCP is the TCP protocol.
	NetworkingProtocolTCP = "TCP"

	// NetworkingProtocolUDP is the UDP protocol.
	NetworkingProtocolUDP = "UDP"
)

type NetworkingPort struct {
	// The port which traffic must match.
	// If unspecified, defaults to all port.
	// +optional
	Port *intstr.IntOrString `json:"port,omitempty"`

	// The protocol which traffic must match.
	// If unspecified, defaults to all protocol.
	// +optional
	Protocol *NetworkingProtocol `json:"protocol,omitempty"`
}

type NetworkingIngressRule struct {
	// List of peers which should be able to access the targets in TargetGroup.
	// If unspecified or empty, defaults to anywhere.
	// +optional
	From []NetworkingPeer `json:"from,omitempty"`

	// List of ports which should be made accessible on the targets in TargetGroup.
	// If unspecified or empty, defaults to all port.
	// +optional
	Ports []NetworkingPort `json:"ports,omitempty"`
}

type TargetGroupBindingNetworking struct {
	// List of ingress rules to allow ELBV2 LoadBalancer to access targets in TargetGroup.
	// +optional
	Ingress []NetworkingIngressRule `json:"ingress,omitempty"`
}

// TargetGroupBindingSpec defines the desired state of TargetGroupBinding
type TargetGroupBindingSpec struct {
	// targetGroupARN is the Amazon Resource Name (ARN) for the TargetGroup.
	TargetGroupARN string `json:"targetGroupARN"`

	// serviceRef is a reference to a Kubernetes Service and ServicePort.
	ServiceRef ServiceReference `json:"serviceRef"`

	// networking provides the networking setup for ELBV2 LoadBalancer to access targets in TargetGroup.
	// +optional
	Networking *TargetGroupBindingNetworking `json:"networking,omitempty"`
}

// TargetGroupBindingStatus defines the observed state of TargetGroupBinding
type TargetGroupBindingStatus struct {
	// The generation observed by the TargetGroupBinding controller.
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true

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
