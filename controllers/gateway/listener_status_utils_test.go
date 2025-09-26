package gateway

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func Test_buildListenerStatus(t *testing.T) {
	tests := []struct {
		name                    string
		controllerName          string
		gateway                 *gwv1.Gateway
		attachedRoutesMap       map[gwv1.SectionName]int32
		validateListenerResults *routeutils.ListenerValidationResults
		expectedCount           int
		expectedError           bool
	}{
		{
			name:           "nil validation results",
			controllerName: "test-controller",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:          "listener1",
							Port:          80,
							Protocol:      gwv1.HTTPProtocolType,
							AllowedRoutes: &gwv1.AllowedRoutes{},
						},
					},
				},
			},
			attachedRoutesMap:       map[gwv1.SectionName]int32{"listener1": 2},
			validateListenerResults: nil,
			expectedCount:           1,
		},
		{
			name:           "with validation results",
			controllerName: "test-controller",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:          "listener1",
							Port:          80,
							Protocol:      gwv1.HTTPProtocolType,
							AllowedRoutes: &gwv1.AllowedRoutes{},
						},
						{
							Name:          "listener2",
							Port:          443,
							Protocol:      gwv1.HTTPSProtocolType,
							AllowedRoutes: &gwv1.AllowedRoutes{},
						},
					},
				},
			},
			attachedRoutesMap: map[gwv1.SectionName]int32{"listener1": 2, "listener2": 1},
			validateListenerResults: &routeutils.ListenerValidationResults{
				Results: map[gwv1.SectionName]routeutils.ListenerValidationResult{
					"listener1": {Reason: gwv1.ListenerReasonAccepted, Message: "sample-message"},
					"listener2": {Reason: gwv1.ListenerReasonPortUnavailable, Message: "Port unavailable"},
				},
			},
			expectedCount: 2,
		},
		{
			name:           "empty listeners",
			controllerName: "test-controller",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{},
				},
			},
			attachedRoutesMap:       map[gwv1.SectionName]int32{},
			validateListenerResults: nil,
			expectedCount:           0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildListenerStatus(tt.controllerName, *tt.gateway, tt.attachedRoutesMap, tt.validateListenerResults)

			assert.Len(t, result, tt.expectedCount)

			for i, listener := range tt.gateway.Spec.Listeners {
				assert.Equal(t, listener.Name, result[i].Name)
				assert.Equal(t, tt.attachedRoutesMap[listener.Name], result[i].AttachedRoutes)
			}

		})
	}
}

func Test_getListenerConditions(t *testing.T) {
	tests := []struct {
		name                     string
		listenerValidationResult *routeutils.ListenerValidationResult
		expectedConditionType    string
		expectedStatus           metav1.ConditionStatus
		expectedReason           string
		gw                       gwv1.Gateway
	}{
		{
			name:                     "nil validation result",
			listenerValidationResult: nil,
			expectedConditionType:    string(gwv1.ListenerConditionAccepted),
			expectedStatus:           metav1.ConditionTrue,
			expectedReason:           string(gwv1.ListenerReasonAccepted),
		},
		{
			name: "hostname conflict",
			listenerValidationResult: &routeutils.ListenerValidationResult{
				Reason:  gwv1.ListenerReasonHostnameConflict,
				Message: "Hostname conflict",
			},
			expectedConditionType: string(gwv1.ListenerConditionConflicted),
			expectedStatus:        metav1.ConditionFalse,
			expectedReason:        string(gwv1.ListenerReasonHostnameConflict),
		},
		{
			name: "protocol conflict",
			listenerValidationResult: &routeutils.ListenerValidationResult{
				Reason:  gwv1.ListenerReasonProtocolConflict,
				Message: "Protocol conflict",
			},
			expectedConditionType: string(gwv1.ListenerConditionConflicted),
			expectedStatus:        metav1.ConditionFalse,
			expectedReason:        string(gwv1.ListenerReasonProtocolConflict),
		},
		{
			name: "port unavailable",
			listenerValidationResult: &routeutils.ListenerValidationResult{
				Reason:  gwv1.ListenerReasonPortUnavailable,
				Message: "Port unavailable",
			},
			expectedConditionType: string(gwv1.ListenerConditionAccepted),
			expectedStatus:        metav1.ConditionFalse,
			expectedReason:        string(gwv1.ListenerReasonPortUnavailable),
		},
		{
			name: "unsupported protocol",
			listenerValidationResult: &routeutils.ListenerValidationResult{
				Reason:  gwv1.ListenerReasonUnsupportedProtocol,
				Message: "Unsupported protocol",
			},
			expectedConditionType: string(gwv1.ListenerConditionAccepted),
			expectedStatus:        metav1.ConditionFalse,
			expectedReason:        string(gwv1.ListenerReasonUnsupportedProtocol),
		},
		{
			name: "invalid route kinds",
			listenerValidationResult: &routeutils.ListenerValidationResult{
				Reason:  gwv1.ListenerReasonInvalidRouteKinds,
				Message: "Invalid route kinds",
			},
			expectedConditionType: string(gwv1.ListenerConditionResolvedRefs),
			expectedStatus:        metav1.ConditionFalse,
			expectedReason:        string(gwv1.ListenerReasonInvalidRouteKinds),
		},
		{
			name: "ref not permitted",
			listenerValidationResult: &routeutils.ListenerValidationResult{
				Reason:  gwv1.ListenerReasonRefNotPermitted,
				Message: "Ref not permitted",
			},
			expectedConditionType: string(gwv1.ListenerConditionResolvedRefs),
			expectedStatus:        metav1.ConditionFalse,
			expectedReason:        string(gwv1.ListenerReasonRefNotPermitted),
		},
		{
			name: "unknown reason defaults to accepted",
			listenerValidationResult: &routeutils.ListenerValidationResult{
				Reason:  "UnknownReason",
				Message: "Unknown",
			},
			expectedConditionType: string(gwv1.ListenerConditionAccepted),
			expectedStatus:        metav1.ConditionTrue,
			expectedReason:        string(gwv1.ListenerReasonAccepted),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conditions := getListenerConditions(tt.gw, tt.listenerValidationResult)

			assert.Len(t, conditions, 1)
			condition := conditions[0]
			assert.Equal(t, tt.expectedConditionType, condition.Type)
			assert.Equal(t, tt.expectedStatus, condition.Status)
			assert.Equal(t, tt.expectedReason, condition.Reason)
			assert.NotZero(t, condition.LastTransitionTime)
		})
	}
}

func Test_buildAcceptedCondition(t *testing.T) {
	tests := []struct {
		name           string
		reason         gwv1.ListenerConditionReason
		message        string
		expectedStatus metav1.ConditionStatus
		gw             gwv1.Gateway
	}{
		{
			name:           "accepted reason",
			reason:         gwv1.ListenerReasonAccepted,
			message:        gateway_constants.ListenerAcceptedMessage,
			expectedStatus: metav1.ConditionTrue,
		},
		{
			name:           "non-accepted reason",
			reason:         gwv1.ListenerReasonPortUnavailable,
			message:        "Port unavailable",
			expectedStatus: metav1.ConditionFalse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := buildAcceptedCondition(tt.gw, tt.reason, tt.message)

			assert.Equal(t, string(gwv1.ListenerConditionAccepted), condition.Type)
			assert.Equal(t, tt.expectedStatus, condition.Status)
			assert.Equal(t, string(tt.reason), condition.Reason)
			assert.Equal(t, tt.message, condition.Message)
			assert.NotZero(t, condition.LastTransitionTime)
		})
	}
}

func Test_buildConflictedCondition(t *testing.T) {

	tests := []struct {
		name           string
		reason         gwv1.ListenerConditionReason
		message        string
		expectedStatus metav1.ConditionStatus
		gw             gwv1.Gateway
	}{
		{
			name:           "accepted reason",
			reason:         gwv1.ListenerReasonAccepted,
			message:        "Accepted",
			expectedStatus: metav1.ConditionTrue,
		},
		{
			name:           "conflict reason",
			reason:         gwv1.ListenerReasonHostnameConflict,
			message:        "Hostname conflict",
			expectedStatus: metav1.ConditionFalse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := buildConflictedCondition(tt.gw, tt.reason, tt.message)

			assert.Equal(t, string(gwv1.ListenerConditionConflicted), condition.Type)
			assert.Equal(t, tt.expectedStatus, condition.Status)
			assert.Equal(t, string(tt.reason), condition.Reason)
			assert.Equal(t, tt.message, condition.Message)
			assert.NotZero(t, condition.LastTransitionTime)
		})
	}
}

func Test_buildResolvedRefsCondition(t *testing.T) {
	tests := []struct {
		name           string
		reason         gwv1.ListenerConditionReason
		message        string
		gw             gwv1.Gateway
		expectedStatus metav1.ConditionStatus
	}{
		{
			name:           "accepted reason",
			reason:         gwv1.ListenerReasonAccepted,
			message:        "Accepted",
			expectedStatus: metav1.ConditionTrue,
		},
		{
			name:           "ref not permitted",
			reason:         gwv1.ListenerReasonRefNotPermitted,
			message:        "Ref not permitted",
			expectedStatus: metav1.ConditionFalse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := buildResolvedRefsCondition(tt.gw, tt.reason, tt.message)

			assert.Equal(t, string(gwv1.ListenerConditionResolvedRefs), condition.Type)
			assert.Equal(t, tt.expectedStatus, condition.Status)
			assert.Equal(t, string(tt.reason), condition.Reason)
			assert.Equal(t, tt.message, condition.Message)
			assert.NotZero(t, condition.LastTransitionTime)
		})
	}
}

func Test_isListenerStatusIdentical(t *testing.T) {
	fixedTime := metav1.NewTime(time.Now())

	tests := []struct {
		name              string
		listenerStatus    []gwv1.ListenerStatus
		listenerStatusOld []gwv1.ListenerStatus
		expected          bool
	}{
		{
			name:              "different lengths",
			listenerStatus:    []gwv1.ListenerStatus{{Name: "test1"}},
			listenerStatusOld: []gwv1.ListenerStatus{{Name: "test1"}, {Name: "test2"}},
			expected:          false,
		},
		{
			name:              "same length but different order",
			listenerStatus:    []gwv1.ListenerStatus{{Name: "test2"}, {Name: "test1"}},
			listenerStatusOld: []gwv1.ListenerStatus{{Name: "test1"}, {Name: "test2"}},
			expected:          true,
		},
		{
			name:              "different names",
			listenerStatus:    []gwv1.ListenerStatus{{Name: "test1"}},
			listenerStatusOld: []gwv1.ListenerStatus{{Name: "test2"}},
			expected:          false,
		},
		{
			name: "different attached routes",
			listenerStatus: []gwv1.ListenerStatus{{
				Name:           "test1",
				AttachedRoutes: 2,
			}},
			listenerStatusOld: []gwv1.ListenerStatus{{
				Name:           "test1",
				AttachedRoutes: 3,
			}},
			expected: false,
		},
		{
			name: "different condition lengths",
			listenerStatus: []gwv1.ListenerStatus{{
				Name:       "test1",
				Conditions: []metav1.Condition{{Type: "Accepted"}},
			}},
			listenerStatusOld: []gwv1.ListenerStatus{{
				Name:       "test1",
				Conditions: []metav1.Condition{{Type: "Accepted"}, {Type: "ResolvedRefs"}},
			}},
			expected: false,
		},
		{
			name: "different condition types",
			listenerStatus: []gwv1.ListenerStatus{{
				Name: "test1",
				Conditions: []metav1.Condition{{
					Type:               "Accepted",
					Status:             metav1.ConditionTrue,
					Reason:             "test-reason",
					Message:            "sample-message",
					LastTransitionTime: fixedTime,
					ObservedGeneration: 5,
				}},
			}},
			listenerStatusOld: []gwv1.ListenerStatus{{
				Name: "test1",
				Conditions: []metav1.Condition{{
					Type:               "ResolvedRefs",
					Status:             metav1.ConditionTrue,
					Reason:             "test-reason",
					Message:            "sample-message",
					LastTransitionTime: fixedTime,
					ObservedGeneration: 5,
				}},
			}},
			expected: false,
		},
		{
			name: "identical statuses",
			listenerStatus: []gwv1.ListenerStatus{{
				Name:           "test1",
				AttachedRoutes: 2,
				Conditions: []metav1.Condition{{
					Type:               "Accepted",
					Status:             metav1.ConditionTrue,
					Reason:             "test-reason",
					Message:            "sample-message",
					LastTransitionTime: fixedTime,
					ObservedGeneration: 5,
				}},
			}},
			listenerStatusOld: []gwv1.ListenerStatus{{
				Name:           "test1",
				AttachedRoutes: 2,
				Conditions: []metav1.Condition{{
					Type:               "Accepted",
					Status:             metav1.ConditionTrue,
					Reason:             "test-reason",
					Message:            "sample-message",
					LastTransitionTime: fixedTime,
					ObservedGeneration: 5,
				}},
			}},
			expected: true,
		},
		{
			name: "different ObservedGeneration",
			listenerStatus: []gwv1.ListenerStatus{{
				Name:           "test1",
				AttachedRoutes: 2,
				Conditions: []metav1.Condition{{
					Type:               "Accepted",
					Status:             metav1.ConditionTrue,
					Reason:             "test-reason",
					Message:            "sample-message",
					LastTransitionTime: fixedTime,
					ObservedGeneration: 1,
				}},
			}},
			listenerStatusOld: []gwv1.ListenerStatus{{
				Name:           "test1",
				AttachedRoutes: 2,
				Conditions: []metav1.Condition{{
					Type:               "Accepted",
					Status:             metav1.ConditionTrue,
					Reason:             "test-reason",
					Message:            "sample-message",
					LastTransitionTime: fixedTime,
					ObservedGeneration: 5,
				}},
			}},
			expected: false,
		},
		{
			name: "identical statuses but in different order",
			listenerStatus: []gwv1.ListenerStatus{
				{
					Name:           "test1",
					AttachedRoutes: 2,
					Conditions: []metav1.Condition{{
						ObservedGeneration: 5,
						Type:               "Accepted",
						Status:             metav1.ConditionTrue,
						Reason:             "test-reason",
						Message:            "sample-message",
						LastTransitionTime: fixedTime,
					}},
				},
				{
					Name:           "test2",
					AttachedRoutes: 5,
					Conditions: []metav1.Condition{{
						Type:               "Accepted",
						Status:             metav1.ConditionFalse,
						Reason:             "test-reason",
						Message:            "test-message",
						LastTransitionTime: fixedTime,
						ObservedGeneration: 5,
					}},
				},
			},
			listenerStatusOld: []gwv1.ListenerStatus{
				{
					Name:           "test2",
					AttachedRoutes: 5,
					Conditions: []metav1.Condition{{
						Type:               "Accepted",
						ObservedGeneration: 5,
						Status:             metav1.ConditionFalse,
						Reason:             "test-reason",
						Message:            "test-message",
						LastTransitionTime: fixedTime,
					}},
				},
				{
					Name:           "test1",
					AttachedRoutes: 2,
					Conditions: []metav1.Condition{{
						Status:             metav1.ConditionTrue,
						Message:            "sample-message",
						ObservedGeneration: 5,
						Type:               "Accepted",
						Reason:             "test-reason",
						LastTransitionTime: fixedTime,
					}},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isListenerStatusIdentical(tt.listenerStatus, tt.listenerStatusOld)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_compareSupportedKinds(t *testing.T) {
	group := gwv1.Group("gateway.networking.k8s.io")

	tests := []struct {
		name     string
		kinds1   []gwv1.RouteGroupKind
		kinds2   []gwv1.RouteGroupKind
		expected bool
	}{
		{
			name:     "different lengths",
			kinds1:   []gwv1.RouteGroupKind{{Group: &group, Kind: "HTTPRoute"}},
			kinds2:   []gwv1.RouteGroupKind{{Group: &group, Kind: "HTTPRoute"}, {Group: &group, Kind: "TCPRoute"}},
			expected: false,
		},
		{
			name:     "same kinds same order",
			kinds1:   []gwv1.RouteGroupKind{{Group: &group, Kind: "HTTPRoute"}},
			kinds2:   []gwv1.RouteGroupKind{{Group: &group, Kind: "HTTPRoute"}},
			expected: true,
		},
		{
			name:     "same kinds different order",
			kinds1:   []gwv1.RouteGroupKind{{Group: &group, Kind: "HTTPRoute"}, {Group: &group, Kind: "TCPRoute"}},
			kinds2:   []gwv1.RouteGroupKind{{Group: &group, Kind: "TCPRoute"}, {Group: &group, Kind: "HTTPRoute"}},
			expected: true,
		},
		{
			name:     "different kinds",
			kinds1:   []gwv1.RouteGroupKind{{Group: &group, Kind: "HTTPRoute"}},
			kinds2:   []gwv1.RouteGroupKind{{Group: &group, Kind: "TCPRoute"}},
			expected: false,
		},
		{
			name:     "duplicate kinds in first slice",
			kinds1:   []gwv1.RouteGroupKind{{Group: &group, Kind: "HTTPRoute"}, {Group: &group, Kind: "HTTPRoute"}},
			kinds2:   []gwv1.RouteGroupKind{{Group: &group, Kind: "HTTPRoute"}},
			expected: false,
		},
		{
			name:     "empty slices",
			kinds1:   []gwv1.RouteGroupKind{},
			kinds2:   []gwv1.RouteGroupKind{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareSupportedKinds(tt.kinds1, tt.kinds2)
			assert.Equal(t, tt.expected, result)
		})
	}
}
