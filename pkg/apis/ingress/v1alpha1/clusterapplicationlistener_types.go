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
)

type ListenerCertificate struct {
	CertificateARN string `json:"certificateARN"`
}

// ApplicationListenerSpec defines the desired state of ApplicationListener
type ApplicationListenerSpec struct {
	LoadBalancerRef corev1.ObjectReference `json:"loadBalancerRef"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int64 `json:"port"`

	// +kubebuilder:validation:Enum=HTTP,HTTPS
	Protocol Protocol `json:"protocol"`

	// +optional
	SSLPolicy string `json:"sslPolicy,omitempty"`

	// +optional
	Certificates []ListenerCertificate `json:"certificates,omitempty"`

	// +kubebuilder:validation:MinItems=1
	DefaultActions []ApplicationListenerAction `json:"defaultActions"`
}

// ApplicationListenerStatus defines the observed state of ApplicationListener
type ApplicationListenerStatus struct {
	// +optional
	ListenerARN string `json:"listenerARN,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// ClusterApplicationListener is the Schema for the clusterapplicationlisteners API
// +k8s:openapi-gen=true
type ClusterApplicationListener struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationListenerSpec   `json:"spec,omitempty"`
	Status ApplicationListenerStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// ClusterApplicationListenerList contains a list of ClusterApplicationListener
type ClusterApplicationListenerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterApplicationListener `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterApplicationListener{}, &ClusterApplicationListenerList{})
}
