package reader

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReadFromClient(t *testing.T) {
	ingDefault := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "ing-default", Namespace: "default"},
	}
	ingProd := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "ing-prod", Namespace: "production"},
	}
	ingStaging := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "ing-staging", Namespace: "staging"},
	}
	svcDefault := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-default", Namespace: "default"},
	}
	svcProd := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-prod", Namespace: "production"},
	}
	svcStaging := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-staging", Namespace: "staging"},
	}
	ic := &networking.IngressClass{
		ObjectMeta: metav1.ObjectMeta{Name: "alb"},
		Spec: networking.IngressClassSpec{
			Controller: "ingress.k8s.aws/alb",
		},
	}

	allObjects := []runtime.Object{ingDefault, ingProd, ingStaging, svcDefault, svcProd, svcStaging, ic}

	tests := []struct {
		name             string
		objects          []runtime.Object
		opts             ClusterReaderOptions
		wantIngresses    []string
		wantServices     []string
		wantIngressClass []string
		wantErr          string
	}{
		{
			name:             "all namespaces",
			objects:          allObjects,
			opts:             ClusterReaderOptions{AllNamespaces: true},
			wantIngresses:    []string{"ing-default", "ing-prod", "ing-staging"},
			wantServices:     []string{"svc-default", "svc-prod", "svc-staging"},
			wantIngressClass: []string{"alb"},
		},
		{
			name:          "single namespace",
			objects:       allObjects,
			opts:          ClusterReaderOptions{Namespaces: []string{"production"}},
			wantIngresses: []string{"ing-prod"},
			wantServices:  []string{"svc-prod"},
		},
		{
			name:          "multiple namespaces",
			objects:       allObjects,
			opts:          ClusterReaderOptions{Namespaces: []string{"production", "staging"}},
			wantIngresses: []string{"ing-prod", "ing-staging"},
			wantServices:  []string{"svc-prod", "svc-staging"},
		},
		{
			name:          "ingress-name fetches specific ingress and namespace services",
			objects:       allObjects,
			opts:          ClusterReaderOptions{Namespaces: []string{"production"}, IngressName: "ing-prod"},
			wantIngresses: []string{"ing-prod"},
			wantServices:  []string{"svc-prod"},
		},
		{
			name:    "ingress-name not found errors",
			objects: allObjects,
			opts:    ClusterReaderOptions{Namespaces: []string{"production"}, IngressName: "nonexistent"},
			wantErr: "failed to get Ingress production/nonexistent",
		},
		{
			name:    "ingress-name with no namespaces errors",
			objects: allObjects,
			opts:    ClusterReaderOptions{IngressName: "ing-prod"},
			wantErr: "IngressName requires exactly one namespace, got 0",
		},
		{
			name:    "ingress-name with multiple namespaces errors",
			objects: allObjects,
			opts:    ClusterReaderOptions{Namespaces: []string{"production", "staging"}, IngressName: "ing-prod"},
			wantErr: "IngressName requires exactly one namespace, got 2",
		},
		{
			name:          "given ingress-name and namespace do not return services from other namespaces",
			objects:       allObjects,
			opts:          ClusterReaderOptions{Namespaces: []string{"production"}, IngressName: "ing-prod"},
			wantIngresses: []string{"ing-prod"},
			wantServices:  []string{"svc-prod"},
		},
		{
			name:          "empty cluster returns empty results",
			objects:       nil,
			opts:          ClusterReaderOptions{AllNamespaces: true},
			wantIngresses: nil,
			wantServices:  nil,
		},
		{
			name:             "ingressclass always returned regardless of namespace filter",
			objects:          allObjects,
			opts:             ClusterReaderOptions{Namespaces: []string{"production"}},
			wantIngresses:    []string{"ing-prod"},
			wantServices:     []string{"svc-prod"},
			wantIngressClass: []string{"alb"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, obj := range tt.objects {
				builder = builder.WithRuntimeObjects(obj)
			}
			k8sClient := builder.Build()

			resources, err := readFromClient(context.Background(), k8sClient, nil, tt.opts)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)

			var gotIngresses []string
			for _, ing := range resources.Ingresses {
				gotIngresses = append(gotIngresses, ing.Name)
			}
			var gotServices []string
			for _, svc := range resources.Services {
				gotServices = append(gotServices, svc.Name)
			}
			var gotIngressClasses []string
			for _, ic := range resources.IngressClasses {
				gotIngressClasses = append(gotIngressClasses, ic.Name)
			}

			assert.ElementsMatch(t, tt.wantIngresses, gotIngresses, "ingresses")
			assert.ElementsMatch(t, tt.wantServices, gotServices, "services")
			if tt.wantIngressClass != nil {
				assert.ElementsMatch(t, tt.wantIngressClass, gotIngressClasses, "ingressclasses")
			}
		})
	}
}
