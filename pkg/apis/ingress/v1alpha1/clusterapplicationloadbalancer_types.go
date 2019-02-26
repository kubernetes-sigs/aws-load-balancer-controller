/*
Copyright 2019 The Kubernetes Authors.

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
)

type LoadBalancerIPAddressType string

const (
	LoadBalancerIPAddressTypeIPV4      = "ipv4"
	LoadBalancerIPAddressTypeDualstack = "dualstack"
)

type LoadBalancerSchema string

const (
	LoadBalancerSchemaInternal       = "internal"
	LoadBalancerSchemaInternetFacing = "internet-facing"
)

type LoadBalancerAccessLogsS3Attributes struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// +optional
	Bucket string `json:"bucket,omitempty"`

	// +optional
	Prefix string `json:"prefix,omitempty"`
}

type LoadBalancerAccessLogsAttributes struct {
	// +optional
	S3 LoadBalancerAccessLogsS3Attributes `json:"s3,omitempty"`
}

type LoadBalancerDeletionProtectionAttributes struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

type LoadBalancerIdleTimeoutAttributes struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4000
	// +optional
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty"`
}

type LoadBalancerRoutingHTTP2Attributes struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

type LoadBalancerRoutingAttributes struct {
	// +optional
	HTTP2 LoadBalancerRoutingHTTP2Attributes `json:"http2,omitempty"`
}

type ApplicationLoadBalancerAttributes struct {
	// +optional
	AccessLogs LoadBalancerAccessLogsAttributes `json:"accessLogs,omitempty"`

	// +optional
	DeletionProtection LoadBalancerDeletionProtectionAttributes `json:"deletionProtection,omitempty"`

	// +optional
	IdleTimeout LoadBalancerIdleTimeoutAttributes `json:"idleTimeout,omitempty"`

	// +optional
	Routing LoadBalancerRoutingAttributes `json:"routing,omitempty"`
}

// ApplicationLoadBalancerSpec defines the desired state of ApplicationLoadBalancer
type ApplicationLoadBalancerSpec struct {
	// +kubebuilder:validation:Enum=ipv4,dualstack
	// +optional
	IPAddressType LoadBalancerIPAddressType `json:"ipAddressType,omitempty"`

	// +kubebuilder:validation:Enum=internal,internet-facing
	// +optional
	Schema LoadBalancerSchema `json:"schema,omitempty"`

	// +optional
	Subnets []string `json:"subnets,omitempty"`

	// +optional
	SecurityGroups []string `json:"securityGroups,omitempty"`

	// +optional
	Attributes ApplicationLoadBalancerAttributes `json:"attributes,omitempty"`

	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// ApplicationLoadBalancerStatus defines the observed state of ApplicationLoadBalancer
type ApplicationLoadBalancerStatus struct {
	// +optional
	LoadBalancerARN string `json:"loadBalancerARN,omitempty"`
	// +optional
	DNSName string `json:"dnsName,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// ClusterApplicationLoadBalancer is the Schema for the clusterapplicationloadbalancers API
// +k8s:openapi-gen=true
type ClusterApplicationLoadBalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationLoadBalancerSpec   `json:"spec,omitempty"`
	Status ApplicationLoadBalancerStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// ClusterApplicationLoadBalancerList contains a list of ClusterApplicationLoadBalancer
type ClusterApplicationLoadBalancerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterApplicationLoadBalancer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterApplicationLoadBalancer{}, &ClusterApplicationLoadBalancerList{})
}
