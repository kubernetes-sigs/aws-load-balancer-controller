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
		gateway                 gwv1.Gateway
		attachedRoutesMap       map[gwv1.SectionName]int32
		validateListenerResults routeutils.ListenerValidationResults
		supportedKinds          []gwv1.RouteGroupKind
		isProgrammed            bool
		expectedListenerCount   int
	}{
		{
			name: "with validation results",
			gateway: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
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
			attachedRoutesMap: map[gwv1.SectionName]int32{"listener1": 1},
			validateListenerResults: routeutils.ListenerValidationResults{
				Results: map[gwv1.SectionName]routeutils.ListenerValidationResult{
					"listener1": {Reason: gwv1.ListenerReasonAccepted, Message: "accepted", SupportedKinds: []gwv1.RouteGroupKind{{
						Kind: "HTTPRoute",
					}}},
				},
			},
			supportedKinds: []gwv1.RouteGroupKind{{
				Kind: "HTTPRoute",
			}},
			isProgrammed:          false,
			expectedListenerCount: 1,
		},
		{
			name: "empty listeners",
			gateway: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{},
				},
			},
			attachedRoutesMap:       map[gwv1.SectionName]int32{},
			validateListenerResults: routeutils.ListenerValidationResults{},
			isProgrammed:            true,
			expectedListenerCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildListenerStatus(tt.gateway, tt.gateway.Spec.Listeners, tt.attachedRoutesMap, tt.validateListenerResults, tt.isProgrammed)

			assert.Len(t, result, tt.expectedListenerCount)

			for i, listener := range tt.gateway.Spec.Listeners {
				assert.Equal(t, listener.Name, result[i].Name)
				assert.Equal(t, tt.attachedRoutesMap[listener.Name], result[i].AttachedRoutes)
				assert.Equal(t, tt.supportedKinds, result[i].SupportedKinds)
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
		gw                       gwv1.Gateway
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
			expectedAcceptedReason:   string(gwv1.ListenerReasonAccepted),
			expectedResolvedReason:   string(gwv1.ListenerReasonResolvedRefs),
			expectedProgrammedReason: string(gwv1.ListenerReasonInvalid),
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
			},
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
			expectedAcceptedReason:   string(gwv1.ListenerReasonAccepted),
			expectedResolvedReason:   string(gwv1.ListenerReasonResolvedRefs),
			expectedProgrammedReason: string(gwv1.ListenerReasonInvalid),
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Generation: 4},
			},
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
			expectedProgrammedReason: string(gwv1.ListenerReasonInvalid),
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Generation: 5},
			},
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
			expectedProgrammedReason: string(gwv1.ListenerReasonInvalid),
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Generation: 6},
			},
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
			expectedProgrammedReason: string(gwv1.ListenerReasonInvalid),
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Generation: 7},
			},
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
			expectedProgrammedReason: string(gwv1.ListenerReasonInvalid),
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Generation: 8},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conditions := getListenerConditions(tt.gw, tt.listenerValidationResult, tt.isProgrammed)

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
				assert.Equal(t, tt.gw.GetGeneration(), condition.ObservedGeneration)
				assert.NotZero(t, condition.LastTransitionTime)
			}
		})
	}
}

func Test_buildProgrammedCondition(t *testing.T) {
	tests := []struct {
		name           string
		isProgrammed   bool
		isAccepted     bool
		expectedStatus metav1.ConditionStatus
		expectedReason string
		gw             gwv1.Gateway
	}{
		{
			name:           "not accepted - should return false with invalid reason",
			isProgrammed:   true,
			isAccepted:     false,
			expectedStatus: metav1.ConditionFalse,
			expectedReason: string(gwv1.ListenerReasonInvalid),
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 5,
				},
			},
		},
		{
			name:           "accepted and programmed - should return true with programmed reason",
			isProgrammed:   true,
			isAccepted:     true,
			expectedStatus: metav1.ConditionTrue,
			expectedReason: string(gwv1.ListenerReasonProgrammed),
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 3,
				},
			},
		},
		{
			name:           "accepted but not programmed - should return false with pending reason",
			isProgrammed:   false,
			isAccepted:     true,
			expectedStatus: metav1.ConditionFalse,
			expectedReason: string(gwv1.ListenerReasonPending),
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
			},
		},
		{
			name:           "not accepted and not programmed - should return false with invalid reason",
			isProgrammed:   false,
			isAccepted:     false,
			expectedStatus: metav1.ConditionFalse,
			expectedReason: string(gwv1.ListenerReasonInvalid),
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := buildProgrammedCondition(tt.gw, tt.isProgrammed, tt.isAccepted)

			assert.Equal(t, string(gwv1.ListenerConditionProgrammed), condition.Type)
			assert.Equal(t, tt.expectedStatus, condition.Status)
			assert.Equal(t, tt.expectedReason, condition.Reason)
			assert.Equal(t, tt.gw.GetGeneration(), condition.ObservedGeneration)
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
			reason:         gwv1.ListenerReasonNoConflicts,
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
