package store

import (
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
)

type Dummy struct {
	cfg                           *config.Configuration
	GetIngressAnnotationsResponse *annotations.Ingress
	GetServiceAnnotationsResponse *annotations.Service

	GetServiceFunc            func(string) (*corev1.Service, error)
	ListNodesFunc             func() []*corev1.Node
	GetNodeInstanceIDFunc     func(*corev1.Node) (string, error)
	GetClusterInstanceIDsFunc func() ([]string, error)

	GetServiceEndpointsFunc func(string) (*corev1.Endpoints, error)
}

// GetConfigMap ...
func (d Dummy) GetConfigMap(key string) (*corev1.ConfigMap, error) {
	return nil, nil
}

// GetService ...
func (d Dummy) GetService(key string) (*corev1.Service, error) {
	return d.GetServiceFunc(key)
}

// GetServiceEndpoints ...
func (d Dummy) GetServiceEndpoints(key string) (*corev1.Endpoints, error) {
	return d.GetServiceEndpointsFunc(key)
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
	return d.ListNodesFunc()
}

// ListIngresses ...
func (d Dummy) ListIngresses() []*extensions.Ingress {
	return nil
}

// GetIngressAnnotations ...
func (d Dummy) GetIngressAnnotations(key string) (*annotations.Ingress, error) {
	return d.GetIngressAnnotationsResponse, nil
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
func (d *Dummy) GetNodeInstanceID(node *corev1.Node) (string, error) {
	return d.GetNodeInstanceIDFunc(node)
}

// GetInstanceIDFromPodIP ...
func (d *Dummy) GetInstanceIDFromPodIP(s string) (string, error) {
	return "", nil
}

// GetClusterInstanceIDs is the mock for dummy store
func (d *Dummy) GetClusterInstanceIDs() ([]string, error) {
	return d.GetClusterInstanceIDsFunc()
}

func NewDummy() *Dummy {
	return &Dummy{
		GetServiceFunc:                func(_ string) (*corev1.Service, error) { return dummy.NewService(), nil },
		ListNodesFunc:                 func() []*corev1.Node { return nil },
		GetNodeInstanceIDFunc:         func(*corev1.Node) (string, error) { return "", nil },
		GetClusterInstanceIDsFunc:     func() ([]string, error) { return nil, nil },
		GetServiceEndpointsFunc:       func(string) (*corev1.Endpoints, error) { return nil, nil },
		GetIngressAnnotationsResponse: annotations.NewIngressDummy(),
		GetServiceAnnotationsResponse: annotations.NewServiceDummy(),
	}
}
