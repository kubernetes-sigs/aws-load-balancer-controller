package quic

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestWorkerIdGenerator_GetWorkerId_ConfigMapNotExists(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)

	client := testclient.NewClientBuilder().WithScheme(scheme).Build()
	generator := newWorkerIdGenerator(client, client)

	workerId, err := generator.getWorkerId(1000)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, workerId, int64(0))
	assert.LessOrEqual(t, workerId, int64(1000))

	// Verify ConfigMap was created
	cm := &corev1.ConfigMap{}
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: workerIdConfigMapNamespace,
		Name:      workerIdConfigMapName,
	}, cm)
	require.NoError(t, err)
	assert.NotEmpty(t, cm.Data[workerIdConfigMapKey])
}

func TestWorkerIdGenerator_GetWorkerId_ConfigMapExists(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workerIdConfigMapName,
			Namespace: workerIdConfigMapNamespace,
		},
		Data: map[string]string{
			workerIdConfigMapKey: "42",
		},
	}

	client := testclient.NewClientBuilder().WithScheme(scheme).WithObjects(existingCM).Build()
	generator := newWorkerIdGenerator(client, client)

	workerId, err := generator.getWorkerId(1000)

	require.NoError(t, err)
	assert.Equal(t, int64(42), workerId)

	// Verify ConfigMap was updated with next ID
	cm := &corev1.ConfigMap{}
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: workerIdConfigMapNamespace,
		Name:      workerIdConfigMapName,
	}, cm)
	require.NoError(t, err)
	assert.Equal(t, "43", cm.Data[workerIdConfigMapKey])
}

func TestWorkerIdGenerator_GetWorkerId_ConfigMapExistsCorruptedData(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workerIdConfigMapName,
			Namespace: workerIdConfigMapNamespace,
		},
		Data: map[string]string{
			workerIdConfigMapKey: "invalid-number",
		},
	}

	client := testclient.NewClientBuilder().WithScheme(scheme).WithObjects(existingCM).Build()
	generator := newWorkerIdGenerator(client, client)

	_, err := generator.getWorkerId(1000)

	assert.Error(t, err)
}

func TestWorkerIdGenerator_GetWorkerId_ConfigMapExistsNoData(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workerIdConfigMapName,
			Namespace: workerIdConfigMapNamespace,
		},
	}

	client := testclient.NewClientBuilder().WithScheme(scheme).WithObjects(existingCM).Build()
	generator := newWorkerIdGenerator(client, client)

	workerId, err := generator.getWorkerId(1000)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, workerId, int64(0))
	assert.LessOrEqual(t, workerId, int64(1000))
}

func TestWorkerIdGenerator_CalculateNextWorkerId(t *testing.T) {
	generator := &workerIdGeneratorImpl{}

	tests := []struct {
		name      string
		currentId int64
		maxId     int64
		expected  int64
	}{
		{
			name:      "normal increment",
			currentId: 5,
			maxId:     100,
			expected:  6,
		},
		{
			name:      "rollover at max",
			currentId: 100,
			maxId:     100,
			expected:  1,
		},
		{
			name:      "zero to one",
			currentId: 0,
			maxId:     10,
			expected:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generator.calculateNextWorkerId(tt.currentId, tt.maxId)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWorkerIdGenerator_GenerateInitialId(t *testing.T) {
	generator := &workerIdGeneratorImpl{}

	maxId := int64(1000)
	id := generator.generateInitialId(maxId)

	assert.GreaterOrEqual(t, id, int64(0))
	assert.LessOrEqual(t, id, maxId)
}

func TestWorkerIdGenerator_StoreWorkerId_CreateNewConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)

	client := testclient.NewClientBuilder().WithScheme(scheme).Build()
	generator := &workerIdGeneratorImpl{
		kubeClient: client,
		apiReader:  client,
	}

	err := generator.storeWorkerId(42, 1000, nil)

	require.NoError(t, err)

	// Verify ConfigMap was created
	cm := &corev1.ConfigMap{}
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: workerIdConfigMapNamespace,
		Name:      workerIdConfigMapName,
	}, cm)
	require.NoError(t, err)
	assert.Equal(t, "43", cm.Data[workerIdConfigMapKey])
}

func TestWorkerIdGenerator_StoreWorkerId_UpdateExistingConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workerIdConfigMapName,
			Namespace: workerIdConfigMapNamespace,
		},
		Data: map[string]string{
			workerIdConfigMapKey: "42",
		},
	}

	client := testclient.NewClientBuilder().WithScheme(scheme).WithObjects(existingCM).Build()
	generator := &workerIdGeneratorImpl{
		kubeClient: client,
		apiReader:  client,
	}

	err := generator.storeWorkerId(42, 1000, existingCM)

	require.NoError(t, err)

	// Verify ConfigMap was updated
	cm := &corev1.ConfigMap{}
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: workerIdConfigMapNamespace,
		Name:      workerIdConfigMapName,
	}, cm)
	require.NoError(t, err)
	assert.Equal(t, "43", cm.Data[workerIdConfigMapKey])
}

// Mock client that returns specific errors for testing
type errorClient struct {
	client.Client
	getError    error
	createError error
	updateError error
}

func (c *errorClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if c.getError != nil {
		return c.getError
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *errorClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.createError != nil {
		return c.createError
	}
	return c.Client.Create(ctx, obj, opts...)
}

func (c *errorClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.updateError != nil {
		return c.updateError
	}
	return c.Client.Update(ctx, obj, opts...)
}

func TestWorkerIdGenerator_GetWorkerId_GetError(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)

	baseClient := testclient.NewClientBuilder().WithScheme(scheme).Build()
	client := &errorClient{
		Client:   baseClient,
		getError: fmt.Errorf("get failed"),
	}

	generator := newWorkerIdGenerator(client, client)

	_, err := generator.getWorkerId(1000)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get failed")
}

func TestWorkerIdGenerator_GetWorkerId_CreateError(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)

	baseClient := testclient.NewClientBuilder().WithScheme(scheme).Build()
	client := &errorClient{
		Client:      baseClient,
		getError:    apierrors.NewNotFound(schema.GroupResource{}, "test"),
		createError: fmt.Errorf("create failed"),
	}

	generator := newWorkerIdGenerator(client, client)

	_, err := generator.getWorkerId(1000)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "create failed")
}

func TestWorkerIdGenerator_GetWorkerId_UpdateError(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workerIdConfigMapName,
			Namespace: workerIdConfigMapNamespace,
		},
		Data: map[string]string{
			workerIdConfigMapKey: "42",
		},
	}

	baseClient := testclient.NewClientBuilder().WithScheme(scheme).WithObjects(existingCM).Build()
	client := &errorClient{
		Client:      baseClient,
		updateError: fmt.Errorf("update failed"),
	}

	generator := newWorkerIdGenerator(client, client)

	_, err := generator.getWorkerId(1000)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "update failed")
}
