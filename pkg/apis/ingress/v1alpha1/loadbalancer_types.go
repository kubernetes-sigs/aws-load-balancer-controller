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
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type IPAddressType string

const (
	IPAddressTypeIPV4      IPAddressType = "ipv4"
	IPAddressTypeDualstack               = "dualstack"
)

func (ipAddressType IPAddressType) String() string {
	return string(ipAddressType)
}

func ParseIPAddressType(ipAddressType string) (IPAddressType, error) {
	switch ipAddressType {
	case string(IPAddressTypeIPV4):
		return IPAddressTypeIPV4, nil
	case string(IPAddressTypeDualstack):
		return IPAddressTypeDualstack, nil
	}
	return IPAddressType(""), errors.Errorf("unknown IPAddressType: %v", ipAddressType)
}

type LoadBalancerSchema string

const (
	LoadBalancerSchemaInternal       LoadBalancerSchema = "internal"
	LoadBalancerSchemaInternetFacing                    = "internet-facing"
)

func (schema LoadBalancerSchema) String() string {
	return string(schema)
}

func ParseLoadBalancerSchema(schema string) (LoadBalancerSchema, error) {
	switch schema {
	case string(LoadBalancerSchemaInternal):
		return LoadBalancerSchemaInternal, nil
	case string(LoadBalancerSchemaInternetFacing):
		return LoadBalancerSchemaInternetFacing, nil
	}
	return LoadBalancerSchema(""), errors.Errorf("unknown Schema: %v", schema)
}

type LoadBalancerAccessLogsS3Attributes struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// +optional
	Bucket string `json:"bucket,omitempty"`

	// +optional
	Prefix string `json:"prefix,omitempty"`
}

type LoadBalancerAccessLogsAttributes struct {
	S3 LoadBalancerAccessLogsS3Attributes `json:"s3"`
}

type LoadBalancerDeletionProtectionAttributes struct {
	Enabled bool `json:"enabled"`
}

type LoadBalancerIdleTimeoutAttributes struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4000
	TimeoutSeconds int64 `json:"timeoutSeconds"`
}

type LoadBalancerRoutingHTTP2Attributes struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

type LoadBalancerRoutingAttributes struct {
	// +optional
	HTTP2 LoadBalancerRoutingHTTP2Attributes `json:"http2,omitempty"`
}

type LoadBalancerAttributes struct {
	// +optional
	DeletionProtection LoadBalancerDeletionProtectionAttributes `json:"deletionProtection,omitempty"`

	// +optional
	AccessLogs LoadBalancerAccessLogsAttributes `json:"accessLogs,omitempty"`

	// +optional
	IdleTimeout LoadBalancerIdleTimeoutAttributes `json:"idleTimeout,omitempty"`

	// +optional
	Routing LoadBalancerRoutingAttributes `json:"routing,omitempty"`
}

type SubnetMapping struct {
	SubnetID string `json:"subnetID"`
}

type SecurityGroupReference struct {
	SecurityGroupRef corev1.LocalObjectReference `json:"securityGroupRef,omitempty"`
	SecurityGroupID  string                      `json:"securityGroupID,omitempty"`
}

// LoadBalancerSpec defines the desired state of LoadBalancer
type LoadBalancerSpec struct {
	LoadBalancerName string `json:"loadBalancerName"`

	LoadBalancerType string `json:"loadBalancerType"`

	// +kubebuilder:validation:Enum=ipv4,dualstack
	// +optional
	IPAddressType IPAddressType `json:"ipAddressType,omitempty"`

	// +kubebuilder:validation:Enum=internal,internet-facing
	// +optional
	Schema LoadBalancerSchema `json:"schema,omitempty"`

	// +kubebuilder:validation:MinItems=2
	SubnetMappings []SubnetMapping `json:"subnetMappings"`

	// +kubebuilder:validation:MinItems=1
	SecurityGroups []SecurityGroupReference `json:"securityGroups"`

	// +optional
	Attributes LoadBalancerAttributes `json:"attributes,omitempty"`

	Listeners []Listener `json:"listeners,omitempty"`

	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// LoadBalancerStatus defines the observed state of LoadBalancer
type LoadBalancerStatus struct {
	// +optional
	ARN     string `json:"arn,omitempty"`
	DNSName string `json:"dnsName,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// LoadBalancer is the Schema for the loadbalancers API
// +k8s:openapi-gen=true
type LoadBalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoadBalancerSpec   `json:"spec,omitempty"`
	Status LoadBalancerStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// LoadBalancerList contains a list of LoadBalancer
type LoadBalancerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoadBalancer `json:"items"`
}
