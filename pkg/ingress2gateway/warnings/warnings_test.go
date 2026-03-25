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
		{
			name: "use-annotation backends should not flagged as missing services",
			resources: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
					Spec: networking.IngressSpec{
						Rules: []networking.IngressRule{{
							IngressRuleValue: networking.IngressRuleValue{
								HTTP: &networking.HTTPIngressRuleValue{
									Paths: []networking.HTTPIngressPath{{
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "my-action",
												Port: networking.ServiceBackendPort{Name: "use-annotation"},
											},
										},
									}},
								},
							},
						}},
					},
				}},
			},
			wantCount: 0,
			wantEmpty: true,
		},
		{
			name: "mix of use-annotation and real service — only real service warned",
			resources: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
					Spec: networking.IngressSpec{
						Rules: []networking.IngressRule{{
							IngressRuleValue: networking.IngressRuleValue{
								HTTP: &networking.HTTPIngressRuleValue{
									Paths: []networking.HTTPIngressPath{
										{
											Backend: networking.IngressBackend{
												Service: &networking.IngressServiceBackend{
													Name: "my-action",
													Port: networking.ServiceBackendPort{Name: "use-annotation"},
												},
											},
										},
										{
											Backend: networking.IngressBackend{
												Service: &networking.IngressServiceBackend{
													Name: "missing-real-svc",
													Port: networking.ServiceBackendPort{Number: 80},
												},
											},
										},
									},
								},
							},
						}},
					},
				}},
			},
			wantCount:    1,
			wantContains: []string{"missing-real-svc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			count := CheckInputResources(tt.resources, &buf)
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
	count := CheckInputResources(resources, &buf)
	assert.Equal(t, 2, count)
	assert.Contains(t, buf.String(), "service-2048")
	assert.Contains(t, buf.String(), "Tip: Use --from-cluster")
}

func TestCheckMissingResources_ExternalTargetGroupWarning(t *testing.T) {
	tests := []struct {
		name         string
		annotations  map[string]string
		wantContains string
		wantEmpty    bool
	}{
		{
			name: "action with targetGroupARN triggers warning",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/actions.forward-ext": `{"type":"forward","targetGroupARN":"arn:aws:elasticloadbalancing:us-west-2:123:targetgroup/my-tg/abc"}`,
			},
			wantContains: "external target groups",
		},
		{
			name: "action with targetGroupName triggers warning",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/actions.forward-ext": `{"type":"forward","targetGroupName":"my-tg"}`,
			},
			wantContains: "external target groups",
		},
		{
			name: "action with serviceName only — no warning",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/actions.forward-svc": `{"type":"forward","forwardConfig":{"targetGroups":[{"serviceName":"svc","servicePort":80}]}}`,
			},
			wantEmpty: true,
		},
		{
			name:        "no action annotations — no warning",
			annotations: map[string]string{},
			wantEmpty:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resources := &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default", Annotations: tt.annotations},
					Spec: networking.IngressSpec{
						Rules: []networking.IngressRule{{
							IngressRuleValue: networking.IngressRuleValue{
								HTTP: &networking.HTTPIngressRuleValue{
									Paths: []networking.HTTPIngressPath{{
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "forward-ext",
												Port: networking.ServiceBackendPort{Name: "use-annotation"},
											},
										},
									}},
								},
							},
						}},
					},
				}},
			}
			var buf bytes.Buffer
			CheckInputResources(resources, &buf)
			if tt.wantEmpty {
				assert.NotContains(t, buf.String(), "external target groups")
			} else {
				assert.Contains(t, buf.String(), tt.wantContains)
			}
		})
	}
}
