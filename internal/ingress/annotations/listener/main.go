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

package listener

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/resolver"
)

type Config struct {
	SslPolicy      *string
	CertificateArn *string
}

type listener struct {
	r resolver.Resolver
}

const (
	DefaultSslPolicy = "ELBSecurityPolicy-2016-08"
)

// NewParser creates a new target group annotation parser
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return listener{r}
}

// Parse parses the annotations contained in the resource
func (l listener) Parse(ing parser.AnnotationInterface) (interface{}, error) {
	sslPolicy, err := parser.GetStringAnnotation("ssl-policy", ing)
	if err != nil {
		sslPolicy = aws.String(DefaultSslPolicy)
	}

	certificateArn, _ := parser.GetStringAnnotation("certificate-arn", ing)

	if certificateArn == nil {
		sslPolicy = nil
	}

	return &Config{
		SslPolicy:      sslPolicy,
		CertificateArn: certificateArn,
	}, nil
}

// Merge merges two config
func (a *Config) Merge(b *Config) *Config {
	return &Config{
		SslPolicy:      parser.MergeString(a.SslPolicy, b.SslPolicy, ""),
		CertificateArn: parser.MergeString(a.CertificateArn, b.CertificateArn, ""),
	}
}
