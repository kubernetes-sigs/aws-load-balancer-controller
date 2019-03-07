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
	corev1 "k8s.io/api/core/v1"
)

const (
	// annotationKubernetesServiceClass picks a specific "class" for the Service.
	annotationKubernetesServiceClass = "kubernetes.io/service.class"

	defaultServiceClass = "nlb"
)

// If watchServiceClass is empty, then both service without class annotation or with class annotation specified as `nlb` will be matched.
// If watchServiceClass is not empty, then only service with class annotation specified as watchServiceClass will be matched
func IsValidService(serviceClass string, service *corev1.Service) bool {
	actualServiceClass := service.GetAnnotations()[annotationKubernetesServiceClass]
	if serviceClass == "" {
		return actualServiceClass == "" || actualServiceClass == defaultServiceClass
	}
	return actualServiceClass == serviceClass
}
