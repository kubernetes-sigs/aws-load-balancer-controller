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

package targetgroup

import (
	"fmt"
	"strings"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/errors"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/resolver"
)

type Config struct {
	Attributes              []*elbv2.TargetGroupAttribute
	BackendProtocol         *string
	HealthyThresholdCount   *int64
	SuccessCodes            *string
	TargetType              *string
	UnhealthyThresholdCount *int64
}

type targetGroup struct {
	r resolver.Resolver
}

const (
	DefaultBackendProtocol         = elbv2.ProtocolEnumHttp
	DefaultHealthyThresholdCount   = 2
	DefaultUnhealthyThresholdCount = 2
	DefaultSuccessCodes            = "200"
)

// NewParser creates a new target group annotation parser
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return targetGroup{r}
}

// Parse parses the annotations contained in the resource
func (tg targetGroup) Parse(ing parser.AnnotationInterface) (interface{}, error) {
	cfg := tg.r.GetConfig()

	targetType, err := parser.GetStringAnnotation("target-type", ing)
	if err != nil {
		targetType = aws.String(cfg.DefaultTargetType)
	}

	if *targetType != elbv2.TargetTypeEnumInstance && *targetType != elbv2.TargetTypeEnumIp {
		return "", errors.NewInvalidAnnotationContent("target-type", *targetType)
	}

	backendProtocol, err := parser.GetStringAnnotation("backend-protocol", ing)
	if err != nil {
		backendProtocol = aws.String(DefaultBackendProtocol)
	}

	healthyThresholdCount, err := parser.GetInt64Annotation("healthy-threshold-count", ing)
	if err != nil {
		healthyThresholdCount = aws.Int64(DefaultHealthyThresholdCount)
	}

	unhealthyThresholdCount, err := parser.GetInt64Annotation("unhealthy-threshold-count", ing)
	if err != nil {
		unhealthyThresholdCount = aws.Int64(DefaultUnhealthyThresholdCount)
	}

	// support legacy successCodes annotation
	successCodes, err := parser.GetStringAnnotation("successCodes", ing)
	if err != nil {
		successCodes = aws.String(DefaultSuccessCodes)
	}

	s, err := parser.GetStringAnnotation("success-codes", ing)
	if err == nil {
		successCodes = s
	}

	attributes, err := parseAttributes(ing)
	if err != nil {
		return nil, err
	}

	return &Config{
		TargetType:              targetType,
		BackendProtocol:         backendProtocol,
		HealthyThresholdCount:   healthyThresholdCount,
		UnhealthyThresholdCount: unhealthyThresholdCount,
		SuccessCodes:            successCodes,
		Attributes:              attributes,
	}, nil
}

// Merge merge two config according to defaults in cfg
func (a *Config) Merge(b *Config, cfg *config.Configuration) *Config {
	attributes := a.Attributes
	if attributes == nil {
		attributes = b.Attributes
	}

	return &Config{
		Attributes:              attributes,
		BackendProtocol:         parser.MergeString(a.BackendProtocol, b.BackendProtocol, DefaultBackendProtocol),
		TargetType:              parser.MergeString(a.TargetType, b.TargetType, cfg.DefaultTargetType),
		SuccessCodes:            parser.MergeString(a.SuccessCodes, b.SuccessCodes, DefaultSuccessCodes),
		HealthyThresholdCount:   parser.MergeInt64(a.HealthyThresholdCount, b.HealthyThresholdCount, DefaultHealthyThresholdCount),
		UnhealthyThresholdCount: parser.MergeInt64(a.UnhealthyThresholdCount, b.UnhealthyThresholdCount, DefaultUnhealthyThresholdCount),
	}
}

func parseAttributes(ing parser.AnnotationInterface) ([]*elbv2.TargetGroupAttribute, error) {
	var invalid []string
	var output []*elbv2.TargetGroupAttribute

	attributes := parser.GetCommaSeparatedStringAnnotation("target-group-attributes", ing)
	for _, attribute := range attributes {
		parts := strings.Split(attribute, "=")
		switch {
		case attribute == "":
			continue
		case len(parts) != 2:
			invalid = append(invalid, attribute)
			continue
		}
		output = append(output, &elbv2.TargetGroupAttribute{
			Key:   aws.String(strings.TrimSpace(parts[0])),
			Value: aws.String(strings.TrimSpace(parts[1])),
		})
	}

	if len(invalid) > 0 {
		return nil, fmt.Errorf("unable to parse `%s` into Key=Value pair(s)", strings.Join(invalid, ", "))
	}
	return output, nil
}

func Dummy() *Config {
	return &Config{
		BackendProtocol:         aws.String(elbv2.ProtocolEnumHttp),
		HealthyThresholdCount:   aws.Int64(2),
		SuccessCodes:            aws.String("200"),
		TargetType:              aws.String(elbv2.TargetTypeEnumInstance),
		UnhealthyThresholdCount: aws.Int64(2),
	}
}
