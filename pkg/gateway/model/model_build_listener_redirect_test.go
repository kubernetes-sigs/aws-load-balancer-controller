package model

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// Mock target group builder for testing
type mockTargetGroupBuilder struct {
	mock.Mock
}

func (m *mockTargetGroupBuilder) buildTargetGroup(stack core.Stack, gw *gwv1.Gateway, listenerPort int32, listenerProtocol elbv2model.Protocol, lbIPType elbv2model.IPAddressType, routeDescriptor routeutils.RouteDescriptor, backend routeutils.Backend) (core.StringToken, error) {
	args := m.Called(stack, gw, listenerPort, listenerProtocol, lbIPType, routeDescriptor, backend)
	return args.Get(0).(core.StringToken), args.Error(1)
}

func (m *mockTargetGroupBuilder) getLocalFrontendNlbData() map[string]*elbv2model.FrontendNlbTargetGroupState {
	args := m.Called()
	return args.Get(0).(map[string]*elbv2model.FrontendNlbTargetGroupState)
}

// Mock route descriptor for testing
type mockRouteDescriptor struct {
	mock.Mock
}

func (m *mockRouteDescriptor) GetAttachedRules() []routeutils.RouteRule {
	args := m.Called()
	return args.Get(0).([]routeutils.RouteRule)
}

func (m *mockRouteDescriptor) GetHostnames() []gwv1.Hostname {
	args := m.Called()
	return args.Get(0).([]gwv1.Hostname)
}

func (m *mockRouteDescriptor) GetParentRefs() []gwv1.ParentReference {
	args := m.Called()
	return args.Get(0).([]gwv1.ParentReference)
}

func (m *mockRouteDescriptor) GetRouteKind() routeutils.RouteKind {
	args := m.Called()
	return args.Get(0).(routeutils.RouteKind)
}

func (m *mockRouteDescriptor) GetRouteGeneration() int64 {
	args := m.Called()
	return args.Get(0).(int64)
}

func (m *mockRouteDescriptor) GetRouteNamespacedName() types.NamespacedName {
	args := m.Called()
	return args.Get(0).(types.NamespacedName)
}

func (m *mockRouteDescriptor) GetRouteIdentifier() string {
	args := m.Called()
	return args.Get(0).(string)
}

func (m *mockRouteDescriptor) GetBackendRefs() []gwv1.BackendRef {
	args := m.Called()
	return args.Get(0).([]gwv1.BackendRef)
}

func (m *mockRouteDescriptor) GetRouteListenerRuleConfigRefs() []gwv1.LocalObjectReference {
	args := m.Called()
	return args.Get(0).([]gwv1.LocalObjectReference)
}

func (m *mockRouteDescriptor) GetCompatibleHostnamesByPort() map[int32][]gwv1.Hostname {
	args := m.Called()
	return args.Get(0).(map[int32][]gwv1.Hostname)
}

// Mock route rule for testing
type mockRouteRule struct {
	mock.Mock
	rule     *gwv1.HTTPRouteRule
	backends []routeutils.Backend
}

func (m *mockRouteRule) GetRawRouteRule() interface{} {
	return m.rule
}

func (m *mockRouteRule) GetBackends() []routeutils.Backend {
	return m.backends
}

func (m *mockRouteRule) GetListenerRuleConfig() *elbv2gw.ListenerRuleConfiguration {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*elbv2gw.ListenerRuleConfiguration)
}

func TestBuildListenerRules_RedirectOnlyRules(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))
	
	tests := []struct {
		name                    string
		rules                   []routeutils.RouteRule
		expectedTGBuilderCalls  int
		description             string
	}{
		{
			name: "redirect-only rule skips target group creation",
			rules: []routeutils.RouteRule{
				&mockRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Scheme: awssdk.String("https"),
								},
							},
						},
					},
					backends: []routeutils.Backend{}, // No backends
				},
			},
			expectedTGBuilderCalls: 0, // Should not call target group builder
			description:            "Redirect-only rules should skip target group creation",
		},
		{
			name: "backend rule creates target groups",
			rules: []routeutils.RouteRule{
				&mockRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{},
					},
					backends: []routeutils.Backend{{Weight: 1}}, // Has backends
				},
			},
			expectedTGBuilderCalls: 1, // Should call target group builder once
			description:            "Backend rules should create target groups",
		},
		{
			name: "mixed rules - redirect-only and backend",
			rules: []routeutils.RouteRule{
				// Redirect-only rule
				&mockRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Hostname: (*gwv1.PreciseHostname)(awssdk.String("example.com")),
								},
							},
						},
					},
					backends: []routeutils.Backend{}, // No backends
				},
				// Backend rule
				&mockRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{},
					},
					backends: []routeutils.Backend{{Weight: 50}, {Weight: 50}}, // Has backends
				},
			},
			expectedTGBuilderCalls: 2, // Should call target group builder twice (for 2 backends)
			description:            "Mixed rules should process independently",
		},
		{
			name: "rule with redirect and backends (mixed rule)",
			rules: []routeutils.RouteRule{
				&mockRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Port: (*gwv1.PortNumber)(awssdk.Int32(443)),
								},
							},
						},
					},
					backends: []routeutils.Backend{{Weight: 100}}, // Has backends
				},
			},
			expectedTGBuilderCalls: 1, // Should call target group builder (not redirect-only)
			description:            "Rules with both redirect and backends should create target groups",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockTGBuilder := &mockTargetGroupBuilder{}
			mockRouteDesc := &mockRouteDescriptor{}
			
			// Setup mock expectations
			mockRouteDesc.On("GetRouteKind").Return(routeutils.HTTPRouteKind)
			mockRouteDesc.On("GetRouteNamespacedName").Return(types.NamespacedName{
				Namespace: "test",
				Name:      "test-route",
			})

			// Setup target group builder expectations
			if tt.expectedTGBuilderCalls > 0 {
				mockTGBuilder.On("buildTargetGroup", 
					mock.Anything, // stack
					mock.Anything, // gateway
					mock.Anything, // listenerPort
					mock.Anything, // listenerProtocol
					mock.Anything, // lbIPType
					mock.Anything, // routeDescriptor
					mock.Anything, // backend
				).Return(core.LiteralStringToken("arn:aws:elasticloadbalancing:us-west-2:123456789012:targetgroup/test/1234567890123456"), nil).Times(tt.expectedTGBuilderCalls)
			}

			// Setup route rule mocks
			for _, rule := range tt.rules {
				if mockRule, ok := rule.(*mockRouteRule); ok {
					mockRule.On("GetListenerRuleConfig").Return((*elbv2gw.ListenerRuleConfiguration)(nil))
				}
			}

			// Create listener builder
			builder := &listenerBuilderImpl{
				loadBalancerType: elbv2model.LoadBalancerTypeApplication,
				tgBuilder:        mockTGBuilder,
				logger:           logger,
			}

			// Create test data
			ctx := context.Background()
			stack := core.NewDefaultStack(core.StackID("test"))
			listener := &elbv2model.Listener{
				Spec: elbv2model.ListenerSpec{
					Protocol: elbv2model.ProtocolHTTP,
				},
			}
			gateway := &gwv1.Gateway{
				ObjectMeta: k8s.ObjectMeta{
					Namespace: "test",
					Name:      "test-gateway",
				},
			}
			port := int32(80)
			routes := map[int32][]routeutils.RouteDescriptor{
				port: {mockRouteDesc},
			}

			// Mock the route descriptor to return our test rules
			mockRouteDesc.On("GetAttachedRules").Return(tt.rules)

			// Execute the function under test
			_, err := builder.buildListenerRules(ctx, stack, listener, elbv2model.IPAddressTypeIPV4, gateway, port, routes)

			// Assertions
			assert.NoError(t, err, tt.description)
			mockTGBuilder.AssertExpectations(t)
			mockRouteDesc.AssertExpectations(t)
		})
	}
}

func TestBuildListenerRules_ErrorHandling(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	tests := []struct {
		name        string
		rules       []routeutils.RouteRule
		setupMocks  func(*mockTargetGroupBuilder, *mockRouteDescriptor)
		expectError bool
		description string
	}{
		{
			name: "target group creation error for backend rule",
			rules: []routeutils.RouteRule{
				&mockRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{},
					},
					backends: []routeutils.Backend{{Weight: 1}},
				},
			},
			setupMocks: func(tgBuilder *mockTargetGroupBuilder, routeDesc *mockRouteDescriptor) {
				tgBuilder.On("buildTargetGroup", 
					mock.Anything, mock.Anything, mock.Anything, mock.Anything, 
					mock.Anything, mock.Anything, mock.Anything,
				).Return(core.StringToken(nil), assert.AnError)
			},
			expectError: true,
			description: "Target group creation errors should be propagated",
		},
		{
			name: "redirect-only rule with target group error should not fail",
			rules: []routeutils.RouteRule{
				&mockRouteRule{
					rule: &gwv1.HTTPRouteRule{
						Filters: []gwv1.HTTPRouteFilter{
							{
								Type: gwv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
									Scheme: awssdk.String("https"),
								},
							},
						},
					},
					backends: []routeutils.Backend{}, // No backends
				},
			},
			setupMocks: func(tgBuilder *mockTargetGroupBuilder, routeDesc *mockRouteDescriptor) {
				// No target group builder calls expected for redirect-only rules
			},
			expectError: false,
			description: "Redirect-only rules should not fail even if target group builder would error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockTGBuilder := &mockTargetGroupBuilder{}
			mockRouteDesc := &mockRouteDescriptor{}
			
			// Setup common mock expectations
			mockRouteDesc.On("GetRouteKind").Return(routeutils.HTTPRouteKind)
			mockRouteDesc.On("GetRouteNamespacedName").Return(types.NamespacedName{
				Namespace: "test",
				Name:      "test-route",
			})
			mockRouteDesc.On("GetAttachedRules").Return(tt.rules)

			// Setup test-specific mocks
			tt.setupMocks(mockTGBuilder, mockRouteDesc)

			// Setup route rule mocks
			for _, rule := range tt.rules {
				if mockRule, ok := rule.(*mockRouteRule); ok {
					mockRule.On("GetListenerRuleConfig").Return((*elbv2gw.ListenerRuleConfiguration)(nil))
				}
			}

			// Create listener builder
			builder := &listenerBuilderImpl{
				loadBalancerType: elbv2model.LoadBalancerTypeApplication,
				tgBuilder:        mockTGBuilder,
				logger:           logger,
			}

			// Create test data
			ctx := context.Background()
			stack := core.NewDefaultStack(core.StackID("test"))
			listener := &elbv2model.Listener{
				Spec: elbv2model.ListenerSpec{
					Protocol: elbv2model.ProtocolHTTP,
				},
			}
			gateway := &gwv1.Gateway{
				ObjectMeta: k8s.ObjectMeta{
					Namespace: "test",
					Name:      "test-gateway",
				},
			}
			port := int32(80)
			routes := map[int32][]routeutils.RouteDescriptor{
				port: {mockRouteDesc},
			}

			// Execute the function under test
			_, err := builder.buildListenerRules(ctx, stack, listener, elbv2model.IPAddressTypeIPV4, gateway, port, routes)

			// Assertions
			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}

			mockTGBuilder.AssertExpectations(t)
			mockRouteDesc.AssertExpectations(t)
		})
	}
}