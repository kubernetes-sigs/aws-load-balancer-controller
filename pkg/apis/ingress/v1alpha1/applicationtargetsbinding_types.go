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

// ApplicationTargetsBindingSpec defines the desired state of ApplicationTargetsBinding
type ApplicationTargetsBindingSpec struct {
	TargetGroupReference `json:",inline"`

	TargetType  TargetType         `json:"targetType"`
	NetworkType NetworkType        `json:"networkType"`
	ServiceName string             `json:"serviceName"`
	ServicePort intstr.IntOrString `json:"servicePort"`
}

// ApplicationTargetsBindingStatus defines the observed state of ApplicationTargetsBinding
type ApplicationTargetsBindingStatus struct {
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ApplicationTargetsBinding is the Schema for the applicationtargetsbindings API
// +k8s:openapi-gen=true
type ApplicationTargetsBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationTargetsBindingSpec   `json:"spec,omitempty"`
	Status ApplicationTargetsBindingStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ApplicationTargetsBindingList contains a list of ApplicationTargetsBinding
type ApplicationTargetsBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApplicationTargetsBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ApplicationTargetsBinding{}, &ApplicationTargetsBindingList{})
}
