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

// GatewayClassParamsSpec defines the desired state of GatewayClassParams
type GatewayClassParamsSpec struct {
	// NamespaceSelector restrict the namespaces of Ingresses that are allowed to specify the IngressClass with this IngressClassParams.
	// * if absent or present but empty, it selects all namespaces.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// Scheme defines the scheme for all Ingresses that belong to IngressClass with this IngressClassParams.
	// +optional
	Scheme *LoadBalancerScheme `json:"scheme,omitempty"`

	// InboundCIDRs specifies the CIDRs that are allowed to access the Gateway that belong to GatewayClass with this GatewayClassParams.
	// +optional
	InboundCIDRs []string `json:"inboundCIDRs,omitempty"`

	// SSLPolicy specifies the SSL Policy for all Ingresses that belong to GatewayClass with this GatewayClassParams.
	// +optional
	SSLPolicy string `json:"sslPolicy,omitEmpty"`

	// Subnets defines the subnets for all Gateways that belong to GatewayClass with this GatewayClassParams.
	// +optional
	Subnets *SubnetSelector `json:"subnets,omitempty"`

	// IPAddressType defines the ip address type for all Gateways that belong to GatewayClass with this GatewayClassParams.
	// +optional
	IPAddressType *IPAddressType `json:"ipAddressType,omitempty"`

	// Tags defines list of Tags on AWS resources provisioned for Ingresses that belong to GatewayClass with this GatewayClassParams.
	Tags []Tag `json:"tags,omitempty"`

	// LoadBalancerAttributes define the custom attributes to LoadBalancers for all Ingress that belong to GatewayClass with this GatewayClassParams.
	// +optional
	LoadBalancerAttributes []Attribute `json:"loadBalancerAttributes,omitempty"`
}

// GatewayClassParams
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="SCHEME",type="string",JSONPath=".spec.scheme",description="The AWS Load Balancer scheme"
// +kubebuilder:printcolumn:name="IP-ADDRESS-TYPE",type="string",JSONPath=".spec.ipAddressType",description="The AWS Load Balancer ipAddressType"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// GatewayClassParams is the Schema for the GatewayClassParams API
type GatewayClassParams struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GatewayClassParamsSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// GatewayClassParamsList contains a list of GatewayClassParams
type GatewayClassParamsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayClassParams `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GatewayClassParams{}, &GatewayClassParamsList{})
}
