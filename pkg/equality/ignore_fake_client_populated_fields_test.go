package equality

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIgnoreFakeClientPopulatedFields(t *testing.T) {
	tests := []struct {
		name         string
		ingressLeft  *networking.Ingress
		ingressRight *networking.Ingress
		wantEquals   bool
	}{
		{
			name: "objects should be equal if only TypeMeta and ObjectMeta.ResourceVersion diffs",
			ingressLeft: &networking.Ingress{
				TypeMeta: v1.TypeMeta{
					Kind:       "ingress",
					APIVersion: "networking.k8s.io/v1beta1",
				},
				ObjectMeta: v1.ObjectMeta{
					ResourceVersion: "0",
					Annotations: map[string]string{
						"k": "v1",
					},
				},
			},
			ingressRight: &networking.Ingress{
				TypeMeta: v1.TypeMeta{
					Kind:       "ingress",
					APIVersion: "networking.k8s.io/v1",
				},
				ObjectMeta: v1.ObjectMeta{
					ResourceVersion: "1",
					Annotations: map[string]string{
						"k": "v1",
					},
				},
			},
			wantEquals: true,
		},
		{
			name: "objects shouldn't be equal if more fields than TypeMeta and ObjectMeta.ResourceVersion diffs",
			ingressLeft: &networking.Ingress{
				TypeMeta: v1.TypeMeta{
					Kind:       "ingress",
					APIVersion: "networking.k8s.io/v1beta1",
				},
				ObjectMeta: v1.ObjectMeta{
					ResourceVersion: "0",
					Annotations: map[string]string{
						"k": "v1",
					},
				},
			},
			ingressRight: &networking.Ingress{
				TypeMeta: v1.TypeMeta{
					Kind:       "ingress",
					APIVersion: "networking.k8s.io/v1",
				},
				ObjectMeta: v1.ObjectMeta{
					ResourceVersion: "1",
					Annotations: map[string]string{
						"k": "v2",
					},
				},
			},
			wantEquals: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := IgnoreFakeClientPopulatedFields()
			gotEquals := cmp.Equal(tt.ingressLeft, tt.ingressRight, opts)
			assert.Equal(t, tt.wantEquals, gotEquals)
		})
	}
}
