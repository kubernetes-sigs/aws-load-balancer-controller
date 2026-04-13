package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestBuildHTTPRoutes(t *testing.T) {
	pathPrefix := networking.PathTypePrefix
	pathExact := networking.PathTypeExact

	tests := []struct {
		name            string
		ing             networking.Ingress
		namespace       string
		gwName          string
		ports           []listenPortEntry
		sslRedirectPort *int32
		wantRoutes      int
		wantErr         string
		check           func(t *testing.T, routes []gwv1.HTTPRoute)
		checkLRCs       func(t *testing.T, lrcs []gatewayv1beta1.ListenerRuleConfiguration)
	}{
		{
			name: "single rule with host and prefix path",
			ing: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{Name: "app"},
				Spec: networking.IngressSpec{
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
			},
			namespace: "default", gwName: "app-gw",
			ports:      []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			wantRoutes: 1,
			check: func(t *testing.T, routes []gwv1.HTTPRoute) {
				r := routes[0]
				assert.Contains(t, r.Name, "app-route-")
				assert.Len(t, r.Spec.Rules, 1)
				assert.Len(t, r.Spec.Hostnames, 1)
				assert.Len(t, r.Spec.ParentRefs, 1)
				assert.Equal(t, gwv1.PathMatchPathPrefix, *r.Spec.Rules[0].Matches[0].Path.Type)
				assert.Equal(t, "/api", *r.Spec.Rules[0].Matches[0].Path.Value)
				assert.Equal(t, gwv1.ObjectName("api-svc"), r.Spec.Rules[0].BackendRefs[0].Name)
			},
		},
		{
			name: "exact path type",
			ing: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{Name: "exact"},
				Spec: networking.IngressSpec{
					Rules: []networking.IngressRule{{
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{{
									Path: "/exact", PathType: &pathExact,
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
			},
			namespace: "default", gwName: "gw",
			ports:      []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			wantRoutes: 1,
			check: func(t *testing.T, routes []gwv1.HTTPRoute) {
				r := routes[0]
				assert.Len(t, r.Spec.Rules, 1)
				assert.Len(t, r.Spec.Hostnames, 0)
				assert.Equal(t, gwv1.PathMatchExact, *r.Spec.Rules[0].Matches[0].Path.Type)
			},
		},
		{
			name: "default backend only (no hostnames) — single route with catch-all",
			ing: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{Name: "def"},
				Spec: networking.IngressSpec{
					DefaultBackend: &networking.IngressBackend{
						Service: &networking.IngressServiceBackend{
							Name: "default-svc", Port: networking.ServiceBackendPort{Number: 80},
						},
					},
				},
			},
			namespace: "ns", gwName: "gw",
			ports:      []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			wantRoutes: 1,
			check: func(t *testing.T, routes []gwv1.HTTPRoute) {
				r := routes[0]
				assert.Contains(t, r.Name, "def-route-")
				assert.Len(t, r.Spec.Hostnames, 0)
				assert.Len(t, r.Spec.Rules, 1)
				assert.Equal(t, gwv1.ObjectName("default-svc"), r.Spec.Rules[0].BackendRefs[0].Name)
				assert.Empty(t, r.Spec.Rules[0].Matches) // no matches = catch-all
			},
		},
		{
			name: "default backend with host rules — separate route for catch-all",
			ing: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{Name: "mixed"},
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
			},
			namespace: "default", gwName: "gw",
			ports:      []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			wantRoutes: 2,
			check: func(t *testing.T, routes []gwv1.HTTPRoute) {
				// Primary route has host rules with hostname
				primary := routes[0]
				assert.Contains(t, primary.Name, "mixed-route-")
				assert.Len(t, primary.Spec.Hostnames, 1)
				assert.Equal(t, gwv1.Hostname("app.example.com"), primary.Spec.Hostnames[0])
				assert.Len(t, primary.Spec.Rules, 1)
				assert.Equal(t, gwv1.ObjectName("api-svc"), primary.Spec.Rules[0].BackendRefs[0].Name)

				// Default route has no hostnames — true catch-all
				defaultRoute := routes[1]
				assert.Contains(t, defaultRoute.Name, "mixed-default-")
				assert.Len(t, defaultRoute.Spec.Hostnames, 0)
				assert.Len(t, defaultRoute.Spec.Rules, 1)
				assert.Equal(t, gwv1.ObjectName("default-svc"), defaultRoute.Spec.Rules[0].BackendRefs[0].Name)
				assert.Empty(t, defaultRoute.Spec.Rules[0].Matches)
			},
		},
		{
			name: "default backend with no-host rules — single route",
			ing: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{Name: "nohost"},
				Spec: networking.IngressSpec{
					DefaultBackend: &networking.IngressBackend{
						Service: &networking.IngressServiceBackend{
							Name: "default-svc", Port: networking.ServiceBackendPort{Number: 80},
						},
					},
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
			namespace: "default", gwName: "gw",
			ports:      []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			wantRoutes: 1,
			check: func(t *testing.T, routes []gwv1.HTTPRoute) {
				r := routes[0]
				assert.Len(t, r.Spec.Hostnames, 0)
				assert.Len(t, r.Spec.Rules, 2) // path rule + default backend in same route
			},
		},
		{
			name: "multiple listeners produce single parentRef without sectionName",
			ing: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{Name: "multi"},
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
			},
			namespace: "default", gwName: "gw",
			ports: []listenPortEntry{
				{Protocol: "HTTP", Port: 80},
				{Protocol: "HTTPS", Port: 443},
			},
			wantRoutes: 1,
			check: func(t *testing.T, routes []gwv1.HTTPRoute) {
				assert.Len(t, routes[0].Spec.ParentRefs, 1)
				assert.Nil(t, routes[0].Spec.ParentRefs[0].SectionName)
				assert.Len(t, routes[0].Spec.Hostnames, 0)
				assert.Len(t, routes[0].Spec.Rules, 1)
			},
		},
		{
			name: "tls hosts deduplicated with rule hosts",
			ing: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{Name: "tls"},
				Spec: networking.IngressSpec{
					TLS: []networking.IngressTLS{{Hosts: []string{"app.example.com"}}},
					Rules: []networking.IngressRule{{
						Host: "app.example.com",
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
			},
			namespace: "default", gwName: "gw",
			ports:      []listenPortEntry{{Protocol: "HTTPS", Port: 443}},
			wantRoutes: 1,
			check: func(t *testing.T, routes []gwv1.HTTPRoute) {
				assert.Len(t, routes[0].Spec.Hostnames, 1) // deduplicated
				assert.Len(t, routes[0].Spec.ParentRefs, 1)
				assert.Len(t, routes[0].Spec.Rules, 1)
			},
		},
		{
			name: "host-header condition splits rule into separate HTTPRoute",
			ing: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ingress",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/actions.rule-path1":    `{"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"200","messageBody":"host test"}}`,
						"alb.ingress.kubernetes.io/conditions.rule-path1": `[{"field":"host-header","hostHeaderConfig":{"values":["anno.example.com"]}}]`,
					},
				},
				Spec: networking.IngressSpec{
					Rules: []networking.IngressRule{{
						Host: "www.example.com",
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{
									{
										Path: "/path1", PathType: &pathExact,
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "rule-path1", Port: networking.ServiceBackendPort{Name: "use-annotation"},
											},
										},
									},
									{
										Path: "/path2", PathType: &pathExact,
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc2", Port: networking.ServiceBackendPort{Number: 80},
											},
										},
									},
								},
							},
						},
					}},
				},
			},
			namespace: "default", gwName: "my-gw",
			ports:      []listenPortEntry{{Protocol: "HTTPS", Port: 443}},
			wantRoutes: 2, // primary route + split route for host-header condition
			check: func(t *testing.T, routes []gwv1.HTTPRoute) {
				// Primary route has only rule-path2 (rule-path1 was split out)
				primary := routes[0]
				assert.Equal(t, []gwv1.Hostname{"www.example.com"}, primary.Spec.Hostnames)
				assert.Len(t, primary.Spec.Rules, 1)
				assert.Equal(t, "/path2", *primary.Spec.Rules[0].Matches[0].Path.Value)

				// Split route has rule-path1 with both hostnames
				split := routes[1]
				assert.Contains(t, split.Spec.Hostnames, gwv1.Hostname("www.example.com"))
				assert.Contains(t, split.Spec.Hostnames, gwv1.Hostname("anno.example.com"))
				assert.Len(t, split.Spec.Rules, 1)
				assert.Equal(t, "/path1", *split.Spec.Rules[0].Matches[0].Path.Value)
			},
		},
		{
			name: "host-header + source-ip conditions on same rule — split route with LRC",
			ing: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "combo",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/actions.rule-combo":    `{"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"200","messageBody":"combo"}}`,
						"alb.ingress.kubernetes.io/conditions.rule-combo": `[{"field":"host-header","hostHeaderConfig":{"values":["api.example.com"]}},{"field":"source-ip","sourceIpConfig":{"values":["10.0.0.0/8"]}}]`,
					},
				},
				Spec: networking.IngressSpec{
					Rules: []networking.IngressRule{{
						Host: "www.example.com",
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{{
									Path: "/combo", PathType: &pathExact,
									Backend: networking.IngressBackend{
										Service: &networking.IngressServiceBackend{
											Name: "rule-combo", Port: networking.ServiceBackendPort{Name: "use-annotation"},
										},
									},
								}},
							},
						},
					}},
				},
			},
			namespace: "default", gwName: "gw",
			ports:      []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			wantRoutes: 2, // empty primary + split route
			check: func(t *testing.T, routes []gwv1.HTTPRoute) {
				split := routes[1]
				assert.Contains(t, split.Spec.Hostnames, gwv1.Hostname("www.example.com"))
				assert.Contains(t, split.Spec.Hostnames, gwv1.Hostname("api.example.com"))
				assert.Len(t, split.Spec.Rules, 1)
				assert.Equal(t, "/combo", *split.Spec.Rules[0].Matches[0].Path.Value)
				assert.True(t, len(split.Spec.Rules[0].Filters) > 0)
			},
		},
		{
			name: "host-header condition without Ingress spec host — only condition hostnames",
			ing: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nohost",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/actions.rule-api":    `{"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"200","messageBody":"api only"}}`,
						"alb.ingress.kubernetes.io/conditions.rule-api": `[{"field":"host-header","hostHeaderConfig":{"values":["api.example.com"]}}]`,
					},
				},
				Spec: networking.IngressSpec{
					Rules: []networking.IngressRule{{
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{{
									Path: "/api", PathType: &pathExact,
									Backend: networking.IngressBackend{
										Service: &networking.IngressServiceBackend{
											Name: "rule-api", Port: networking.ServiceBackendPort{Name: "use-annotation"},
										},
									},
								}},
							},
						},
					}},
				},
			},
			namespace: "default", gwName: "gw",
			ports:      []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			wantRoutes: 2, // empty primary + split route
			check: func(t *testing.T, routes []gwv1.HTTPRoute) {
				primary := routes[0]
				assert.Len(t, primary.Spec.Hostnames, 0)
				assert.Len(t, primary.Spec.Rules, 0)

				split := routes[1]
				assert.Equal(t, []gwv1.Hostname{"api.example.com"}, split.Spec.Hostnames)
				assert.Len(t, split.Spec.Rules, 1)
				assert.Equal(t, "/api", *split.Spec.Rules[0].Matches[0].Path.Value)
			},
		},
		{
			name: "source-ip condition without host-header — no split, stays in primary route",
			ing: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nosplit",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/actions.rule-src":    `{"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"200","messageBody":"source ip only"}}`,
						"alb.ingress.kubernetes.io/conditions.rule-src": `[{"field":"source-ip","sourceIpConfig":{"values":["10.0.0.0/8"]}}]`,
					},
				},
				Spec: networking.IngressSpec{
					Rules: []networking.IngressRule{{
						Host: "www.example.com",
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{{
									Path: "/src", PathType: &pathExact,
									Backend: networking.IngressBackend{
										Service: &networking.IngressServiceBackend{
											Name: "rule-src", Port: networking.ServiceBackendPort{Name: "use-annotation"},
										},
									},
								}},
							},
						},
					}},
				},
			},
			namespace: "default", gwName: "gw",
			ports:      []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			wantRoutes: 1, // no split — source-ip goes to LRC, not hostnames
			check: func(t *testing.T, routes []gwv1.HTTPRoute) {
				r := routes[0]
				assert.Equal(t, []gwv1.Hostname{"www.example.com"}, r.Spec.Hostnames)
				assert.Len(t, r.Spec.Rules, 1)
				assert.Equal(t, "/src", *r.Spec.Rules[0].Matches[0].Path.Value)
			},
		},
		{
			name: "ssl-redirect produces redirect route on HTTP and rules route on HTTPS",
			ing: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ssl",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/ssl-redirect": "443",
					},
				},
				Spec: networking.IngressSpec{
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
			},
			namespace: "default", gwName: "gw",
			ports: []listenPortEntry{
				{Protocol: "HTTP", Port: 80},
				{Protocol: "HTTPS", Port: 443},
			},
			sslRedirectPort: int32Ptr(443),
			wantRoutes:      2, // rules route + redirect route
			check: func(t *testing.T, routes []gwv1.HTTPRoute) {
				// First route: rules, attached to HTTPS listener only
				rulesRoute := routes[0]
				require.NotNil(t, rulesRoute.Spec.ParentRefs[0].SectionName)
				assert.Equal(t, gwv1.SectionName("https-443"), *rulesRoute.Spec.ParentRefs[0].SectionName)
				assert.Len(t, rulesRoute.Spec.Rules, 1)
				assert.Equal(t, "/api", *rulesRoute.Spec.Rules[0].Matches[0].Path.Value)

				// Second route: redirect, attached to HTTP listener only
				redirectRoute := routes[1]
				require.NotNil(t, redirectRoute.Spec.ParentRefs[0].SectionName)
				assert.Equal(t, gwv1.SectionName("http-80"), *redirectRoute.Spec.ParentRefs[0].SectionName)
				require.Len(t, redirectRoute.Spec.Rules, 1)
				require.Len(t, redirectRoute.Spec.Rules[0].Filters, 1)
				assert.Equal(t, gwv1.HTTPRouteFilterRequestRedirect, redirectRoute.Spec.Rules[0].Filters[0].Type)
				assert.Equal(t, "https", *redirectRoute.Spec.Rules[0].Filters[0].RequestRedirect.Scheme)
				assert.Equal(t, gwv1.PortNumber(443), *redirectRoute.Spec.Rules[0].Filters[0].RequestRedirect.Port)
				assert.Equal(t, 301, *redirectRoute.Spec.Rules[0].Filters[0].RequestRedirect.StatusCode)
			},
		},
		{
			name: "jwt-validation annotation produces LRC with jwt-validation action",
			ing: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "jwt",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/jwt-validation": `{"jwksEndpoint":"https://example.com/.well-known/jwks.json","issuer":"https://example.com","additionalClaims":[{"name":"scope","format":"space-separated-values","values":["read","write"]}]}`,
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
											Name: "api-svc", Port: networking.ServiceBackendPort{Number: 80},
										},
									},
								}},
							},
						},
					}},
				},
			},
			namespace: "default", gwName: "gw",
			ports:      []listenPortEntry{{Protocol: "HTTPS", Port: 443}},
			wantRoutes: 1,
			check: func(t *testing.T, routes []gwv1.HTTPRoute) {
				r := routes[0]
				require.Len(t, r.Spec.Rules, 1)
				// Rule should have an ExtensionRef filter pointing to the LRC
				require.Len(t, r.Spec.Rules[0].Filters, 1)
				assert.Equal(t, gwv1.HTTPRouteFilterExtensionRef, r.Spec.Rules[0].Filters[0].Type)
			},
			checkLRCs: func(t *testing.T, lrcs []gatewayv1beta1.ListenerRuleConfiguration) {
				require.Len(t, lrcs, 1)
				require.Len(t, lrcs[0].Spec.Actions, 1)
				action := lrcs[0].Spec.Actions[0]
				assert.Equal(t, gatewayv1beta1.ActionTypeJwtValidation, action.Type)
				require.NotNil(t, action.JwtValidationConfig)
				assert.Equal(t, "https://example.com/.well-known/jwks.json", action.JwtValidationConfig.JwksEndpoint)
				assert.Equal(t, "https://example.com", action.JwtValidationConfig.Issuer)
				require.Len(t, action.JwtValidationConfig.AdditionalClaims, 1)
				assert.Equal(t, "scope", action.JwtValidationConfig.AdditionalClaims[0].Name)
			},
		},
		{
			name: "auth-type and jwt-validation both set — mutual exclusivity error",
			ing: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "conflict",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/auth-type":        "cognito",
						"alb.ingress.kubernetes.io/auth-idp-cognito": `{"userPoolARN":"arn:pool","userPoolClientID":"cid","userPoolDomain":"dom"}`,
						"alb.ingress.kubernetes.io/jwt-validation":   `{"jwksEndpoint":"https://example.com/jwks","issuer":"https://example.com"}`,
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
											Name: "svc", Port: networking.ServiceBackendPort{Number: 80},
										},
									},
								}},
							},
						},
					}},
				},
			},
			namespace: "default", gwName: "gw",
			ports:   []listenPortEntry{{Protocol: "HTTPS", Port: 443}},
			wantErr: "only one pre-routing action is allowed",
		},
		{
			name: "jwt-validation with multiple paths — single LRC action, not duplicated",
			ing: networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "jwt-multi",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/jwt-validation": `{"jwksEndpoint":"https://example.com/jwks","issuer":"https://example.com"}`,
					},
				},
				Spec: networking.IngressSpec{
					Rules: []networking.IngressRule{{
						Host: "api.example.com",
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{
									{
										Path: "/api", PathType: &pathPrefix,
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "api-svc", Port: networking.ServiceBackendPort{Number: 80},
											},
										},
									},
									{
										Path: "/health", PathType: &pathExact,
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "api-svc", Port: networking.ServiceBackendPort{Number: 80},
											},
										},
									},
								},
							},
						},
					}},
				},
			},
			namespace: "default", gwName: "gw",
			ports:      []listenPortEntry{{Protocol: "HTTPS", Port: 443}},
			wantRoutes: 1,
			check: func(t *testing.T, routes []gwv1.HTTPRoute) {
				r := routes[0]
				require.Len(t, r.Spec.Rules, 2)
				// Both rules should have an ExtensionRef filter
				for _, rule := range r.Spec.Rules {
					require.Len(t, rule.Filters, 1)
					assert.Equal(t, gwv1.HTTPRouteFilterExtensionRef, rule.Filters[0].Type)
				}
			},
			checkLRCs: func(t *testing.T, lrcs []gatewayv1beta1.ListenerRuleConfiguration) {
				require.Len(t, lrcs, 1)
				// Only one jwt-validation action despite two paths sharing the LRC
				require.Len(t, lrcs[0].Spec.Actions, 1)
				assert.Equal(t, gatewayv1beta1.ActionTypeJwtValidation, lrcs[0].Spec.Actions[0].Type)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes, _, lrcs, err := buildHTTPRoutes(tt.ing, tt.namespace, tt.gwName, tt.ports, nil, tt.sslRedirectPort)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, routes, tt.wantRoutes)
			if tt.check != nil {
				tt.check(t, routes)
			}
			if tt.checkLRCs != nil {
				tt.checkLRCs(t, lrcs)
			}
		})
	}
}

func TestToGatewayPathType(t *testing.T) {
	prefix := networking.PathTypePrefix
	exact := networking.PathTypeExact
	implSpec := networking.PathTypeImplementationSpecific
	unknown := networking.PathType("Unknown")

	got, err := toGatewayPathType(nil, false)
	assert.NoError(t, err)
	assert.Equal(t, gwv1.PathMatchPathPrefix, got)

	got, err = toGatewayPathType(&prefix, false)
	assert.NoError(t, err)
	assert.Equal(t, gwv1.PathMatchPathPrefix, got)

	got, err = toGatewayPathType(&exact, false)
	assert.NoError(t, err)
	assert.Equal(t, gwv1.PathMatchExact, got)

	got, err = toGatewayPathType(&implSpec, false)
	assert.NoError(t, err)
	assert.Equal(t, gwv1.PathMatchPathPrefix, got)

	got, err = toGatewayPathType(&implSpec, true)
	assert.NoError(t, err)
	assert.Equal(t, gwv1.PathMatchRegularExpression, got)

	_, err = toGatewayPathType(&unknown, false)
	assert.Error(t, err)
}

func TestDeduplicateHostnames(t *testing.T) {
	input := []gwv1.Hostname{"a.com", "A.COM", "b.com", "a.com"}
	result := deduplicateHostnames(input)
	require.Len(t, result, 2)
	assert.Equal(t, gwv1.Hostname("a.com"), result[0])
	assert.Equal(t, gwv1.Hostname("b.com"), result[1])
}

func TestResolveServicePort(t *testing.T) {
	svcMap := map[string]corev1.Service{
		"default/my-svc": {
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "http", Port: 80},
					{Name: "https", Port: 443},
				},
			},
		},
	}

	// Numeric port — returned directly, no lookup needed
	port, err := resolveServicePort(networking.ServiceBackendPort{Number: 8080}, "default", "my-svc", svcMap)
	assert.NoError(t, err)
	assert.Equal(t, int32(8080), port)

	// Named port — resolved from Service
	port, err = resolveServicePort(networking.ServiceBackendPort{Name: "http"}, "default", "my-svc", svcMap)
	assert.NoError(t, err)
	assert.Equal(t, int32(80), port)

	// Named port — Service not found
	_, err = resolveServicePort(networking.ServiceBackendPort{Name: "http"}, "default", "missing-svc", svcMap)
	assert.Error(t, err)

	// Named port — port name not found on Service
	_, err = resolveServicePort(networking.ServiceBackendPort{Name: "grpc"}, "default", "my-svc", svcMap)
	assert.Error(t, err)

	// No port number or name
	_, err = resolveServicePort(networking.ServiceBackendPort{}, "default", "my-svc", svcMap)
	assert.Error(t, err)
}

func int32Ptr(v int32) *int32 { return &v }

func TestBuildHTTPRoutes_DefaultBackendError(t *testing.T) {
	pathPrefix := networking.PathTypePrefix
	ing := networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "app"},
		Spec: networking.IngressSpec{
			DefaultBackend: &networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: "missing-svc",
					Port: networking.ServiceBackendPort{Name: "http"},
				},
			},
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
	}
	// No services provided — named port resolution for defaultBackend will fail
	_, _, _, err := buildHTTPRoutes(ing, "default", "gw",
		[]listenPortEntry{{Protocol: "HTTP", Port: 80}}, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "defaultBackend")
}
