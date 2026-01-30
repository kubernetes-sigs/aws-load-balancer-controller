package gateway

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func Test_updateGatewayClassLastProcessedConfig(t *testing.T) {
	testCases := []struct {
		name            string
		gwClass         gwv1.GatewayClass
		lbConf          *elbv2gw.LoadBalancerConfiguration
		expectedVersion string
		noPatch         bool
	}{
		{
			name: "no lb conf, no prior annotation",
			gwClass: gwv1.GatewayClass{
				ObjectMeta: v1.ObjectMeta{
					Name: "gwclass",
				},
			},
			expectedVersion: "",
		},
		{
			name: "no lb conf, with prior annotation",
			gwClass: gwv1.GatewayClass{
				ObjectMeta: v1.ObjectMeta{
					Name: "gwclass",
					Annotations: map[string]string{
						gatewayClassAnnotationLastProcessedConfig:          "foo",
						gatewayClassAnnotationLastProcessedConfigTimestamp: "0",
					},
				},
			},
			expectedVersion: "",
		},
		{
			name: "with lb conf, no prior annotation",
			lbConf: &elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: v1.ObjectMeta{
					ResourceVersion: "bar",
				},
			},
			gwClass: gwv1.GatewayClass{
				ObjectMeta: v1.ObjectMeta{
					Name: "gwclass",
				},
			},
			expectedVersion: "bar",
		},
		{
			name: "with lb conf, with prior annotation",
			lbConf: &elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: v1.ObjectMeta{
					ResourceVersion: "bar",
				},
			},
			gwClass: gwv1.GatewayClass{
				ObjectMeta: v1.ObjectMeta{
					Name: "gwclass",
					Annotations: map[string]string{
						gatewayClassAnnotationLastProcessedConfig:          "foo",
						gatewayClassAnnotationLastProcessedConfigTimestamp: "0",
					},
				},
			},
			expectedVersion: "bar",
		},
		{
			name: "no change in stored version should not trigger patch",
			lbConf: &elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: v1.ObjectMeta{
					ResourceVersion: "foo",
				},
			},
			gwClass: gwv1.GatewayClass{
				ObjectMeta: v1.ObjectMeta{
					Name: "gwclass",
					Annotations: map[string]string{
						gatewayClassAnnotationLastProcessedConfig:          "foo",
						gatewayClassAnnotationLastProcessedConfigTimestamp: "10",
					},
				},
			},
			expectedVersion: "foo",
			noPatch:         true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutils.GenerateTestClient()
			original := tc.gwClass.DeepCopy()
			err := client.Create(context.Background(), original)
			assert.NoError(t, err)
			err = updateGatewayClassLastProcessedConfig(context.Background(), client, original, tc.lbConf)
			assert.NoError(t, err)
			stored := &gwv1.GatewayClass{}
			err = client.Get(context.Background(), k8s.NamespacedName(original), stored)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedVersion, stored.Annotations[gatewayClassAnnotationLastProcessedConfig])

			ts, err := strconv.Atoi(stored.Annotations[gatewayClassAnnotationLastProcessedConfigTimestamp])
			assert.NoError(t, err)
			assert.NotZero(t, ts)
			if tc.noPatch {
				assert.Equal(t, tc.gwClass.Annotations[gatewayClassAnnotationLastProcessedConfigTimestamp], stored.Annotations[gatewayClassAnnotationLastProcessedConfigTimestamp])
			}
		})
	}
}

func Test_updateGatewayClassAcceptedCondition(t *testing.T) {
	testCases := []struct {
		name    string
		gwClass gwv1.GatewayClass
		status  metav1.ConditionStatus
		reason  string
		message string

		expectedConditions []metav1.Condition
		noPatch            bool
	}{
		{
			name: "nil conditions",
			gwClass: gwv1.GatewayClass{
				ObjectMeta: v1.ObjectMeta{
					Name: "gwclass",
				},
			},
		},
		{
			name: "no conditions",
			gwClass: gwv1.GatewayClass{
				ObjectMeta: v1.ObjectMeta{
					Name: "gwclass",
				},
				Status: gwv1.GatewayClassStatus{
					Conditions: make([]v1.Condition, 0),
				},
			},
		},
		{
			name:    "flip condition to true",
			status:  metav1.ConditionTrue,
			reason:  "flip to true",
			message: "test message",
			gwClass: gwv1.GatewayClass{
				ObjectMeta: v1.ObjectMeta{
					Name:       "gwclass-flip-true",
					Generation: 100,
				},
				Status: gwv1.GatewayClassStatus{
					Conditions: []v1.Condition{
						{
							Type:               "other condition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 100,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               string(gwv1.GatewayClassReasonAccepted),
							Status:             metav1.ConditionFalse,
							ObservedGeneration: 100,
							Reason:             "",
							Message:            "",
						},
					},
				},
			},
			expectedConditions: []v1.Condition{
				{
					Type:               "other condition",
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 100,
					Reason:             "other reason",
					Message:            "other message",
				},
				{
					Type:               string(gwv1.GatewayClassReasonAccepted),
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 100,
					Reason:             "flip to true",
					Message:            "test message",
				},
			},
		},
		{
			name:    "flip condition to false",
			status:  metav1.ConditionFalse,
			reason:  "flip to false",
			message: "test message",
			gwClass: gwv1.GatewayClass{
				ObjectMeta: v1.ObjectMeta{
					Name:       "gwclass-flip",
					Generation: 100,
				},
				Status: gwv1.GatewayClassStatus{
					Conditions: []v1.Condition{
						{
							Type:               "other condition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 100,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               string(gwv1.GatewayClassReasonAccepted),
							Status:             metav1.ConditionFalse,
							ObservedGeneration: 100,
							Reason:             "",
							Message:            "",
						},
					},
				},
			},
			expectedConditions: []v1.Condition{
				{
					Type:               "other condition",
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 100,
					Reason:             "other reason",
					Message:            "other message",
				},
				{
					Type:               string(gwv1.GatewayClassReasonAccepted),
					Status:             metav1.ConditionFalse,
					ObservedGeneration: 100,
					Reason:             "flip to false",
					Message:            "test message",
				},
			},
		},
		{
			name:    "no change results in no patch",
			status:  metav1.ConditionFalse,
			reason:  "reason",
			message: "msg",
			gwClass: gwv1.GatewayClass{
				ObjectMeta: v1.ObjectMeta{
					Name:       "gwclass-flip",
					Generation: 100,
				},
				Status: gwv1.GatewayClassStatus{
					Conditions: []v1.Condition{
						{
							Type:               "other condition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 100,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               string(gwv1.GatewayClassReasonAccepted),
							Status:             metav1.ConditionFalse,
							ObservedGeneration: 100,
							Reason:             "reason",
							Message:            "msg",
						},
					},
				},
			},
			expectedConditions: []v1.Condition{
				{
					Type:               "other condition",
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 100,
					Reason:             "other reason",
					Message:            "other message",
				},
				{
					Type:               string(gwv1.GatewayClassReasonAccepted),
					Status:             metav1.ConditionFalse,
					ObservedGeneration: 100,
					Reason:             "reason",
					Message:            "msg",
				},
			},
		},
		{
			name:    "update observation generation in Accepted condition result in patch",
			status:  metav1.ConditionFalse,
			reason:  "reason",
			message: "msg",
			gwClass: gwv1.GatewayClass{
				ObjectMeta: v1.ObjectMeta{
					Name:       "gwclass-flip",
					Generation: 100,
				},
				Status: gwv1.GatewayClassStatus{
					Conditions: []v1.Condition{
						{
							Type:               "other condition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 100,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               string(gwv1.GatewayClassReasonAccepted),
							Status:             metav1.ConditionFalse,
							ObservedGeneration: 99,
							Reason:             "reason",
							Message:            "msg",
						},
					},
				},
			},
			expectedConditions: []v1.Condition{
				{
					Type:               "other condition",
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 100,
					Reason:             "other reason",
					Message:            "other message",
				},
				{
					Type:               string(gwv1.GatewayClassReasonAccepted),
					Status:             metav1.ConditionFalse,
					ObservedGeneration: 100,
					Reason:             "reason",
					Message:            "msg",
				},
			},
		},
		{
			name:    "update observation generation in other conditions result in no patch",
			status:  metav1.ConditionFalse,
			reason:  "reason",
			message: "msg",
			gwClass: gwv1.GatewayClass{
				ObjectMeta: v1.ObjectMeta{
					Name:       "gwclass-flip",
					Generation: 100,
				},
				Status: gwv1.GatewayClassStatus{
					Conditions: []v1.Condition{
						{
							Type:               "other condition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 99,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               string(gwv1.GatewayClassReasonAccepted),
							Status:             metav1.ConditionFalse,
							ObservedGeneration: 100,
							Reason:             "reason",
							Message:            "msg",
						},
					},
				},
			},
			expectedConditions: []v1.Condition{
				{
					Type:               "other condition",
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 99,
					Reason:             "other reason",
					Message:            "other message",
				},
				{
					Type:               string(gwv1.GatewayClassReasonAccepted),
					Status:             metav1.ConditionFalse,
					ObservedGeneration: 100,
					Reason:             "reason",
					Message:            "msg",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := testutils.GenerateTestClient()
			err := mockClient.Create(context.Background(), &tc.gwClass)
			assert.NoError(t, err)
			time.Sleep(1 * time.Second)

			stored := &gwv1.GatewayClass{}
			err = mockClient.Get(context.Background(), k8s.NamespacedName(&tc.gwClass), stored)
			assert.NoError(t, err)

			err = updateGatewayClassAcceptedCondition(context.Background(), mockClient, tc.gwClass.DeepCopy(), tc.status, tc.reason, tc.message)
			assert.NoError(t, err)
			stored = &gwv1.GatewayClass{}
			err = mockClient.Get(context.Background(), k8s.NamespacedName(&tc.gwClass), stored)
			assert.NoError(t, err)

			fixedTime := metav1.NewTime(time.Now())

			// In order to use equals(), we need to make the time fields are fixed.
			if tc.expectedConditions != nil {
				for i := range tc.expectedConditions {
					tmp := &tc.expectedConditions[i]
					tmp.LastTransitionTime = fixedTime
				}
			}

			if stored.Status.Conditions != nil {
				for i := range stored.Status.Conditions {
					tmp := &stored.Status.Conditions[i]
					tmp.LastTransitionTime = fixedTime
				}
			}

			assert.Equal(t, tc.expectedConditions, stored.Status.Conditions)
		})
	}
}

func Test_prepareGatewayConditionUpdate(t *testing.T) {

	longString := ""
	for i := 0; i < 50000; i++ {
		longString = fmt.Sprintf("%s%s", longString, "a")
	}
	truncatedString := ""
	for i := 0; i < 32700; i++ {
		truncatedString = fmt.Sprintf("%s%s", truncatedString, "a")
	}
	truncatedString = fmt.Sprintf("%s...", truncatedString)

	testCases := []struct {
		name                string
		gw                  gwv1.Gateway
		targetConditionType string
		newStatus           metav1.ConditionStatus
		reason              string
		message             string

		expectedGw gwv1.Gateway
		expected   bool
	}{
		{
			name: "target condition not found",
			gw: gwv1.Gateway{
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "other condition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 1000,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               "other condition2",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 1001,
							Reason:             "other reason2",
							Message:            "other message2",
						},
					},
				},
			},
			expectedGw: gwv1.Gateway{
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "other condition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 1000,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               "other condition2",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 1001,
							Reason:             "other reason2",
							Message:            "other message2",
						},
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 0,
							Reason:             "other reason",
							Message:            "other message",
						},
					},
				},
			},
			targetConditionType: string(gwv1.GatewayConditionAccepted),
			newStatus:           metav1.ConditionTrue,
			reason:              "other reason",
			message:             "other message",
			expected:            true,
		},
		{
			name: "target condition found",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 50,
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "other condition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionFalse,
							ObservedGeneration: 50,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               "other condition2",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason2",
							Message:            "other message2",
						},
					},
				},
			},
			expectedGw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 50,
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "other condition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionTrue,
							Reason:             "new reason",
							Message:            "new message",
							ObservedGeneration: 50,
						},
						{
							Type:               "other condition2",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason2",
							Message:            "other message2",
						},
					},
				},
			},
			targetConditionType: string(gwv1.GatewayConditionAccepted),
			newStatus:           metav1.ConditionTrue,
			reason:              "new reason",
			message:             "new message",
			expected:            true,
		},
		{
			name: "target condition found - long message truncated",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 50,
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "other condition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionFalse,
							ObservedGeneration: 50,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               "other condition2",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason2",
							Message:            "other message2",
						},
					},
				},
			},
			expectedGw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 50,
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "other condition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionTrue,
							Reason:             "new reason",
							Message:            truncatedString,
							ObservedGeneration: 50,
						},
						{
							Type:               "other condition2",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason2",
							Message:            "other message2",
						},
					},
				},
			},
			targetConditionType: string(gwv1.GatewayConditionAccepted),
			newStatus:           metav1.ConditionTrue,
			reason:              "new reason",
			message:             longString,
			expected:            true,
		},
		{
			name: "target condition found already correct",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 50,
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "other condition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionTrue,
							Reason:             "new reason",
							Message:            "new message",
							ObservedGeneration: 50,
						},
						{
							Type:               "other condition2",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason2",
							Message:            "other message2",
						},
					},
				},
			},
			expectedGw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 50,
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "other condition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionTrue,
							Reason:             "new reason",
							Message:            "new message",
							ObservedGeneration: 50,
						},
						{
							Type:               "other condition2",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason2",
							Message:            "other message2",
						},
					},
				},
			},
			targetConditionType: string(gwv1.GatewayConditionAccepted),
			newStatus:           metav1.ConditionTrue,
			reason:              "new reason",
			message:             "new message",
		},
		{
			name: "target condition found - long message truncated",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 50,
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "other condition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionTrue,
							Reason:             "other reason",
							Message:            truncatedString,
							ObservedGeneration: 50,
						},
						{
							Type:               "other condition2",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason2",
							Message:            "other message2",
						},
					},
				},
			},
			expectedGw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 50,
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "other condition",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason",
							Message:            "other message",
						},
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionTrue,
							Reason:             "other reason",
							Message:            truncatedString,
							ObservedGeneration: 50,
						},
						{
							Type:               "other condition2",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 50,
							Reason:             "other reason2",
							Message:            "other message2",
						},
					},
				},
			},
			targetConditionType: string(gwv1.GatewayConditionAccepted),
			newStatus:           metav1.ConditionTrue,
			reason:              "other reason",
			message:             longString,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := prepareGatewayConditionUpdate(&tc.gw, tc.targetConditionType, tc.newStatus, tc.reason, tc.message)

			// In order to use equals(), we need to make the time fields are fixed.
			fixedTime := metav1.NewTime(time.Now())
			if tc.gw.Status.Conditions != nil {
				for i := range tc.gw.Status.Conditions {
					tmp := &tc.gw.Status.Conditions[i]
					tmp.LastTransitionTime = fixedTime
				}
			}

			if tc.expectedGw.Status.Conditions != nil {
				for i := range tc.expectedGw.Status.Conditions {
					tmp := &tc.expectedGw.Status.Conditions[i]
					tmp.LastTransitionTime = fixedTime
				}
			}

			assert.Equal(t, tc.expected, res)
			assert.Equal(t, tc.expectedGw, tc.gw)
		})
	}
}

func Test_generateRouteList(t *testing.T) {
	testCases := []struct {
		name     string
		routes   map[int32][]routeutils.RouteDescriptor
		expected string
	}{
		{
			name:     "no routes",
			routes:   make(map[int32][]routeutils.RouteDescriptor),
			expected: "",
		},
		{
			name: "some routes",
			routes: map[int32][]routeutils.RouteDescriptor{
				1: {
					&routeutils.MockRoute{
						Name:      "1-1-r",
						Namespace: "1-1-ns",
						Kind:      routeutils.GRPCRouteKind,
					},
					&routeutils.MockRoute{
						Name:      "1-2-r",
						Namespace: "1-2-ns",
						Kind:      routeutils.TCPRouteKind,
					},
					&routeutils.MockRoute{
						Name:      "1-3-r",
						Namespace: "1-3-ns",
						Kind:      routeutils.HTTPRouteKind,
					},
					&routeutils.MockRoute{
						Name:      "1-4-r",
						Namespace: "1-4-ns",
						Kind:      routeutils.UDPRouteKind,
					},
				},
				2: {
					&routeutils.MockRoute{
						Name:      "2-1-r",
						Namespace: "2-1-ns",
						Kind:      routeutils.GRPCRouteKind,
					},
					&routeutils.MockRoute{
						Name:      "2-2-r",
						Namespace: "2-2-ns",
						Kind:      routeutils.TCPRouteKind,
					},
					&routeutils.MockRoute{
						Name:      "2-3-r",
						Namespace: "2-3-ns",
						Kind:      routeutils.HTTPRouteKind,
					},
					&routeutils.MockRoute{
						Name:      "2-4-r",
						Namespace: "2-4-ns",
						Kind:      routeutils.UDPRouteKind,
					},
				},
				3: {
					&routeutils.MockRoute{
						Name:      "3-1-r",
						Namespace: "3-1-ns",
						Kind:      routeutils.GRPCRouteKind,
					},
					&routeutils.MockRoute{
						Name:      "3-2-r",
						Namespace: "3-2-ns",
						Kind:      routeutils.TCPRouteKind,
					},
					&routeutils.MockRoute{
						Name:      "3-3-r",
						Namespace: "3-3-ns",
						Kind:      routeutils.HTTPRouteKind,
					},
					&routeutils.MockRoute{
						Name:      "3-4-r",
						Namespace: "3-4-ns",
						Kind:      routeutils.UDPRouteKind,
					},
				},
				4: {
					&routeutils.MockRoute{
						Name:      "4-1-r",
						Namespace: "4-1-ns",
						Kind:      routeutils.GRPCRouteKind,
					},
					&routeutils.MockRoute{
						Name:      "4-2-r",
						Namespace: "4-2-ns",
						Kind:      routeutils.TCPRouteKind,
					},
					&routeutils.MockRoute{
						Name:      "4-3-r",
						Namespace: "4-3-ns",
						Kind:      routeutils.HTTPRouteKind,
					},
					&routeutils.MockRoute{
						Name:      "4-4-r",
						Namespace: "4-4-ns",
						Kind:      routeutils.UDPRouteKind,
					},
				},
			},
			expected: "(GRPCRoute, 1-1-ns:1-1-r),(GRPCRoute, 2-1-ns:2-1-r),(GRPCRoute, 3-1-ns:3-1-r),(GRPCRoute, 4-1-ns:4-1-r),(HTTPRoute, 1-3-ns:1-3-r),(HTTPRoute, 2-3-ns:2-3-r),(HTTPRoute, 3-3-ns:3-3-r),(HTTPRoute, 4-3-ns:4-3-r),(TCPRoute, 1-2-ns:1-2-r),(TCPRoute, 2-2-ns:2-2-r),(TCPRoute, 3-2-ns:3-2-r),(TCPRoute, 4-2-ns:4-2-r),(UDPRoute, 1-4-ns:1-4-r),(UDPRoute, 2-4-ns:2-4-r),(UDPRoute, 3-4-ns:3-4-r),(UDPRoute, 4-4-ns:4-4-r)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := generateRouteList(tc.routes)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func Test_StringsSimilar(t *testing.T) {
	testCases := []struct {
		name      string
		a         string
		b         string
		threshold float64
		expected  bool
	}{
		{
			name:      "identical messages",
			a:         "failed to create load balancer",
			b:         "failed to create load balancer",
			threshold: 0.85,
			expected:  true,
		},
		{
			name:      "messages with different AWS request IDs",
			a:         "operation error Elastic Load Balancingv2: CreateRule, https response error StatusCode: 400, RequestID: f25747d6-52b6-4db6-bde2-a38aed0fa036, InvalidLoadBalancerAction: The redirect configuration is not valid because it creates a loop",
			b:         "operation error Elastic Load Balancingv2: CreateRule, https response error StatusCode: 400, RequestID: a1b2c3d4-e5f6-7890-abcd-ef1234567890, InvalidLoadBalancerAction: The redirect configuration is not valid because it creates a loop",
			threshold: 0.85,
			expected:  true,
		},
		{
			name:      "completely different messages",
			a:         "failed to create load balancer",
			b:         "subnet not found in availability zone",
			threshold: 0.85,
			expected:  false,
		},
		{
			name:      "empty and non-empty",
			a:         "",
			b:         "some error",
			threshold: 0.85,
			expected:  false,
		},
		{
			name:      "both empty",
			a:         "",
			b:         "",
			threshold: 0.85,
			expected:  true,
		},
		{
			name:      "messages with different timestamps in long message",
			a:         "operation failed at 2024-01-15T10:30:00Z: connection timeout while connecting to the backend service endpoint",
			b:         "operation failed at 2024-01-15T10:31:00Z: connection timeout while connecting to the backend service endpoint",
			threshold: 0.85,
			expected:  true,
		},
		{
			name:      "higher threshold rejects similar messages",
			a:         "operation error Elastic Load Balancingv2: CreateRule, https response error StatusCode: 400, RequestID: f25747d6-52b6-4db6-bde2-a38aed0fa036, InvalidLoadBalancerAction: error",
			b:         "operation error Elastic Load Balancingv2: CreateRule, https response error StatusCode: 400, RequestID: a1b2c3d4-e5f6-7890-abcd-ef1234567890, InvalidLoadBalancerAction: error",
			threshold: 0.95,
			expected:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := algorithm.StringsSimilar(tc.a, tc.b, tc.threshold)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_LevenshteinDistance(t *testing.T) {
	testCases := []struct {
		name     string
		a        string
		b        string
		expected int
	}{
		{
			name:     "identical strings",
			a:        "hello",
			b:        "hello",
			expected: 0,
		},
		{
			name:     "one insertion",
			a:        "hello",
			b:        "helloo",
			expected: 1,
		},
		{
			name:     "one deletion",
			a:        "hello",
			b:        "helo",
			expected: 1,
		},
		{
			name:     "one substitution",
			a:        "hello",
			b:        "hallo",
			expected: 1,
		},
		{
			name:     "empty first string",
			a:        "",
			b:        "hello",
			expected: 5,
		},
		{
			name:     "empty second string",
			a:        "hello",
			b:        "",
			expected: 5,
		},
		{
			name:     "both empty",
			a:        "",
			b:        "",
			expected: 0,
		},
		{
			name:     "completely different",
			a:        "abc",
			b:        "xyz",
			expected: 3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := algorithm.LevenshteinDistance(tc.a, tc.b)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_prepareGatewayConditionUpdate_TransitionTime(t *testing.T) {
	recentTime := metav1.NewTime(time.Now().Add(-30 * time.Second)) // 30 seconds ago
	oldTime := metav1.NewTime(time.Now().Add(-2 * time.Minute))     // 2 minutes ago

	testCases := []struct {
		name                string
		gw                  gwv1.Gateway
		targetConditionType string
		newStatus           metav1.ConditionStatus
		reason              string
		message             string
		expected            bool
		expectedMessage     string
	}{
		{
			name: "status change always updates regardless of transition time",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 50,
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionFalse,
							Reason:             "same reason",
							Message:            "same message",
							ObservedGeneration: 50,
							LastTransitionTime: recentTime,
						},
					},
				},
			},
			targetConditionType: string(gwv1.GatewayConditionAccepted),
			newStatus:           metav1.ConditionTrue,
			reason:              "same reason",
			message:             "same message",
			expected:            true,
			expectedMessage:     "same message",
		},
		{
			name: "reason change always updates regardless of transition time",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 50,
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionFalse,
							Reason:             "old reason",
							Message:            "same message",
							ObservedGeneration: 50,
							LastTransitionTime: recentTime,
						},
					},
				},
			},
			targetConditionType: string(gwv1.GatewayConditionAccepted),
			newStatus:           metav1.ConditionFalse,
			reason:              "new reason",
			message:             "same message",
			expected:            true,
			expectedMessage:     "same message",
		},
		{
			name: "generation change always updates regardless of transition time",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 51,
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionFalse,
							Reason:             "same reason",
							Message:            "same message",
							ObservedGeneration: 50,
							LastTransitionTime: recentTime,
						},
					},
				},
			},
			targetConditionType: string(gwv1.GatewayConditionAccepted),
			newStatus:           metav1.ConditionFalse,
			reason:              "same reason",
			message:             "same message",
			expected:            true,
			expectedMessage:     "same message",
		},
		{
			name: "message only change with recent transition time - should NOT update",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 50,
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionFalse,
							Reason:             "same reason",
							Message:            "old error message with RequestID: abc123",
							ObservedGeneration: 50,
							LastTransitionTime: recentTime,
						},
					},
				},
			},
			targetConditionType: string(gwv1.GatewayConditionAccepted),
			newStatus:           metav1.ConditionFalse,
			reason:              "same reason",
			message:             "new error message with RequestID: xyz789",
			expected:            false,
			expectedMessage:     "old error message with RequestID: abc123",
		},
		{
			name: "message only change with old transition time and dissimilar message - should update",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 50,
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionFalse,
							Reason:             "same reason",
							Message:            "old error: connection timeout",
							ObservedGeneration: 50,
							LastTransitionTime: oldTime,
						},
					},
				},
			},
			targetConditionType: string(gwv1.GatewayConditionAccepted),
			newStatus:           metav1.ConditionFalse,
			reason:              "same reason",
			message:             "new error: subnet not found",
			expected:            true,
			expectedMessage:     "new error: subnet not found",
		},
		{
			name: "message only change with old transition time but similar message - should NOT update",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 50,
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionFalse,
							Reason:             "same reason",
							Message:            "operation error Elastic Load Balancingv2: CreateRule, https response error StatusCode: 400, RequestID: f25747d6-52b6-4db6-bde2-a38aed0fa036, InvalidLoadBalancerAction: error",
							ObservedGeneration: 50,
							LastTransitionTime: oldTime,
						},
					},
				},
			},
			targetConditionType: string(gwv1.GatewayConditionAccepted),
			newStatus:           metav1.ConditionFalse,
			reason:              "same reason",
			message:             "operation error Elastic Load Balancingv2: CreateRule, https response error StatusCode: 400, RequestID: a1b2c3d4-e5f6-7890-abcd-ef1234567890, InvalidLoadBalancerAction: error",
			expected:            false,
			expectedMessage:     "operation error Elastic Load Balancingv2: CreateRule, https response error StatusCode: 400, RequestID: f25747d6-52b6-4db6-bde2-a38aed0fa036, InvalidLoadBalancerAction: error",
		},
		{
			name: "identical message should NOT update",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 50,
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:               string(gwv1.GatewayConditionAccepted),
							Status:             metav1.ConditionFalse,
							Reason:             "same reason",
							Message:            "same message",
							ObservedGeneration: 50,
							LastTransitionTime: oldTime,
						},
					},
				},
			},
			targetConditionType: string(gwv1.GatewayConditionAccepted),
			newStatus:           metav1.ConditionFalse,
			reason:              "same reason",
			message:             "same message",
			expected:            false,
			expectedMessage:     "same message",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := prepareGatewayConditionUpdate(&tc.gw, tc.targetConditionType, tc.newStatus, tc.reason, tc.message)
			assert.Equal(t, tc.expected, res)

			// Find the target condition and verify the message
			for _, cond := range tc.gw.Status.Conditions {
				if cond.Type == tc.targetConditionType {
					assert.Equal(t, tc.expectedMessage, cond.Message)
					break
				}
			}
		})
	}
}
