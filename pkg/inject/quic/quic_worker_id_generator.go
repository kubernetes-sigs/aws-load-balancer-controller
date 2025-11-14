package quic

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"math/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
)

const (
	workerIdConfigMapName      = "quic-server-id-worker-tracker"
	workerIdConfigMapNamespace = "kube-system"
	workerIdConfigMapKey       = "nextWorkerId"
)

func newWorkerIdGenerator(kubeClient client.Client, apiReader client.Reader) workerIdGenerator {
	return &workerIdGeneratorImpl{
		kubeClient: kubeClient,
		apiReader:  apiReader,
	}
}

type workerIdGenerator interface {
	getWorkerId(maxId int64) (int64, error)
}

type workerIdGeneratorImpl struct {
	kubeClient client.Client
	apiReader  client.Reader
}

func (w *workerIdGeneratorImpl) getWorkerId(maxId int64) (int64, error) {
	cm := &corev1.ConfigMap{}

	err := w.apiReader.Get(context.Background(), client.ObjectKey{
		Namespace: workerIdConfigMapNamespace,
		Name:      workerIdConfigMapName,
	}, cm)

	var workerId int64
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return 0, err
		}
		// Initial case -- no CM yet.
		workerId = w.generateInitialId(maxId)
		// mark as nil to signal that create is needed.
		cm = nil
	}

	var idFound bool
	var storedValue string
	if cm != nil && cm.Data != nil {
		storedValue, idFound = cm.Data[workerIdConfigMapKey]
	}

	if !idFound {
		// Corrupted data?
		workerId = w.generateInitialId(maxId)
	} else {
		workerId, err = strconv.ParseInt(storedValue, 10, 64)
		if err != nil {
			return 0, err
		}
	}
	return workerId, w.storeWorkerId(workerId, maxId, cm)
}

func (w *workerIdGeneratorImpl) storeWorkerId(currentId int64, maxId int64, oldCm *corev1.ConfigMap) error {

	nextId := w.calculateNextWorkerId(currentId, maxId)
	if oldCm == nil {
		newCm := &corev1.ConfigMap{}
		newCm.Name = workerIdConfigMapName
		newCm.Namespace = workerIdConfigMapNamespace
		newCm.Data = make(map[string]string)
		newCm.Data[workerIdConfigMapKey] = fmt.Sprintf("%d", nextId)
		return w.kubeClient.Create(context.Background(), newCm)
	}

	if oldCm.Data == nil {
		oldCm.Data = make(map[string]string)
	}
	oldCm.Data[workerIdConfigMapKey] = fmt.Sprintf("%d", nextId)
	return w.kubeClient.Update(context.Background(), oldCm)
}

func (w *workerIdGeneratorImpl) generateInitialId(maxId int64) int64 {
	return rand.Int63n(maxId + 1)
}

func (w *workerIdGeneratorImpl) calculateNextWorkerId(currentId, maxId int64) int64 {
	return (currentId + 1) % maxId
}

var _ workerIdGenerator = &workerIdGeneratorImpl{}
