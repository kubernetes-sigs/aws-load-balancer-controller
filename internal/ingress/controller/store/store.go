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
	"time"

	"github.com/blang/semver"
	"github.com/eapache/channels"
	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/scheme"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/class"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"
)

// Storer is the interface that wraps the required methods to gather information
// about ingresses, services, secrets and ingress annotations.
type Storer interface {
	// GetConfigMap returns the ConfigMap matching key.
	GetConfigMap(key string) (*corev1.ConfigMap, error)

	// GetService returns the Service matching key.
	GetService(key string) (*corev1.Service, error)

	// GetServiceEndpoints returns the Endpoints of a Service matching key.
	GetServiceEndpoints(key string) (*corev1.Endpoints, error)

	// GetServiceAnnotations returns the parsed annotations of an Service matching key. if ingress is non-nil, merges ingress annotations into the service.
	GetServiceAnnotations(key string, ingress *annotations.Ingress) (*annotations.Service, error)

	// GetIngress returns the Ingress matching key.
	GetIngress(key string) (*extensions.Ingress, error)

	// ListNodes returns a list of all Nodes in the store.
	ListNodes() []*corev1.Node

	// ListIngresses returns a list of all Ingresses in the store.
	ListIngresses() []*extensions.Ingress

	// GetIngressAnnotations returns the parsed annotations of an Ingress matching key.
	GetIngressAnnotations(key string) (*annotations.Ingress, error)

	// GetConfig returns the controller configuration
	GetConfig() *config.Configuration

	// Run initiates the synchronization of the controllers
	Run(stopCh chan struct{})

	// GetInstanceIDFromPodIP gets the instance id of the node running a pod
	GetInstanceIDFromPodIP(string) (string, error)

	// GetNodeInstanceID gets the instance id of node
	GetNodeInstanceID(node *corev1.Node) (string, error)

	// GetClusterInstanceIDs gets id of all instances inside cluster
	GetClusterInstanceIDs() ([]string, error)
}

// EventType type of event associated with an informer
type EventType string

const (
	// CreateEvent event associated with new objects in an informer
	CreateEvent EventType = "CREATE"
	// UpdateEvent event associated with an object update in an informer
	UpdateEvent EventType = "UPDATE"
	// DeleteEvent event associated when an object is removed from an informer
	DeleteEvent EventType = "DELETE"
	// ConfigurationEvent event associated when a controller configuration object is created or updated
	ConfigurationEvent EventType = "CONFIGURATION"
)

// Event holds the context of an event.
type Event struct {
	Type EventType
	Obj  interface{}
}

// Informer defines the required SharedIndexInformers that interact with the API server.
type Informer struct {
	Ingress   cache.SharedIndexInformer
	Endpoint  cache.SharedIndexInformer
	Service   cache.SharedIndexInformer
	Node      cache.SharedIndexInformer
	Pod       cache.SharedIndexInformer
	ConfigMap cache.SharedIndexInformer
}

// Lister contains object listers (stores).
type Lister struct {
	Ingress           IngressLister
	Service           ServiceLister
	Node              NodeLister
	Pod               PodLister
	Endpoint          EndpointLister
	ConfigMap         ConfigMapLister
	IngressAnnotation IngressAnnotationsLister
	ServiceAnnotation ServiceAnnotationsLister
}

// NotExistsError is returned when an object does not exist in a local store.
type NotExistsError string

// Error implements the error interface.
func (e NotExistsError) Error() string {
	return fmt.Sprintf("no object matching key %q in local store", string(e))
}

// Run initiates the synchronization of the informers against the API server.
func (i *Informer) Run(stopCh chan struct{}) {
	go i.Endpoint.Run(stopCh)
	go i.Service.Run(stopCh)
	go i.Node.Run(stopCh)
	go i.Pod.Run(stopCh)
	go i.ConfigMap.Run(stopCh)

	// wait for all involved caches to be synced before processing items
	// from the queue
	if !cache.WaitForCacheSync(stopCh,
		i.Endpoint.HasSynced,
		i.Service.HasSynced,
		i.ConfigMap.HasSynced,
		i.Node.HasSynced,
		i.Pod.HasSynced,
	) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
	}

	// in big clusters, deltas can keep arriving even after HasSynced
	// functions have returned 'true'
	time.Sleep(1 * time.Second)

	// we can start syncing ingress objects only after other caches are
	// ready, because ingress rules require content from other listers, and
	// 'add' events get triggered in the handlers during caches population.
	go i.Ingress.Run(stopCh)
	if !cache.WaitForCacheSync(stopCh,
		i.Ingress.HasSynced,
	) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
	}
}

// k8sStore internal Storer implementation using informers and thread safe stores
type k8sStore struct {
	// informer contains the cache Informers
	informers *Informer

	// listers contains the cache.Store interfaces used in the ingress controller
	listers *Lister

	ingannotations annotations.Extractor
	svcannotations annotations.Extractor

	// updateCh
	updateCh *channels.RingChannel

	// configuration
	cfg *config.Configuration

	// mu protects against simultaneous invocations of syncSecret
	mu *sync.Mutex
}

// New creates a new object store to be used in the ingress controller
func New(cfg *config.Configuration, updateCh *channels.RingChannel) Storer {

	store := &k8sStore{
		informers: &Informer{},
		listers:   &Lister{},
		updateCh:  updateCh,
		mu:        &sync.Mutex{},
		cfg:       cfg,
	}

	// cfg.Client.CoreV1()
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.V(2).Infof)
	eventBroadcaster.StartRecordingToSink(&clientcorev1.EventSinkImpl{
		Interface: cfg.Client.CoreV1().Events(cfg.Namespace),
	})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{
		Component: "aws-alb-ingress-controller",
	})

	// k8sStore fulfils resolver.Resolver interface
	store.ingannotations = annotations.NewIngressAnnotationExtractor(store)
	store.svcannotations = annotations.NewServiceAnnotationExtractor(store)

	store.listers.IngressAnnotation.Store = cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	store.listers.ServiceAnnotation.Store = cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)

	// create informers factory, enable and assign required informers
	infFactory := informers.NewSharedInformerFactoryWithOptions(cfg.Client, cfg.ResyncPeriod,
		informers.WithNamespace(cfg.Namespace),
		informers.WithTweakListOptions(func(*metav1.ListOptions) {}))

	store.informers.Ingress = infFactory.Extensions().V1beta1().Ingresses().Informer()
	store.listers.Ingress.Store = store.informers.Ingress.GetStore()

	store.informers.Endpoint = infFactory.Core().V1().Endpoints().Informer()
	store.listers.Endpoint.Store = store.informers.Endpoint.GetStore()

	store.informers.ConfigMap = infFactory.Core().V1().ConfigMaps().Informer()
	store.listers.ConfigMap.Store = store.informers.ConfigMap.GetStore()

	store.informers.Service = infFactory.Core().V1().Services().Informer()
	store.listers.Service.Store = store.informers.Service.GetStore()

	store.informers.Node = infFactory.Core().V1().Nodes().Informer()
	store.listers.Node.Store = store.informers.Node.GetStore()

	store.informers.Pod = infFactory.Core().V1().Pods().Informer()
	store.listers.Pod.Store = store.informers.Pod.GetStore()

	ingEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ing := obj.(*extensions.Ingress)
			if !class.IsValid(ing) {
				a, _ := parser.GetStringAnnotation(class.IngressKey, ing)
				glog.V(2).Infof("ignoring add for ingress %v based on annotation %v with value %v", ing.Name, class.IngressKey, a)
				return
			}
			recorder.Eventf(ing, corev1.EventTypeNormal, "CREATE", fmt.Sprintf("Ingress %s/%s", ing.Namespace, ing.Name))

			store.extractIngressAnnotations(ing)

			updateCh.In() <- Event{
				Type: CreateEvent,
				Obj:  obj,
			}
		},
		DeleteFunc: func(obj interface{}) {
			ing, ok := obj.(*extensions.Ingress)
			if !ok {
				// If we reached here it means the ingress was deleted but its final state is unrecorded.
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					glog.Errorf("couldn't get object from tombstone %#v", obj)
					return
				}
				ing, ok = tombstone.Obj.(*extensions.Ingress)
				if !ok {
					glog.Errorf("Tombstone contained object that is not an Ingress: %#v", obj)
					return
				}
			}
			if !class.IsValid(ing) {
				glog.Infof("ignoring delete for ingress %v based on annotation %v", ing.Name, class.IngressKey)
				return
			}
			recorder.Eventf(ing, corev1.EventTypeNormal, "DELETE", fmt.Sprintf("Ingress %s/%s", ing.Namespace, ing.Name))

			store.listers.IngressAnnotation.Delete(ing)

			updateCh.In() <- Event{
				Type: DeleteEvent,
				Obj:  obj,
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			oldIng := old.(*extensions.Ingress)
			curIng := cur.(*extensions.Ingress)
			validOld := class.IsValid(oldIng)
			validCur := class.IsValid(curIng)
			if !validOld && validCur {
				glog.Infof("creating ingress %v based on annotation %v", curIng.Name, class.IngressKey)
				recorder.Eventf(curIng, corev1.EventTypeNormal, "CREATE", fmt.Sprintf("Ingress %s/%s", curIng.Namespace, curIng.Name))
			} else if validOld && !validCur {
				glog.Infof("removing ingress %v based on annotation %v", curIng.Name, class.IngressKey)
				recorder.Eventf(curIng, corev1.EventTypeNormal, "DELETE", fmt.Sprintf("Ingress %s/%s", curIng.Namespace, curIng.Name))
			} else if validCur && !reflect.DeepEqual(old, cur) {
				recorder.Eventf(curIng, corev1.EventTypeNormal, "UPDATE", fmt.Sprintf("Ingress %s/%s", curIng.Namespace, curIng.Name))
			}

			store.extractIngressAnnotations(curIng)

			updateCh.In() <- Event{
				Type: UpdateEvent,
				Obj:  cur,
			}
		},
	}

	epEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			updateCh.In() <- Event{
				Type: CreateEvent,
				Obj:  obj,
			}
		},
		DeleteFunc: func(obj interface{}) {
			updateCh.In() <- Event{
				Type: DeleteEvent,
				Obj:  obj,
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			oep := old.(*corev1.Endpoints)
			cep := cur.(*corev1.Endpoints)
			if !reflect.DeepEqual(cep.Subsets, oep.Subsets) {
				updateCh.In() <- Event{
					Type: UpdateEvent,
					Obj:  cur,
				}
			}
		},
	}

	svcEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			svc := obj.(*corev1.Service)
			store.extractServiceAnnotations(svc)

			updateCh.In() <- Event{
				Type: CreateEvent,
				Obj:  obj,
			}
		},
		DeleteFunc: func(obj interface{}) {
			svc := obj.(*corev1.Service)
			store.extractServiceAnnotations(svc)
			updateCh.In() <- Event{
				Type: DeleteEvent,
				Obj:  obj,
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			if !reflect.DeepEqual(old, cur) {
				svc := cur.(*corev1.Service)
				store.extractServiceAnnotations(svc)
				updateCh.In() <- Event{
					Type: UpdateEvent,
					Obj:  cur,
				}
			}
		},
	}

	cmEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			cm := obj.(*corev1.ConfigMap)
			key := k8s.MetaNamespaceKey(cm)
			// updates to configuration configmaps can trigger an update
			if key == cfg.ConfigMapName {
				recorder.Eventf(cm, corev1.EventTypeNormal, "CREATE", fmt.Sprintf("ConfigMap %v", key))
				if key == cfg.ConfigMapName {
					store.setConfig(cm)
				}
				updateCh.In() <- Event{
					Type: ConfigurationEvent,
					Obj:  obj,
				}
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			if !reflect.DeepEqual(old, cur) {
				cm := cur.(*corev1.ConfigMap)
				key := k8s.MetaNamespaceKey(cm)
				// updates to configuration configmaps can trigger an update
				if key == cfg.ConfigMapName {
					recorder.Eventf(cm, corev1.EventTypeNormal, "UPDATE", fmt.Sprintf("ConfigMap %v", key))
					if key == cfg.ConfigMapName {
						store.setConfig(cm)
					}

					ings := store.listers.IngressAnnotation.List()
					for _, ingKey := range ings {
						key := k8s.MetaNamespaceKey(ingKey)
						ing, err := store.GetIngress(key)
						if err != nil {
							glog.Errorf("could not find Ingress %v in local store: %v", key, err)
							continue
						}
						store.extractIngressAnnotations(ing)
					}

					updateCh.In() <- Event{
						Type: ConfigurationEvent,
						Obj:  cur,
					}
				}
			}
		},
	}

	store.informers.Ingress.AddEventHandler(ingEventHandler)
	store.informers.Endpoint.AddEventHandler(epEventHandler)
	store.informers.ConfigMap.AddEventHandler(cmEventHandler)
	store.informers.Service.AddEventHandler(svcEventHandler)
	// TODO Node events

	// do not wait for informers to read the configmap configuration
	// ns, name, _ := k8s.ParseNameNS(configmap)
	// cm, err := client.CoreV1().ConfigMaps(ns).Get(name, metav1.GetOptions{})
	// if err != nil {
	// 	glog.Warningf("Unexpected error reading configuration configmap: %v", err)
	// }

	// store.setConfig(cm)
	return store
}

// extractIngressAnnotations parses ingress annotations converting the value of the
// annotation to a go struct and also information about the referenced secrets
func (s *k8sStore) extractIngressAnnotations(ing *extensions.Ingress) {
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

// objectRefAnnotationNsKey returns an object reference formatted as a
// 'namespace/name' key from the given annotation name.
func objectRefAnnotationNsKey(ann string, ing *extensions.Ingress) (string, error) {
	annValue, err := parser.GetStringAnnotation(ann, ing)
	if annValue == nil {
		return "", err
	}

	secrNs, secrName, err := cache.SplitMetaNamespaceKey(*annValue)
	if secrName == "" {
		return "", err
	}

	if secrNs == "" {
		return fmt.Sprintf("%v/%v", ing.Namespace, secrName), nil
	}
	return *annValue, nil
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

// GetIngress returns the Ingress matching key.
func (s k8sStore) GetIngress(key string) (*extensions.Ingress, error) {
	return s.listers.Ingress.ByKey(key)
}

// GetConfig returns the controller configuration.
func (s k8sStore) GetConfig() *config.Configuration {
	return s.cfg
}

// ListIngresses returns the list of Ingresses
func (s k8sStore) ListIngresses() []*extensions.Ingress {
	// filter ingress rules
	var ingresses []*extensions.Ingress
	for _, item := range s.listers.Ingress.List() {
		ing := item.(*extensions.Ingress)
		if !class.IsValid(ing) {
			continue
		}

		for ri, rule := range ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}

			for pi, path := range rule.HTTP.Paths {
				if path.Path == "" {
					ing.Spec.Rules[ri].HTTP.Paths[pi].Path = "/"
				}
			}
		}

		ingresses = append(ingresses, ing)
	}

	return ingresses
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
		sa.Merge(ingress, s.cfg)
	}

	return sa, nil
}

// GetConfigMap returns the ConfigMap matching key.
func (s k8sStore) GetConfigMap(key string) (*corev1.ConfigMap, error) {
	return s.listers.ConfigMap.ByKey(key)
}

// GetServiceEndpoints returns the Endpoints of a Service matching key.
func (s k8sStore) GetServiceEndpoints(key string) (*corev1.Endpoints, error) {
	return s.listers.Endpoint.ByKey(key)
}

func (s *k8sStore) setConfig(cmap *corev1.ConfigMap) {
	// s.backendConfig = ngx_template.ReadConfig(cmap.Data)
	return
}

// Run initiates the synchronization of the informers
func (s k8sStore) Run(stopCh chan struct{}) {
	// start informers
	s.informers.Run(stopCh)
}

func (s *k8sStore) GetNodeInstanceID(node *corev1.Node) (string, error) {
	nodeVersion, _ := semver.ParseTolerant(node.Status.NodeInfo.KubeletVersion)
	if nodeVersion.Major == 1 && nodeVersion.Minor <= 10 {
		return node.Spec.DoNotUse_ExternalID, nil
	}

	providerId := node.Spec.ProviderID
	if providerId == "" {
		return "", fmt.Errorf("No providerID found for node %s", node.ObjectMeta.Name)
	}

	p := strings.Split(providerId, "/")
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

func (s *k8sStore) GetClusterInstanceIDs() (result []string, err error) {
	for _, node := range s.ListNodes() {
		instanceID, err := s.GetNodeInstanceID(node)
		if err != nil {
			return nil, err
		}
		result = append(result, instanceID)
	}
	return result, nil
}
