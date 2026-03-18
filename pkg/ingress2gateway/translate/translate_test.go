package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	gwconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway"
	constants "sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// TestTranslate tests the end-to-end orchestration: InputResources → OutputResources.
func TestTranslate(t *testing.T) {
	pathPrefix := networking.PathTypePrefix

	tests := []struct {
		name               string
		input              *ingress2gateway.InputResources
		wantErr            bool
		wantGatewayCount   int
		wantHTTPRouteCount int
		wantLBConfigCount  int
		wantTGConfigCount  int
		check              func(t *testing.T, out *ingress2gateway.OutputResources)
	}{
		{
			name: "basic ingress produces all resource types",
			input: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-app", Namespace: "production",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/scheme":      "internet-facing",
							"alb.ingress.kubernetes.io/target-type": "ip",
						},
					},
					Spec: networking.IngressSpec{
						Rules: []networking.IngressRule{{
							Host: "api.example.com",
							IngressRuleValue: networking.IngressRuleValue{
								HTTP: &networking.HTTPIngressRuleValue{
									Paths: []networking.HTTPIngressPath{{
										Path: "/api", PathType: &pathPrefix,
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "api-svc", Port: networking.ServiceBackendPort{Number: 8080},
											},
										},
									}},
								},
							},
						}},
					},
				}},
			},
			wantGatewayCount: 1, wantHTTPRouteCount: 1, wantLBConfigCount: 1, wantTGConfigCount: 1,
			check: func(t *testing.T, out *ingress2gateway.OutputResources) {
				// GatewayClass
				assert.Equal(t, constants.GatewayClassName, out.GatewayClass.Name)
				assert.Equal(t, gwv1.GatewayController(gwconstants.ALBGatewayController), out.GatewayClass.Spec.ControllerName)
				// Gateway references LB config
				require.NotNil(t, out.Gateways[0].Spec.Infrastructure)
				// HTTPRoute has correct hostname
				assert.Len(t, out.HTTPRoutes[0].Spec.Hostnames, 1)
				// LB config has scheme
				require.NotNil(t, out.LoadBalancerConfigurations[0].Spec.Scheme)
				// TG config has target type
				require.NotNil(t, out.TargetGroupConfigurations[0].Spec.DefaultConfiguration.TargetType)
			},
		},
		{
			name: "IngressClassParams overrides scheme",
			input: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{{
					ObjectMeta: metav1.ObjectMeta{
						Name: "icp-app", Namespace: "default",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/scheme": "internal",
						},
					},
					Spec: networking.IngressSpec{
						IngressClassName: ptr.To("alb"),
						DefaultBackend: &networking.IngressBackend{
							Service: &networking.IngressServiceBackend{
								Name: "svc", Port: networking.ServiceBackendPort{Number: 80},
							},
						},
					},
				}},
				IngressClasses: []networking.IngressClass{{
					ObjectMeta: metav1.ObjectMeta{Name: "alb"},
					Spec: networking.IngressClassSpec{
						Parameters: &networking.IngressClassParametersReference{
							Kind: "IngressClassParams", Name: "alb-params",
						},
					},
				}},
				IngressClassParams: []elbv2api.IngressClassParams{{
					ObjectMeta: metav1.ObjectMeta{Name: "alb-params"},
					Spec: elbv2api.IngressClassParamsSpec{
						Scheme: ptr.To(elbv2api.LoadBalancerSchemeInternetFacing),
					},
				}},
			},
			wantGatewayCount: 1, wantHTTPRouteCount: 1, wantLBConfigCount: 1, wantTGConfigCount: 0,
			check: func(t *testing.T, out *ingress2gateway.OutputResources) {
				// ICP should override Ingress annotation
				require.NotNil(t, out.LoadBalancerConfigurations[0].Spec.Scheme)
				assert.Equal(t, gatewayv1beta1.LoadBalancerSchemeInternetFacing, *out.LoadBalancerConfigurations[0].Spec.Scheme)
			},
		},
		{
			name: "service annotations override ingress for TG fields",
			input: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{{
					ObjectMeta: metav1.ObjectMeta{
						Name: "svc-override", Namespace: "default",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/target-type": "instance",
						},
					},
					Spec: networking.IngressSpec{
						Rules: []networking.IngressRule{{
							IngressRuleValue: networking.IngressRuleValue{
								HTTP: &networking.HTTPIngressRuleValue{
									Paths: []networking.HTTPIngressPath{{
										Path: "/", PathType: &pathPrefix,
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "override-svc", Port: networking.ServiceBackendPort{Number: 80},
											},
										},
									}},
								},
							},
						}},
					},
				}},
				Services: []corev1.Service{{
					ObjectMeta: metav1.ObjectMeta{
						Name: "override-svc", Namespace: "default",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/target-type": "ip",
						},
					},
				}},
			},
			wantGatewayCount: 1, wantHTTPRouteCount: 1, wantLBConfigCount: 0, wantTGConfigCount: 1,
			check: func(t *testing.T, out *ingress2gateway.OutputResources) {
				// Service annotation should win
				require.NotNil(t, out.TargetGroupConfigurations[0].Spec.DefaultConfiguration.TargetType)
				assert.Equal(t, gatewayv1beta1.TargetTypeIP, *out.TargetGroupConfigurations[0].Spec.DefaultConfiguration.TargetType)
			},
		},
		{
			name: "no annotations produces minimal output",
			input: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{{
					ObjectMeta: metav1.ObjectMeta{Name: "bare", Namespace: "default"},
					Spec: networking.IngressSpec{
						Rules: []networking.IngressRule{{
							IngressRuleValue: networking.IngressRuleValue{
								HTTP: &networking.HTTPIngressRuleValue{
									Paths: []networking.HTTPIngressPath{{
										Path: "/", PathType: &pathPrefix,
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc", Port: networking.ServiceBackendPort{Number: 80},
											},
										},
									}},
								},
							},
						}},
					},
				}},
			},
			wantGatewayCount: 1, wantHTTPRouteCount: 1, wantLBConfigCount: 0, wantTGConfigCount: 0,
			check: func(t *testing.T, out *ingress2gateway.OutputResources) {
				// Gateway should have no parametersRef
				assert.Nil(t, out.Gateways[0].Spec.Infrastructure)
			},
		},
		{
			name: "defaultBackend with host rules produces separate catch-all route",
			input: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{{
					ObjectMeta: metav1.ObjectMeta{Name: "mixed", Namespace: "demo"},
					Spec: networking.IngressSpec{
						DefaultBackend: &networking.IngressBackend{
							Service: &networking.IngressServiceBackend{
								Name: "default-svc", Port: networking.ServiceBackendPort{Number: 80},
							},
						},
						Rules: []networking.IngressRule{{
							Host: "app.example.com",
							IngressRuleValue: networking.IngressRuleValue{
								HTTP: &networking.HTTPIngressRuleValue{
									Paths: []networking.HTTPIngressPath{{
										Path: "/api", PathType: &pathPrefix,
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "api-svc", Port: networking.ServiceBackendPort{Number: 80},
											},
										},
									}},
								},
							},
						}},
					},
				}},
			},
			wantGatewayCount: 1, wantHTTPRouteCount: 2, wantLBConfigCount: 0, wantTGConfigCount: 0,
			check: func(t *testing.T, out *ingress2gateway.OutputResources) {
				// Primary route has hostname
				assert.Contains(t, out.HTTPRoutes[0].Name, "mixed-route-")
				assert.Len(t, out.HTTPRoutes[0].Spec.Hostnames, 1)
				assert.Len(t, out.HTTPRoutes[0].Spec.Rules, 1)

				// Default route has no hostname — true catch-all
				assert.Contains(t, out.HTTPRoutes[1].Name, "mixed-default-")
				assert.Len(t, out.HTTPRoutes[1].Spec.Hostnames, 0)
				assert.Len(t, out.HTTPRoutes[1].Spec.Rules, 1)
				assert.Empty(t, out.HTTPRoutes[1].Spec.Rules[0].Matches)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Translate(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, out)

			assert.Len(t, out.Gateways, tt.wantGatewayCount)
			assert.Len(t, out.HTTPRoutes, tt.wantHTTPRouteCount)
			assert.Len(t, out.LoadBalancerConfigurations, tt.wantLBConfigCount)
			assert.Len(t, out.TargetGroupConfigurations, tt.wantTGConfigCount)

			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}
