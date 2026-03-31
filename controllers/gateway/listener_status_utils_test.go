package gateway

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func Test_buildListenerStatus(t *testing.T) {
	tests := []struct {
		name                    string
		generation              int64
		validateListenerResults routeutils.ListenerValidationResults
		supportedKinds          []gwv1.RouteGroupKind
		isProgrammed            bool
		expectedListenerCount   int
	}{
		{
			name:       "with validation results",
			generation: 2,
			validateListenerResults: routeutils.ListenerValidationResults{
				Results: map[gwv1.SectionName]routeutils.ListenerValidationResult{
					"listener1": {Reason: gwv1.ListenerReasonAccepted, Message: "accepted", SupportedKinds: []gwv1.RouteGroupKind{{
						Kind: "HTTPRoute",
					}}, AttachedRoutesCount: 1},
				},
			},
			supportedKinds: []gwv1.RouteGroupKind{{
				Kind: "HTTPRoute",
			}},
			isProgrammed:          false,
			expectedListenerCount: 1,
		},
		{
			name:                    "empty listeners",
			generation:              3,
			validateListenerResults: routeutils.ListenerValidationResults{},
			isProgrammed:            true,
			expectedListenerCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildListenerStatus(tt.generation, tt.validateListenerResults, tt.isProgrammed, generateListenerStatus)

			assert.Len(t, result, tt.expectedListenerCount)

			for i, listener := range result {
				//assert.Equal(t, tt.attachedRoutesMap[listener.Name], listener.AttachedRoutes)
				assert.Equal(t, tt.supportedKinds, listener.SupportedKinds)
				assert.Len(t, result[i].Conditions, 4)
			}
		})
	}
}

func Test_getListenerConditions(t *testing.T) {
	tests := []struct {
		name                     string
		listenerValidationResult routeutils.ListenerValidationResult
		isProgrammed             bool
		expectedConditionCount   int
		expectedConflictReason   string
		expectedAcceptedReason   string
		expectedResolvedReason   string
		expectedProgrammedReason string
		generation               int64
	}{
		{
			name: "validation result with hostname conflict and programmed false",
			listenerValidationResult: routeutils.ListenerValidationResult{
				Reason:  gwv1.ListenerReasonHostnameConflict,
				Message: "Hostname conflict",
			},
			isProgrammed:             false,
			expectedConditionCount:   4,
			expectedConflictReason:   string(gwv1.ListenerReasonHostnameConflict),
			expectedAcceptedReason:   string(gwv1.ListenerReasonHostnameConflict),
			expectedResolvedReason:   string(gwv1.ListenerReasonResolvedRefs),
			expectedProgrammedReason: string(gwv1.ListenerReasonHostnameConflict),
			generation:               3,
		},
		{
			name: "validation result with protocol conflict",
			listenerValidationResult: routeutils.ListenerValidationResult{
				Reason:  gwv1.ListenerReasonProtocolConflict,
				Message: "Protocol conflict",
			},
			isProgrammed:             false,
			expectedConditionCount:   4,
			expectedConflictReason:   string(gwv1.ListenerReasonProtocolConflict),
			expectedAcceptedReason:   string(gwv1.ListenerReasonProtocolConflict),
			expectedResolvedReason:   string(gwv1.ListenerReasonResolvedRefs),
			expectedProgrammedReason: string(gwv1.ListenerReasonProtocolConflict),
			generation:               4,
		},
		{
			name: "validation result with port unavailable",
			listenerValidationResult: routeutils.ListenerValidationResult{
				Reason:  gwv1.ListenerReasonPortUnavailable,
				Message: "Port unavailable",
			},
			isProgrammed:             false,
			expectedConditionCount:   4,
			expectedConflictReason:   string(gwv1.ListenerReasonNoConflicts),
			expectedAcceptedReason:   string(gwv1.ListenerReasonPortUnavailable),
			expectedResolvedReason:   string(gwv1.ListenerReasonResolvedRefs),
			expectedProgrammedReason: string(gwv1.ListenerReasonPortUnavailable),
			generation:               5,
		},
		{
			name: "validation result with unsupported protocol",
			listenerValidationResult: routeutils.ListenerValidationResult{
				Reason:  gwv1.ListenerReasonUnsupportedProtocol,
				Message: "Unsupported protocol",
			},
			isProgrammed:             false,
			expectedConditionCount:   4,
			expectedConflictReason:   string(gwv1.ListenerReasonNoConflicts),
			expectedAcceptedReason:   string(gwv1.ListenerReasonUnsupportedProtocol),
			expectedResolvedReason:   string(gwv1.ListenerReasonResolvedRefs),
			expectedProgrammedReason: string(gwv1.ListenerReasonUnsupportedProtocol),
			generation:               6,
		},
		{
			name: "validation result with invalid route kinds",
			listenerValidationResult: routeutils.ListenerValidationResult{
				Reason:  gwv1.ListenerReasonInvalidRouteKinds,
				Message: "Invalid route kinds",
			},
			isProgrammed:             false,
			expectedConditionCount:   4,
			expectedConflictReason:   string(gwv1.ListenerReasonNoConflicts),
			expectedAcceptedReason:   string(gwv1.ListenerReasonAccepted),
			expectedResolvedReason:   string(gwv1.ListenerReasonInvalidRouteKinds),
			expectedProgrammedReason: string(gwv1.ListenerReasonInvalidRouteKinds),
			generation:               7,
		},
		{
			name: "validation result with ref not permitted",
			listenerValidationResult: routeutils.ListenerValidationResult{
				Reason:  gwv1.ListenerReasonRefNotPermitted,
				Message: "Ref not permitted",
			},
			isProgrammed:             false,
			expectedConditionCount:   4,
			expectedConflictReason:   string(gwv1.ListenerReasonNoConflicts),
			expectedAcceptedReason:   string(gwv1.ListenerReasonAccepted),
			expectedResolvedReason:   string(gwv1.ListenerReasonRefNotPermitted),
			expectedProgrammedReason: string(gwv1.ListenerReasonRefNotPermitted),
			generation:               8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conditions := getListenerConditions(tt.generation, tt.listenerValidationResult, tt.isProgrammed)

			assert.Len(t, conditions, tt.expectedConditionCount)

			// Find each condition type and verify
			conditionMap := make(map[string]metav1.Condition)
			for _, condition := range conditions {
				conditionMap[condition.Type] = condition
			}

			// Verify Conflicted condition
			conflictCondition := conditionMap[string(gwv1.ListenerConditionConflicted)]
			assert.Equal(t, tt.expectedConflictReason, conflictCondition.Reason)

			// Verify Accepted condition
			acceptedCondition := conditionMap[string(gwv1.ListenerConditionAccepted)]
			assert.Equal(t, tt.expectedAcceptedReason, acceptedCondition.Reason)

			// Verify ResolvedRefs condition
			resolvedCondition := conditionMap[string(gwv1.ListenerConditionResolvedRefs)]
			assert.Equal(t, tt.expectedResolvedReason, resolvedCondition.Reason)

			// Verify Programmed condition
			programmedCondition := conditionMap[string(gwv1.ListenerConditionProgrammed)]
			assert.Equal(t, tt.expectedProgrammedReason, programmedCondition.Reason)

			// Verify all conditions have proper ObservedGeneration
			for _, condition := range conditions {
				assert.Equal(t, tt.generation, condition.ObservedGeneration)
				assert.NotZero(t, condition.LastTransitionTime)
			}
		})
	}
}

func Test_buildProgrammedCondition(t *testing.T) {
	tests := []struct {
		name           string
		isProgrammed   bool
		listenerReason string
		expectedStatus metav1.ConditionStatus
		expectedReason string
		generation     int64
	}{
		{
			name:           "not accepted - hostname conflict",
			isProgrammed:   true,
			listenerReason: string(gwv1.ListenerReasonHostnameConflict),
			expectedStatus: metav1.ConditionFalse,
			expectedReason: string(gwv1.ListenerReasonHostnameConflict),
			generation:     5,
		},
		{
			name:           "accepted and programmed - should return true with programmed reason",
			isProgrammed:   true,
			listenerReason: string(gwv1.ListenerReasonAccepted),
			expectedStatus: metav1.ConditionTrue,
			expectedReason: string(gwv1.ListenerReasonProgrammed),
			generation:     3,
		},
		{
			name:           "accepted but not programmed - should return false with pending reason",
			isProgrammed:   false,
			listenerReason: string(gwv1.ListenerReasonAccepted),
			expectedStatus: metav1.ConditionFalse,
			expectedReason: string(gwv1.ListenerReasonPending),
			generation:     1,
		},
		{
			name:           "not accepted and not programmed - should return false with invalid reason",
			isProgrammed:   false,
			listenerReason: string(gwv1.ListenerReasonHostnameConflict),
			expectedStatus: metav1.ConditionFalse,
			expectedReason: string(gwv1.ListenerReasonHostnameConflict),
			generation:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := buildProgrammedCondition(tt.generation, tt.isProgrammed, tt.listenerReason)

			assert.Equal(t, string(gwv1.ListenerConditionProgrammed), condition.Type)
			assert.Equal(t, tt.expectedStatus, condition.Status)
			assert.Equal(t, tt.expectedReason, condition.Reason)
			assert.Equal(t, tt.generation, condition.ObservedGeneration)
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
			condition := buildAcceptedCondition(0, tt.reason, tt.message)

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
	}{
		{
			name:           "accepted reason",
			reason:         gwv1.ListenerReasonNoConflicts,
			message:        "Accepted",
			expectedStatus: metav1.ConditionFalse,
		},
		{
			name:           "conflict reason",
			reason:         gwv1.ListenerReasonHostnameConflict,
			message:        "Hostname conflict",
			expectedStatus: metav1.ConditionTrue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := buildConflictedCondition(0, tt.reason, tt.message)

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
		expectedStatus metav1.ConditionStatus
	}{
		{
			name:           "accepted reason",
			reason:         gwv1.ListenerReasonResolvedRefs,
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
			condition := buildResolvedRefsCondition(0, tt.reason, tt.message)

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
		{
			name: "multiple types in conditions - same content but different order",
			listenerStatus: []gwv1.ListenerStatus{{
				Name: "test1",
				Conditions: []metav1.Condition{
					{
						Type:               "Accepted",
						Status:             metav1.ConditionTrue,
						Reason:             "test-reason",
						Message:            "sample-message",
						LastTransitionTime: fixedTime,
						ObservedGeneration: 5,
					},
					{
						Type:               "ResolvedRefs",
						Status:             metav1.ConditionTrue,
						Reason:             "test-reason",
						Message:            "sample-message",
						LastTransitionTime: fixedTime,
						ObservedGeneration: 5,
					},
					{
						Type:               "Programmed",
						Status:             metav1.ConditionTrue,
						Reason:             "test-reason",
						Message:            "sample-message",
						LastTransitionTime: fixedTime,
						ObservedGeneration: 5,
					},
				},
			}},
			listenerStatusOld: []gwv1.ListenerStatus{{
				Name: "test1",
				Conditions: []metav1.Condition{
					{
						Type:               "Programmed",
						Message:            "sample-message",
						LastTransitionTime: fixedTime,
						ObservedGeneration: 5,
						Status:             metav1.ConditionTrue,
						Reason:             "test-reason",
					},
					{
						Type:               "Accepted",
						LastTransitionTime: fixedTime,
						Status:             metav1.ConditionTrue,
						Reason:             "test-reason",
						Message:            "sample-message",
						ObservedGeneration: 5,
					},
					{
						Type:               "ResolvedRefs",
						LastTransitionTime: fixedTime,
						ObservedGeneration: 5,
						Status:             metav1.ConditionTrue,
						Reason:             "test-reason",
						Message:            "sample-message",
					},
				},
			}},
			expected: true,
		},
		{
			name: "multiple conditions in one listener with different length",
			listenerStatus: []gwv1.ListenerStatus{
				{
					Name:           "test1",
					AttachedRoutes: 2,
					Conditions: []metav1.Condition{
						{
							ObservedGeneration: 5,
							Type:               "Accepted",
							Status:             metav1.ConditionTrue,
							Reason:             "test-reason",
							Message:            "sample-message",
							LastTransitionTime: fixedTime,
						},
						{
							ObservedGeneration: 5,
							Type:               "ResolvedRefs",
							Status:             metav1.ConditionTrue,
							Reason:             "test-reason",
							Message:            "sample-message",
							LastTransitionTime: fixedTime,
						},
					},
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
			expected: false,
		},
		{
			name: "multiple conditions in one listener with same length different order",
			listenerStatus: []gwv1.ListenerStatus{
				{
					Name:           "test1",
					AttachedRoutes: 2,
					Conditions: []metav1.Condition{
						{
							ObservedGeneration: 5,
							Type:               "Accepted",
							Status:             metav1.ConditionTrue,
							Reason:             "test-reason",
							Message:            "sample-message",
							LastTransitionTime: fixedTime,
						},
						{
							ObservedGeneration: 5,
							Type:               "ResolvedRefs",
							Status:             metav1.ConditionTrue,
							Reason:             "test-reason",
							Message:            "sample-message",
							LastTransitionTime: fixedTime,
						},
					},
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
					Conditions: []metav1.Condition{
						{
							ObservedGeneration: 5,
							Type:               "ResolvedRefs",
							Status:             metav1.ConditionTrue,
							Reason:             "test-reason",
							Message:            "sample-message",
							LastTransitionTime: fixedTime,
						},
						{
							Status:             metav1.ConditionTrue,
							Message:            "sample-message",
							ObservedGeneration: 5,
							Type:               "Accepted",
							Reason:             "test-reason",
							LastTransitionTime: fixedTime,
						},
					},
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

func Test_buildListenerSetStatus(t *testing.T) {
	tests := []struct {
		name                      string
		listenerSetNsn            types.NamespacedName
		results                   routeutils.ListenerValidationResults
		isGatewayProgrammed       bool
		expectedAccepted          bool
		expectedAcceptedReason    string
		expectedAcceptedMessage   string
		expectedProgrammed        bool
		expectedProgrammedReason  string
		expectedProgrammedMessage string
		expectedGeneration        int64
		expectedListenerCount     int
	}{
		{
			name: "all valid listeners, gateway programmed",
			listenerSetNsn: types.NamespacedName{
				Namespace: "test-ns",
				Name:      "test-ls",
			},
			results: routeutils.ListenerValidationResults{
				Results: map[gwv1.SectionName]routeutils.ListenerValidationResult{
					"listener1": {
						IsValid:             true,
						Reason:              gwv1.ListenerReasonAccepted,
						Message:             "accepted",
						SupportedKinds:      []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
						AttachedRoutesCount: 2,
					},
				},
				Generation: 5,
				HasErrors:  false,
			},
			isGatewayProgrammed:       true,
			expectedAccepted:          true,
			expectedAcceptedReason:    string(gwv1.ListenerSetReasonAccepted),
			expectedAcceptedMessage:   string(gwv1.ListenerSetReasonAccepted),
			expectedProgrammed:        true,
			expectedProgrammedReason:  string(gwv1.ListenerSetReasonProgrammed),
			expectedProgrammedMessage: string(gwv1.ListenerSetReasonProgrammed),
			expectedGeneration:        5,
			expectedListenerCount:     1,
		},
		{
			name: "has errors but some valid listeners, gateway programmed",
			listenerSetNsn: types.NamespacedName{
				Namespace: "test-ns",
				Name:      "test-ls",
			},
			results: routeutils.ListenerValidationResults{
				Results: map[gwv1.SectionName]routeutils.ListenerValidationResult{
					"listener1": {
						IsValid:        true,
						Reason:         gwv1.ListenerReasonAccepted,
						Message:        "accepted",
						SupportedKinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
					},
					"listener2": {
						IsValid: false,
						Reason:  gwv1.ListenerReasonInvalidRouteKinds,
						Message: "invalid",
					},
				},
				Generation: 3,
				HasErrors:  true,
			},
			isGatewayProgrammed:       true,
			expectedAccepted:          true,
			expectedAcceptedReason:    string(gwv1.ListenerSetReasonListenersNotValid),
			expectedAcceptedMessage:   "Some listeners are not valid",
			expectedProgrammed:        true,
			expectedProgrammedReason:  string(gwv1.ListenerSetReasonProgrammed),
			expectedProgrammedMessage: string(gwv1.ListenerSetReasonProgrammed),
			expectedGeneration:        3,
			expectedListenerCount:     2,
		},
		{
			name: "no valid listeners, gateway programmed",
			listenerSetNsn: types.NamespacedName{
				Namespace: "test-ns",
				Name:      "test-ls",
			},
			results: routeutils.ListenerValidationResults{
				Results: map[gwv1.SectionName]routeutils.ListenerValidationResult{
					"listener1": {
						IsValid: false,
						Reason:  gwv1.ListenerReasonInvalidRouteKinds,
						Message: "invalid",
					},
				},
				Generation: 2,
				HasErrors:  true,
			},
			isGatewayProgrammed:       true,
			expectedAccepted:          false,
			expectedAcceptedReason:    string(gwv1.ListenerSetReasonListenersNotValid),
			expectedAcceptedMessage:   "Some listeners are not valid",
			expectedProgrammed:        false,
			expectedProgrammedReason:  string(gwv1.ListenerSetReasonListenersNotValid),
			expectedProgrammedMessage: "No valid listeners to materialize",
			expectedGeneration:        2,
			expectedListenerCount:     1,
		},
		{
			name: "valid listeners but gateway not programmed",
			listenerSetNsn: types.NamespacedName{
				Namespace: "test-ns",
				Name:      "test-ls",
			},
			results: routeutils.ListenerValidationResults{
				Results: map[gwv1.SectionName]routeutils.ListenerValidationResult{
					"listener1": {
						IsValid:        true,
						Reason:         gwv1.ListenerReasonAccepted,
						Message:        "accepted",
						SupportedKinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
					},
				},
				Generation: 4,
				HasErrors:  false,
			},
			isGatewayProgrammed:       false,
			expectedAccepted:          true,
			expectedAcceptedReason:    string(gwv1.ListenerSetReasonAccepted),
			expectedAcceptedMessage:   string(gwv1.ListenerSetReasonAccepted),
			expectedProgrammed:        false,
			expectedProgrammedReason:  string(gwv1.ListenerSetReasonPending),
			expectedProgrammedMessage: "Parent gateway not yet programmed",
			expectedGeneration:        4,
			expectedListenerCount:     1,
		},
		{
			name: "no valid listeners and gateway not programmed",
			listenerSetNsn: types.NamespacedName{
				Namespace: "test-ns",
				Name:      "test-ls",
			},
			results: routeutils.ListenerValidationResults{
				Results: map[gwv1.SectionName]routeutils.ListenerValidationResult{
					"listener1": {
						IsValid: false,
						Reason:  gwv1.ListenerReasonInvalidRouteKinds,
						Message: "invalid",
					},
				},
				Generation: 1,
				HasErrors:  true,
			},
			isGatewayProgrammed:       false,
			expectedAccepted:          false,
			expectedAcceptedReason:    string(gwv1.ListenerSetReasonListenersNotValid),
			expectedAcceptedMessage:   "Some listeners are not valid",
			expectedProgrammed:        false,
			expectedProgrammedReason:  string(gwv1.ListenerSetReasonPending),
			expectedProgrammedMessage: "Parent gateway not yet programmed",
			expectedGeneration:        1,
			expectedListenerCount:     1,
		},
		{
			name: "empty results, gateway programmed",
			listenerSetNsn: types.NamespacedName{
				Namespace: "default",
				Name:      "empty-ls",
			},
			results: routeutils.ListenerValidationResults{
				Results:    map[gwv1.SectionName]routeutils.ListenerValidationResult{},
				Generation: 7,
				HasErrors:  false,
			},
			isGatewayProgrammed:       true,
			expectedAccepted:          false,
			expectedAcceptedReason:    string(gwv1.ListenerSetReasonAccepted),
			expectedAcceptedMessage:   string(gwv1.ListenerSetReasonAccepted),
			expectedProgrammed:        false,
			expectedProgrammedReason:  string(gwv1.ListenerSetReasonListenersNotValid),
			expectedProgrammedMessage: "No valid listeners to materialize",
			expectedGeneration:        7,
			expectedListenerCount:     0,
		},
		{
			name: "multiple valid listeners, gateway programmed",
			listenerSetNsn: types.NamespacedName{
				Namespace: "prod-ns",
				Name:      "multi-ls",
			},
			results: routeutils.ListenerValidationResults{
				Results: map[gwv1.SectionName]routeutils.ListenerValidationResult{
					"listener1": {
						IsValid:             true,
						Reason:              gwv1.ListenerReasonAccepted,
						Message:             "accepted",
						SupportedKinds:      []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
						AttachedRoutesCount: 3,
					},
					"listener2": {
						IsValid:             true,
						Reason:              gwv1.ListenerReasonAccepted,
						Message:             "accepted",
						SupportedKinds:      []gwv1.RouteGroupKind{{Kind: "TCPRoute"}},
						AttachedRoutesCount: 1,
					},
				},
				Generation: 10,
				HasErrors:  false,
			},
			isGatewayProgrammed:       true,
			expectedAccepted:          true,
			expectedAcceptedReason:    string(gwv1.ListenerSetReasonAccepted),
			expectedAcceptedMessage:   string(gwv1.ListenerSetReasonAccepted),
			expectedProgrammed:        true,
			expectedProgrammedReason:  string(gwv1.ListenerSetReasonProgrammed),
			expectedProgrammedMessage: string(gwv1.ListenerSetReasonProgrammed),
			expectedGeneration:        10,
			expectedListenerCount:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statusData, listenerEntryStatuses := buildListenerSetStatus(tt.listenerSetNsn, tt.results, tt.isGatewayProgrammed)

			// Verify metadata
			assert.Equal(t, tt.listenerSetNsn.Name, statusData.ListenerSetMetadata.ListenerSetName)
			assert.Equal(t, tt.listenerSetNsn.Namespace, statusData.ListenerSetMetadata.ListenerSetNamespace)
			assert.Equal(t, tt.expectedGeneration, statusData.ListenerSetMetadata.Generation)

			// Verify accepted status
			assert.Equal(t, tt.expectedAccepted, statusData.ListenerSetStatusInfo.Accepted)
			assert.Equal(t, tt.expectedAcceptedReason, statusData.ListenerSetStatusInfo.AcceptedReason)
			assert.Equal(t, tt.expectedAcceptedMessage, statusData.ListenerSetStatusInfo.AcceptedMessage)

			// Verify programmed status
			assert.Equal(t, tt.expectedProgrammed, statusData.ListenerSetStatusInfo.Programmed)
			assert.Equal(t, tt.expectedProgrammedReason, statusData.ListenerSetStatusInfo.ProgrammedReason)
			assert.Equal(t, tt.expectedProgrammedMessage, statusData.ListenerSetStatusInfo.ProgrammedMessage)

			// Verify listener entry statuses count
			assert.Len(t, listenerEntryStatuses, tt.expectedListenerCount)

			// Verify each listener entry has 4 conditions
			for _, les := range listenerEntryStatuses {
				assert.Len(t, les.Conditions, 4)
			}
		})
	}
}
