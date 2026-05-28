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

func Test_clearDryRunPlanAnnotation(t *testing.T) {
	tests := []struct {
		name           string
		ingAnnotations map[string]string
	}{
		{
			name: "removes annotation when present",
			ingAnnotations: map[string]string{
				dryRunPlanAnnotation:               `{"id":"test-stack"}`,
				"alb.ingress.kubernetes.io/scheme": "internet-facing",
			},
		},
		{
			name: "no-op when annotation absent",
			ingAnnotations: map[string]string{
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

			err := clearDryRunPlanAnnotation(context.Background(), k8sClient, ing)
			require.NoError(t, err)

			updatedIng := &networking.Ingress{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      "test-ingress",
				Namespace: "default",
			}, updatedIng)
			require.NoError(t, err)
			_, hasPlan := updatedIng.Annotations[dryRunPlanAnnotation]
			assert.False(t, hasPlan, "dry-run-plan annotation should be absent after clear")
			assert.Equal(t, "internet-facing", updatedIng.Annotations["alb.ingress.kubernetes.io/scheme"])
		})
	}
}

func Test_clearDryRunPlanAnnotation_standaloneIngress(t *testing.T) {
	ing := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "standalone",
			Namespace: "default",
			Annotations: map[string]string{
				dryRunPlanAnnotation:               `{"id":"old-plan"}`,
				"alb.ingress.kubernetes.io/scheme": "internal",
			},
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ing).
		Build()

	err := clearDryRunPlanAnnotation(context.Background(), k8sClient, ing)
	require.NoError(t, err)

	updatedIng := &networking.Ingress{}
	err = k8sClient.Get(context.Background(), types.NamespacedName{
		Name:      "standalone",
		Namespace: "default",
	}, updatedIng)
	require.NoError(t, err)
	_, hasPlan := updatedIng.Annotations[dryRunPlanAnnotation]
	assert.False(t, hasPlan, "dry-run-plan annotation should be removed from standalone ingress")
	assert.Equal(t, "internal", updatedIng.Annotations["alb.ingress.kubernetes.io/scheme"])
}

func Test_clearDryRunPlanAnnotation_multiMemberGroup(t *testing.T) {
	primary := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "member-1",
			Namespace: "default",
			Annotations: map[string]string{
				dryRunPlanAnnotation:                    `{"id":"group-plan"}`,
				"alb.ingress.kubernetes.io/group.name":  "my-group",
				"alb.ingress.kubernetes.io/group.order": "1",
			},
		},
	}
	secondary := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "member-2",
			Namespace: "default",
			Annotations: map[string]string{
				"alb.ingress.kubernetes.io/group.name":  "my-group",
				"alb.ingress.kubernetes.io/group.order": "2",
			},
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(primary, secondary).
		Build()

	for _, ing := range []*networking.Ingress{primary, secondary} {
		err := clearDryRunPlanAnnotation(context.Background(), k8sClient, ing)
		require.NoError(t, err)
	}

	updatedPrimary := &networking.Ingress{}
	err := k8sClient.Get(context.Background(), types.NamespacedName{Name: "member-1", Namespace: "default"}, updatedPrimary)
	require.NoError(t, err)
	_, hasPlan := updatedPrimary.Annotations[dryRunPlanAnnotation]
	assert.False(t, hasPlan, "primary should have dry-run-plan removed")
	assert.Equal(t, "my-group", updatedPrimary.Annotations["alb.ingress.kubernetes.io/group.name"])

	updatedSecondary := &networking.Ingress{}
	err = k8sClient.Get(context.Background(), types.NamespacedName{Name: "member-2", Namespace: "default"}, updatedSecondary)
	require.NoError(t, err)
	_, hasPlan = updatedSecondary.Annotations[dryRunPlanAnnotation]
	assert.False(t, hasPlan, "secondary should remain without dry-run-plan")
}

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
