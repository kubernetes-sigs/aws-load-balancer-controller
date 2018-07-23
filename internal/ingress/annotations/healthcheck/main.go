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

package healthcheck

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/errors"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/resolver"
)

const (
	DefaultPath            = "/"
	DefaultPort            = "traffic-port"
	DefaultIntervalSeconds = 15
	DefaultTimeoutSeconds  = 5
)

// Config returns the URL and method to use check the status of
// the upstream server/s
type Config struct {
	Path            *string
	Port            *string
	Protocol        *string
	IntervalSeconds *int64
	TimeoutSeconds  *int64
}

type healthCheck struct {
	r resolver.Resolver
}

// NewParser creates a new health check annotation parser
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return healthCheck{r}
}

// Parse the annotations contained in the resource
func (hc healthCheck) Parse(ing parser.AnnotationInterface) (interface{}, error) {
	seconds, err := parser.GetInt64Annotation("healthcheck-interval-seconds", ing)
	if err != nil {
		if err != errors.ErrMissingAnnotations {
			return nil, err
		}
		seconds = aws.Int64(DefaultIntervalSeconds)
	}

	path, err := parser.GetStringAnnotation("healthcheck-path", ing)
	if err == errors.ErrMissingAnnotations {
		path = aws.String(DefaultPath)
	}

	port, err := parser.GetStringAnnotation("healthcheck-port", ing)
	if err == errors.ErrMissingAnnotations {
		port = aws.String(DefaultPort)
	}

	protocol, _ := parser.GetStringAnnotation("healthcheck-protocol", ing)

	timeoutSeconds, err := parser.GetInt64Annotation("healthcheck-timeout-seconds", ing)
	if err != nil {
		if err != errors.ErrMissingAnnotations {
			return nil, err
		}
		timeoutSeconds = aws.Int64(DefaultTimeoutSeconds)
	}

	if *timeoutSeconds >= *seconds {
		return nil, fmt.Errorf("healthcheck timeout must be less than healthcheck interval. Timeout was: %d. Interval was %d",
			*timeoutSeconds, *seconds)
	}

	return &Config{
		IntervalSeconds: seconds,
		Path:            path,
		Port:            port,
		Protocol:        protocol,
		TimeoutSeconds:  timeoutSeconds,
	}, nil
}

func (a *Config) Merge(b Config) {
	a.Path = parser.MergeString(a.Path, b.Path, DefaultPath)
	a.Port = parser.MergeString(a.Port, b.Port, DefaultPort)
	a.Protocol = parser.MergeString(a.Protocol, b.Protocol, "")
	a.IntervalSeconds = parser.MergeInt64(a.IntervalSeconds, b.IntervalSeconds, DefaultIntervalSeconds)
	a.TimeoutSeconds = parser.MergeInt64(a.TimeoutSeconds, b.TimeoutSeconds, DefaultTimeoutSeconds)
}
