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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type HealthCheckMatcher struct {
	HTTPCode string `json:"intervalSeconds"`
}

type HealthCheckConfig struct {
	// +kubebuilder:validation:Minimum=5
	// +kubebuilder:validation:Maximum=300
	// +optional
	IntervalSeconds int64 `json:"intervalSeconds,omitempty"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	Path string `json:"path,omitempty"`

	// +kubebuilder:validation:Pattern=^(traffic-port|\d+)$
	// +optional
	Port intstr.IntOrString `json:"port,omitempty"`

	// +kubebuilder:validation:Enum=HTTP,HTTPS,TCP,TLS
	// +optional
	Protocol Protocol `json:"protocol,omitempty"`

	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=120
	// +optional
	TimeoutSeconds int64 `json:"timeoutSeconds,omitempty"`

	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=10
	// +optional
	HealthyThresholdCount int64 `json:"healthyThresholdCount,omitempty"`

	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=10
	// +optional
	UnhealthyThresholdCount int64 `json:"unhealthyThresholdCount,omitempty"`

	// +optional
	Matcher HealthCheckMatcher `json:"matcher,omitempty"`
}

type TargetGroupDeregistrationDelayAttributes struct {
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=3600
	TimeoutSeconds int64 `json:"timeoutSeconds"`
}

type TargetGroupSlowStartAttributes struct {
	// +kubebuilder:validation:Minimum=30
	// +kubebuilder:validation:Maximum=900
	DurationSeconds int64 `json:"durationSeconds"`
}

type TargetGroupStickinessType string

func (stickinessType TargetGroupStickinessType) String() string {
	return string(stickinessType)
}

const (
	TargetGroupStickinessTypeLBCookie TargetGroupStickinessType = "lb_cookie"
)

func ParseTargetGroupStickinessType(stickinessType string) (TargetGroupStickinessType, error) {
	switch stickinessType {
	case string(TargetGroupStickinessTypeLBCookie):
		return TargetGroupStickinessTypeLBCookie, nil
	}
	return TargetGroupStickinessType(""), errors.Errorf("unknown stickinessType: %v", stickinessType)
}

type LBCookieConfig struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=604800
	DurationSeconds int64 `json:"durationSeconds"`
}

type TargetGroupStickinessAttributes struct {
	Enabled bool `json:"enabled"`

	// +kubebuilder:validation:Enum=lb_cookie
	Type TargetGroupStickinessType `json:"type"`

	// +optional
	LBCookie LBCookieConfig `json:"lbCookie,omitempty"`
}

type TargetGroupAttributes struct {
	// +optional
	DeregistrationDelay TargetGroupDeregistrationDelayAttributes `json:"deregistrationDelay,omitempty"`

	// +optional
	SlowStart TargetGroupSlowStartAttributes `json:"slowStart,omitempty"`

	// +optional
	Stickiness TargetGroupStickinessAttributes `json:"stickiness,omitempty"`
}

type TargetType string

const (
	TargetTypeInstance TargetType = "instance"
	TargetTypeIP                  = "ip"
)

func (targetType TargetType) String() string {
	return string(targetType)
}

func ParseTargetType(targetType string) (TargetType, error) {
	switch targetType {
	case string(TargetTypeInstance):
		return TargetTypeInstance, nil
	case string(TargetTypeIP):
		return TargetTypeIP, nil
	}
	return TargetType(""), errors.Errorf("unknown targetType: %v", targetType)
}

// TargetGroupSpec defines the desired state of TargetGroup
type TargetGroupSpec struct {
	TargetGroupName string `json:"targetGroupName"`

	// +kubebuilder:validation:Enum=instance,ip
	TargetType TargetType `json:"targetType"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int64 `json:"port"`

	// +kubebuilder:validation:Enum=HTTP,HTTPS
	Protocol Protocol `json:"protocol"`

	// +optional
	HealthCheckConfig HealthCheckConfig `json:"healthCheckConfig,omitempty"`

	// +optional
	Attributes TargetGroupAttributes `json:"attributes,omitempty"`

	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// TargetGroupStatus defines the observed state of TargetGroup
type TargetGroupStatus struct {
	// +optional
	ARN string `json:"arn,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// TargetGroup is the Schema for the targetgroups API
// +k8s:openapi-gen=true
type TargetGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TargetGroupSpec   `json:"spec,omitempty"`
	Status TargetGroupStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// TargetGroupList contains a list of TargetGroup
type TargetGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TargetGroup `json:"items"`
}
