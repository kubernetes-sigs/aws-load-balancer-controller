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

type ApplicationListenerProtocol string

const (
	ApplicationListenerProtocolHTTP  = "HTTP"
	ApplicationListenerProtocolHTTPS = "HTTPS"
)

type ApplicationListenerActionType string

type OnUnauthenticatedRequestAction string

const (
	OnUnauthenticatedRequestActionDeny         = "deny"
	OnUnauthenticatedRequestActionAllow        = "allow"
	OnUnauthenticatedRequestActionAuthenticate = "authenticate"
)

const (
	ApplicationListenerActionTypeAuthenticateCognito = "authenticate-cognito"
	ApplicationListenerActionTypeAuthenticateOIDC    = "authenticate-oidc"
	ApplicationListenerActionTypeForward             = "forward"
	ApplicationListenerActionTypeRedirect            = "redirect"
	ApplicationListenerActionTypeFixedResponse       = "fixed-response"
)

type ListenerCertificate struct {
	CertificateARN string `json:"certificateARN"`
}

type AuthenticateConfig struct {
	// +optional
	AuthenticationRequestExtraParams map[string]string `json:"authenticationRequestExtraParams,omitempty"`

	// +kubebuilder:validation:Enum=deny,allow,authenticate
	// +optional
	OnUnauthenticatedRequest OnUnauthenticatedRequestAction `json:"onUnauthenticatedRequest,omitempty"`

	// +optional
	Scope string `json:"scope,omitempty"`

	// +optional
	SessionCookieName string `json:"sessionCookieName,omitempty"`

	// +optional
	SessionTimeout string `json:"sessionTimeout,omitempty"`
}

type AuthenticateCognitoConfig struct {
	AuthenticateConfig `json:",inline"`

	UserPoolARN string `json:"userPoolARN"`

	UserPoolClientID string `json:"userPoolClientID"`

	UserPoolDomain string `json:"userPoolDomain"`
}

type AuthenticateOIDCConfig struct {
	AuthenticateConfig `json:",inline"`

	Issuer string `json:"issuer"`

	AuthorizationEndpoint string `json:"authorizationEndpoint"`

	TokenEndpoint string `json:"tokenEndpoint"`

	UserInfoEndpoint string `json:"userInfoEndpoint"`

	// SecretRef references the secret that contains clientID & clientSecret for OIDC.
	SecretRef corev1.LocalObjectReference `json:"secretRef"`
}

type FixedResponseConfig struct {
	// +kubebuilder:validation:Enum=text/plain,text/css,text/html,application/javascript,application/json
	// +optional
	ContentType string `json:"contentType,omitempty"`

	// +kubebuilder:validation:MinLength=0
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	MessageBody string `json:"messageBody,omitempty"`

	// +kubebuilder:validation:Pattern=^(2|4|5)\d\d$
	// +optional
	StatusCode string `json:"statusCode"`
}

type RedirectConfig struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +optional
	Host string `json:"host,omitempty"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:Pattern=^/.*
	// +optional
	Path string `json:"path,omitempty"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int64 `json:"port,omitempty"`

	// +kubebuilder:validation:Pattern=^(HTTPS?|#\{protocol\})$
	// +optional
	Protocol string `json:"protocol,omitempty"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +optional
	Query string `json:"query,omitempty"`

	// +kubebuilder:validation:Enum=HTTP_301,HTTP_302
	// +optional
	StatusCode string `json:"statusCode,omitempty"`
}

type ForwardConfig struct {
	// +optional
	TargetGroupRef corev1.ObjectReference `json:"targetGroupRef,omitempty"`

	// +optional
	TargetGroupARN string `json:"targetGroupARN,omitempty"`
}

type ApplicationListenerAction struct {
	// +kubebuilder:validation:Enum=authenticate-cognito,authenticate-oidc,forward,redirect,fixed-response
	Type ApplicationListenerActionType `json:"type"`

	// +optional
	AuthenticateCognitoConfig *AuthenticateCognitoConfig `json:"authenticateCognitoConfig,omitempty"`

	// +optional
	AuthenticateOIDCConfig *AuthenticateOIDCConfig `json:"authenticateOidcConfig,omitempty"`

	// +optional
	FixedResponseConfig *FixedResponseConfig `json:"fixedResponseConfig,omitempty"`

	// +optional
	RedirectConfig *RedirectConfig `json:"redirectConfig,omitempty"`

	// +optional
	ForwardConfig *ForwardConfig `json:"forwardConfig,omitempty"`
}

// ApplicationListenerSpec defines the desired state of ApplicationListener
type ApplicationListenerSpec struct {
	LoadBalancerRef corev1.ObjectReference `json:"loadBalancerRef"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int64 `json:"port"`

	// +kubebuilder:validation:Enum=HTTP,HTTPS
	Protocol ApplicationListenerProtocol `json:"protocol"`

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
