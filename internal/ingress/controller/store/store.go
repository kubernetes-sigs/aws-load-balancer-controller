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

package store

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/blang/semver"
	"github.com/golang/glog"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/class"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Storer is the interface that wraps the required methods to gather information
// about ingresses, services, secrets and ingress annotations.
type Storer interface {
	// GetService returns the Service matching key.
	GetService(key string) (*corev1.Service, error)

	// GetServiceEndpoints returns the Endpoints of a Service matching key.
	GetServiceEndpoints(key string) (*corev1.Endpoints, error)

	// GetServiceAnnotations returns the parsed annotations of an Service matching key. if ingress is non-nil, merges ingress annotations into the service.
	GetServiceAnnotations(key string, ingress *annotations.Ingress) (*annotations.Service, error)

	// ListNodes returns a list of all Nodes in the store.
	ListNodes() []*corev1.Node

	// GetIngressAnnotations returns the parsed annotations of an Ingress matching key.
	GetIngressAnnotations(key string) (*annotations.Ingress, error)

	// GetConfig returns the controller configuration
	GetConfig() *config.Configuration

	// GetInstanceIDFromPodIP gets the instance id of the node running a pod
	GetInstanceIDFromPodIP(string) (string, error)

	// GetNodeInstanceID gets the instance id of node
	GetNodeInstanceID(node *corev1.Node) (string, error)
}

// Informer defines the required SharedIndexInformers that interact with the API server.
type Informer struct {
	Ingress  cache.SharedIndexInformer
	Service  cache.SharedIndexInformer
	Endpoint cache.SharedIndexInformer
	Node     cache.SharedIndexInformer
	Pod      cache.SharedIndexInformer
}

// Lister contains object listers (stores).
type Lister struct {
	Ingress           IngressLister
	Service           ServiceLister
	Endpoint          EndpointLister
	Node              NodeLister
	Pod               PodLister
	IngressAnnotation IngressAnnotationsLister
	ServiceAnnotation ServiceAnnotationsLister
}

// NotExistsError is returned when an object does not exist in a local store.
type NotExistsError string

// Error implements the error interface.
func (e NotExistsError) Error() string {
	return fmt.Sprintf("no object matching key %q in local store", string(e))
}

// k8sStore internal Storer implementation using informers and thread safe stores
type k8sStore struct {
	// informer contains the cache Informers
	informers *Informer

	// listers contains the cache.Store interfaces used in the ingress controller
	listers *Lister

	ingannotations annotations.Extractor
	svcannotations annotations.Extractor

	// configuration
	cfg *config.Configuration

	// mu protects against simultaneous invocations of syncSecret
	mu *sync.Mutex
}

// New creates a new object store to be used in the ingress controller
func New(mgr manager.Manager, cfg *config.Configuration) (Storer, error) {
	store := &k8sStore{
		informers: &Informer{},
		listers:   &Lister{},
		cfg:       cfg,
		mu:        &sync.Mutex{},
	}

	// k8sStore fulfils resolver.Resolver interface
	store.ingannotations = annotations.NewIngressAnnotationExtractor(store)
	store.svcannotations = annotations.NewServiceAnnotationExtractor(store)
	store.listers.IngressAnnotation.Store = cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	store.listers.ServiceAnnotation.Store = cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)

	mgrCache := mgr.GetCache()
	var err error
	store.informers.Ingress, err = mgrCache.GetInformer(&networking.Ingress{})
	if err != nil {
		return nil, err
	}
	store.listers.Ingress.Store = store.informers.Ingress.GetStore()

	store.informers.Service, err = mgrCache.GetInformer(&corev1.Service{})
	if err != nil {
		return nil, err
	}
	store.listers.Service.Store = store.informers.Service.GetStore()

	store.informers.Endpoint, err = mgrCache.GetInformer(&corev1.Endpoints{})
	if err != nil {
		return nil, err
	}
	store.listers.Endpoint.Store = store.informers.Endpoint.GetStore()

	store.informers.Node, err = mgrCache.GetInformer(&corev1.Node{})
	if err != nil {
		return nil, err
	}
	store.listers.Node.Store = store.informers.Node.GetStore()

	store.informers.Pod, err = mgrCache.GetInformer(&corev1.Pod{})
	if err != nil {
		return nil, err
	}
	store.listers.Pod.Store = store.informers.Pod.GetStore()

	ingEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ing := obj.(*networking.Ingress)
			if !class.IsValidIngress(cfg.IngressClass, ing) {
				return
			}
			store.extractIngressAnnotations(ing)
		},
		DeleteFunc: func(obj interface{}) {
			ing, ok := obj.(*networking.Ingress)
			if !ok {
				// If we reached here it means the ingress was deleted but its final state is unrecorded.
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					glog.Errorf("couldn't get object from tombstone %#v", obj)
					return
				}
				ing, ok = tombstone.Obj.(*networking.Ingress)
				if !ok {
					glog.Errorf("Tombstone contained object that is not an Ingress: %#v", obj)
					return
				}
			}
			if !class.IsValidIngress(cfg.IngressClass, ing) {
				return
			}
			_ = store.listers.IngressAnnotation.Delete(ing)
		},
		UpdateFunc: func(old, cur interface{}) {
			curIng := cur.(*networking.Ingress)
			if !class.IsValidIngress(cfg.IngressClass, curIng) {
				return
			}
			store.extractIngressAnnotations(curIng)
		},
	}

	svcEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			svc := obj.(*corev1.Service)
			store.extractServiceAnnotations(svc)
		},
		DeleteFunc: func(obj interface{}) {
			svc := obj.(*corev1.Service)
			store.extractServiceAnnotations(svc)
		},
		UpdateFunc: func(old, cur interface{}) {
			if !reflect.DeepEqual(old, cur) {
				svc := cur.(*corev1.Service)
				store.extractServiceAnnotations(svc)
			}
		},
	}

	store.informers.Ingress.AddEventHandler(ingEventHandler)
	store.informers.Service.AddEventHandler(svcEventHandler)
	return store, nil
}

// extractIngressAnnotations parses ingress annotations converting the value of the
// annotation to a go struct and also information about the referenced secrets
func (s *k8sStore) extractIngressAnnotations(ing *networking.Ingress) {
	key := k8s.MetaNamespaceKey(ing)
	glog.V(3).Infof("updating annotations information for ingress %v", key)

	anns := s.ingannotations.ExtractIngress(ing)

	err := s.listers.IngressAnnotation.Update(anns)
	if err != nil {
		glog.Error(err)
	}
}

// extractServiceAnnotations parses service annotations converting the value of the
// annotation to a go struct and also information about the referenced secrets
func (s *k8sStore) extractServiceAnnotations(svc *corev1.Service) {
	key := k8s.MetaNamespaceKey(svc)
	glog.V(3).Infof("updating annotations information for service %v", key)

	anns := s.svcannotations.ExtractService(svc)
	err := s.listers.ServiceAnnotation.Update(anns)
	if err != nil {
		glog.Error(err)
	}
}

// GetService returns the Service matching key.
func (s k8sStore) GetService(key string) (*corev1.Service, error) {
	return s.listers.Service.ByKey(key)
}

// ListNodes returns the list of Nodes
func (s k8sStore) ListNodes() []*corev1.Node {
	var nodes []*corev1.Node
	for _, item := range s.listers.Node.List() {
		n := item.(*corev1.Node)

		if !class.IsValidNode(n) {
			continue
		}

		nodes = append(nodes, n)
	}

	return nodes
}

// GetConfig returns the controller configuration.
func (s k8sStore) GetConfig() *config.Configuration {
	return s.cfg
}

// GetIngressAnnotations returns the parsed annotations of an Ingress matching key.
func (s k8sStore) GetIngressAnnotations(key string) (*annotations.Ingress, error) {
	ia, err := s.listers.IngressAnnotation.ByKey(key)
	if err != nil {
		return nil, err
	}

	return ia, nil
}

// GetServiceAnnotations returns the parsed annotations of an Service matching key.
func (s k8sStore) GetServiceAnnotations(key string, ingress *annotations.Ingress) (*annotations.Service, error) {
	sa, err := s.listers.ServiceAnnotation.ByKey(key)
	if err != nil {
		return nil, err
	}

	if ingress != nil {
		return sa.Merge(ingress, s.cfg), nil
	}

	return sa, nil
}

// GetServiceEndpoints returns the Endpoints of a Service matching key.
func (s k8sStore) GetServiceEndpoints(key string) (*corev1.Endpoints, error) {
	return s.listers.Endpoint.ByKey(key)
}

func (s *k8sStore) GetNodeInstanceID(node *corev1.Node) (string, error) {
	nodeVersion, _ := semver.ParseTolerant(node.Status.NodeInfo.KubeletVersion)
	if nodeVersion.Major == 1 && nodeVersion.Minor <= 10 {
		return node.Spec.DoNotUse_ExternalID, nil
	}

	providerID := node.Spec.ProviderID
	if providerID == "" {
		return "", fmt.Errorf("No providerID found for node %s", node.ObjectMeta.Name)
	}

	p := strings.Split(providerID, "/")
	return p[len(p)-1], nil
}

func (s *k8sStore) GetInstanceIDFromPodIP(ip string) (string, error) {

	var hostIP string
	for _, item := range s.listers.Pod.List() {
		pod := item.(*corev1.Pod)
		if pod.Status.PodIP == ip {
			hostIP = pod.Status.HostIP
			break
		}
	}

	if hostIP == "" {
		return "", fmt.Errorf("Unable to locate a host for pod ip: %v", ip)
	}

	for _, item := range s.listers.Node.List() {
		node := item.(*corev1.Node)
		for _, addr := range node.Status.Addresses {
			if addr.Address == hostIP {
				return s.GetNodeInstanceID(node)
			}
		}
	}

	return "", fmt.Errorf("Unable to locate a host for pod ip: %v", ip)
}
