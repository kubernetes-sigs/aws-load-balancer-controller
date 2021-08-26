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

// +kubebuilder:validation:Enum=ipv4;dualstack
// IPAddressType is the ip address type of load balancer.
type IPAddressType string

const (
	IPAddressTypeIPV4      IPAddressType = "ipv4"
	IPAddressTypeDualStack IPAddressType = "dualstack"
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

// IngressClassParamsSpec defines the desired state of IngressClassParams
type IngressClassParamsSpec struct {
	// NamespaceSelector restrict the namespaces of Ingresses that are allowed to specify the IngressClass with this IngressClassParams.
	// * if absent or present but empty, it selects all namespaces.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// Group defines the IngressGroup for all Ingresses that belong to IngressClass with this IngressClassParams.
	// +optional
	Group *IngressGroup `json:"group,omitempty"`

	// Scheme defines the scheme for all Ingresses that belong to IngressClass with this IngressClassParams.
	// +optional
	Scheme *LoadBalancerScheme `json:"scheme,omitempty"`

	// IPAddressType defines the ip address type for all Ingresses that belong to IngressClass with this IngressClassParams.
	// +optional
	IPAddressType *IPAddressType `json:"ipAddressType,omitempty"`

	// Tags defines list of Tags on AWS resources provisioned for Ingresses that belong to IngressClass with this IngressClassParams.
	Tags []Tag `json:"tags,omitempty"`

	// LoadBalancerAttributes define the custom attributes to LoadBalancers for all Ingress that that belong to IngressClass with this IngressClassParams.
	// +optional
	LoadBalancerAttributes []Attribute `json:"loadBalancerAttributes,omitempty"`
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
