package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

func TestResolveWatchedNamespaces(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	namespaces := []runtime.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ingress"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   "dev-alice",
			Labels: map[string]string{"tenant": "true"},
		}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   "dev-bob",
			Labels: map[string]string{"tenant": "true"},
		}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   "legacy-sandbox",
			Labels: map[string]string{"tenant": "false"},
		}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   "team-platform",
			Labels: map[string]string{"team": "platform"},
		}},
	}
	clientset := fake.NewSimpleClientset(namespaces...)

	tests := []struct {
		name    string
		cfg     RuntimeConfig
		want    []string
		wantErr string
	}{
		{
			name: "all namespaces when neither flag is set",
			cfg:  RuntimeConfig{},
			want: nil,
		},
		{
			name: "watch-namespace returns single namespace",
			cfg:  RuntimeConfig{WatchNamespace: "ingress"},
			want: []string{"ingress"},
		},
		{
			name: "namespace-selector !tenant matches namespaces without the tenant key",
			cfg:  RuntimeConfig{NamespaceSelector: "!tenant"},
			want: []string{"kube-system", "ingress", "team-platform"},
		},
		{
			name: "namespace-selector tenant!=true matches missing key and other values",
			cfg:  RuntimeConfig{NamespaceSelector: "tenant!=true"},
			want: []string{"kube-system", "ingress", "legacy-sandbox", "team-platform"},
		},
		{
			name: "namespace-selector equality",
			cfg:  RuntimeConfig{NamespaceSelector: "team=platform"},
			want: []string{"team-platform"},
		},
		{
			name:    "mutually exclusive flags",
			cfg:     RuntimeConfig{WatchNamespace: "default", NamespaceSelector: "!tenant"},
			wantErr: "mutually exclusive",
		},
		{
			name:    "invalid selector",
			cfg:     RuntimeConfig{NamespaceSelector: "tenant!!!"},
			wantErr: "invalid --namespace-selector",
		},
		{
			name:    "selector matching nothing",
			cfg:     RuntimeConfig{NamespaceSelector: "doesnotexist=true"},
			wantErr: "matched no namespaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveWatchedNamespaces(context.Background(), clientset, tt.cfg)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestBuildRuntimeOptions_NamespaceSelectorScopesCache(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	rtCfg := RuntimeConfig{
		MetricsBindAddress:     "0",
		HealthProbeBindAddress: "0",
		SyncPeriod:             defaultSyncPeriod,
	}
	opts, err := BuildRuntimeOptions(rtCfg, scheme, []string{"ingress", "kube-system"})
	require.NoError(t, err)
	require.Equal(t, map[string]cache.Config{
		"ingress":     {},
		"kube-system": {},
	}, opts.Cache.DefaultNamespaces)
}

func TestBuildRuntimeOptions_AllNamespacesLeavesDefaultNamespacesUnset(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	rtCfg := RuntimeConfig{
		MetricsBindAddress:     "0",
		HealthProbeBindAddress: "0",
		SyncPeriod:             defaultSyncPeriod,
	}
	opts, err := BuildRuntimeOptions(rtCfg, scheme, nil)
	require.NoError(t, err)
	assert.Nil(t, opts.Cache.DefaultNamespaces)
}
