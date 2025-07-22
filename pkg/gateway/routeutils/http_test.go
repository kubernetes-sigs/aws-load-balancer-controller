package routeutils

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

func Test_ConvertHTTPRuleToRouteRule(t *testing.T) {

	rule := &gwv1.HTTPRouteRule{
		Name:               (*gwv1.SectionName)(awssdk.String("my-name")),
		Matches:            []gwv1.HTTPRouteMatch{},
		Filters:            []gwv1.HTTPRouteFilter{},
		BackendRefs:        []gwv1.HTTPBackendRef{},
		SessionPersistence: &gwv1.SessionPersistence{},
	}

	backends := []Backend{
		{}, {},
	}

	result := convertHTTPRouteRule(rule, backends)

	assert.Equal(t, backends, result.GetBackends())
	assert.Equal(t, rule, result.GetRawRouteRule().(*gwv1.HTTPRouteRule))
}

func Test_ListHTTPRoutes(t *testing.T) {
	k8sClient := testutils.GenerateTestClient()

	k8sClient.Create(context.Background(), &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo1",
			Namespace: "bar1",
		},
		Spec: gwv1.HTTPRouteSpec{
			Hostnames: []gwv1.Hostname{
				"host1",
			},
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{},
						{},
					},
				},
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{},
						{},
						{},
						{},
					},
				},
				{
					BackendRefs: []gwv1.HTTPBackendRef{},
				},
			},
		},
	})

	k8sClient.Create(context.Background(), &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo2",
			Namespace: "bar2",
		},
		Spec: gwv1.HTTPRouteSpec{
			Hostnames: []gwv1.Hostname{
				"host2",
			},
			Rules: nil,
		},
	})

	k8sClient.Create(context.Background(), &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo3",
			Namespace: "bar3",
		},
	})

	result, err := ListHTTPRoutes(context.Background(), k8sClient)

	assert.NoError(t, err)

	itemMap := make(map[string]string)
	for _, v := range result {
		routeNsn := v.GetRouteNamespacedName()
		itemMap[routeNsn.Namespace] = routeNsn.Name
		assert.Equal(t, HTTPRouteKind, v.GetRouteKind())
		assert.NotNil(t, v.GetRawRoute())

		if routeNsn.Name == "foo1" {
			assert.Equal(t, []gwv1.Hostname{
				"host1",
			}, v.GetHostnames())
			assert.Equal(t, 6, len(v.GetBackendRefs()))
		}

		if routeNsn.Name == "foo2" {
			assert.Equal(t, []gwv1.Hostname{
				"host2",
			}, v.GetHostnames())
			assert.Equal(t, 0, len(v.GetBackendRefs()))
		}

		if routeNsn.Name == "foo3" {
			assert.Equal(t, 0, len(v.GetHostnames()))
			assert.Equal(t, 0, len(v.GetBackendRefs()))
		}

	}

	assert.Equal(t, "foo1", itemMap["bar1"])
	assert.Equal(t, "foo2", itemMap["bar2"])
	assert.Equal(t, "foo3", itemMap["bar3"])
}

func Test_HTTP_LoadAttachedRules(t *testing.T) {
	weight := 0
	mockLoader := func(ctx context.Context, k8sClient client.Client, typeSpecificBackend interface{}, backendRef gwv1.BackendRef, routeIdentifier types.NamespacedName, routeKind RouteKind) (*Backend, error, error) {
		weight++
		return &Backend{
			Weight: weight,
		}, nil, nil
	}

	routeDescription := httpRouteDescription{
		route: &gwv1.HTTPRoute{
			Spec: gwv1.HTTPRouteSpec{Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{},
						{},
					},
				},
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{},
						{},
						{},
						{},
					},
				},
				{
					BackendRefs: []gwv1.HTTPBackendRef{},
				},
			}},
		},
		rules:           nil,
		ruleAccumulator: newAttachedRuleAccumulator[gwv1.HTTPRouteRule](mockLoader),
	}

	result, errs := routeDescription.loadAttachedRules(context.Background(), nil)
	assert.Equal(t, 0, len(errs))
	convertedRules := result.GetAttachedRules()
	assert.Equal(t, 3, len(convertedRules))

	assert.Equal(t, 2, len(convertedRules[0].GetBackends()))
	assert.Equal(t, 4, len(convertedRules[1].GetBackends()))
	assert.Equal(t, 0, len(convertedRules[2].GetBackends()))
}

func Test_HTTP_GetListenerRuleConfigs(t *testing.T) {
	tests := []struct {
		name     string
		route    *gwv1.HTTPRoute
		expected []gwv1.LocalObjectReference
	}{
		{
			name: "route with no rules",
			route: &gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{},
			},
			expected: []gwv1.LocalObjectReference{},
		},
		{
			name: "route with rules but no filters",
			route: &gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{},
								{},
							},
						},
					},
				},
			},
			expected: []gwv1.LocalObjectReference{},
		},
		{
			name: "route with filters but none are listener rule configurations",
			route: &gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					Rules: []gwv1.HTTPRouteRule{
						{
							Filters: []gwv1.HTTPRouteFilter{
								{
									Type: gwv1.HTTPRouteFilterRequestRedirect,
									RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
										Port: (*gwv1.PortNumber)(awssdk.Int32(80)),
									},
								},
								{
									Type: gwv1.HTTPRouteFilterExtensionRef,
									ExtensionRef: &gwv1.LocalObjectReference{
										Group: "SomeOtherGroup",
										Kind:  "SomeOtherKind",
										Name:  "test-config",
									},
								},
							},
						},
					},
				},
			},
			expected: []gwv1.LocalObjectReference{},
		},
		{
			name: "route with one matching listener rule configuration",
			route: &gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					Rules: []gwv1.HTTPRouteRule{
						{
							Filters: []gwv1.HTTPRouteFilter{
								{
									Type: gwv1.HTTPRouteFilterRequestRedirect,
									RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
										Port: (*gwv1.PortNumber)(awssdk.Int32(80)),
									},
								},
								{
									Type: gwv1.HTTPRouteFilterExtensionRef,
									ExtensionRef: &gwv1.LocalObjectReference{
										Group: constants.ControllerCRDGroupVersion,
										Kind:  constants.ListenerRuleConfiguration,
										Name:  "test-config-1",
									},
								},
							},
						},
					},
				},
			},
			expected: []gwv1.LocalObjectReference{
				{
					Group: constants.ControllerCRDGroupVersion,
					Kind:  constants.ListenerRuleConfiguration,
					Name:  "test-config-1",
				},
			},
		},
		{
			name: "route with multiple matching listener rule configurations",
			route: &gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					Rules: []gwv1.HTTPRouteRule{
						{
							Filters: []gwv1.HTTPRouteFilter{
								{
									Type: gwv1.HTTPRouteFilterExtensionRef,
									ExtensionRef: &gwv1.LocalObjectReference{
										Group: constants.ControllerCRDGroupVersion,
										Kind:  constants.ListenerRuleConfiguration,
										Name:  "test-config-1",
									},
								},
							},
						},
						{
							Filters: []gwv1.HTTPRouteFilter{
								{
									Type: gwv1.HTTPRouteFilterRequestRedirect,
									RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
										Port: (*gwv1.PortNumber)(awssdk.Int32(80)),
									},
								},
							},
						},
						{
							Filters: []gwv1.HTTPRouteFilter{
								{
									Type: gwv1.HTTPRouteFilterExtensionRef,
									ExtensionRef: &gwv1.LocalObjectReference{
										Group: constants.ControllerCRDGroupVersion,
										Kind:  constants.ListenerRuleConfiguration,
										Name:  "test-config-2",
									},
								},
							},
						},
					},
				},
			},
			expected: []gwv1.LocalObjectReference{
				{
					Group: constants.ControllerCRDGroupVersion,
					Kind:  constants.ListenerRuleConfiguration,
					Name:  "test-config-1",
				},
				{
					Group: constants.ControllerCRDGroupVersion,
					Kind:  constants.ListenerRuleConfiguration,
					Name:  "test-config-2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpRoute := &httpRouteDescription{
				route: tt.route,
			}

			got := httpRoute.GetListenerRuleConfigs()

			// Check if the length matches
			assert.Equal(t, len(tt.expected), len(got), "Expected %d rule configs, got %d", len(tt.expected), len(got))

			// Check if all expected items exist in the result
			for _, expected := range tt.expected {
				found := false
				for _, actual := range got {
					if expected.Group == actual.Group && expected.Kind == actual.Kind && expected.Name == actual.Name {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected listener rule config %s not found in result", expected.Name)
			}
		})
	}
}
