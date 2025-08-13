package routeutils

import (
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

// Test FilterRoutesByListenerRuleCfg
func Test_FilterRoutesByListenerRuleCfg(t *testing.T) {
	namespace := "test-ns"
	ruleConfigName := "test-rule-config"

	tests := []struct {
		name          string
		routes        []preLoadRouteDescriptor
		ruleConfig    *elbv2gw.ListenerRuleConfiguration
		expectedCount int
	}{
		{
			name:          "Nil ruleConfig returns empty slice",
			routes:        []preLoadRouteDescriptor{},
			ruleConfig:    nil,
			expectedCount: 0,
		},
		{
			name:   "Empty routes returns empty slice",
			routes: []preLoadRouteDescriptor{},
			ruleConfig: &elbv2gw.ListenerRuleConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ruleConfigName,
					Namespace: namespace,
				},
			},
			expectedCount: 0,
		},
		{
			name: "Single routes matching rule config",
			routes: []preLoadRouteDescriptor{
				&mockPreLoadRouteDescriptor{
					listenerRuleConfigurations: []gwv1.LocalObjectReference{
						{
							Group: "gateway.k8s.aws",
							Kind:  "ListenerRuleConfiguration",
							Name:  gwv1.ObjectName(ruleConfigName),
						},
					},
					namespacedName: types.NamespacedName{
						Namespace: namespace,
						Name:      "route-1",
					},
				},
				&mockPreLoadRouteDescriptor{
					listenerRuleConfigurations: []gwv1.LocalObjectReference{
						{
							Group: "gateway.k8s.aws",
							Kind:  "ListenerRuleConfiguration",
							Name:  "other-rule-config",
						},
					},
					namespacedName: types.NamespacedName{
						Namespace: namespace,
						Name:      "route-2",
					},
				},
			},
			ruleConfig: &elbv2gw.ListenerRuleConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ruleConfigName,
					Namespace: namespace,
				},
			},
			expectedCount: 1,
		},
		{
			name: "Routes with no matching rule config",
			routes: []preLoadRouteDescriptor{
				&mockPreLoadRouteDescriptor{
					namespacedName: types.NamespacedName{
						Namespace: namespace,
						Name:      "route-1",
					},
				},
				&mockPreLoadRouteDescriptor{
					listenerRuleConfigurations: []gwv1.LocalObjectReference{
						{
							Group: "gateway.k8s.aws",
							Kind:  "ListenerRuleConfiguration",
							Name:  "other-rule-config",
						},
					},
					namespacedName: types.NamespacedName{
						Namespace: namespace,
						Name:      "route-2",
					},
				},
			},
			ruleConfig: &elbv2gw.ListenerRuleConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ruleConfigName,
					Namespace: namespace,
				},
			},
			expectedCount: 0,
		},
		{
			name: "Multiple matching routes",
			routes: []preLoadRouteDescriptor{
				&mockPreLoadRouteDescriptor{
					listenerRuleConfigurations: []gwv1.LocalObjectReference{
						{
							Group: "gateway.k8s.aws",
							Kind:  "ListenerRuleConfiguration",
							Name:  gwv1.ObjectName(ruleConfigName),
						},
					},
					namespacedName: types.NamespacedName{
						Namespace: namespace,
						Name:      "route-1",
					},
				},
				&mockPreLoadRouteDescriptor{
					namespacedName: types.NamespacedName{
						Namespace: namespace,
						Name:      "route-2",
					},
				},
				&mockPreLoadRouteDescriptor{
					listenerRuleConfigurations: []gwv1.LocalObjectReference{
						{
							Group: "gateway.k8s.aws",
							Kind:  "ListenerRuleConfiguration",
							Name:  gwv1.ObjectName(ruleConfigName),
						},
					},
					namespacedName: types.NamespacedName{
						Namespace: namespace,
						Name:      "route-3",
					},
				},
			},
			ruleConfig: &elbv2gw.ListenerRuleConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ruleConfigName,
					Namespace: namespace,
				},
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := FilterRoutesByListenerRuleCfg(tt.routes, tt.ruleConfig)
			assert.Len(t, filtered, tt.expectedCount)
		})
	}
}

// Test isListenerRuleConfigReferredByRoute
func Test_isListenerRuleConfigReferredByRoute(t *testing.T) {
	testNamespace := "test-namespace"
	testRuleCfgName := "test-rule-config"

	tests := []struct {
		name       string
		route      preLoadRouteDescriptor
		ruleConfig types.NamespacedName
		expected   bool
	}{
		{
			name: "route with direct reference to rule configuration",
			route: mockPreLoadRouteDescriptor{
				listenerRuleConfigurations: []gwv1.LocalObjectReference{
					{
						Group: "gateway.k8s.aws",
						Kind:  "ListenerRuleConfiguration",
						Name:  gwv1.ObjectName(testRuleCfgName),
					},
				},
				namespacedName: types.NamespacedName{
					Namespace: testNamespace,
					Name:      "route-1",
				},
			},
			ruleConfig: types.NamespacedName{
				Namespace: testNamespace,
				Name:      testRuleCfgName,
			},
			expected: true,
		},
		{
			name: "route with reference to a different rule configuration",
			route: mockPreLoadRouteDescriptor{
				listenerRuleConfigurations: []gwv1.LocalObjectReference{
					{
						Group: "gateway.k8s.aws",
						Kind:  "ListenerRuleConfiguration",
						Name:  gwv1.ObjectName("other-rule-config"),
					},
				},
				namespacedName: types.NamespacedName{
					Namespace: testNamespace,
					Name:      "route-1",
				},
			},
			ruleConfig: types.NamespacedName{
				Namespace: testNamespace,
				Name:      testRuleCfgName,
			},
			expected: false,
		},
		{
			name: "route with no rule configuration",
			route: mockPreLoadRouteDescriptor{
				namespacedName: types.NamespacedName{
					Namespace: testNamespace,
					Name:      "route-1",
				},
			},
			ruleConfig: types.NamespacedName{
				Namespace: testNamespace,
				Name:      testRuleCfgName,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isListenerRuleConfigReferredByRoute(tt.route, tt.ruleConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}
