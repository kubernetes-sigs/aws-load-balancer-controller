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
	"k8s.io/apimachinery/pkg/util/intstr"
)

// IPPermission defines one ingress rule.
type IPPermission struct {
	// +optional
	Description string             `json:"description,omitempty"`
	FromPort    int64              `json:"fromPort"`
	ToPort      int64              `json:"toPort"`
	IPProtocol  intstr.IntOrString `json:"ipProtocol"`
	// +optional
	CIDRIP string `json:"cidrIP,omitempty"`
	// +optional
	CIDRIPV6 string `json:"cidrIPV6,omitempty"`
}

// SecurityGroupSpec defines the desired state of SecurityGroup
type SecurityGroupSpec struct {
	SecurityGroupName string `json:"securityGroupName"`

	// +optional
	Description string `json:"description,omitempty"`

	// +optional
	Permissions []IPPermission `json:"permissions,omitempty"`

	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SecurityGroupStatus defines the observed state of SecurityGroup
type SecurityGroupStatus struct {
	// +optional
	ID string `json:"id,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// SecurityGroup is the Schema for the securitygroups API
// +k8s:openapi-gen=true
type SecurityGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecurityGroupSpec   `json:"spec,omitempty"`
	Status SecurityGroupStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// SecurityGroupList contains a list of SecurityGroup
type SecurityGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecurityGroup `json:"items"`
}
