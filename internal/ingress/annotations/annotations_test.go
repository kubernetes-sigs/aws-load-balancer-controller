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

	apiv1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albcache"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/resolver"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

var (
	annotationHealthcheckIntervalSeconds = parser.GetAnnotationWithPrefix("healthcheck-interval-seconds")
	annotationScheme                     = parser.GetAnnotationWithPrefix("scheme")
	annotationSubnets                    = parser.GetAnnotationWithPrefix("subnets")
)

func init() {
	albcache.NewCache(metric.DummyCollector{})
	os.Setenv("AWS_VPC_ID", "vpc-id")
	albrgt.RGTsvc = &albrgt.Dummy{}
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

func buildIngress() *extensions.Ingress {
	defaultBackend := extensions.IngressBackend{
		ServiceName: "default-backend",
		ServicePort: intstr.FromInt(80),
	}

	return &extensions.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: apiv1.NamespaceDefault,
		},
		Spec: extensions.IngressSpec{
			Backend: &extensions.IngressBackend{
				ServiceName: "default-backend",
				ServicePort: intstr.FromInt(80),
			},
			Rules: []extensions.IngressRule{
				{
					Host: "foo.bar.com",
					IngressRuleValue: extensions.IngressRuleValue{
						HTTP: &extensions.HTTPIngressRuleValue{
							Paths: []extensions.HTTPIngressPath{
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
	ec := NewIngressAnnotationExtractor(mockCfg{})
	ing := buildIngress()

	fooAnns := []struct {
		annotations map[string]string
		euint       int64
		euport      string
	}{
		{map[string]string{annotationHealthcheckIntervalSeconds: "15", annotationScheme: "internal", annotationSubnets: "subnet-asdas"}, 15, "traffic-port"},
		// {map[string]string{}, 0, ""},
		// {nil, 0, ""},
	}

	albrgt.RGTsvc.SetResponse(&albrgt.Resources{
		TargetGroups: map[string]util.ELBv2Tags{"arn": util.ELBv2Tags{&elbv2.Tag{
			Key:   aws.String("kubernetes.io/service-name"),
			Value: aws.String("namespace/service-name"),
		}}}}, nil)

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
