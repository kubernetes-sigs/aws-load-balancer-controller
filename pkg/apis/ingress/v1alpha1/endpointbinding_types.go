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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EndpointBindingSpec defines the desired state of EndpointBinding
type EndpointBindingSpec struct {
	TargetGroup TargetGroupReference `json:"targetGroup"`
	TargetType  TargetType           `json:"targetType"`

	ServiceRef  corev1.ObjectReference `json:"serviceRef"`
	ServicePort intstr.IntOrString     `json:"servicePort"`
}

// EndpointBindingStatus defines the observed state of EndpointBinding
type EndpointBindingStatus struct {
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// EndpointBinding is the Schema for the endpointbindings API
// +k8s:openapi-gen=true
type EndpointBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointBindingSpec   `json:"spec,omitempty"`
	Status EndpointBindingStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// EndpointBindingList contains a list of EndpointBinding
type EndpointBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EndpointBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EndpointBinding{}, &EndpointBindingList{})
}
