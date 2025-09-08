package k8s

import (
	"context"
	"fmt"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/event"

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
	MonitorSecrets(consumerID string, secrets []types.NamespacedName)

	// GetSecret retrieves from cache (if monitoring) or falls back to API
	GetSecret(ctx context.Context, k8sClient client.Client, secretKey types.NamespacedName) (*corev1.Secret, error)
}

func NewSecretsManager(clientSet kubernetes.Interface, secretsEventChan chan<- event.TypedGenericEvent[*corev1.Secret], logger logr.Logger) *defaultSecretsManager {
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
	secretsEventChan chan<- event.TypedGenericEvent[*corev1.Secret]
	clientSet        kubernetes.Interface
	logger           logr.Logger
}

type secretItem struct {
	store     cache.Store
	rt        *cache.Reflector
	consumers sets.String

	stopCh chan struct{}
}

func (m *defaultSecretsManager) MonitorSecrets(consumerID string, secrets []types.NamespacedName) {
	m.logger.V(1).Info("Monitoring secrets", "consumer", consumerID, "secrets", secrets)
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
		item.consumers.Insert(consumerID)
	}

	// Perform garbage collection
	var cleanupSecrets []types.NamespacedName
	for secret, secretItem := range m.secretMap {
		if inputSecrets.Has(secret.String()) {
			continue
		}
		if secretItem.consumers.Has(consumerID) {
			secretItem.consumers.Delete(consumerID)
		}
		if secretItem.consumers.Len() == 0 {
			cleanupSecrets = append(cleanupSecrets, secret)
		}
	}
	for _, secret := range cleanupSecrets {
		m.logger.V(1).Info("secret no longer needs monitoring, stopping the watch", "item", secret)
		m.secretMap[secret].stopReflector()
		delete(m.secretMap, secret)
	}
}

func (m *defaultSecretsManager) GetSecret(ctx context.Context, k8sClient client.Client, secretKey types.NamespacedName) (*corev1.Secret, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Check if we're monitoring this secret (has cached data)
	if secretItem, exists := m.secretMap[secretKey]; exists {
		obj, exists, err := secretItem.store.GetByKey(secretKey.String())
		if err != nil {
			return nil, fmt.Errorf("error retrieving secret from cache: %w", err)
		}
		if exists {
			if secret, ok := obj.(*corev1.Secret); ok {
				m.logger.V(1).Info("Secret retrieved from cache", "secret", secretKey)
				return secret.DeepCopy(), nil // Return copy to prevent mutations
			}
		}
		// Cache miss - secret might be deleted, fall through to API call
		m.logger.V(1).Info("Secret not found in cache, falling back to API", "secret", secretKey)
	}

	// Fallback
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, secretKey, secret); err != nil {
		return nil, err
	}

	return secret, nil
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
		consumers: make(sets.String),
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
