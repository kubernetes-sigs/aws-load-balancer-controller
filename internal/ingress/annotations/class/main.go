/*
Copyright 2015 The Kubernetes Authors.

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

package class

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
)

const (
	// annotationKubernetesIngressClass picks a specific "class" for the Ingress.
	annotationKubernetesIngressClass = "kubernetes.io/ingress.class"

	defaultIngressClass = "alb"
)

// If watchIngressClass is empty, then both ingress without class annotation or with class annotation specified as `alb` will be matched.
// If watchIngressClass is not empty, then only ingress with class annotation specified as watchIngressClass will be matched
func IsValidIngress(ingressClass string, ingress *extensions.Ingress) bool {
	actualIngressClass := ingress.GetAnnotations()[annotationKubernetesIngressClass]
	if ingressClass == "" {
		return actualIngressClass == "" || actualIngressClass == defaultIngressClass
	}
	return actualIngressClass == ingressClass
}

// TODO: change this to in-sync with https://github.com/kubernetes/kubernetes/blob/13705ac81e00f154434b5c66c1ad92ac84960d7f/pkg/controller/service/service_controller.go#L592(relies on node's ready condition instead of AWS API)
// IsValidNode returns true if the given Node has valid annotations
func IsValidNode(n *corev1.Node) bool {
	if s, ok := n.ObjectMeta.Labels["eks.amazonaws.com/compute-type"]; ok && s == "fargate" {
		return false
	}
	if _, ok := n.ObjectMeta.Labels["node-role.kubernetes.io/master"]; ok {
		return false
	}
	if s, ok := n.ObjectMeta.Labels["alpha.service-controller.kubernetes.io/exclude-balancer"]; ok {
		if strings.ToUpper(s) == "TRUE" {
			return false
		}
	}
	return true
}
