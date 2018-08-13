package store

import (
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albcache"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"

	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
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
func (d Dummy) GetServicePort(backend extensions.IngressBackend, namespace, targetType string) (int, error) {
	return 8080, nil
}

// GetTargets ...
func (d Dummy) GetTargets(mode *string, namespace string, svc string, port int) (albelbv2.TargetDescriptions, error) {
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
