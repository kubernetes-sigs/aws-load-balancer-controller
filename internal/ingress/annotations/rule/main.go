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

package rule

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/resolver"
)

type Config struct {
	IgnoreHostHeader *bool
}

type rule struct {
	r resolver.Resolver
}

const (
	DefaultIgnoreHostHeader = false
)

// NewParser creates a new target group annotation parser
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return rule{r}
}

// Parse parses the annotations contained in the resource
func (r rule) Parse(ing parser.AnnotationInterface) (interface{}, error) {
	ignoreHostHeader, err := parser.GetBoolAnnotation("ignore-host-header", ing)
	if err != nil {
		ignoreHostHeader = aws.Bool(DefaultIgnoreHostHeader)
	}

	return &Config{
		IgnoreHostHeader: ignoreHostHeader,
	}, nil
}

func (a *Config) Merge(b *Config) {
	a.IgnoreHostHeader = parser.MergeBool(a.IgnoreHostHeader, b.IgnoreHostHeader, DefaultIgnoreHostHeader)
}
