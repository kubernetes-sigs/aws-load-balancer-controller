/*
Copyright 2017 The Kubernetes Authors.

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
	"testing"

	"github.com/stretchr/testify/assert"

	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsValidIngress(t *testing.T) {
	for _, tc := range []struct {
		Name          string
		IngressClass  string
		Ingress       networking.Ingress
		ExpectedValid bool
	}{
		{
			Name:         "IngressClass not set, matches ingress without ingressClass",
			IngressClass: "",
			Ingress: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			ExpectedValid: true,
		},
		{
			Name:         "IngressClass not set, matches ingress empty ingressClass",
			IngressClass: "",
			Ingress: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotationKubernetesIngressClass: ""},
				},
			},
			ExpectedValid: true,
		},
		{
			Name:         "IngressClass not set, matches default ingressClass",
			IngressClass: "",
			Ingress: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotationKubernetesIngressClass: defaultIngressClass},
				},
			},
			ExpectedValid: true,
		},
		{
			Name:         "IngressClass not set, don't match ingressClass other than default one",
			IngressClass: "",
			Ingress: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotationKubernetesIngressClass: "nginx"},
				},
			},
			ExpectedValid: false,
		},

		{
			Name:         "IngressClass set to default, don't matches ingress without ingressClass",
			IngressClass: defaultIngressClass,
			Ingress: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			ExpectedValid: false,
		},
		{
			Name:         "IngressClass set to default, don't matches ingress empty ingressClass",
			IngressClass: defaultIngressClass,
			Ingress: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotationKubernetesIngressClass: ""},
				},
			},
			ExpectedValid: false,
		},
		{
			Name:         "IngressClass set to default, matches default ingressClass",
			IngressClass: defaultIngressClass,
			Ingress: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotationKubernetesIngressClass: defaultIngressClass},
				},
			},
			ExpectedValid: true,
		},
		{
			Name:         "IngressClass set to default, don't match ingressClass other than default one",
			IngressClass: defaultIngressClass,
			Ingress: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotationKubernetesIngressClass: "nginx"},
				},
			},
			ExpectedValid: false,
		},

		{
			Name:         "IngressClass set to custom, don't matches ingress without ingressClass",
			IngressClass: "custom",
			Ingress: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			ExpectedValid: false,
		},
		{
			Name:         "IngressClass set to custom, don't matches ingress empty ingressClass",
			IngressClass: "custom",
			Ingress: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotationKubernetesIngressClass: ""},
				},
			},
			ExpectedValid: false,
		},
		{
			Name:         "IngressClass set to custom, don't matches default ingressClass",
			IngressClass: "custom",
			Ingress: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotationKubernetesIngressClass: defaultIngressClass},
				},
			},
			ExpectedValid: false,
		},
		{
			Name:         "IngressClass set to custom ingressClass of custom",
			IngressClass: "custom",
			Ingress: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotationKubernetesIngressClass: "custom"},
				},
			},
			ExpectedValid: true,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			actualValid := IsValidIngress(tc.IngressClass, &tc.Ingress)
			assert.Equal(t, tc.ExpectedValid, actualValid)
		})
	}
}
