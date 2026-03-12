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

func TestCheckMissingResources_NoWarnings(t *testing.T) {
	resources := &ingress2gateway.InputResources{
		Ingresses: []networking.Ingress{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: networking.IngressSpec{
					IngressClassName: ptr.To("alb"),
					Rules: []networking.IngressRule{
						{
							Host: "example.com",
							IngressRuleValue: networking.IngressRuleValue{
								HTTP: &networking.HTTPIngressRuleValue{
									Paths: []networking.HTTPIngressPath{
										{
											Backend: networking.IngressBackend{
												Service: &networking.IngressServiceBackend{
													Name: "my-svc",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Services: []corev1.Service{
			{ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "default"}},
		},
		IngressClasses: []networking.IngressClass{
			{ObjectMeta: metav1.ObjectMeta{Name: "alb"}},
		},
	}

	var buf bytes.Buffer
	count := CheckMissingResources(resources, &buf)
	assert.Equal(t, 0, count)
	assert.Empty(t, buf.String())
}

func TestCheckMissingResources_MissingService(t *testing.T) {
	resources := &ingress2gateway.InputResources{
		Ingresses: []networking.Ingress{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: networking.IngressSpec{
					Rules: []networking.IngressRule{
						{
							IngressRuleValue: networking.IngressRuleValue{
								HTTP: &networking.HTTPIngressRuleValue{
									Paths: []networking.HTTPIngressPath{
										{
											Backend: networking.IngressBackend{
												Service: &networking.IngressServiceBackend{
													Name: "missing-svc",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	count := CheckMissingResources(resources, &buf)
	assert.Equal(t, 1, count)
	assert.Contains(t, buf.String(), "missing-svc")
	assert.Contains(t, buf.String(), "WARNING")
}

func TestCheckMissingResources_MissingIngressClass(t *testing.T) {
	resources := &ingress2gateway.InputResources{
		Ingresses: []networking.Ingress{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: networking.IngressSpec{
					IngressClassName: ptr.To("missing-class"),
				},
			},
		},
	}

	var buf bytes.Buffer
	count := CheckMissingResources(resources, &buf)
	assert.Equal(t, 1, count)
	assert.Contains(t, buf.String(), "missing-class")
	assert.Contains(t, buf.String(), "IngressClass")
}

func TestCheckMissingResources_MissingDefaultBackendService(t *testing.T) {
	resources := &ingress2gateway.InputResources{
		Ingresses: []networking.Ingress{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: networking.IngressSpec{
					DefaultBackend: &networking.IngressBackend{
						Service: &networking.IngressServiceBackend{
							Name: "default-backend-svc",
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	count := CheckMissingResources(resources, &buf)
	assert.Equal(t, 1, count)
	assert.Contains(t, buf.String(), "default-backend-svc")
}

func TestCheckMissingResources_FromFile(t *testing.T) {
	// End-to-end: read a file with an Ingress referencing a missing Service,
	// then verify the warning is emitted correctly.
	resources, err := reader.ReadFromFiles([]string{"../utils/test_files/ingress_missing_service.yaml"})
	require.NoError(t, err)
	require.Len(t, resources.Ingresses, 1)

	var buf bytes.Buffer
	count := CheckMissingResources(resources, &buf)
	assert.Equal(t, 2, count) // 1 missing Service + 1 missing IngressClass
	assert.Contains(t, buf.String(), "service-2048")
	assert.Contains(t, buf.String(), "Service-level annotation overrides may be missing")
	assert.Contains(t, buf.String(), `IngressClass "alb"`)
	assert.Contains(t, buf.String(), "Tip: Use --from-cluster")
}
