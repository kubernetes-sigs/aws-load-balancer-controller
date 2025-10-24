package gateway

import (
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestDeferredReconcilerConstructor(t *testing.T) {
	dq := workqueue.NewDelayingQueue()
	defer dq.ShutDown()
	k8sClient := testclient.NewClientBuilder().Build()
	logger := logr.New(&log.NullLogSink{})

	d := NewRouteReconciler(dq, k8sClient, logger)

	deferredReconciler := d.(*routeReconcilerImpl)
	assert.Equal(t, dq, deferredReconciler.queue)
	assert.Equal(t, k8sClient, deferredReconciler.k8sClient)
	assert.Equal(t, logger, deferredReconciler.logger)
}

func Test_isRouteStatusIdentical(t *testing.T) {
	tests := []struct {
		name     string
		routeOld client.Object
		routeNew client.Object
		want     bool
	}{
		{
			name: "identical route status",
			routeOld: &gwv1.HTTPRoute{
				Status: gwv1.HTTPRouteStatus{
					RouteStatus: gwv1.RouteStatus{
						Parents: []gwv1.RouteParentStatus{
							{
								ParentRef: gwv1.ParentReference{
									Name: "gateway-1",
								},
								ControllerName: "example.com/controller",
								Conditions: []metav1.Condition{
									{
										Type:    "Accepted",
										Status:  metav1.ConditionTrue,
										Reason:  "Accepted",
										Message: "Route accepted",
									},
								},
							},
						},
					},
				},
			},
			routeNew: &gwv1.HTTPRoute{
				Status: gwv1.HTTPRouteStatus{
					RouteStatus: gwv1.RouteStatus{
						Parents: []gwv1.RouteParentStatus{
							{
								ParentRef: gwv1.ParentReference{
									Name: "gateway-1",
								},
								ControllerName: "example.com/controller",
								Conditions: []metav1.Condition{
									{
										Type:    "Accepted",
										Status:  metav1.ConditionTrue,
										Reason:  "Accepted",
										Message: "Route accepted",
									},
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "different route status",
			routeOld: &gwv1.HTTPRoute{
				Status: gwv1.HTTPRouteStatus{
					RouteStatus: gwv1.RouteStatus{
						Parents: []gwv1.RouteParentStatus{
							{
								ParentRef: gwv1.ParentReference{
									Name: "gateway-1",
								},
								ControllerName: "example.com/controller",
								Conditions: []metav1.Condition{
									{
										Type:    "Accepted",
										Status:  metav1.ConditionTrue,
										Reason:  "Accepted",
										Message: "Route accepted",
									},
								},
							},
						},
					},
				},
			},
			routeNew: &gwv1.HTTPRoute{
				Status: gwv1.HTTPRouteStatus{
					RouteStatus: gwv1.RouteStatus{
						Parents: []gwv1.RouteParentStatus{
							{
								ParentRef: gwv1.ParentReference{
									Name: "gateway-1",
								},
								ControllerName: "example.com/controller",
								Conditions: []metav1.Condition{
									{
										Type:    "Accepted",
										Status:  metav1.ConditionFalse,
										Reason:  "Not Accepted",
										Message: "Route not accepted",
									},
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "different number of parents",
			routeOld: &gwv1.HTTPRoute{
				Status: gwv1.HTTPRouteStatus{
					RouteStatus: gwv1.RouteStatus{
						Parents: []gwv1.RouteParentStatus{
							{
								ParentRef: gwv1.ParentReference{
									Name: "gateway-1",
								},
							},
						},
					},
				},
			},
			routeNew: &gwv1.HTTPRoute{
				Status: gwv1.HTTPRouteStatus{
					RouteStatus: gwv1.RouteStatus{
						Parents: []gwv1.RouteParentStatus{
							{
								ParentRef: gwv1.ParentReference{
									Name: "gateway-1",
								},
							},
							{
								ParentRef: gwv1.ParentReference{
									Name: "gateway-2",
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "different controller names",
			routeOld: &gwv1.HTTPRoute{
				Status: gwv1.HTTPRouteStatus{
					RouteStatus: gwv1.RouteStatus{
						Parents: []gwv1.RouteParentStatus{
							{
								ParentRef: gwv1.ParentReference{
									Name: "gateway-1",
								},
								ControllerName: "controller-1",
							},
						},
					},
				},
			},
			routeNew: &gwv1.HTTPRoute{
				Status: gwv1.HTTPRouteStatus{
					RouteStatus: gwv1.RouteStatus{
						Parents: []gwv1.RouteParentStatus{
							{
								ParentRef: gwv1.ParentReference{
									Name: "gateway-1",
								},
								ControllerName: "controller-2",
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "different conditions",
			routeOld: &gwv1.HTTPRoute{
				Status: gwv1.HTTPRouteStatus{
					RouteStatus: gwv1.RouteStatus{
						Parents: []gwv1.RouteParentStatus{
							{
								ParentRef: gwv1.ParentReference{
									Name: "gateway-1",
								},
								Conditions: []metav1.Condition{
									{
										Type:   "Ready",
										Status: metav1.ConditionTrue,
									},
								},
							},
						},
					},
				},
			},
			routeNew: &gwv1.HTTPRoute{
				Status: gwv1.HTTPRouteStatus{
					RouteStatus: gwv1.RouteStatus{
						Parents: []gwv1.RouteParentStatus{
							{
								ParentRef: gwv1.ParentReference{
									Name: "gateway-1",
								},
								Conditions: []metav1.Condition{
									{
										Type:   "Ready",
										Status: metav1.ConditionFalse,
									},
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "different parent references",
			routeOld: &gwv1.HTTPRoute{
				Status: gwv1.HTTPRouteStatus{
					RouteStatus: gwv1.RouteStatus{
						Parents: []gwv1.RouteParentStatus{
							{
								ParentRef: gwv1.ParentReference{
									Name: "gateway-1",
								},
							},
						},
					},
				},
			},
			routeNew: &gwv1.HTTPRoute{
				Status: gwv1.HTTPRouteStatus{
					RouteStatus: gwv1.RouteStatus{
						Parents: []gwv1.RouteParentStatus{
							{
								ParentRef: gwv1.ParentReference{
									Name: "gateway-2",
								},
							},
						},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &routeReconcilerImpl{}
			got := d.isRouteStatusIdentical(tt.routeOld, tt.routeNew)
			if got != tt.want {
				t.Errorf("isRouteStatusIdentical() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getParentStatusKey(t *testing.T) {
	portPtr := func(p gwv1.PortNumber) *gwv1.PortNumber {
		return &p
	}

	tests := []struct {
		name   string
		status gwv1.RouteParentStatus
		want   string
	}{
		{
			name: "provide all fields",
			status: gwv1.RouteParentStatus{
				ParentRef: gwv1.ParentReference{
					Group:       (*gwv1.Group)(ptr.To("networking.k8s.io")),
					Kind:        (*gwv1.Kind)(ptr.To("Gateway")),
					Namespace:   (*gwv1.Namespace)(ptr.To("test-namespace")),
					Name:        "test-gateway",
					SectionName: (*gwv1.SectionName)(ptr.To("test-section")),
					Port:        portPtr(80),
				},
			},
			want: "networking.k8s.io/Gateway/test-namespace/test-gateway/test-section/80",
		},
		{
			name: "no section or port",
			status: gwv1.RouteParentStatus{
				ParentRef: gwv1.ParentReference{
					Group:     (*gwv1.Group)(ptr.To("networking.k8s.io")),
					Kind:      (*gwv1.Kind)(ptr.To("Gateway")),
					Namespace: (*gwv1.Namespace)(ptr.To("test-namespace")),
					Name:      "test-gateway",
				},
			},
			want: "networking.k8s.io/Gateway/test-namespace/test-gateway//",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getParentStatusKey(tt.status.ParentRef)
			if got != tt.want {
				t.Errorf("getParentStatusKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnqueue(t *testing.T) {
	tests := []struct {
		name                    string
		routeData               routeutils.RouteData
		routeStatusInfo         routeutils.RouteStatusInfo
		routeMetadataDescriptor routeutils.RouteMetadata
		parentRefGateway        routeutils.ParentRefGateway

		validateEnqueued func(t *testing.T, enqueued []routeutils.EnqueuedType) // Use the type here
	}{
		{
			name: "enqueue with accepted status",
			routeData: routeutils.RouteData{
				RouteStatusInfo: routeutils.RouteStatusInfo{
					Accepted:     true,
					ResolvedRefs: true,
				},
				RouteMetadata: routeutils.RouteMetadata{
					RouteName:      "test-name",
					RouteNamespace: "test-namespace",
					RouteKind:      "test-kind",
				},
				ParentRefGateway: routeutils.ParentRefGateway{},
			},
			validateEnqueued: func(t *testing.T, enqueued []routeutils.EnqueuedType) {
				assert.Len(t, enqueued, 1)
				assert.Equal(t, true, enqueued[0].RouteData.RouteStatusInfo.Accepted)
				assert.Equal(t, true, enqueued[0].RouteData.RouteStatusInfo.ResolvedRefs)
				assert.Equal(t, "test-name", enqueued[0].RouteData.RouteMetadata.RouteName)
				assert.Equal(t, "test-namespace", enqueued[0].RouteData.RouteMetadata.RouteNamespace)
				assert.Equal(t, "test-kind", enqueued[0].RouteData.RouteMetadata.RouteKind)
			},
		},
		{
			name: "enqueue with rejected status",
			routeData: routeutils.RouteData{
				RouteStatusInfo: routeutils.RouteStatusInfo{
					Accepted:     false,
					ResolvedRefs: false,
				},
				RouteMetadata: routeutils.RouteMetadata{
					RouteName:      "test-name",
					RouteNamespace: "test-namespace",
					RouteKind:      "test-kind",
				},
				ParentRefGateway: routeutils.ParentRefGateway{},
			},
			validateEnqueued: func(t *testing.T, enqueued []routeutils.EnqueuedType) {
				assert.Len(t, enqueued, 1)
				assert.Equal(t, false, enqueued[0].RouteData.RouteStatusInfo.Accepted)
				assert.Equal(t, false, enqueued[0].RouteData.RouteStatusInfo.ResolvedRefs)
				assert.Equal(t, "test-name", enqueued[0].RouteData.RouteMetadata.RouteName)
				assert.Equal(t, "test-namespace", enqueued[0].RouteData.RouteMetadata.RouteNamespace)
				assert.Equal(t, "test-kind", enqueued[0].RouteData.RouteMetadata.RouteKind)
			},
		},
		{
			name: "enqueue with empty route name",
			routeData: routeutils.RouteData{
				RouteStatusInfo: routeutils.RouteStatusInfo{
					Accepted:     false,
					ResolvedRefs: false,
				},
				RouteMetadata: routeutils.RouteMetadata{
					RouteNamespace: "test-namespace",
					RouteKind:      "test-kind",
				},
				ParentRefGateway: routeutils.ParentRefGateway{},
			},
			validateEnqueued: func(t *testing.T, enqueued []routeutils.EnqueuedType) {
				assert.Len(t, enqueued, 1)
				assert.Equal(t, false, enqueued[0].RouteData.RouteStatusInfo.Accepted)
				assert.Equal(t, false, enqueued[0].RouteData.RouteStatusInfo.ResolvedRefs)
				assert.Equal(t, "", enqueued[0].RouteData.RouteMetadata.RouteName)
				assert.Equal(t, "test-namespace", enqueued[0].RouteData.RouteMetadata.RouteNamespace)
				assert.Equal(t, "test-kind", enqueued[0].RouteData.RouteMetadata.RouteKind)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := routeutils.NewMockRouteReconciler()

			mock.Enqueue(tt.routeData)

			tt.validateEnqueued(t, mock.Enqueued)
		})
	}
}

func Test_updateRouteStatus(t *testing.T) {
	tests := []struct {
		name           string
		route          client.Object
		routeData      routeutils.RouteData
		validateResult func(t *testing.T, route client.Object)
	}{
		{
			name: "update HTTPRoute status - condition accepted",
			route: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "test-namespace",
				},
				Spec: gwv1.HTTPRouteSpec{
					Hostnames: []gwv1.Hostname{"example.com"},
				},
			},
			routeData: routeutils.RouteData{
				RouteStatusInfo: routeutils.RouteStatusInfo{
					Accepted:     true,
					ResolvedRefs: true,
					Reason:       string(gwv1.RouteConditionAccepted),
					Message:      "route accepted",
				},
				ParentRefGateway: routeutils.ParentRefGateway{
					Name:      "test-gateway",
					Namespace: "test-namespace",
				},
			},
			validateResult: func(t *testing.T, route client.Object) {
				httpRoute := route.(*gwv1.HTTPRoute)
				if httpRoute.Status.Parents == nil {
					assert.Len(t, httpRoute.Status.Parents, 0)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logr.New(&log.NullLogSink{})
			reconciler := &routeReconcilerImpl{
				logger: logger,
			}
			err := reconciler.updateRouteStatus(tt.route, tt.routeData)
			assert.NoError(t, err)
			if tt.validateResult != nil {
				tt.validateResult(t, tt.route)
			}
		})
	}
}

func Test_setConditionsWithRouteStatusInfo(t *testing.T) {
	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 1,
		},
	}

	tests := []struct {
		name           string
		info           routeutils.RouteStatusInfo
		validateResult func(t *testing.T, conditions []metav1.Condition)
	}{
		{
			name: "accepted true and resolvedRef true",
			info: routeutils.RouteStatusInfo{
				Accepted:     true,
				ResolvedRefs: true,
				Reason:       string(gwv1.RouteConditionAccepted),
				Message:      "route accepted",
			},
			validateResult: func(t *testing.T, conditions []metav1.Condition) {
				assert.Len(t, conditions, 2)
				acceptedCondition := findCondition(conditions, string(gwv1.RouteConditionAccepted))
				assert.NotNil(t, acceptedCondition)
				assert.Equal(t, metav1.ConditionTrue, acceptedCondition.Status)

				resolvedRefCondition := findCondition(conditions, string(gwv1.RouteConditionResolvedRefs))
				assert.NotNil(t, resolvedRefCondition)
				assert.Equal(t, metav1.ConditionTrue, resolvedRefCondition.Status)
			},
		},
		{
			name: "accepted false and resolvedRef true",
			info: routeutils.RouteStatusInfo{
				Accepted:     false,
				ResolvedRefs: true,
				Reason:       string(gwv1.RouteReasonNotAllowedByListeners),
				Message:      "route not allowed by listeners",
			},
			validateResult: func(t *testing.T, conditions []metav1.Condition) {
				assert.Len(t, conditions, 2)
				acceptedCondition := findCondition(conditions, string(gwv1.RouteConditionAccepted))
				assert.NotNil(t, acceptedCondition)
				assert.Equal(t, metav1.ConditionFalse, acceptedCondition.Status)

				resolvedRefCondition := findCondition(conditions, string(gwv1.RouteConditionResolvedRefs))
				assert.NotNil(t, resolvedRefCondition)
				assert.Equal(t, metav1.ConditionTrue, resolvedRefCondition.Status)
			},
		},
		{
			name: "accepted false and resolvedRef false",
			info: routeutils.RouteStatusInfo{
				Accepted:     false,
				ResolvedRefs: false,
				Reason:       string(gwv1.RouteReasonBackendNotFound),
				Message:      "backend not found",
			},
			validateResult: func(t *testing.T, conditions []metav1.Condition) {
				assert.Len(t, conditions, 2)
				acceptedCondition := findCondition(conditions, string(gwv1.RouteConditionAccepted))
				assert.NotNil(t, acceptedCondition)
				assert.Equal(t, metav1.ConditionFalse, acceptedCondition.Status)

				resolvedRefCondition := findCondition(conditions, string(gwv1.RouteConditionResolvedRefs))
				assert.NotNil(t, resolvedRefCondition)
				assert.Equal(t, metav1.ConditionFalse, resolvedRefCondition.Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logr.New(&log.NullLogSink{})
			reconciler := &routeReconcilerImpl{
				logger: logger,
			}
			parentStatus := &gwv1.RouteParentStatus{}
			reconciler.setConditionsWithRouteStatusInfo(route, parentStatus, tt.info)
			if tt.validateResult != nil {
				tt.validateResult(t, parentStatus.Conditions)
			}
		})
	}
}

func Test_areConditionsEqual(t *testing.T) {
	tests := []struct {
		name     string
		oldCon   []metav1.Condition
		newCon   []metav1.Condition
		expected bool
	}{
		{
			name: "same conditions - true",
			oldCon: []metav1.Condition{
				{
					Type:   string(gwv1.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				},
				{
					Type:   string(gwv1.RouteConditionResolvedRefs),
					Status: metav1.ConditionTrue,
				},
			},
			newCon: []metav1.Condition{
				{
					Type:   string(gwv1.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				},
				{
					Type:   string(gwv1.RouteConditionResolvedRefs),
					Status: metav1.ConditionTrue,
				},
			},
			expected: true,
		},
		{
			name: "different conditions - false",
			oldCon: []metav1.Condition{
				{
					Type:   string(gwv1.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
				},
				{
					Type:   string(gwv1.RouteConditionResolvedRefs),
					Status: metav1.ConditionTrue,
				},
			},
			newCon: []metav1.Condition{
				{
					Type:   string(gwv1.RouteConditionAccepted),
					Status: metav1.ConditionFalse,
				},
				{
					Type:   string(gwv1.RouteConditionResolvedRefs),
					Status: metav1.ConditionTrue,
				},
			},
			expected: false,
		},
		{
			name: "different conditions on Reason - false",
			oldCon: []metav1.Condition{
				{
					Type:   string(gwv1.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
					Reason: "good",
				},
				{
					Type:   string(gwv1.RouteConditionResolvedRefs),
					Status: metav1.ConditionTrue,
				},
			},
			newCon: []metav1.Condition{
				{
					Type:   string(gwv1.RouteConditionAccepted),
					Status: metav1.ConditionTrue,
					Reason: "test-good",
				},
				{
					Type:   string(gwv1.RouteConditionResolvedRefs),
					Status: metav1.ConditionTrue,
				},
			},
			expected: false,
		},
		{
			name: "different conditions on LastTransitionTime - true",
			oldCon: []metav1.Condition{
				{
					Type:               string(gwv1.RouteConditionAccepted),
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
				},
				{
					Type:   string(gwv1.RouteConditionResolvedRefs),
					Status: metav1.ConditionTrue,
				},
			},
			newCon: []metav1.Condition{
				{
					Type:               string(gwv1.RouteConditionAccepted),
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
				},
				{
					Type:   string(gwv1.RouteConditionResolvedRefs),
					Status: metav1.ConditionTrue,
				},
			},
			expected: true,
		},
		{
			name: "different conditions on ObservedGeneration - false",
			oldCon: []metav1.Condition{
				{
					Type:               string(gwv1.RouteConditionAccepted),
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 1,
				},
				{
					Type:   string(gwv1.RouteConditionResolvedRefs),
					Status: metav1.ConditionTrue,
				},
			},
			newCon: []metav1.Condition{
				{
					Type:               string(gwv1.RouteConditionAccepted),
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 2,
				},
				{
					Type:   string(gwv1.RouteConditionResolvedRefs),
					Status: metav1.ConditionTrue,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := areConditionsEqual(tt.oldCon, tt.newCon)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// helper function
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}
