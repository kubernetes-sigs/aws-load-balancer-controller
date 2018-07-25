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

package parser

import (
	"fmt"
	"strconv"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/errors"
)

var (
	// AnnotationsPrefix defines the common prefix used in the nginx ingress controller
	AnnotationsPrefix = "alb.ingress.kubernetes.io"
)

type AnnotationInterface interface {
	GetAnnotations() map[string]string
}

// IngressAnnotation has a method to parse annotations located in Ingress
type IngressAnnotation interface {
	Parse(ing AnnotationInterface) (interface{}, error)
}

// ServiceAnnotation has a method to parse annotations located in Service
type ServiceAnnotation interface {
	Parse(svc AnnotationInterface) (interface{}, error)
}

type ingAnnotations map[string]string

func (a ingAnnotations) parseBool(name string) (*bool, error) {
	val, ok := a[name]
	if ok {
		b, err := strconv.ParseBool(val)
		if err != nil {
			return nil, errors.NewInvalidAnnotationContent(name, val)
		}
		return &b, nil
	}
	return nil, errors.ErrMissingAnnotations
}

func (a ingAnnotations) parseString(name string) (*string, error) {
	val, ok := a[name]
	if ok {
		return &val, nil
	}
	return nil, errors.ErrMissingAnnotations
}

func (a ingAnnotations) parseInt64(name string) (*int64, error) {
	val, ok := a[name]
	if ok {
		i, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return nil, errors.NewInvalidAnnotationContent(name, val)
		}
		return &i, nil
	}
	return nil, errors.ErrMissingAnnotations
}

func checkAnnotation(name string, ing AnnotationInterface) error {
	if ing == nil || len(ing.GetAnnotations()) == 0 {
		return errors.ErrMissingAnnotations
	}
	if name == "" {
		return errors.ErrInvalidAnnotationName
	}

	return nil
}

// GetBoolAnnotation extracts a boolean from an Ingress annotation
func GetBoolAnnotation(name string, ing AnnotationInterface) (*bool, error) {
	v := GetAnnotationWithPrefix(name)
	err := checkAnnotation(v, ing)
	if err != nil {
		return nil, err
	}
	return ingAnnotations(ing.GetAnnotations()).parseBool(v)
}

// GetStringAnnotation extracts a string from an Ingress annotation
func GetStringAnnotation(name string, ing AnnotationInterface) (*string, error) {
	v := GetAnnotationWithPrefix(name)
	err := checkAnnotation(v, ing)
	if err != nil {
		return nil, err
	}
	return ingAnnotations(ing.GetAnnotations()).parseString(v)
}

// GetInt64Annotation extracts an int from an Ingress annotation
func GetInt64Annotation(name string, ing AnnotationInterface) (*int64, error) {
	v := GetAnnotationWithPrefix(name)
	err := checkAnnotation(v, ing)
	if err != nil {
		return nil, err
	}
	return ingAnnotations(ing.GetAnnotations()).parseInt64(v)
}

// GetAnnotationWithPrefix returns the prefix of ingress annotations
func GetAnnotationWithPrefix(suffix string) string {
	return fmt.Sprintf("%v/%v", AnnotationsPrefix, suffix)
}

// MergeString replaces a with b if it is undefined or the default value d
func MergeString(a, b *string, d string) *string {
	if b == nil {
		return a
	}

	if a == nil {
		return b
	}

	if *a == d {
		return b
	}

	return a
}

// MergeInt64 replaces a with b if it is undefined or the default value d
func MergeInt64(a, b *int64, d int64) *int64 {
	if b == nil {
		return a
	}

	if a == nil {
		return b
	}

	if *a == d {
		return b
	}

	return a
}

// MergeBool replaces a with b if it is undefined or the default value d
func MergeBool(a, b *bool, d bool) *bool {
	if b == nil {
		return a
	}

	if a == nil {
		return b
	}

	if *a == d {
		return b
	}

	return a
}

type String interface {
	Merge(*String)
}
