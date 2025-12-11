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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ALBTargetControlConfigSpec defines the desired state of ALBTargetControlConfig
type ALBTargetControlConfigSpec struct {
	// Image specifies the container image for the ALB target control agent sidecar.
	// The agent is available as a Docker image at: public.ecr.aws/aws-elb/target-optimizer/target-control-agent:latest
	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// DataAddress specifies the socket (IP:port) where the agent receives application traffic from the load balancer.
	// The port in this socket is the application traffic port you configure for your target group.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^.+:[0-9]+$`
	DataAddress string `json:"dataAddress"`

	// ControlAddress specifies the socket (IP:port) where the load balancer exchanges management traffic with agents.
	// The port in the socket is the target control port you configure for the target group.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^.+:[0-9]+$`
	ControlAddress string `json:"controlAddress"`

	// DestinationAddress specifies the socket (IP:port) where the agent proxies application traffic.
	// Your application should be listening on this port.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^.+:[0-9]+$`
	DestinationAddress string `json:"destinationAddress"`

	// MaxConcurrency specifies the maximum number of concurrent requests that the target receives from the load balancer.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1000
	// +kubebuilder:default=1
	MaxConcurrency int32 `json:"maxConcurrency,omitempty"`

	// TLSCertPath specifies the location of the TLS certificate that the agent provides to the load balancer during TLS handshake.
	// By default, the agent generates a self-signed certificate in-memory.
	// +optional
	TLSCertPath *string `json:"tlsCertPath,omitempty"`

	// TLSKeyPath specifies the location of the private key corresponding to the TLS certificate that the agent provides to the load balancer during TLS handshake.
	// By default, the agent generates a private key in memory.
	// +optional
	TLSKeyPath *string `json:"tlsKeyPath,omitempty"`

	// TLSSecurityPolicy specifies the ELB security policy that you configure for the target group.
	// +optional
	TLSSecurityPolicy *string `json:"tlsSecurityPolicy,omitempty"`

	// ProtocolVersion specifies the protocol through which the load balancer communicates with the agent.
	// Possible values are HTTP1, HTTP2, GRPC.
	// +optional
	// +kubebuilder:validation:Enum=HTTP1;HTTP2;GRPC
	ProtocolVersion *string `json:"protocolVersion,omitempty"`

	// RustLog specifies the log level of the agent process. The agent software is written in Rust.
	// Possible values are debug, info, and error.
	// +optional
	// +kubebuilder:validation:Enum=debug;info;error
	RustLog *string `json:"rustLog,omitempty"`

	// Resources specifies the resource requirements for the ALB target control agent sidecar
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:object:generate=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="IMAGE",type="string",JSONPath=".spec.image",description="The ALB target control agent sidecar image"
// +kubebuilder:printcolumn:name="DESTINATION",type="string",JSONPath=".spec.destinationAddress",description="Application destination address"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// ALBTargetControlConfig is the Schema for the albtargetcontrolconfigs API
type ALBTargetControlConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ALBTargetControlConfigSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// ALBTargetControlConfigList contains a list of ALBTargetControlConfig
type ALBTargetControlConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ALBTargetControlConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ALBTargetControlConfig{}, &ALBTargetControlConfigList{})
}
