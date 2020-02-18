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

package annotations

import (
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/targetgroup"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	"github.com/stretchr/testify/assert"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/healthcheck"
	apiv1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/resolver"
)

var (
	annotationHealthcheckIntervalSeconds = parser.GetAnnotationWithPrefix("healthcheck-interval-seconds")
	annotationScheme                     = parser.GetAnnotationWithPrefix("scheme")
	annotationSubnets                    = parser.GetAnnotationWithPrefix("subnets")
)

func init() {
	os.Setenv("AWS_VPC_ID", "vpc-id")
}

type mockCfg struct {
	resolver.Mock
	MockSecrets  map[string]*apiv1.Secret
	MockServices map[string]*apiv1.Service
}

func (m mockCfg) GetSecret(name string) (*apiv1.Secret, error) {
	return m.MockSecrets[name], nil
}

func (m mockCfg) GetService(name string) (*apiv1.Service, error) {
	return m.MockServices[name], nil
}

func buildIngress() *networking.Ingress {
	defaultBackend := networking.IngressBackend{
		ServiceName: "default-backend",
		ServicePort: intstr.FromInt(80),
	}

	return &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: apiv1.NamespaceDefault,
		},
		Spec: networking.IngressSpec{
			Backend: &networking.IngressBackend{
				ServiceName: "default-backend",
				ServicePort: intstr.FromInt(80),
			},
			Rules: []networking.IngressRule{
				{
					Host: "foo.bar.com",
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{
									Path:    "/foo",
									Backend: defaultBackend,
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestHealthCheck(t *testing.T) {
	cfg := mockCfg{}
	ec := Extractor{
		map[string]parser.IngressAnnotation{
			"HealthCheck": healthcheck.NewParser(cfg),
		},
	}
	ing := buildIngress()

	fooAnns := []struct {
		annotations map[string]string
		euint       int64
		euport      string
	}{
		{map[string]string{annotationHealthcheckIntervalSeconds: "15", annotationScheme: elbv2.LoadBalancerSchemeEnumInternal, annotationSubnets: "subnet-asdas"}, 15, "traffic-port"},
	}

	for _, foo := range fooAnns {
		ing.SetAnnotations(foo.annotations)
		r := ec.ExtractIngress(ing)
		if r.Error != nil {
			t.Errorf(r.Error.Error())
		}

		hc := r.HealthCheck

		if *hc.IntervalSeconds != foo.euint {
			t.Errorf("Returned %v but expected %v for IntervalSeconds", *hc.IntervalSeconds, foo.euport)
		}

		if *hc.Port != foo.euport {
			t.Errorf("Returned %v but expected %v for Port", *hc.Port, foo.euport)
		}
	}
}

func TestMerge(t *testing.T) {
	for _, tc := range []struct {
		Source         *Service
		Target         *Ingress
		Config         *config.Configuration
		ExpectedResult *Service
	}{
		{
			Source: &Service{
				HealthCheck: &healthcheck.Config{
					Path:            aws.String("PathA"),
					Port:            aws.String("PortA"),
					Protocol:        aws.String("udp"),
					IntervalSeconds: aws.Int64(42),
					TimeoutSeconds:  aws.Int64(43),
				},
				TargetGroup: &targetgroup.Config{
					Attributes: []*elbv2.TargetGroupAttribute{
						{
							Key:   aws.String("keyA"),
							Value: aws.String("valueA"),
						},
					},
					BackendProtocol:         aws.String(elbv2.ProtocolEnumHttps),
					TargetType:              aws.String("ip"),
					SuccessCodes:            aws.String("404"),
					HealthyThresholdCount:   aws.Int64(8),
					UnhealthyThresholdCount: aws.Int64(9),
				},
			},
			Target: &Ingress{
				HealthCheck: &healthcheck.Config{
					Path:            aws.String("PathB"),
					Port:            aws.String("PortB"),
					Protocol:        aws.String("tcp"),
					IntervalSeconds: aws.Int64(52),
					TimeoutSeconds:  aws.Int64(53),
				},
				TargetGroup: &targetgroup.Config{
					Attributes: []*elbv2.TargetGroupAttribute{
						{
							Key:   aws.String("keyB"),
							Value: aws.String("valueB"),
						},
					},
					BackendProtocol:         aws.String(elbv2.ProtocolEnumHttp),
					TargetType:              aws.String("instance"),
					SuccessCodes:            aws.String("500"),
					HealthyThresholdCount:   aws.Int64(10),
					UnhealthyThresholdCount: aws.Int64(11),
				},
			},
			Config: &config.Configuration{
				DefaultTargetType: "instance",
			},
			ExpectedResult: &Service{
				HealthCheck: &healthcheck.Config{
					Path:            aws.String("PathA"),
					Port:            aws.String("PortA"),
					Protocol:        aws.String("udp"),
					IntervalSeconds: aws.Int64(42),
					TimeoutSeconds:  aws.Int64(43),
				},
				TargetGroup: &targetgroup.Config{
					Attributes: []*elbv2.TargetGroupAttribute{
						{
							Key:   aws.String("keyA"),
							Value: aws.String("valueA"),
						},
					},
					BackendProtocol:         aws.String(elbv2.ProtocolEnumHttps),
					TargetType:              aws.String("ip"),
					SuccessCodes:            aws.String("404"),
					HealthyThresholdCount:   aws.Int64(8),
					UnhealthyThresholdCount: aws.Int64(9),
				},
			},
		},
		{
			Source: &Service{
				HealthCheck: &healthcheck.Config{
					Path:            aws.String(healthcheck.DefaultPath),
					Port:            aws.String(healthcheck.DefaultPort),
					Protocol:        aws.String("tcp"),
					IntervalSeconds: aws.Int64(healthcheck.DefaultIntervalSeconds),
					TimeoutSeconds:  aws.Int64(healthcheck.DefaultTimeoutSeconds),
				},
				TargetGroup: &targetgroup.Config{
					Attributes:              nil,
					BackendProtocol:         aws.String(targetgroup.DefaultBackendProtocol),
					TargetType:              aws.String("instance"),
					SuccessCodes:            aws.String(targetgroup.DefaultSuccessCodes),
					HealthyThresholdCount:   aws.Int64(targetgroup.DefaultHealthyThresholdCount),
					UnhealthyThresholdCount: aws.Int64(targetgroup.DefaultUnhealthyThresholdCount),
				},
			},
			Target: &Ingress{
				HealthCheck: &healthcheck.Config{
					Path:            aws.String("PathB"),
					Port:            aws.String("PortB"),
					Protocol:        aws.String("tcp"),
					IntervalSeconds: aws.Int64(52),
					TimeoutSeconds:  aws.Int64(53),
				},
				TargetGroup: &targetgroup.Config{
					Attributes: []*elbv2.TargetGroupAttribute{
						{
							Key:   aws.String("keyB"),
							Value: aws.String("valueB"),
						},
					},
					BackendProtocol:         aws.String(elbv2.ProtocolEnumHttp),
					TargetType:              aws.String("ip"),
					SuccessCodes:            aws.String("500"),
					HealthyThresholdCount:   aws.Int64(10),
					UnhealthyThresholdCount: aws.Int64(11),
				},
			},
			Config: &config.Configuration{
				DefaultTargetType: "instance",
			},
			ExpectedResult: &Service{
				HealthCheck: &healthcheck.Config{
					Path:            aws.String("PathB"),
					Port:            aws.String("PortB"),
					Protocol:        aws.String("tcp"),
					IntervalSeconds: aws.Int64(52),
					TimeoutSeconds:  aws.Int64(53),
				},
				TargetGroup: &targetgroup.Config{
					Attributes: []*elbv2.TargetGroupAttribute{
						{
							Key:   aws.String("keyB"),
							Value: aws.String("valueB"),
						},
					},
					BackendProtocol:         aws.String(elbv2.ProtocolEnumHttp),
					TargetType:              aws.String("ip"),
					SuccessCodes:            aws.String("500"),
					HealthyThresholdCount:   aws.Int64(10),
					UnhealthyThresholdCount: aws.Int64(11),
				},
			},
		},
	} {
		actualResult := tc.Source.Merge(tc.Target, tc.Config)
		assert.Equal(t, tc.ExpectedResult, actualResult)
	}
}
