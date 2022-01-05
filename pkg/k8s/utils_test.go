package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	rbac "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestNamespacedName(t *testing.T) {
	tests := []struct {
		name string
		obj  metav1.Object
		want types.NamespacedName
	}{
		{
			name: "cluster-scoped object",
			obj: &rbac.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ingress",
				},
			},
			want: types.NamespacedName{
				Namespace: "",
				Name:      "ingress",
			},
		},
		{
			name: "namespace-scoped object",
			obj: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
				},
			},
			want: types.NamespacedName{
				Namespace: "namespace",
				Name:      "ingress",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NamespacedName(tt.obj)
			assert.Equal(t, tt.want, got)
		})
	}
}
