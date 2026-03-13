package reader

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReadFromClient_AllNamespaces(t *testing.T) {
	ing := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ing", Namespace: "default"},
		Spec:       networking.IngressSpec{},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
	}
	ic := &networking.IngressClass{
		ObjectMeta: metav1.ObjectMeta{Name: "alb"},
		Spec: networking.IngressClassSpec{
			Controller: "ingress.k8s.aws/alb",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ing, svc, ic).
		Build()

	resources, err := readFromClient(context.Background(), k8sClient, nil, ClusterReaderOptions{
		AllNamespaces: true,
	})
	require.NoError(t, err)

	assert.Len(t, resources.Ingresses, 1)
	assert.Equal(t, "test-ing", resources.Ingresses[0].Name)

	assert.Len(t, resources.Services, 1)
	assert.Equal(t, "test-svc", resources.Services[0].Name)

	assert.Len(t, resources.IngressClasses, 1)
	assert.Equal(t, "alb", resources.IngressClasses[0].Name)
}

func TestReadFromClient_NamespaceFilter(t *testing.T) {
	ingDefault := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "ing-default", Namespace: "default"},
	}
	ingProd := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "ing-prod", Namespace: "production"},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ingDefault, ingProd).
		Build()

	resources, err := readFromClient(context.Background(), k8sClient, nil, ClusterReaderOptions{
		Namespace: "production",
	})
	require.NoError(t, err)

	assert.Len(t, resources.Ingresses, 1)
	assert.Equal(t, "ing-prod", resources.Ingresses[0].Name)
}

func TestReadFromClient_EmptyCluster(t *testing.T) {
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	resources, err := readFromClient(context.Background(), k8sClient, nil, ClusterReaderOptions{
		AllNamespaces: true,
	})
	require.NoError(t, err)

	assert.Len(t, resources.Ingresses, 0)
	assert.Len(t, resources.Services, 0)
	assert.Len(t, resources.IngressClasses, 0)
}
