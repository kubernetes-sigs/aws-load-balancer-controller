package warnings

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/reader"
)

func TestCheckMissingResources(t *testing.T) {
	tests := []struct {
		name         string
		resources    *ingress2gateway.InputResources
		wantCount    int
		wantContains []string
		wantEmpty    bool
	}{
		{
			name: "no warnings when all resources present",
			resources: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
						Spec: networking.IngressSpec{
							IngressClassName: ptr.To("alb"),
							Rules: []networking.IngressRule{{
								IngressRuleValue: networking.IngressRuleValue{
									HTTP: &networking.HTTPIngressRuleValue{
										Paths: []networking.HTTPIngressPath{{
											Backend: networking.IngressBackend{
												Service: &networking.IngressServiceBackend{Name: "my-svc"},
											},
										}},
									},
								},
							}},
						},
					},
				},
				Services:       []corev1.Service{{ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "default"}}},
				IngressClasses: []networking.IngressClass{{ObjectMeta: metav1.ObjectMeta{Name: "alb"}}},
			},
			wantCount: 0,
			wantEmpty: true,
		},
		{
			name: "missing service in rule backend",
			resources: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
					Spec: networking.IngressSpec{
						Rules: []networking.IngressRule{{
							IngressRuleValue: networking.IngressRuleValue{
								HTTP: &networking.HTTPIngressRuleValue{
									Paths: []networking.HTTPIngressPath{{
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{Name: "missing-svc"},
										},
									}},
								},
							},
						}},
					},
				}},
			},
			wantCount:    1,
			wantContains: []string{"missing-svc", "WARNING"},
		},
		{
			name: "missing ingress class",
			resources: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
					Spec:       networking.IngressSpec{IngressClassName: ptr.To("missing-class")},
				}},
			},
			wantCount:    1,
			wantContains: []string{"missing-class", "IngressClass"},
		},
		{
			name: "missing default backend service",
			resources: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
					Spec: networking.IngressSpec{
						DefaultBackend: &networking.IngressBackend{
							Service: &networking.IngressServiceBackend{Name: "default-backend-svc"},
						},
					},
				}},
			},
			wantCount:    1,
			wantContains: []string{"default-backend-svc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			count := CheckMissingResources(tt.resources, &buf)
			assert.Equal(t, tt.wantCount, count)
			if tt.wantEmpty {
				assert.Empty(t, buf.String())
			}
			for _, s := range tt.wantContains {
				assert.Contains(t, buf.String(), s)
			}
		})
	}
}

func TestCheckMissingResources_FromFile(t *testing.T) {
	resources, err := reader.ReadFromFiles([]string{"../utils/test_files/ingress_missing_service.yaml"})
	require.NoError(t, err)
	require.Len(t, resources.Ingresses, 1)

	var buf bytes.Buffer
	count := CheckMissingResources(resources, &buf)
	assert.Equal(t, 2, count)
	assert.Contains(t, buf.String(), "service-2048")
	assert.Contains(t, buf.String(), "Tip: Use --from-cluster")
}
