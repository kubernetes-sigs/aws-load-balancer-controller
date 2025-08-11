package routeutils

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

func Test_ConvertGRPCRuleToRouteRule(t *testing.T) {

	rule := &gwv1.GRPCRouteRule{
		Name:               (*gwv1.SectionName)(awssdk.String("my-name")),
		Matches:            []gwv1.GRPCRouteMatch{},
		Filters:            []gwv1.GRPCRouteFilter{},
		BackendRefs:        []gwv1.GRPCBackendRef{},
		SessionPersistence: &gwv1.SessionPersistence{},
	}

	backends := []Backend{
		{}, {},
	}

	listenerRuleCfg := &elbv2gw.ListenerRuleConfiguration{}

	result := convertGRPCRouteRule(rule, backends, listenerRuleCfg)

	assert.Equal(t, backends, result.GetBackends())
	assert.Equal(t, rule, result.GetRawRouteRule().(*gwv1.GRPCRouteRule))
}

func Test_ListGRPCRoutes(t *testing.T) {
	k8sClient := testutils.GenerateTestClient()

	k8sClient.Create(context.Background(), &gwv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo1",
			Namespace: "bar1",
		},
		Spec: gwv1.GRPCRouteSpec{
			Hostnames: []gwv1.Hostname{
				"host1",
			},
			Rules: []gwv1.GRPCRouteRule{
				{
					BackendRefs: []gwv1.GRPCBackendRef{
						{},
						{},
					},
				},
				{
					BackendRefs: []gwv1.GRPCBackendRef{
						{},
						{},
						{},
						{},
					},
				},
				{
					BackendRefs: []gwv1.GRPCBackendRef{},
				},
			},
		},
	})

	k8sClient.Create(context.Background(), &gwv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo2",
			Namespace: "bar2",
		},
		Spec: gwv1.GRPCRouteSpec{
			Hostnames: []gwv1.Hostname{
				"host2",
			},
			Rules: nil,
		},
	})

	k8sClient.Create(context.Background(), &gwv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo3",
			Namespace: "bar3",
		},
	})

	result, err := ListGRPCRoutes(context.Background(), k8sClient)

	assert.NoError(t, err)

	itemMap := make(map[string]string)
	for _, v := range result {
		routeNsn := v.GetRouteNamespacedName()
		itemMap[routeNsn.Namespace] = routeNsn.Name
		assert.Equal(t, GRPCRouteKind, v.GetRouteKind())
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

func Test_GRPC_LoadAttachedRules(t *testing.T) {
	weight := 0
	mockBackendLoader := func(ctx context.Context, k8sClient client.Client, typeSpecificBackend interface{}, backendRef gwv1.BackendRef, routeIdentifier types.NamespacedName, routeKind RouteKind) (*Backend, error, error) {
		weight++
		return &Backend{
			Weight: weight,
		}, nil, nil
	}
	mockListenerRuleConfigLoader := func(ctx context.Context, k8sClient client.Client, routeIdentifier types.NamespacedName, routeKind RouteKind, listenerRuleConfigRefs []gwv1.LocalObjectReference) (*elbv2gw.ListenerRuleConfiguration, error, error) {
		return &elbv2gw.ListenerRuleConfiguration{}, nil, nil
	}

	routeDescription := grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			Spec: gwv1.GRPCRouteSpec{Rules: []gwv1.GRPCRouteRule{
				{
					BackendRefs: []gwv1.GRPCBackendRef{
						{},
						{},
					},
					Filters: []gwv1.GRPCRouteFilter{
						{
							Type: gwv1.GRPCRouteFilterExtensionRef,
							ExtensionRef: &gwv1.LocalObjectReference{
								Group: constants.ControllerCRDGroupVersion,
								Kind:  constants.ListenerRuleConfiguration,
								Name:  "test-config-1",
							},
						},
					},
				},
				{
					BackendRefs: []gwv1.GRPCBackendRef{
						{},
						{},
						{},
						{},
					},
					Filters: []gwv1.GRPCRouteFilter{
						{
							Type: gwv1.GRPCRouteFilterExtensionRef,
							ExtensionRef: &gwv1.LocalObjectReference{
								Group: constants.ControllerCRDGroupVersion,
								Kind:  constants.ListenerRuleConfiguration,
								Name:  "test-config-1",
							},
						},
					},
				},
				{
					BackendRefs: []gwv1.GRPCBackendRef{},
					Filters: []gwv1.GRPCRouteFilter{
						{
							Type: gwv1.GRPCRouteFilterExtensionRef,
							ExtensionRef: &gwv1.LocalObjectReference{
								Group: constants.ControllerCRDGroupVersion,
								Kind:  constants.ListenerRuleConfiguration,
								Name:  "test-config-1",
							},
						},
					},
				},
			}},
		},
		rules:           nil,
		ruleAccumulator: newAttachedRuleAccumulator[gwv1.GRPCRouteRule](mockBackendLoader, mockListenerRuleConfigLoader),
	}

	result, errs := routeDescription.loadAttachedRules(context.Background(), nil)
	assert.Equal(t, 0, len(errs))
	convertedRules := result.GetAttachedRules()
	assert.Equal(t, 3, len(convertedRules))

	assert.Equal(t, 2, len(convertedRules[0].GetBackends()))
	assert.Equal(t, 4, len(convertedRules[1].GetBackends()))
	assert.Equal(t, 0, len(convertedRules[2].GetBackends()))

	expectedConfig := &elbv2gw.ListenerRuleConfiguration{}

	assert.Equal(t, expectedConfig, convertedRules[0].GetListenerRuleConfig())
	assert.Equal(t, expectedConfig, convertedRules[1].GetListenerRuleConfig())
	assert.Equal(t, expectedConfig, convertedRules[2].GetListenerRuleConfig())
}

func Test_GRPC_GetListenerRuleConfigRefs(t *testing.T) {
	tests := []struct {
		name     string
		route    *gwv1.GRPCRoute
		expected []gwv1.LocalObjectReference
	}{
		{
			name: "route with no rules",
			route: &gwv1.GRPCRoute{
				Spec: gwv1.GRPCRouteSpec{},
			},
			expected: []gwv1.LocalObjectReference{},
		},
		{
			name: "route with rules but no filters",
			route: &gwv1.GRPCRoute{
				Spec: gwv1.GRPCRouteSpec{
					Rules: []gwv1.GRPCRouteRule{
						{
							Matches: []gwv1.GRPCRouteMatch{
								{
									Method: &gwv1.GRPCMethodMatch{
										Service: awssdk.String("TestService"),
										Method:  awssdk.String("TestMethod"),
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
			name: "route with filters but none are listener rule configurations",
			route: &gwv1.GRPCRoute{
				Spec: gwv1.GRPCRouteSpec{
					Rules: []gwv1.GRPCRouteRule{
						{
							Filters: []gwv1.GRPCRouteFilter{
								{
									Type: gwv1.GRPCRouteFilterExtensionRef,
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
			route: &gwv1.GRPCRoute{
				Spec: gwv1.GRPCRouteSpec{
					Rules: []gwv1.GRPCRouteRule{
						{
							Filters: []gwv1.GRPCRouteFilter{
								{
									Type: gwv1.GRPCRouteFilterExtensionRef,
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
			route: &gwv1.GRPCRoute{
				Spec: gwv1.GRPCRouteSpec{
					Rules: []gwv1.GRPCRouteRule{
						{
							Filters: []gwv1.GRPCRouteFilter{
								{
									Type: gwv1.GRPCRouteFilterExtensionRef,
									ExtensionRef: &gwv1.LocalObjectReference{
										Group: constants.ControllerCRDGroupVersion,
										Kind:  constants.ListenerRuleConfiguration,
										Name:  "test-config-1",
									},
								},
							},
						},
						{
							Filters: []gwv1.GRPCRouteFilter{},
						},
						{
							Filters: []gwv1.GRPCRouteFilter{
								{
									Type: gwv1.GRPCRouteFilterExtensionRef,
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
			grpcRoute := &grpcRouteDescription{
				route: tt.route,
			}

			got := grpcRoute.GetRouteListenerRuleConfigRefs()

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
