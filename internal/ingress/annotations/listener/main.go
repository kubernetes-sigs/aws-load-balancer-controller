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

// TODO: Validate policy and cert
// Parse parses the annotations contained in the resource
func (l listener) Parse(ing parser.AnnotationInterface) (interface{}, error) {
	sslPolicy, err := parser.GetStringAnnotation("ssl-policy", ing)
	if err == nil {
		// in := &elbv2.DescribeSSLPoliciesInput{
		// 	Names: []*string{
		// 		a.SslPolicy,
		// 	},
		// }
		// if _, err := albelbv2.ELBV2svc.DescribeSSLPolicies(in); err != nil {
		// 	if aerr, ok := err.(awserr.Error); ok {
		// 		switch aerr.Code() {
		// 		case elbv2.ErrCodeSSLPolicyNotFoundException:
		// 			return fmt.Errorf("%s: %s", elbv2.ErrCodeSSLPolicyNotFoundException, aerr.Error())
		// 		default:
		// 			return fmt.Errorf("Error: %s", aerr.Error())
		// 		}
		// 	} else {
		// 		return fmt.Errorf("Error: %s", aerr.Error())
		// 	}
		// }
	} else {
		sslPolicy = aws.String(DefaultSslPolicy)
	}

	certificateArn, err := parser.GetStringAnnotation("certificate-arn", ing)
	if err == nil {
		// 	if e := albacm.ACMsvc.CertExists(a.CertificateArn); !e {
		// 		if albiam.IAMsvc.CertExists(a.CertificateArn) {
		// 			return nil
		// 		}
		// 		return fmt.Errorf("ACM certificate ARN does not exist. ARN: %s", *a.CertificateArn)
		// 	}
	}

	if certificateArn == nil {
		sslPolicy = nil
	}

	return &Config{
		SslPolicy:      sslPolicy,
		CertificateArn: certificateArn,
	}, nil
}

func (a *Config) Merge(b *Config) {
	a.CertificateArn = parser.MergeString(a.CertificateArn, b.CertificateArn, "")
	a.SslPolicy = parser.MergeString(a.SslPolicy, b.SslPolicy, "")
}
