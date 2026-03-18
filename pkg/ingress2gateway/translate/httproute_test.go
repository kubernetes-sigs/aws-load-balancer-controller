package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestBuildHTTPRoutes(t *testing.T) {
	pathPrefix := networking.PathTypePrefix
	pathExact := networking.PathTypeExact

	tests := []struct {
		name       string
		ing        networking.Ingress
		namespace  string
		gwName     string
		ports      []listenPortEntry
		wantRoutes int
		check      func(t *testing.T, routes []gwv1.HTTPRoute)
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
			name: "multiple listeners produce multiple parentRefs",
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
				assert.Len(t, routes[0].Spec.ParentRefs, 2)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes := buildHTTPRoutes(tt.ing, tt.namespace, tt.gwName, tt.ports)
			require.Len(t, routes, tt.wantRoutes)
			if tt.check != nil {
				tt.check(t, routes)
			}
		})
	}
}

func TestToGatewayPathType(t *testing.T) {
	prefix := networking.PathTypePrefix
	exact := networking.PathTypeExact
	implSpec := networking.PathTypeImplementationSpecific

	assert.Equal(t, gwv1.PathMatchPathPrefix, toGatewayPathType(nil, false))
	assert.Equal(t, gwv1.PathMatchPathPrefix, toGatewayPathType(&prefix, false))
	assert.Equal(t, gwv1.PathMatchExact, toGatewayPathType(&exact, false))
	assert.Equal(t, gwv1.PathMatchPathPrefix, toGatewayPathType(&implSpec, false))
	assert.Equal(t, gwv1.PathMatchRegularExpression, toGatewayPathType(&implSpec, true))
}

func TestDeduplicateHostnames(t *testing.T) {
	input := []gwv1.Hostname{"a.com", "A.COM", "b.com", "a.com"}
	result := deduplicateHostnames(input)
	require.Len(t, result, 2)
	assert.Equal(t, gwv1.Hostname("a.com"), result[0])
	assert.Equal(t, gwv1.Hostname("b.com"), result[1])
}
