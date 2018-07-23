/*
Copyright 2016 The Kubernetes Authors.

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

package parser

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func buildIngress() *extensions.Ingress {
	return &extensions.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "foo",
			Namespace: api.NamespaceDefault,
		},
		Spec: extensions.IngressSpec{},
	}
}

func TestGetBoolAnnotation(t *testing.T) {
	ing := buildIngress()

	_, err := GetBoolAnnotation("", nil)
	if err == nil {
		t.Errorf("expected error but retuned nil")
	}

	tests := []struct {
		name   string
		field  string
		value  string
		exp    *bool
		expErr bool
	}{
		{"valid - false", "bool", "false", aws.Bool(false), false},
		{"valid - true", "bool", "true", aws.Bool(true), false},
	}

	data := map[string]string{}
	ing.SetAnnotations(data)

	for _, test := range tests {
		data[GetAnnotationWithPrefix(test.field)] = test.value

		u, err := GetBoolAnnotation(test.field, ing)
		if test.expErr {
			if err == nil {
				t.Errorf("%v: expected error but retuned nil", test.name)
			}
			continue
		}
		if *u != *test.exp {
			t.Errorf("%v: expected \"%v\" but \"%v\" was returned", test.name, *test.exp, *u)
		}

		delete(data, test.field)
	}
}

func TestGetStringAnnotation(t *testing.T) {
	ing := buildIngress()

	_, err := GetStringAnnotation("", nil)
	if err == nil {
		t.Errorf("expected error but retuned nil")
	}

	tests := []struct {
		name   string
		field  string
		value  string
		exp    string
		expErr bool
	}{
		{"valid - A", "string", "A", "A", false},
		{"valid - B", "string", "B", "B", false},
	}

	data := map[string]string{}
	ing.SetAnnotations(data)

	for _, test := range tests {
		data[GetAnnotationWithPrefix(test.field)] = test.value

		s, err := GetStringAnnotation(test.field, ing)
		if test.expErr {
			if err == nil {
				t.Errorf("%v: expected error but retuned nil", test.name)
			}
			continue
		}
		if *s != test.exp {
			t.Errorf("%v: expected \"%v\" but \"%v\" was returned", test.name, test.exp, s)
		}

		delete(data, test.field)
	}
}

func TestGetIntAnnotation(t *testing.T) {
	ing := buildIngress()

	_, err := GetInt64Annotation("", nil)
	if err == nil {
		t.Errorf("expected error but retuned nil")
	}

	tests := []struct {
		name   string
		field  string
		value  string
		exp    int64
		expErr bool
	}{
		{"valid - A", "string", "1", 1, false},
		{"valid - B", "string", "2", 2, false},
	}

	data := map[string]string{}
	ing.SetAnnotations(data)

	for _, test := range tests {
		data[GetAnnotationWithPrefix(test.field)] = test.value

		s, err := GetInt64Annotation(test.field, ing)
		if test.expErr {
			if err == nil {
				t.Errorf("%v: expected error but retuned nil", test.name)
			}
			continue
		}
		if *s != test.exp {
			t.Errorf("%v: expected \"%v\" but \"%v\" was returned", test.name, test.exp, s)
		}

		delete(data, test.field)
	}
}

func TestMergeString(t *testing.T) {
	tests := []struct {
		name string
		a    *string
		b    *string
		d    string
		exp  string
	}{
		{"valid - all undefined", nil, nil, "", ""},
		{"valid - defined default", nil, nil, "ignored", ""},

		{"valid - all defined", aws.String("defaultVal"), aws.String("desiredVal"), "defaultVal", "desiredVal"},

		{"valid - undefined default", aws.String("desiredVal"), aws.String("b"), "", "desiredVal"},

		{"valid - defined b", nil, aws.String("desiredVal"), "", "desiredVal"},
		{"valid - defined b, defined default", nil, aws.String("desiredVal"), "ignored", "desiredVal"},

		{"valid - defined a", aws.String("desiredVal"), nil, "", "desiredVal"},
		{"valid - defined a, defined default", aws.String("desiredVal"), nil, "ignored", "desiredVal"},
	}

	for _, test := range tests {
		test.a = MergeString(test.a, test.b, test.d)
		if test.a == nil && test.exp != "" {
			t.Errorf("%v: expected \"%v\" but \"%v\" was returned", test.name, test.exp, test.a)
		}
		if test.a != nil && *test.a != test.exp {
			t.Errorf("%v: expected \"%v\" but \"%v\" was returned", test.name, test.exp, *test.a)
		}
	}
}
