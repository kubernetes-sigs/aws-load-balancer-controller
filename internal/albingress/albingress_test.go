package albingress

import (
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/healthcheck"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/listener"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/rule"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albcache"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/targetgroup"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"k8s.io/api/extensions/v1beta1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var a *ALBIngress

func init() {
	albcache.NewCache(metric.DummyCollector{})
	os.Setenv("AWS_VPC_ID", "vpc-id")
}

func TestNewALBIngressFromIngress(t *testing.T) {
	options := &NewALBIngressFromIngressOptions{
		Ingress: &extensions.Ingress{
			Spec: extensions.IngressSpec{
				Rules: []v1beta1.IngressRule{
					v1beta1.IngressRule{
						Host: "example.com",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{
									v1beta1.HTTPIngressPath{
										Path: "/",
										Backend: v1beta1.IngressBackend{
											ServicePort: intstr.FromInt(80),
											ServiceName: "testService",
										},
									},
								},
							},
						},
					},
				},
			},
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"alb.ingress.kubernetes.io/subnets":         "subnet-1,subnet-2",
					"alb.ingress.kubernetes.io/security-groups": "sg-1",
					"alb.ingress.kubernetes.io/scheme":          "internet-facing",
				},
				ClusterName: "testCluster",
				Namespace:   "test",
				Name:        "testIngress",
			},
		},
		ClusterName:   "testCluster",
		ALBNamePrefix: "albNamePrefix",
		Store: &store.Dummy{
			GetServiceAnnotationsResponse: &annotations.Service{
				TargetGroup: &targetgroup.Config{},
				Rule:        &rule.Config{},
				Listener:    &listener.Config{},
				HealthCheck: &healthcheck.Config{},
			},
			GetIngressAnnotationsResponse: &annotations.Ingress{
				LoadBalancer: &loadbalancer.Config{},
				TargetGroup: &targetgroup.Config{
					TargetType:      aws.String("instance"),
					BackendProtocol: aws.String("HTTP"),
				},
				Tags:        &tags.Config{},
				Rule:        &rule.Config{},
				Listener:    &listener.Config{},
				HealthCheck: &healthcheck.Config{},
			}},
	}
	ingress := NewALBIngressFromIngress(options)
	if ingress == nil {
		t.Errorf("NewALBIngressFromIngress returned nil")
	}
}
