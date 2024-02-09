package k8s

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sync"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// SecretsManager manages the secret resources needed by the controller
type SecretsManager interface {
	// MonitorSecrets manages the individual watches for the given secrets
	MonitorSecrets(ingressGroupID string, secrets []types.NamespacedName)
}

func NewSecretsManager(clientSet kubernetes.Interface, secretsEventChan chan<- event.GenericEvent, logger logr.Logger) *defaultSecretsManager {
	return &defaultSecretsManager{
		mutex:            sync.Mutex{},
		secretMap:        make(map[types.NamespacedName]*secretItem),
		secretsEventChan: secretsEventChan,
		clientSet:        clientSet,
		logger:           logger,
	}
}

var _ SecretsManager = &defaultSecretsManager{}

type defaultSecretsManager struct {
	mutex            sync.Mutex
	secretMap        map[types.NamespacedName]*secretItem
	secretsEventChan chan<- event.GenericEvent
	clientSet        kubernetes.Interface
	queue            workqueue.RateLimitingInterface
	logger           logr.Logger
}

type secretItem struct {
	store     cache.Store
	rt        *cache.Reflector
	ingresses sets.String

	stopCh chan struct{}
}

func (m *defaultSecretsManager) MonitorSecrets(ingressGroupID string, secrets []types.NamespacedName) {
	m.logger.V(1).Info("Monitoring secrets", "groupID", ingressGroupID, "secrets", secrets)
	m.mutex.Lock()
	defer m.mutex.Unlock()

	inputSecrets := make(sets.String)
	for _, secret := range secrets {
		inputSecrets.Insert(secret.String())
		item, exists := m.secretMap[secret]
		if !exists {
			m.logger.V(1).Info("secret is not being monitored, adding watch", "item", secret)
			item = m.newReflector(secret.Namespace, secret.Name)
			m.secretMap[secret] = item
		}
		item.ingresses.Insert(ingressGroupID)
	}

	// Perform garbage collection
	var cleanupSecrets []types.NamespacedName
	for secret, secretItem := range m.secretMap {
		if inputSecrets.Has(secret.String()) {
			continue
		}
		if secretItem.ingresses.Has(ingressGroupID) {
			secretItem.ingresses.Delete(ingressGroupID)
		}
		if secretItem.ingresses.Len() == 0 {
			cleanupSecrets = append(cleanupSecrets, secret)
		}
	}
	for _, secret := range cleanupSecrets {
		m.logger.V(1).Info("secret no longer needs monitoring, stopping the watch", "item", secret)
		m.secretMap[secret].stopReflector()
		delete(m.secretMap, secret)
	}
}

func (m *defaultSecretsManager) newReflector(namespace, name string) *secretItem {
	fieldSelector := fields.Set{"metadata.name": name}.AsSelector().String()
	listFunc := func(options metav1.ListOptions) (runtime.Object, error) {
		options.FieldSelector = fieldSelector
		return m.clientSet.CoreV1().Secrets(namespace).List(context.TODO(), options)
	}
	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		options.FieldSelector = fieldSelector
		return m.clientSet.CoreV1().Secrets(namespace).Watch(context.TODO(), options)
	}
	store := m.newStore()
	rt := cache.NewNamedReflector(
		fmt.Sprintf("secret-%s/%s", namespace, name),
		&cache.ListWatch{ListFunc: listFunc, WatchFunc: watchFunc},
		&corev1.Secret{},
		store,
		0,
	)
	item := &secretItem{
		store:     store,
		rt:        rt,
		ingresses: make(sets.String),
		stopCh:    make(chan struct{}),
	}
	go item.startReflector()
	return item
}

func (m *defaultSecretsManager) newStore() *SecretsStore {
	return NewSecretsStore(m.secretsEventChan, cache.MetaNamespaceKeyFunc, m.logger)
}

func (s *secretItem) stopReflector() {
	close(s.stopCh)
}

func (s *secretItem) startReflector() {
	s.rt.Run(s.stopCh)
}
