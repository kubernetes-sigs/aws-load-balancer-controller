package store

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
)

type Dummy struct {
	GetIngressAnnotationsResponse *annotations.Ingress
	GetServiceAnnotationsResponse *annotations.Service
}

// GetConfigMap...
func (d *Dummy) GetConfigMap(key string) (*corev1.ConfigMap, error) {
	return nil, nil
}

// GetService...
func (d *Dummy) GetService(key string) (*corev1.Service, error) {
	return nil, nil
}

// GetServiceEndpoints...
func (d *Dummy) GetServiceEndpoints(key string) (*corev1.Endpoints, error) {
	return nil, nil
}

// GetServiceAnnotations...
func (d *Dummy) GetServiceAnnotations(key string) (*annotations.Service, error) {
	return d.GetServiceAnnotationsResponse, nil
}

// GetIngress...
func (d *Dummy) GetIngress(key string) (*extensions.Ingress, error) {
	return nil, nil
}

// ListNodes...
func (d *Dummy) ListNodes() []*corev1.Node {
	return nil
}

// ListIngresses...
func (d *Dummy) ListIngresses() []*extensions.Ingress {
	return nil
}

// GetIngressAnnotations...
func (d *Dummy) GetIngressAnnotations(key string) (*annotations.Ingress, error) {
	return d.GetIngressAnnotationsResponse, nil
}

// GetServicePort...
func (d *Dummy) GetServicePort(serviceKey, serviceType string, backendPort int32) (*int64, error) {
	return aws.Int64(8080), nil
}

// GetTargets...
func (d *Dummy) GetTargets(mode *string, namespace string, svc string, port *int64) albelbv2.TargetDescriptions {
	return nil
}

// Run...
func (d *Dummy) Run(stopCh chan struct{}) {
	return
}
