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
