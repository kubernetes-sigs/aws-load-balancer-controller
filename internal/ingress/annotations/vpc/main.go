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

package log

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	ing_errors "github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/errors"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/resolver"
)

type TargetType string

type targetType struct {
	r resolver.Resolver
}

const (
	DefaultTargetType = "instance"
)

// NewParser creates a new target type annotation parser
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return targetType{r}
}

// ParseAnnotations parses the annotations contained in the ingress
// rule used to configure upstream check parameters
func (tt targetType) Parse(ing parser.AnnotationInterface) (interface{}, error) {
	targetType, err := parser.GetStringAnnotation("target-type", ing)
	if err != nil {
		targetType = aws.String(DefaultTargetType)
	}

	if *targetType != "instance" && *targetType != "pod" {
		return "", ing_errors.NewInvalidAnnotationContent("target-type", targetType)
	}

	t := TargetType(*targetType)
	return &t, nil
}

func (a *TargetType) Merge(b *TargetType) {
	a_, b_ := string(*a), string(*b)
	r := parser.MergeString(&a_, &b_, DefaultTargetType)
	*a = TargetType(*r)
}
