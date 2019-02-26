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

type RuleConditionField string

const (
	RuleConditionFieldHostHeader  = "host-header"
	RuleConditionFieldPathPattern = "path-pattern"
)

type HostHeaderConditionConfig struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1
	Values []string `json:"values"`
}

type PathPatternConditionConfig struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1
	Values []string `json:"values"`
}

type ApplicationListenerRuleCondition struct {
	Field RuleConditionField `json:"field"`

	// +optional
	HostHeaderConfig *HostHeaderConditionConfig `json:"hostHeaderConfig,omitempty"`

	// +optional
	PathPatternConfig *PathPatternConditionConfig `json:"pathPatternConfig,omitempty"`
}

// ApplicationListenerRuleSpec defines the desired state of ApplicationListenerRule
type ApplicationListenerRuleSpec struct {
	ListenerRef corev1.LocalObjectReference

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=50000
	Priority int64 `json:"priority"`

	// +kubebuilder:validation:MinItems=1
	Conditions []ApplicationListenerRuleCondition `json:"conditions"`

	// +kubebuilder:validation:MinItems=1
	Actions []ApplicationListenerAction `json:"actions"`
}

// ApplicationListenerRuleStatus defines the observed state of ApplicationListenerRule
type ApplicationListenerRuleStatus struct {
	ListenerRuleARN string `json:"listenerRuleARN,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// ClusterApplicationListenerRule is the Schema for the clusterapplicationlistenerrules API
// +k8s:openapi-gen=true
type ClusterApplicationListenerRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationListenerRuleSpec   `json:"spec,omitempty"`
	Status ApplicationListenerRuleStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// ClusterApplicationListenerRuleList contains a list of ClusterApplicationListenerRule
type ClusterApplicationListenerRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterApplicationListenerRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterApplicationListenerRule{}, &ClusterApplicationListenerRuleList{})
}
