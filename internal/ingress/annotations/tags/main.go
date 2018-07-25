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

package tags

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/resolver"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

type Config struct {
	LoadBalancer []*elbv2.Tag
}

type targetGroup struct {
	r resolver.Resolver
}

// NewParser creates a new target group annotation parser
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return targetGroup{r}
}

// Parse parses the annotations contained in the resource
func (tg targetGroup) Parse(ing parser.AnnotationInterface) (interface{}, error) {
	var lbtags []*elbv2.Tag

	v, err := parser.GetStringAnnotation("tags", ing)
	if err == nil {
		var badTags []string
		rawTags := util.NewAWSStringSlice(*v)

		for _, rawTag := range rawTags {
			parts := strings.Split(*rawTag, "=")
			switch {
			case *rawTag == "":
				continue
			case len(parts) < 2:
				badTags = append(badTags, *rawTag)
				continue
			}
			lbtags = append(lbtags, &elbv2.Tag{
				Key:   aws.String(parts[0]),
				Value: aws.String(parts[1]),
			})
		}

		if len(badTags) > 0 {
			return nil, fmt.Errorf("Unable to parse `%s` into Key=Value pair(s)", strings.Join(badTags, ", "))
		}
	}

	return &Config{
		LoadBalancer: lbtags,
	}, nil
}

func (a *Config) Merge(b *Config) {
	if a.LoadBalancer == nil {
		if b.LoadBalancer != nil {
			a.LoadBalancer = b.LoadBalancer
		}
	}
}
