package ingress

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_patchDryRunPlanAnnotation(t *testing.T) {
	tests := []struct {
		name                 string
		ingAnnotations       map[string]string
		planJSON             string
		wantAnnotation       string
		wantOtherAnnotations map[string]string
	}{
		{
			name:           "writes annotation when not present",
			ingAnnotations: nil,
			planJSON:       `{"id":"test-stack"}`,
			wantAnnotation: `{"id":"test-stack"}`,
		},
		{
			name: "skips patch when value unchanged",
			ingAnnotations: map[string]string{
				dryRunPlanAnnotation: `{"id":"test-stack"}`,
			},
			planJSON:       `{"id":"test-stack"}`,
			wantAnnotation: `{"id":"test-stack"}`,
		},
		{
			name: "updates annotation when value changed",
			ingAnnotations: map[string]string{
				dryRunPlanAnnotation: `{"id":"old-stack"}`,
			},
			planJSON:       `{"id":"new-stack"}`,
			wantAnnotation: `{"id":"new-stack"}`,
		},
		{
			name: "preserves existing annotations",
			ingAnnotations: map[string]string{
				"alb.ingress.kubernetes.io/scheme": "internet-facing",
			},
			planJSON:       `{"id":"test-stack"}`,
			wantAnnotation: `{"id":"test-stack"}`,
			wantOtherAnnotations: map[string]string{
				"alb.ingress.kubernetes.io/scheme": "internet-facing",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "default",
					Annotations: tt.ingAnnotations,
				},
			}

			scheme := runtime.NewScheme()
			require.NoError(t, clientgoscheme.AddToScheme(scheme))

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ing).
				Build()

			err := patchDryRunPlanAnnotation(context.Background(), k8sClient, ing, tt.planJSON)
			require.NoError(t, err)

			// Verify the annotation was persisted
			updatedIng := &networking.Ingress{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      "test-ingress",
				Namespace: "default",
			}, updatedIng)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAnnotation, updatedIng.Annotations[dryRunPlanAnnotation])

			// Verify other annotations were not clobbered
			for k, v := range tt.wantOtherAnnotations {
				assert.Equal(t, v, updatedIng.Annotations[k], "annotation %s should be preserved", k)
			}
		})
	}
}
