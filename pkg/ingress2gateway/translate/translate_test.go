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
			name: "IngressClassParams creates LBConfig when no LB annotations present",
			input: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{{
					ObjectMeta: metav1.ObjectMeta{
						Name: "no-annos", Namespace: "default",
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
				require.NotNil(t, out.LoadBalancerConfigurations[0].Spec.Scheme)
				assert.Equal(t, gatewayv1beta1.LoadBalancerSchemeInternetFacing, *out.LoadBalancerConfigurations[0].Spec.Scheme)
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
		{
			name: "cognito auth annotations produce LRC with authenticate-cognito action",
			input: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{{
					ObjectMeta: metav1.ObjectMeta{
						Name: "auth-app", Namespace: "prod",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/auth-type":                       "cognito",
							"alb.ingress.kubernetes.io/auth-idp-cognito":                `{"userPoolARN":"arn:aws:cognito-idp:us-west-2:123456789:userpool/pool-id","userPoolClientID":"client-id","userPoolDomain":"my-domain"}`,
							"alb.ingress.kubernetes.io/auth-scope":                      "email openid",
							"alb.ingress.kubernetes.io/auth-on-unauthenticated-request": "deny",
							"alb.ingress.kubernetes.io/auth-session-cookie":             "my-cookie",
							"alb.ingress.kubernetes.io/auth-session-timeout":            "3600",
							"alb.ingress.kubernetes.io/certificate-arn":                 "arn:aws:acm:us-west-2:123456789:certificate/cert-id",
						},
					},
					Spec: networking.IngressSpec{
						Rules: []networking.IngressRule{{
							Host: "app.example.com",
							IngressRuleValue: networking.IngressRuleValue{
								HTTP: &networking.HTTPIngressRuleValue{
									Paths: []networking.HTTPIngressPath{{
										Path: "/", PathType: &pathPrefix,
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "web-svc", Port: networking.ServiceBackendPort{Number: 443},
											},
										},
									}},
								},
							},
						}},
					},
				}},
			},
			wantGatewayCount: 1, wantHTTPRouteCount: 1, wantLBConfigCount: 1, wantTGConfigCount: 0,
			check: func(t *testing.T, out *ingress2gateway.OutputResources) {
				// Should produce exactly one LRC
				require.Len(t, out.ListenerRuleConfigurations, 1)
				lrc := out.ListenerRuleConfigurations[0]
				require.Len(t, lrc.Spec.Actions, 1)

				action := lrc.Spec.Actions[0]
				assert.Equal(t, gatewayv1beta1.ActionTypeAuthenticateCognito, action.Type)
				require.NotNil(t, action.AuthenticateCognitoConfig)

				cfg := action.AuthenticateCognitoConfig
				assert.Equal(t, "arn:aws:cognito-idp:us-west-2:123456789:userpool/pool-id", cfg.UserPoolArn)
				assert.Equal(t, "client-id", cfg.UserPoolClientID)
				assert.Equal(t, "my-domain", cfg.UserPoolDomain)
				require.NotNil(t, cfg.Scope)
				assert.Equal(t, "email openid", *cfg.Scope)
				require.NotNil(t, cfg.OnUnauthenticatedRequest)
				assert.Equal(t, gatewayv1beta1.AuthenticateCognitoActionConditionalBehaviorEnumDeny, *cfg.OnUnauthenticatedRequest)
				require.NotNil(t, cfg.SessionCookieName)
				assert.Equal(t, "my-cookie", *cfg.SessionCookieName)
				require.NotNil(t, cfg.SessionTimeout)
				assert.Equal(t, int64(3600), *cfg.SessionTimeout)

				// HTTPRoute rule should have ExtensionRef filter pointing to the LRC
				require.Len(t, out.HTTPRoutes[0].Spec.Rules, 1)
				rule := out.HTTPRoutes[0].Spec.Rules[0]
				found := false
				for _, f := range rule.Filters {
					if f.Type == gwv1.HTTPRouteFilterExtensionRef && f.ExtensionRef != nil && string(f.ExtensionRef.Name) == lrc.Name {
						found = true
					}
				}
				assert.True(t, found, "HTTPRoute rule should have ExtensionRef filter for LRC %s", lrc.Name)
			},
		},
		{
			name: "oidc auth annotations produce LRC with authenticate-oidc action and secret preserved",
			input: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{{
					ObjectMeta: metav1.ObjectMeta{
						Name: "oidc-app", Namespace: "prod",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/auth-type":                       "oidc",
							"alb.ingress.kubernetes.io/auth-idp-oidc":                   `{"issuer":"https://example.com","authorizationEndpoint":"https://auth.example.com","tokenEndpoint":"https://token.example.com","userInfoEndpoint":"https://userinfo.example.com","secretName":"my-oidc-secret"}`,
							"alb.ingress.kubernetes.io/auth-scope":                      "email openid",
							"alb.ingress.kubernetes.io/auth-on-unauthenticated-request": "allow",
							"alb.ingress.kubernetes.io/auth-session-cookie":             "oidc-cookie",
							"alb.ingress.kubernetes.io/auth-session-timeout":            "1800",
							"alb.ingress.kubernetes.io/certificate-arn":                 "arn:aws:acm:us-west-2:123456789:certificate/cert-id",
						},
					},
					Spec: networking.IngressSpec{
						Rules: []networking.IngressRule{{
							Host: "oidc.example.com",
							IngressRuleValue: networking.IngressRuleValue{
								HTTP: &networking.HTTPIngressRuleValue{
									Paths: []networking.HTTPIngressPath{{
										Path: "/", PathType: &pathPrefix,
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "oidc-svc", Port: networking.ServiceBackendPort{Number: 443},
											},
										},
									}},
								},
							},
						}},
					},
				}},
			},
			wantGatewayCount: 1, wantHTTPRouteCount: 1, wantLBConfigCount: 1, wantTGConfigCount: 0,
			check: func(t *testing.T, out *ingress2gateway.OutputResources) {
				require.Len(t, out.ListenerRuleConfigurations, 1)
				lrc := out.ListenerRuleConfigurations[0]
				require.Len(t, lrc.Spec.Actions, 1)

				action := lrc.Spec.Actions[0]
				assert.Equal(t, gatewayv1beta1.ActionTypeAuthenticateOIDC, action.Type)
				require.NotNil(t, action.AuthenticateOIDCConfig)

				cfg := action.AuthenticateOIDCConfig
				assert.Equal(t, "https://example.com", cfg.Issuer)
				assert.Equal(t, "https://auth.example.com", cfg.AuthorizationEndpoint)
				assert.Equal(t, "https://token.example.com", cfg.TokenEndpoint)
				assert.Equal(t, "https://userinfo.example.com", cfg.UserInfoEndpoint)

				// Secret reference preserved
				require.NotNil(t, cfg.Secret)
				assert.Equal(t, "my-oidc-secret", cfg.Secret.Name)

				require.NotNil(t, cfg.Scope)
				assert.Equal(t, "email openid", *cfg.Scope)
				require.NotNil(t, cfg.OnUnauthenticatedRequest)
				assert.Equal(t, gatewayv1beta1.AuthenticateOidcActionConditionalBehaviorEnumAllow, *cfg.OnUnauthenticatedRequest)
				require.NotNil(t, cfg.SessionCookieName)
				assert.Equal(t, "oidc-cookie", *cfg.SessionCookieName)
				require.NotNil(t, cfg.SessionTimeout)
				assert.Equal(t, int64(1800), *cfg.SessionTimeout)

				// HTTPRoute rule should have ExtensionRef filter
				require.Len(t, out.HTTPRoutes[0].Spec.Rules, 1)
				rule := out.HTTPRoutes[0].Spec.Rules[0]
				found := false
				for _, f := range rule.Filters {
					if f.Type == gwv1.HTTPRouteFilterExtensionRef && f.ExtensionRef != nil && string(f.ExtensionRef.Name) == lrc.Name {
						found = true
					}
				}
				assert.True(t, found, "HTTPRoute rule should have ExtensionRef filter for LRC %s", lrc.Name)
			},
		},
		{
			name: "grouped ingresses produce one shared Gateway",
			input: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "api", Namespace: "demo",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/group.name": "shared-alb",
								"alb.ingress.kubernetes.io/scheme":     "internet-facing",
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
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "web", Namespace: "demo",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/group.name": "shared-alb",
								"alb.ingress.kubernetes.io/scheme":     "internet-facing",
							},
						},
						Spec: networking.IngressSpec{
							Rules: []networking.IngressRule{{
								Host: "web.example.com",
								IngressRuleValue: networking.IngressRuleValue{
									HTTP: &networking.HTTPIngressRuleValue{
										Paths: []networking.HTTPIngressPath{{
											Path: "/", PathType: &pathPrefix,
											Backend: networking.IngressBackend{
												Service: &networking.IngressServiceBackend{
													Name: "web-svc", Port: networking.ServiceBackendPort{Number: 80},
												},
											},
										}},
									},
								},
							}},
						},
					},
				},
			},
			wantGatewayCount: 1, wantHTTPRouteCount: 2, wantLBConfigCount: 1, wantTGConfigCount: 0,
			check: func(t *testing.T, out *ingress2gateway.OutputResources) {
				// One shared Gateway
				assert.Contains(t, out.Gateways[0].Name, "grp-gw")
				// Two separate HTTPRoutes
				assert.Len(t, out.HTTPRoutes[0].Spec.Hostnames, 1)
				assert.Len(t, out.HTTPRoutes[1].Spec.Hostnames, 1)
			},
		},
		{
			name: "cross-namespace group gets allowedRoutes All",
			input: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "api", Namespace: "team-a",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/group.name": "cross-ns",
							},
						},
						Spec: networking.IngressSpec{
							Rules: []networking.IngressRule{{
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
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "web", Namespace: "team-b",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/group.name": "cross-ns",
							},
						},
						Spec: networking.IngressSpec{
							Rules: []networking.IngressRule{{
								IngressRuleValue: networking.IngressRuleValue{
									HTTP: &networking.HTTPIngressRuleValue{
										Paths: []networking.HTTPIngressPath{{
											Path: "/web", PathType: &pathPrefix,
											Backend: networking.IngressBackend{
												Service: &networking.IngressServiceBackend{
													Name: "web-svc", Port: networking.ServiceBackendPort{Number: 80},
												},
											},
										}},
									},
								},
							}},
						},
					},
				},
			},
			wantGatewayCount: 1, wantHTTPRouteCount: 2, wantLBConfigCount: 0, wantTGConfigCount: 0,
			check: func(t *testing.T, out *ingress2gateway.OutputResources) {
				gw := out.Gateways[0]
				// Gateway should have allowedRoutes with From: All
				require.Len(t, gw.Spec.Listeners, 1)
				require.NotNil(t, gw.Spec.Listeners[0].AllowedRoutes)
				require.NotNil(t, gw.Spec.Listeners[0].AllowedRoutes.Namespaces)
				assert.Equal(t, gwv1.NamespacesFromAll, *gw.Spec.Listeners[0].AllowedRoutes.Namespaces.From)
				assert.Nil(t, gw.Spec.Listeners[0].AllowedRoutes.Namespaces.Selector)

				// Cross-namespace HTTPRoute should have namespace in parentRef
				for _, route := range out.HTTPRoutes {
					if route.Namespace != gw.Namespace {
						require.NotNil(t, route.Spec.ParentRefs[0].Namespace)
						assert.Equal(t, gwv1.Namespace(gw.Namespace), *route.Spec.ParentRefs[0].Namespace)
					}
				}
			},
		},
		{
			name: "conflicting scheme in group errors",
			input: &ingress2gateway.InputResources{
				Ingresses: []networking.Ingress{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "a", Namespace: "ns",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/group.name": "conflict",
								"alb.ingress.kubernetes.io/scheme":     "internal",
							},
						},
						Spec: networking.IngressSpec{DefaultBackend: &networking.IngressBackend{
							Service: &networking.IngressServiceBackend{Name: "svc", Port: networking.ServiceBackendPort{Number: 80}},
						}},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "b", Namespace: "ns",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/group.name": "conflict",
								"alb.ingress.kubernetes.io/scheme":     "internet-facing",
							},
						},
						Spec: networking.IngressSpec{DefaultBackend: &networking.IngressBackend{
							Service: &networking.IngressServiceBackend{Name: "svc", Port: networking.ServiceBackendPort{Number: 80}},
						}},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Normalize namespaces like Migrate() does
			tt.input.NormalizeNamespaces()
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
