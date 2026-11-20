package routeutils

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/gateway/constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func Test_findStaleParentStatuses(t *testing.T) {
	albGW := gwv1.ParentReference{Name: "alb-gw", Namespace: (*gwv1.Namespace)(new("kube-system")), SectionName: (*gwv1.SectionName)(new("https"))}
	otherGW := gwv1.ParentReference{Name: "nginx-gw", Namespace: (*gwv1.Namespace)(new("nginx"))}
	localGW := gwv1.ParentReference{Name: "local-gw"}

	statusFor := func(ref gwv1.ParentReference, controller string) gwv1.RouteParentStatus {
		return gwv1.RouteParentStatus{ParentRef: ref, ControllerName: gwv1.GatewayController(controller)}
	}

	testCases := []struct {
		name       string
		parentRefs []gwv1.ParentReference
		statuses   []gwv1.RouteParentStatus
		expected   []gwv1.ParentReference
	}{
		{
			name:       "no status",
			parentRefs: []gwv1.ParentReference{albGW},
		},
		{
			name:       "our status entry is still referenced",
			parentRefs: []gwv1.ParentReference{albGW},
			statuses:   []gwv1.RouteParentStatus{statusFor(albGW, gateway_constants.ALBGatewayController)},
		},
		{
			name:       "route moved to a gateway of another controller",
			parentRefs: []gwv1.ParentReference{otherGW},
			statuses: []gwv1.RouteParentStatus{
				statusFor(otherGW, "example.com/other-controller"),
				statusFor(albGW, gateway_constants.ALBGatewayController),
			},
			expected: []gwv1.ParentReference{albGW},
		},
		{
			name:     "entries owned by other controllers are ignored",
			statuses: []gwv1.RouteParentStatus{statusFor(otherGW, "example.com/other-controller")},
		},
		{
			name:       "parentRef without namespace defaults to the route namespace",
			parentRefs: []gwv1.ParentReference{{Name: "local-gw", Namespace: (*gwv1.Namespace)(new("route-ns"))}},
			statuses:   []gwv1.RouteParentStatus{statusFor(localGW, gateway_constants.ALBGatewayController)},
		},
		{
			name:       "section name changed",
			parentRefs: []gwv1.ParentReference{{Name: "alb-gw", Namespace: (*gwv1.Namespace)(new("kube-system")), SectionName: (*gwv1.SectionName)(new("http"))}},
			statuses:   []gwv1.RouteParentStatus{statusFor(albGW, gateway_constants.ALBGatewayController)},
			expected:   []gwv1.ParentReference{albGW},
		},
		{
			name:     "all parentRefs removed",
			statuses: []gwv1.RouteParentStatus{statusFor(albGW, gateway_constants.ALBGatewayController)},
			expected: []gwv1.ParentReference{albGW},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			route := &mockRoute{
				namespacedName: types.NamespacedName{Namespace: "route-ns", Name: "route"},
				routeKind:      HTTPRouteKind,
				parentRefs:     tc.parentRefs,
				parentStatuses: tc.statuses,
			}
			assert.Equal(t, tc.expected, findStaleParentStatuses(route, gateway_constants.ALBGatewayController))
		})
	}
}

func Test_routeDescriptors_GetParentStatuses(t *testing.T) {
	parents := []gwv1.RouteParentStatus{{
		ParentRef:      gwv1.ParentReference{Name: "gw"},
		ControllerName: gwv1.GatewayController(gateway_constants.NLBGatewayController),
	}}
	routeStatus := gwv1.RouteStatus{Parents: parents}

	descriptors := map[string]preLoadRouteDescriptor{
		"HTTPRoute": convertHTTPRoute(gwv1.HTTPRoute{Status: gwv1.HTTPRouteStatus{RouteStatus: routeStatus}}),
		"GRPCRoute": convertGRPCRoute(gwv1.GRPCRoute{Status: gwv1.GRPCRouteStatus{RouteStatus: routeStatus}}),
		"TLSRoute":  convertTLSRoute(gwv1.TLSRoute{Status: gwv1.TLSRouteStatus{RouteStatus: routeStatus}}),
		"TCPRoute":  convertTCPRoute(gwv1.TCPRoute{Status: gwv1.TCPRouteStatus{RouteStatus: routeStatus}}),
		"UDPRoute":  convertUDPRoute(gwv1.UDPRoute{Status: gwv1.UDPRouteStatus{RouteStatus: routeStatus}}),
	}
	for kind, d := range descriptors {
		t.Run(kind, func(t *testing.T) {
			assert.Equal(t, parents, d.GetParentStatuses())
		})
	}
}

func Test_LoadRoutesForGateway_enqueuesStaleParentStatusCleanup(t *testing.T) {
	staleRef := gwv1.ParentReference{Name: "gw", Namespace: (*gwv1.Namespace)(new("gw-ns"))}
	detachedRoute := &mockRoute{
		namespacedName: types.NamespacedName{Namespace: "app-ns", Name: "detached"},
		routeKind:      HTTPRouteKind,
		generation:     5,
		parentRefs:     []gwv1.ParentReference{{Name: "nginx-gw", Namespace: (*gwv1.Namespace)(new("nginx"))}},
		parentStatuses: []gwv1.RouteParentStatus{
			{ParentRef: staleRef, ControllerName: gwv1.GatewayController(gateway_constants.ALBGatewayController)},
		},
	}
	unrelatedRoute := &mockRoute{
		namespacedName: types.NamespacedName{Namespace: "app-ns", Name: "unrelated"},
		routeKind:      HTTPRouteKind,
	}
	preLoaded := []preLoadRouteDescriptor{detachedRoute, unrelatedRoute}

	routeReconciler := NewMockRouteReconciler()
	loader := loaderImpl{
		mapper: &mockMapper{
			t:              t,
			expectedRoutes: preLoaded,
			mapToReturn:    map[int32][]preLoadRouteDescriptor{},
		},
		allRouteLoaders: map[RouteKind]func(ctx context.Context, k8sClient client.Client, opts ...client.ListOption) ([]preLoadRouteDescriptor, error){
			HTTPRouteKind: func(ctx context.Context, k8sClient client.Client, opts ...client.ListOption) ([]preLoadRouteDescriptor, error) {
				return preLoaded, nil
			},
		},
		logger:         logr.Discard(),
		routeSubmitter: routeReconciler,
		lsLoader:       &mockListenerSetLoader{},
	}

	_, err := loader.LoadRoutesForGateway(context.Background(), gwv1.Gateway{ObjectMeta: v1.ObjectMeta{Name: "gw", Namespace: "gw-ns"}},
		&routeFilterImpl{acceptedKinds: sets.New[RouteKind](HTTPRouteKind)}, gateway_constants.ALBGatewayController, nil)
	assert.NoError(t, err)

	if assert.Len(t, routeReconciler.Enqueued, 1) {
		rd := routeReconciler.Enqueued[0].RouteData
		assert.Equal(t, "detached", rd.RouteMetadata.RouteName)
		assert.Equal(t, "app-ns", rd.RouteMetadata.RouteNamespace)
		assert.Equal(t, int64(5), rd.RouteMetadata.RouteGeneration)
		assert.Equal(t, staleRef, rd.ParentRef)
	}
}
