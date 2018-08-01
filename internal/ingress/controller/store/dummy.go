package store

import (
	"github.com/aws/aws-sdk-go/aws"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albcache"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"

	api "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type Dummy struct {
	cfg                           *config.Configuration
	GetIngressAnnotationsResponse *annotations.Ingress
	GetServiceAnnotationsResponse *annotations.Service
}

// GetConfigMap ...
func (d Dummy) GetConfigMap(key string) (*corev1.ConfigMap, error) {
	return nil, nil
}

// GetService ...
func (d Dummy) GetService(key string) (*corev1.Service, error) {
	return nil, nil
}

// GetServiceEndpoints ...
func (d Dummy) GetServiceEndpoints(key string) (*corev1.Endpoints, error) {
	return nil, nil
}

// GetServiceAnnotations ...
func (d Dummy) GetServiceAnnotations(key string, ingress *annotations.Ingress) (*annotations.Service, error) {
	return d.GetServiceAnnotationsResponse, nil
}

// GetIngress ...
func (d Dummy) GetIngress(key string) (*extensions.Ingress, error) {
	return nil, nil
}

// ListNodes ...
func (d Dummy) ListNodes() []*corev1.Node {
	return nil
}

// ListIngresses ...
func (d Dummy) ListIngresses() []*extensions.Ingress {
	return nil
}

// GetIngressAnnotations ...
func (d Dummy) GetIngressAnnotations(key string) (*annotations.Ingress, error) {
	return d.GetIngressAnnotationsResponse, nil
}

// GetServicePort ...
func (d Dummy) GetServicePort(serviceKey, serviceType string, backendPort int32) (*int64, error) {
	return aws.Int64(8080), nil
}

// GetTargets ...
func (d Dummy) GetTargets(mode *string, namespace string, svc string, port *int64) (albelbv2.TargetDescriptions, error) {
	return nil, nil
}

// Run ...
func (d Dummy) Run(stopCh chan struct{}) {
	return
}

// GetConfig ...
func (d Dummy) GetConfig() *config.Configuration {
	if d.cfg != nil {
		return d.cfg
	}
	return config.NewDefault()
}

// SetConfig ...
func (d *Dummy) SetConfig(c *config.Configuration) {
	d.cfg = c
}

// GetInstanceIDFromPodIP ...
func (d *Dummy) GetInstanceIDFromPodIP(s string) (string, error) {
	return "", nil
}

func NewDummy() *Dummy {
	albcache.NewCache(metric.DummyCollector{})
	return &Dummy{
		GetIngressAnnotationsResponse: annotations.NewIngressDummy(),
		GetServiceAnnotationsResponse: annotations.NewServiceDummy(),
	}
}

func NewDummyIngress() *extensions.Ingress {
	albcache.NewCache(metric.DummyCollector{})
	ports := []int64{
		int64(80),
		int64(443),
		int64(8080),
	}
	hosts := []string{
		"1.test.domain",
		"2.test.domain",
		"3.test.domain",
	}
	paths := []string{
		"/",
		"/store",
		"/store/dev",
	}
	svcs := []string{
		"1service",
		"2service",
		"3service",
	}
	svcPorts := []int{
		30001,
		30002,
		30003,
	}

	ing := &extensions.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "ingress1",
			Namespace: api.NamespaceDefault,
		},
		Spec: extensions.IngressSpec{
			Backend: &extensions.IngressBackend{
				ServiceName: "default-backend",
				ServicePort: intstr.FromInt(80),
			},
		},
	}
	for i := range ports {
		extRules := extensions.IngressRule{
			Host: hosts[i],
			IngressRuleValue: extensions.IngressRuleValue{
				HTTP: &extensions.HTTPIngressRuleValue{
					Paths: []extensions.HTTPIngressPath{{
						Path: paths[i],
						Backend: extensions.IngressBackend{
							ServiceName: svcs[i],
							ServicePort: intstr.FromInt(svcPorts[i]),
						},
					},
					},
				},
			},
		}
		ing.Spec.Rules = append(ing.Spec.Rules, extRules)
	}
	return ing

}
