package routeutils

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// mockAttachmentResult configures what the mock returns for a given listener+route+parentRef combo.
type mockAttachmentResult struct {
	compatibleHostnames []gwv1.Hostname
	failedRouteData     *RouteData
	err                 error
}

// mockListenerAttachmentHelper implements listenerAttachmentHelper for testing.
// Results are keyed by "{parentRefKind}-{parentRefName}-{listenerName}-{listenerPort}-{routeName}-{routeNamespace}".
type mockListenerAttachmentHelper struct {
	results map[string]mockAttachmentResult
}

func mockAttachmentKey(parentRef gwv1.ParentReference, listener gwv1.Listener, route preLoadRouteDescriptor) string {
	kind := gatewayKind
	if parentRef.Kind != nil {
		kind = string(*parentRef.Kind)
	}

	nsn := route.GetRouteNamespacedName()
	parentRefNamespace := nsn.Namespace
	if parentRef.Namespace != nil {
		parentRefNamespace = string(*parentRef.Namespace)
	}

	return fmt.Sprintf("%s-%s-%s-%s-%d-%s-%s-%s", kind, parentRef.Name, parentRefNamespace, listener.Name, listener.Port, nsn.Name, nsn.Namespace, route.GetRouteKind())
}

func (m *mockListenerAttachmentHelper) listenerAllowsAttachment(_ context.Context, _ string, listener gwv1.Listener, route preLoadRouteDescriptor, matchedParentRef gwv1.ParentReference, _ map[int32]sets.Set[gwv1.Hostname], _ map[int32]sets.Set[gwv1.Hostname]) ([]gwv1.Hostname, *RouteData, error) {
	key := mockAttachmentKey(matchedParentRef, listener, route)
	if result, ok := m.results[key]; ok {
		return result.compatibleHostnames, result.failedRouteData, result.err
	}
	// Default: reject attachment
	rd := GenerateRouteData(false, true, string(gwv1.RouteReasonNotAllowedByListeners), "mock rejection", route.GetRouteNamespacedName(), route.GetRouteKind(), route.GetRouteGeneration(), matchedParentRef)
	return nil, &rd, nil
}

// helper to build a gateway parentRef for routes
func gwParentRef(gwName string) gwv1.ParentReference {
	return gwv1.ParentReference{
		Name: gwv1.ObjectName(gwName),
	}
}

// helper to build a gateway parentRef with a specific section name
func gwParentRefWithSection(gwName string, sectionName string) gwv1.ParentReference {
	sn := gwv1.SectionName(sectionName)
	return gwv1.ParentReference{
		Name:        gwv1.ObjectName(gwName),
		SectionName: &sn,
	}
}

// helper to build a gateway parentRef with a specific port
func gwParentRefWithPort(gwName string, port int32) gwv1.ParentReference {
	p := gwv1.PortNumber(port)
	return gwv1.ParentReference{
		Name: gwv1.ObjectName(gwName),
		Port: &p,
	}
}

// helper to build a listener set parentRef for routes
func lsParentRef(lsName, lsNamespace string) gwv1.ParentReference {
	kind := gwv1.Kind(listenerSetKind)
	ns := gwv1.Namespace(lsNamespace)
	return gwv1.ParentReference{
		Kind:      &kind,
		Name:      gwv1.ObjectName(lsName),
		Namespace: &ns,
	}
}

func makeHTTPRoute(name, namespace string, parentRefs ...gwv1.ParentReference) preLoadRouteDescriptor {
	return convertHTTPRoute(gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: parentRefs,
			},
		},
	})
}

func makeGRPCRoute(name, namespace string, parentRefs ...gwv1.ParentReference) preLoadRouteDescriptor {
	return convertGRPCRoute(gwv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gwv1.GRPCRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: parentRefs,
			},
		},
	})
}

func emptyListenerSetLoadResult() listenerSetLoadResult {
	return listenerSetLoadResult{
		listenersPerListenerSet: make(map[types.NamespacedName][]listenerSetListenerSource),
		acceptedListenerSets:    make(map[types.NamespacedName]gwv1.ListenerSet),
	}
}

func Test_mapListenersAndRoutes(t *testing.T) {
	gw := gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw1",
			Namespace: "ns-gw",
		},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType},
				{Name: "https", Port: 443, Protocol: gwv1.HTTPSProtocolType},
			},
		},
	}

	route1 := makeHTTPRoute("route1", "ns-gw", gwParentRef("gw1"))
	route2 := makeHTTPRoute("route2", "ns-gw", gwParentRef("gw1"))

	testCases := []struct {
		name                      string
		gw                        gwv1.Gateway
		listeners                 allListeners
		routes                    []preLoadRouteDescriptor
		attachmentResults         map[string]mockAttachmentResult
		expectedRoutesByPort      map[int32][]preLoadRouteDescriptor
		expectedRoutesPerListener map[gwv1.SectionName]int32
		expectedFailedCount       int
		expectedMatchedParentRefs map[string][]gwv1.ParentReference
		expectErr                 bool
	}{
		{
			name:      "no routes, no output",
			gw:        gw,
			listeners: allListeners{GatewayListeners: gw.Spec.Listeners, ListenerSetListeners: emptyListenerSetLoadResult()},
			routes:    []preLoadRouteDescriptor{},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {},
				443: {},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 0, "https": 0},
			expectedFailedCount:       0,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{},
		},
		{
			name:      "routes mapped to specific listeners",
			gw:        gw,
			listeners: allListeners{GatewayListeners: gw.Spec.Listeners, ListenerSetListeners: emptyListenerSetLoadResult()},
			routes:    []preLoadRouteDescriptor{route1, route2},
			attachmentResults: map[string]mockAttachmentResult{
				// route1 allowed on http only
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[0], route1): {compatibleHostnames: nil},
				// route2 allowed on https only
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[1], route2): {compatibleHostnames: nil},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {route1},
				443: {route2},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 1, "https": 1},
			expectedFailedCount:       0,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{
				route1.GetRouteIdentifier(): {gwParentRef("gw1")},
				route2.GetRouteIdentifier(): {gwParentRef("gw1")},
			},
		},
		{
			name:      "all routes to all listeners",
			gw:        gw,
			listeners: allListeners{GatewayListeners: gw.Spec.Listeners, ListenerSetListeners: emptyListenerSetLoadResult()},
			routes:    []preLoadRouteDescriptor{route1, route2},
			attachmentResults: map[string]mockAttachmentResult{
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[0], route1): {},
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[1], route1): {},
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[0], route2): {},
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[1], route2): {},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {route1, route2},
				443: {route1, route2},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 2, "https": 2},
			expectedFailedCount:       0,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{
				route1.GetRouteIdentifier(): {gwParentRef("gw1")},
				route2.GetRouteIdentifier(): {gwParentRef("gw1")},
			},
		},
		{
			name: "deduplication when multiple listeners share a port",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "ns-gw"},
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{Name: "http-a", Port: 80},
						{Name: "http-b", Port: 80},
					},
				},
			},
			listeners: allListeners{
				GatewayListeners: []gwv1.Listener{
					{Name: "http-a", Port: 80},
					{Name: "http-b", Port: 80},
				},
				ListenerSetListeners: emptyListenerSetLoadResult(),
			},
			routes: []preLoadRouteDescriptor{route1},
			attachmentResults: map[string]mockAttachmentResult{
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gwv1.Listener{Name: "http-a", Port: 80}, route1): {},
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gwv1.Listener{Name: "http-b", Port: 80}, route1): {},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80: {route1}, // deduplicated
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http-a": 1, "http-b": 1},
			expectedFailedCount:       0,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{
				route1.GetRouteIdentifier(): {gwParentRef("gw1")},
			},
		},
		{
			name:      "route rejected by all listeners produces failed route",
			gw:        gw,
			listeners: allListeners{GatewayListeners: gw.Spec.Listeners, ListenerSetListeners: emptyListenerSetLoadResult()},
			routes:    []preLoadRouteDescriptor{route1},
			// no attachment results → default mock rejection for all combos
			attachmentResults: map[string]mockAttachmentResult{},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {},
				443: {},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 0, "https": 0},
			expectedFailedCount:       2, // one rejection per listener
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{},
		},
		{
			name:              "route with wrong gateway name is ignored",
			gw:                gw,
			listeners:         allListeners{GatewayListeners: gw.Spec.Listeners, ListenerSetListeners: emptyListenerSetLoadResult()},
			routes:            []preLoadRouteDescriptor{makeHTTPRoute("route-wrong", "ns-gw", gwParentRef("other-gw"))},
			attachmentResults: map[string]mockAttachmentResult{},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {},
				443: {},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 0, "https": 0},
			expectedFailedCount:       0,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{},
		},
		{
			name:      "attachment helper error is propagated",
			gw:        gw,
			listeners: allListeners{GatewayListeners: gw.Spec.Listeners, ListenerSetListeners: emptyListenerSetLoadResult()},
			routes:    []preLoadRouteDescriptor{route1},
			attachmentResults: map[string]mockAttachmentResult{
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[0], route1): {err: fmt.Errorf("k8s error")},
			},
			expectErr: true,
		},
		{
			name:      "compatible hostnames are accumulated",
			gw:        gw,
			listeners: allListeners{GatewayListeners: gw.Spec.Listeners, ListenerSetListeners: emptyListenerSetLoadResult()},
			routes:    []preLoadRouteDescriptor{route1},
			attachmentResults: map[string]mockAttachmentResult{
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[0], route1): {compatibleHostnames: []gwv1.Hostname{"foo.example.com"}},
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[1], route1): {compatibleHostnames: []gwv1.Hostname{"bar.example.com"}},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {route1},
				443: {route1},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 1, "https": 1},
			expectedFailedCount:       0,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{
				route1.GetRouteIdentifier(): {gwParentRef("gw1")},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mapper := listenerToRouteMapperImpl{
				listenerAttachmentHelper: &mockListenerAttachmentHelper{results: tc.attachmentResults},
				logger:                   logr.Discard(),
			}
			result, err := mapper.mapListenersAndRoutes(context.Background(), tc.gw, tc.listeners, tc.routes)

			if tc.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, len(tc.expectedRoutesByPort), len(result.routesByPort))
			for port, expectedRoutes := range tc.expectedRoutesByPort {
				assert.ElementsMatch(t, expectedRoutes, result.routesByPort[port], "port %d", port)
			}
			if tc.expectedRoutesPerListener != nil {
				assert.Equal(t, tc.expectedRoutesPerListener, result.routesPerListener)
			}
			assert.Equal(t, tc.expectedFailedCount, len(result.failedRoutes))
			assert.Equal(t, tc.expectedMatchedParentRefs, result.matchedParentRefs)
		})
	}
}

func Test_mapListenersAndRoutes_sectionName(t *testing.T) {
	gw := gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "ns-gw"},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "http", Port: 80},
				{Name: "https", Port: 443},
			},
		},
	}

	testCases := []struct {
		name                      string
		routes                    []preLoadRouteDescriptor
		attachmentResults         map[string]mockAttachmentResult
		expectedRoutesByPort      map[int32][]preLoadRouteDescriptor
		expectedRoutesPerListener map[gwv1.SectionName]int32
		expectedFailedCount       int
	}{
		{
			name:   "route with sectionName targets specific listener",
			routes: []preLoadRouteDescriptor{makeHTTPRoute("route1", "ns-gw", gwParentRefWithSection("gw1", "https"))},
			attachmentResults: map[string]mockAttachmentResult{
				// parentRef has no Kind set (nil → defaults to Gateway in the key function)
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[1], makeHTTPRoute("route1", "ns-gw", gwParentRefWithSection("gw1", "https"))): {},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {},
				443: {makeHTTPRoute("route1", "ns-gw", gwParentRefWithSection("gw1", "https"))},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 0, "https": 1},
			expectedFailedCount:       0,
		},
		{
			name:              "route with non-existent sectionName produces NoMatchingParent failure",
			routes:            []preLoadRouteDescriptor{makeHTTPRoute("route1", "ns-gw", gwParentRefWithSection("gw1", "nonexistent"))},
			attachmentResults: map[string]mockAttachmentResult{},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {},
				443: {},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 0, "https": 0},
			expectedFailedCount:       1,
		},
		{
			name:   "route with port filter only matches listeners on that port",
			routes: []preLoadRouteDescriptor{makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 443))},
			attachmentResults: map[string]mockAttachmentResult{
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[1], makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 443))): {},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {},
				443: {makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 443))},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 0, "https": 1},
			expectedFailedCount:       0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mapper := listenerToRouteMapperImpl{
				listenerAttachmentHelper: &mockListenerAttachmentHelper{results: tc.attachmentResults},
				logger:                   logr.Discard(),
			}
			listeners := allListeners{GatewayListeners: gw.Spec.Listeners, ListenerSetListeners: emptyListenerSetLoadResult()}
			result, err := mapper.mapListenersAndRoutes(context.Background(), gw, listeners, tc.routes)

			assert.NoError(t, err)
			assert.Equal(t, len(tc.expectedRoutesByPort), len(result.routesByPort))
			for port, expectedRoutes := range tc.expectedRoutesByPort {
				assert.ElementsMatch(t, expectedRoutes, result.routesByPort[port], "port %d", port)
			}
			assert.Equal(t, tc.expectedRoutesPerListener, result.routesPerListener)
			assert.Equal(t, tc.expectedFailedCount, len(result.failedRoutes))
		})
	}
}

func Test_mapListenersAndRoutes_differentRouteKinds(t *testing.T) {
	gw := gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "ns-gw"},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "https", Port: 443, Protocol: gwv1.HTTPSProtocolType},
			},
		},
	}

	httpRoute := makeHTTPRoute("my-route", "ns-gw", gwParentRef("gw1"))
	grpcRoute := makeGRPCRoute("my-route", "ns-gw", gwParentRef("gw1"))

	mapper := listenerToRouteMapperImpl{
		listenerAttachmentHelper: &mockListenerAttachmentHelper{
			results: map[string]mockAttachmentResult{
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[0], httpRoute): {},
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[0], grpcRoute): {},
			},
		},
		logger: logr.Discard(),
	}

	listeners := allListeners{GatewayListeners: gw.Spec.Listeners, ListenerSetListeners: emptyListenerSetLoadResult()}
	result, err := mapper.mapListenersAndRoutes(context.Background(), gw, listeners, []preLoadRouteDescriptor{httpRoute, grpcRoute})

	assert.NoError(t, err)
	// Both routes should appear on port 443 since they have different route identifiers (kind is part of the ID)
	assert.Equal(t, 1, len(result.routesByPort))
	assert.ElementsMatch(t, []preLoadRouteDescriptor{httpRoute, grpcRoute}, result.routesByPort[443])
	assert.Equal(t, map[gwv1.SectionName]int32{"https": 2}, result.routesPerListener)
	assert.Equal(t, 0, len(result.failedRoutes))
	assert.Equal(t, map[string][]gwv1.ParentReference{
		httpRoute.GetRouteIdentifier(): {gwParentRef("gw1")},
		grpcRoute.GetRouteIdentifier(): {gwParentRef("gw1")},
	}, result.matchedParentRefs)
}

func Test_mapListenersAndRoutes_listenerSet(t *testing.T) {
	gw := gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "ns-gw"},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "http", Port: 80},
			},
		},
	}

	lsNsn := types.NamespacedName{Namespace: "ns-gw", Name: "my-ls"}
	lsListener := gwv1.Listener{Name: "ls-http", Port: 8080}

	listenerSetResult := listenerSetLoadResult{
		listenersPerListenerSet: map[types.NamespacedName][]listenerSetListenerSource{
			lsNsn: {
				{
					parentRef: gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Name: "my-ls", Namespace: "ns-gw"}},
					listener:  lsListener,
				},
			},
		},
		acceptedListenerSets: map[types.NamespacedName]gwv1.ListenerSet{
			lsNsn: {ObjectMeta: metav1.ObjectMeta{Name: "my-ls", Namespace: "ns-gw"}},
		},
	}

	// Route attaching to the gateway
	gwRoute := makeHTTPRoute("gw-route", "ns-gw", gwParentRef("gw1"))
	// Route attaching to the listener set
	lsRoute := makeHTTPRoute("ls-route", "ns-gw", lsParentRef("my-ls", "ns-gw"))

	lsParent := lsParentRef("my-ls", "ns-gw")

	mapper := listenerToRouteMapperImpl{
		listenerAttachmentHelper: &mockListenerAttachmentHelper{
			results: map[string]mockAttachmentResult{
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[0], gwRoute): {},
				mockAttachmentKey(lsParent, lsListener, lsRoute):                                    {},
			},
		},
		logger: logr.Discard(),
	}

	listeners := allListeners{GatewayListeners: gw.Spec.Listeners, ListenerSetListeners: listenerSetResult}
	result, err := mapper.mapListenersAndRoutes(context.Background(), gw, listeners, []preLoadRouteDescriptor{gwRoute, lsRoute})

	assert.NoError(t, err)
	assert.ElementsMatch(t, []preLoadRouteDescriptor{gwRoute}, result.routesByPort[80])
	assert.ElementsMatch(t, []preLoadRouteDescriptor{lsRoute}, result.routesByPort[8080])
	assert.Equal(t, 0, len(result.failedRoutes))
	assert.Equal(t, map[string][]gwv1.ParentReference{
		gwRoute.GetRouteIdentifier(): {gwParentRef("gw1")},
		lsRoute.GetRouteIdentifier(): {lsParent},
	}, result.matchedParentRefs)
}

func Test_mapListenersAndRoutes_listenerSetAndGatewaySameListenerName(t *testing.T) {
	gw := gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "ns-gw"},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "shared-name", Port: 80},
			},
		},
	}

	lsNsn := types.NamespacedName{Namespace: "ns-gw", Name: "my-ls"}
	// Listener set listener has the same name as the gateway listener but different port
	lsListener := gwv1.Listener{Name: "shared-name", Port: 9090}

	listenerSetResult := listenerSetLoadResult{
		listenersPerListenerSet: map[types.NamespacedName][]listenerSetListenerSource{
			lsNsn: {{
				parentRef: gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Name: "my-ls", Namespace: "ns-gw"}},
				listener:  lsListener,
			}},
		},
		acceptedListenerSets: map[types.NamespacedName]gwv1.ListenerSet{
			lsNsn: {ObjectMeta: metav1.ObjectMeta{Name: "my-ls", Namespace: "ns-gw"}},
		},
	}

	gwRoute := makeHTTPRoute("gw-route", "ns-gw", gwParentRef("gw1"))
	lsRoute := makeHTTPRoute("ls-route", "ns-gw", lsParentRef("my-ls", "ns-gw"))

	lsParent := lsParentRef("my-ls", "ns-gw")

	mapper := listenerToRouteMapperImpl{
		listenerAttachmentHelper: &mockListenerAttachmentHelper{
			results: map[string]mockAttachmentResult{
				// Gateway listener "shared-name" on port 80 allows gwRoute
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[0], gwRoute): {},
				// ListenerSet listener "shared-name" on port 9090 allows lsRoute
				mockAttachmentKey(lsParent, lsListener, lsRoute): {},
			},
		},
		logger: logr.Discard(),
	}

	listeners := allListeners{GatewayListeners: gw.Spec.Listeners, ListenerSetListeners: listenerSetResult}
	result, err := mapper.mapListenersAndRoutes(context.Background(), gw, listeners, []preLoadRouteDescriptor{gwRoute, lsRoute})

	assert.NoError(t, err)
	// Routes should be on different ports despite listeners sharing a name
	assert.ElementsMatch(t, []preLoadRouteDescriptor{gwRoute}, result.routesByPort[80])
	assert.ElementsMatch(t, []preLoadRouteDescriptor{lsRoute}, result.routesByPort[9090])
	assert.Equal(t, 0, len(result.failedRoutes))
}

func Test_mapListenersAndRoutes_compatibleHostnamesAccumulated(t *testing.T) {
	gw := gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "ns-gw"},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "http-a", Port: 80},
				{Name: "http-b", Port: 80},
			},
		},
	}

	route := makeHTTPRoute("route1", "ns-gw", gwParentRef("gw1"))

	mapper := listenerToRouteMapperImpl{
		listenerAttachmentHelper: &mockListenerAttachmentHelper{
			results: map[string]mockAttachmentResult{
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[0], route): {compatibleHostnames: []gwv1.Hostname{"a.example.com"}},
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[1], route): {compatibleHostnames: []gwv1.Hostname{"b.example.com"}},
			},
		},
		logger: logr.Discard(),
	}

	listeners := allListeners{GatewayListeners: gw.Spec.Listeners, ListenerSetListeners: emptyListenerSetLoadResult()}
	result, err := mapper.mapListenersAndRoutes(context.Background(), gw, listeners, []preLoadRouteDescriptor{route})

	assert.NoError(t, err)
	// Both hostnames should be accumulated for the route on port 80
	routeID := route.GetRouteIdentifier()
	hostnameSet := result.compatibleHostnamesByPort[80][routeID]
	assert.True(t, hostnameSet.Has("a.example.com"))
	assert.True(t, hostnameSet.Has("b.example.com"))
	assert.Equal(t, 2, hostnameSet.Len())
}

func Test_mapListenersAndRoutes_partialAttachmentSucceeds(t *testing.T) {
	// Per Gateway API spec: if at least one listener accepts, the route is considered attached
	gw := gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "ns-gw"},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "http", Port: 80},
				{Name: "https", Port: 443},
			},
		},
	}

	route := makeHTTPRoute("route1", "ns-gw", gwParentRef("gw1"))

	rejectedRD := GenerateRouteData(false, true, string(gwv1.RouteReasonNotAllowedByListeners), "rejected", route.GetRouteNamespacedName(), route.GetRouteKind(), route.GetRouteGeneration(), gwParentRef("gw1"))

	mapper := listenerToRouteMapperImpl{
		listenerAttachmentHelper: &mockListenerAttachmentHelper{
			results: map[string]mockAttachmentResult{
				// http listener accepts
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[0], route): {},
				// https listener rejects
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[1], route): {failedRouteData: &rejectedRD},
			},
		},
		logger: logr.Discard(),
	}

	listeners := allListeners{GatewayListeners: gw.Spec.Listeners, ListenerSetListeners: emptyListenerSetLoadResult()}
	result, err := mapper.mapListenersAndRoutes(context.Background(), gw, listeners, []preLoadRouteDescriptor{route})

	assert.NoError(t, err)
	// Route should still be mapped since at least one listener accepted
	assert.ElementsMatch(t, []preLoadRouteDescriptor{route}, result.routesByPort[80])
	// No failed routes because partial attachment counts as success
	assert.Equal(t, 0, len(result.failedRoutes))
	assert.Equal(t, map[string][]gwv1.ParentReference{
		route.GetRouteIdentifier(): {gwParentRef("gw1")},
	}, result.matchedParentRefs)
}

func Test_extractListenersFromSources(t *testing.T) {
	sources := []listenerSetListenerSource{
		{listener: gwv1.Listener{Name: "a", Port: 80}},
		{listener: gwv1.Listener{Name: "b", Port: 443}},
	}
	result := extractListenersFromSources(sources)
	assert.Equal(t, 2, len(result))
	assert.Equal(t, gwv1.SectionName("a"), result[0].Name)
	assert.Equal(t, gwv1.SectionName("b"), result[1].Name)
}

func Test_mapListenersAndRoutes_gatewayAndListenerSetCombinations(t *testing.T) {
	gw := gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "ns-gw"},
	}
	lsNsn := types.NamespacedName{Namespace: "ns-gw", Name: "my-ls"}
	lsObj := gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Name: "my-ls", Namespace: "ns-gw"}}
	lsParent := lsParentRef("my-ls", "ns-gw")
	gwParent := gwParentRef("gw1")

	testCases := []struct {
		name                      string
		gwListeners               []gwv1.Listener
		lsListeners               []gwv1.Listener
		routes                    []preLoadRouteDescriptor
		attachmentResults         map[string]mockAttachmentResult
		expectedRoutesByPort      map[int32][]preLoadRouteDescriptor
		expectedRoutesPerListener map[gwv1.SectionName]int32
		expectedFailedCount       int
		expectedMatchedParentRefs map[string][]gwv1.ParentReference
	}{
		{
			name:        "distinct listeners from gateway and listener set",
			gwListeners: []gwv1.Listener{{Name: "gw-http", Port: 80}},
			lsListeners: []gwv1.Listener{{Name: "ls-https", Port: 443}},
			routes: []preLoadRouteDescriptor{
				makeHTTPRoute("gw-route", "ns-gw", gwParent),
				makeHTTPRoute("ls-route", "ns-gw", lsParent),
			},
			attachmentResults: map[string]mockAttachmentResult{
				mockAttachmentKey(gwParent, gwv1.Listener{Name: "gw-http", Port: 80}, makeHTTPRoute("gw-route", "ns-gw", gwParent)):   {},
				mockAttachmentKey(lsParent, gwv1.Listener{Name: "ls-https", Port: 443}, makeHTTPRoute("ls-route", "ns-gw", lsParent)): {},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {makeHTTPRoute("gw-route", "ns-gw", gwParent)},
				443: {makeHTTPRoute("ls-route", "ns-gw", lsParent)},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"gw-http": 1},
			expectedFailedCount:       0,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{
				makeHTTPRoute("gw-route", "ns-gw", gwParent).GetRouteIdentifier(): {gwParent},
				makeHTTPRoute("ls-route", "ns-gw", lsParent).GetRouteIdentifier(): {lsParent},
			},
		},
		{
			name:        "distinct listeners with route overlap",
			gwListeners: []gwv1.Listener{{Name: "gw-http", Port: 80}},
			lsListeners: []gwv1.Listener{{Name: "ls-https", Port: 443}},
			routes: []preLoadRouteDescriptor{
				// This route attaches to both the gateway and the listener set
				makeHTTPRoute("shared-route", "ns-gw", gwParent, lsParent),
			},
			attachmentResults: map[string]mockAttachmentResult{
				mockAttachmentKey(gwParent, gwv1.Listener{Name: "gw-http", Port: 80}, makeHTTPRoute("shared-route", "ns-gw", gwParent, lsParent)):   {},
				mockAttachmentKey(lsParent, gwv1.Listener{Name: "ls-https", Port: 443}, makeHTTPRoute("shared-route", "ns-gw", gwParent, lsParent)): {},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {makeHTTPRoute("shared-route", "ns-gw", gwParent, lsParent)},
				443: {makeHTTPRoute("shared-route", "ns-gw", gwParent, lsParent)},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"gw-http": 1},
			expectedFailedCount:       0,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{
				makeHTTPRoute("shared-route", "ns-gw", gwParent, lsParent).GetRouteIdentifier(): {gwParent, lsParent},
			},
		},
		{
			name:        "same section name on gateway and listener set",
			gwListeners: []gwv1.Listener{{Name: "shared-name", Port: 80}},
			lsListeners: []gwv1.Listener{{Name: "shared-name", Port: 443}},
			routes: []preLoadRouteDescriptor{
				makeHTTPRoute("gw-route", "ns-gw", gwParent),
				makeHTTPRoute("ls-route", "ns-gw", lsParent),
			},
			attachmentResults: map[string]mockAttachmentResult{
				mockAttachmentKey(gwParent, gwv1.Listener{Name: "shared-name", Port: 80}, makeHTTPRoute("gw-route", "ns-gw", gwParent)):  {},
				mockAttachmentKey(lsParent, gwv1.Listener{Name: "shared-name", Port: 443}, makeHTTPRoute("ls-route", "ns-gw", lsParent)): {},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {makeHTTPRoute("gw-route", "ns-gw", gwParent)},
				443: {makeHTTPRoute("ls-route", "ns-gw", lsParent)},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"shared-name": 1},
			expectedFailedCount:       0,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{
				makeHTTPRoute("gw-route", "ns-gw", gwParent).GetRouteIdentifier(): {gwParent},
				makeHTTPRoute("ls-route", "ns-gw", lsParent).GetRouteIdentifier(): {lsParent},
			},
		},
		{
			name:        "same port on gateway and listener set",
			gwListeners: []gwv1.Listener{{Name: "gw-http", Port: 80}},
			lsListeners: []gwv1.Listener{{Name: "ls-http", Port: 80}},
			routes: []preLoadRouteDescriptor{
				makeHTTPRoute("gw-route", "ns-gw", gwParent),
				makeHTTPRoute("ls-route", "ns-gw", lsParent),
			},
			attachmentResults: map[string]mockAttachmentResult{
				mockAttachmentKey(gwParent, gwv1.Listener{Name: "gw-http", Port: 80}, makeHTTPRoute("gw-route", "ns-gw", gwParent)): {},
				mockAttachmentKey(lsParent, gwv1.Listener{Name: "ls-http", Port: 80}, makeHTTPRoute("ls-route", "ns-gw", lsParent)): {},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80: {makeHTTPRoute("gw-route", "ns-gw", gwParent), makeHTTPRoute("ls-route", "ns-gw", lsParent)},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"gw-http": 1},
			expectedFailedCount:       0,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{
				makeHTTPRoute("gw-route", "ns-gw", gwParent).GetRouteIdentifier(): {gwParent},
				makeHTTPRoute("ls-route", "ns-gw", lsParent).GetRouteIdentifier(): {lsParent},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gw := gw.DeepCopy()
			gw.Spec.Listeners = tc.gwListeners

			lsSources := make([]listenerSetListenerSource, 0, len(tc.lsListeners))
			for _, l := range tc.lsListeners {
				lsSources = append(lsSources, listenerSetListenerSource{parentRef: lsObj, listener: l})
			}

			listeners := allListeners{
				GatewayListeners: tc.gwListeners,
				ListenerSetListeners: listenerSetLoadResult{
					listenersPerListenerSet: map[types.NamespacedName][]listenerSetListenerSource{lsNsn: lsSources},
					acceptedListenerSets:    map[types.NamespacedName]gwv1.ListenerSet{lsNsn: lsObj},
				},
			}

			mapper := listenerToRouteMapperImpl{
				listenerAttachmentHelper: &mockListenerAttachmentHelper{results: tc.attachmentResults},
				logger:                   logr.Discard(),
			}

			result, err := mapper.mapListenersAndRoutes(context.Background(), *gw, listeners, tc.routes)
			assert.NoError(t, err)
			assert.Equal(t, len(tc.expectedRoutesByPort), len(result.routesByPort), "routesByPort length mismatch")
			for port, expectedRoutes := range tc.expectedRoutesByPort {
				assert.ElementsMatch(t, expectedRoutes, result.routesByPort[port], "port %d", port)
			}
			assert.Equal(t, tc.expectedRoutesPerListener, result.routesPerListener)
			assert.Equal(t, tc.expectedFailedCount, len(result.failedRoutes))
			assert.Equal(t, tc.expectedMatchedParentRefs, result.matchedParentRefs)
		})
	}
}

// helper to build a gateway parentRef with both section name and port
func gwParentRefWithSectionAndPort(gwName string, sectionName string, port int32) gwv1.ParentReference {
	sn := gwv1.SectionName(sectionName)
	p := gwv1.PortNumber(port)
	return gwv1.ParentReference{
		Name:        gwv1.ObjectName(gwName),
		SectionName: &sn,
		Port:        &p,
	}
}

func Test_mapListenersAndRoutes_sectionNameWithPortFiltering(t *testing.T) {
	gw := gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "ns-gw"},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "http", Port: 80},
				{Name: "https", Port: 443},
			},
		},
	}
	listeners := allListeners{GatewayListeners: gw.Spec.Listeners, ListenerSetListeners: emptyListenerSetLoadResult()}

	testCases := []struct {
		name                      string
		routes                    []preLoadRouteDescriptor
		attachmentResults         map[string]mockAttachmentResult
		expectedRoutesByPort      map[int32][]preLoadRouteDescriptor
		expectedRoutesPerListener map[gwv1.SectionName]int32
		expectedFailedCount       int
		expectedMatchedParentRefs map[string][]gwv1.ParentReference
	}{
		{
			name: "sectionName matches and port matches - route attaches",
			routes: []preLoadRouteDescriptor{
				makeHTTPRoute("route1", "ns-gw", gwParentRefWithSectionAndPort("gw1", "https", 443)),
			},
			attachmentResults: map[string]mockAttachmentResult{
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gw.Spec.Listeners[1],
					makeHTTPRoute("route1", "ns-gw", gwParentRefWithSectionAndPort("gw1", "https", 443))): {},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {},
				443: {makeHTTPRoute("route1", "ns-gw", gwParentRefWithSectionAndPort("gw1", "https", 443))},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 0, "https": 1},
			expectedFailedCount:       0,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{
				makeHTTPRoute("route1", "ns-gw", gwParentRefWithSectionAndPort("gw1", "https", 443)).GetRouteIdentifier(): {
					gwParentRefWithSectionAndPort("gw1", "https", 443),
				},
			},
		},
		{
			name: "sectionName matches but port mismatches - route is rejected with NoMatchingParent",
			routes: []preLoadRouteDescriptor{
				// Route targets listener "https" but specifies port 80 (https listener is on 443)
				makeHTTPRoute("route1", "ns-gw", gwParentRefWithSectionAndPort("gw1", "https", 80)),
			},
			attachmentResults: map[string]mockAttachmentResult{},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {},
				443: {},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 0, "https": 0},
			// The route gets a NoMatchingParent failure because the sectionName lookup succeeds
			// but the port check fails, so no attachment is attempted and failedRoutes gets a
			// synthetic NoMatchingParent entry.
			expectedFailedCount:       1,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{},
		},
		{
			name: "sectionName matches, port matches, but listener rejects - route fails with listener rejection",
			routes: []preLoadRouteDescriptor{
				makeHTTPRoute("route1", "ns-gw", gwParentRefWithSectionAndPort("gw1", "http", 80)),
			},
			// No entry in attachmentResults → mock returns default rejection
			attachmentResults: map[string]mockAttachmentResult{},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {},
				443: {},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 0, "https": 0},
			// Attachment was attempted (sectionName+port matched) but the listener rejected it,
			// so we get 1 failed route from the listener rejection itself.
			expectedFailedCount:       1,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mapper := listenerToRouteMapperImpl{
				listenerAttachmentHelper: &mockListenerAttachmentHelper{results: tc.attachmentResults},
				logger:                   logr.Discard(),
			}
			result, err := mapper.mapListenersAndRoutes(context.Background(), gw, listeners, tc.routes)

			assert.NoError(t, err)
			assert.Equal(t, len(tc.expectedRoutesByPort), len(result.routesByPort))
			for port, expectedRoutes := range tc.expectedRoutesByPort {
				assert.ElementsMatch(t, expectedRoutes, result.routesByPort[port], "port %d", port)
			}
			assert.Equal(t, tc.expectedRoutesPerListener, result.routesPerListener)
			assert.Equal(t, tc.expectedFailedCount, len(result.failedRoutes))
			assert.Equal(t, tc.expectedMatchedParentRefs, result.matchedParentRefs)
		})
	}
}

func Test_mapListenersAndRoutes_portFilteringNoSectionName(t *testing.T) {
	testCases := []struct {
		name                      string
		gwListeners               []gwv1.Listener
		parentRef                 gwv1.ParentReference
		attachmentResults         map[string]mockAttachmentResult
		expectedRoutesByPort      map[int32][]preLoadRouteDescriptor
		expectedRoutesPerListener map[gwv1.SectionName]int32
		expectedFailedCount       int
		expectedMatchedParentRefs map[string][]gwv1.ParentReference
	}{
		{
			name:        "port filter with 1 listener - matching port attaches",
			gwListeners: []gwv1.Listener{{Name: "http", Port: 80}},
			parentRef:   gwParentRefWithPort("gw1", 80),
			attachmentResults: map[string]mockAttachmentResult{
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gwv1.Listener{Name: "http", Port: 80},
					makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 80))): {},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80: {makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 80))},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 1},
			expectedFailedCount:       0,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{
				makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 80)).GetRouteIdentifier(): {
					gwParentRefWithPort("gw1", 80),
				},
			},
		},
		{
			name:        "port filter with 1 listener - non-matching port produces NoMatchingParent",
			gwListeners: []gwv1.Listener{{Name: "http", Port: 80}},
			parentRef:   gwParentRefWithPort("gw1", 443),
			// No listeners match port 443, so no attachment is attempted
			attachmentResults: map[string]mockAttachmentResult{},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80: {},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 0},
			expectedFailedCount:       1,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{},
		},
		{
			name: "port filter with 2 listeners - only matching port listener is tried",
			gwListeners: []gwv1.Listener{
				{Name: "http", Port: 80},
				{Name: "https", Port: 443},
			},
			parentRef: gwParentRefWithPort("gw1", 443),
			attachmentResults: map[string]mockAttachmentResult{
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gwv1.Listener{Name: "https", Port: 443},
					makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 443))): {},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {},
				443: {makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 443))},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 0, "https": 1},
			expectedFailedCount:       0,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{
				makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 443)).GetRouteIdentifier(): {
					gwParentRefWithPort("gw1", 443),
				},
			},
		},
		{
			name: "port filter with 2 listeners sharing same port - route attaches to both",
			gwListeners: []gwv1.Listener{
				{Name: "http-a", Port: 80},
				{Name: "http-b", Port: 80},
			},
			parentRef: gwParentRefWithPort("gw1", 80),
			attachmentResults: map[string]mockAttachmentResult{
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gwv1.Listener{Name: "http-a", Port: 80},
					makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 80))): {},
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gwv1.Listener{Name: "http-b", Port: 80},
					makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 80))): {},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80: {makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 80))},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http-a": 1, "http-b": 1},
			expectedFailedCount:       0,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{
				makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 80)).GetRouteIdentifier(): {
					gwParentRefWithPort("gw1", 80),
				},
			},
		},
		{
			name: "port filter with 3 listeners - only the 2 matching port listeners are tried",
			gwListeners: []gwv1.Listener{
				{Name: "http", Port: 80},
				{Name: "https", Port: 443},
				{Name: "alt-https", Port: 443},
			},
			parentRef: gwParentRefWithPort("gw1", 443),
			attachmentResults: map[string]mockAttachmentResult{
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gwv1.Listener{Name: "https", Port: 443},
					makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 443))): {},
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gwv1.Listener{Name: "alt-https", Port: 443},
					makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 443))): {},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:  {},
				443: {makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 443))},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 0, "https": 1, "alt-https": 1},
			expectedFailedCount:       0,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{
				makeHTTPRoute("route1", "ns-gw", gwParentRefWithPort("gw1", 443)).GetRouteIdentifier(): {
					gwParentRefWithPort("gw1", 443),
				},
			},
		},
		{
			name: "port filter with 3 listeners - no listeners match the port",
			gwListeners: []gwv1.Listener{
				{Name: "http", Port: 80},
				{Name: "https", Port: 443},
				{Name: "grpc", Port: 50051},
			},
			parentRef:         gwParentRefWithPort("gw1", 8080),
			attachmentResults: map[string]mockAttachmentResult{},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:    {},
				443:   {},
				50051: {},
			},
			expectedRoutesPerListener: map[gwv1.SectionName]int32{"http": 0, "https": 0, "grpc": 0},
			expectedFailedCount:       1,
			expectedMatchedParentRefs: map[string][]gwv1.ParentReference{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gw := gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "ns-gw"},
				Spec:       gwv1.GatewaySpec{Listeners: tc.gwListeners},
			}
			route := makeHTTPRoute("route1", "ns-gw", tc.parentRef)
			listeners := allListeners{GatewayListeners: tc.gwListeners, ListenerSetListeners: emptyListenerSetLoadResult()}

			mapper := listenerToRouteMapperImpl{
				listenerAttachmentHelper: &mockListenerAttachmentHelper{results: tc.attachmentResults},
				logger:                   logr.Discard(),
			}
			result, err := mapper.mapListenersAndRoutes(context.Background(), gw, listeners, []preLoadRouteDescriptor{route})

			assert.NoError(t, err)
			assert.Equal(t, len(tc.expectedRoutesByPort), len(result.routesByPort), "routesByPort length mismatch")
			for port, expectedRoutes := range tc.expectedRoutesByPort {
				assert.ElementsMatch(t, expectedRoutes, result.routesByPort[port], "port %d", port)
			}
			assert.Equal(t, tc.expectedRoutesPerListener, result.routesPerListener)
			assert.Equal(t, tc.expectedFailedCount, len(result.failedRoutes))
			assert.Equal(t, tc.expectedMatchedParentRefs, result.matchedParentRefs)
		})
	}
}

func Test_mapListenersAndRoutes_attachedListeners(t *testing.T) {
	testCases := []struct {
		name                      string
		gwListeners               []gwv1.Listener
		lsListeners               []gwv1.Listener
		routes                    []preLoadRouteDescriptor
		attachmentResults         map[string]mockAttachmentResult
		expectedAttachedListeners []gwv1.Listener
		expectedRoutesByPort      map[int32][]preLoadRouteDescriptor
	}{
		{
			name:                      "no listeners produces empty attachedListeners",
			gwListeners:               []gwv1.Listener{},
			routes:                    []preLoadRouteDescriptor{},
			attachmentResults:         map[string]mockAttachmentResult{},
			expectedAttachedListeners: []gwv1.Listener{},
		},
		{
			name:        "gateway listeners are included in attachedListeners",
			gwListeners: []gwv1.Listener{{Name: "http", Port: 80}, {Name: "https", Port: 443}},
			routes:      []preLoadRouteDescriptor{},
			attachmentResults: map[string]mockAttachmentResult{},
			expectedAttachedListeners: []gwv1.Listener{
				{Name: "http", Port: 80},
				{Name: "https", Port: 443},
			},
		},
		{
			name:        "single gateway listener is included",
			gwListeners: []gwv1.Listener{{Name: "http", Port: 80}},
			routes:      []preLoadRouteDescriptor{},
			attachmentResults: map[string]mockAttachmentResult{},
			expectedAttachedListeners: []gwv1.Listener{
				{Name: "http", Port: 80},
			},
		},
		{
			name:        "listener set listeners are included in attachedListeners",
			gwListeners: []gwv1.Listener{{Name: "http", Port: 80}},
			lsListeners: []gwv1.Listener{{Name: "ls-https", Port: 443}},
			routes:      []preLoadRouteDescriptor{},
			attachmentResults: map[string]mockAttachmentResult{},
			expectedAttachedListeners: []gwv1.Listener{
				{Name: "http", Port: 80},
				{Name: "ls-https", Port: 443},
			},
		},
		{
			name:        "gateway and listener set listeners with same name are both included",
			gwListeners: []gwv1.Listener{{Name: "shared", Port: 80}},
			lsListeners: []gwv1.Listener{{Name: "shared", Port: 443}},
			routes:      []preLoadRouteDescriptor{},
			attachmentResults: map[string]mockAttachmentResult{},
			expectedAttachedListeners: []gwv1.Listener{
				{Name: "shared", Port: 80},
				{Name: "shared", Port: 443},
			},
		},
		{
			name:        "listeners are included regardless of whether routes attach",
			gwListeners: []gwv1.Listener{{Name: "http", Port: 80}, {Name: "https", Port: 443}},
			routes:      []preLoadRouteDescriptor{makeHTTPRoute("route1", "ns-gw", gwParentRef("gw1"))},
			attachmentResults: map[string]mockAttachmentResult{
				// Only http listener accepts the route; https rejects
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gwv1.Listener{Name: "http", Port: 80},
					makeHTTPRoute("route1", "ns-gw", gwParentRef("gw1"))): {},
			},
			expectedAttachedListeners: []gwv1.Listener{
				{Name: "http", Port: 80},
				{Name: "https", Port: 443},
			},
		},
		{
			name:        "listener with zero routes is still returned in attachedListeners and routesByPort",
			gwListeners: []gwv1.Listener{{Name: "http", Port: 80}, {Name: "https", Port: 443}, {Name: "grpc", Port: 50051}},
			routes:      []preLoadRouteDescriptor{makeHTTPRoute("route1", "ns-gw", gwParentRef("gw1"))},
			attachmentResults: map[string]mockAttachmentResult{
				// Only the http listener on port 80 accepts the route; https and grpc get no routes at all
				mockAttachmentKey(gwv1.ParentReference{Name: "gw1"}, gwv1.Listener{Name: "http", Port: 80},
					makeHTTPRoute("route1", "ns-gw", gwParentRef("gw1"))): {},
			},
			expectedAttachedListeners: []gwv1.Listener{
				{Name: "http", Port: 80},
				{Name: "https", Port: 443},
				{Name: "grpc", Port: 50051},
			},
			expectedRoutesByPort: map[int32][]preLoadRouteDescriptor{
				80:    {makeHTTPRoute("route1", "ns-gw", gwParentRef("gw1"))},
				443:   {},
				50051: {},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gw := gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "ns-gw"},
				Spec:       gwv1.GatewaySpec{Listeners: tc.gwListeners},
			}

			lsNsn := types.NamespacedName{Namespace: "ns-gw", Name: "my-ls"}
			lsObj := gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Name: "my-ls", Namespace: "ns-gw"}}

			var listenerSetResult listenerSetLoadResult
			if len(tc.lsListeners) > 0 {
				lsSources := make([]listenerSetListenerSource, 0, len(tc.lsListeners))
				for _, l := range tc.lsListeners {
					lsSources = append(lsSources, listenerSetListenerSource{parentRef: lsObj, listener: l})
				}
				listenerSetResult = listenerSetLoadResult{
					listenersPerListenerSet: map[types.NamespacedName][]listenerSetListenerSource{lsNsn: lsSources},
					acceptedListenerSets:    map[types.NamespacedName]gwv1.ListenerSet{lsNsn: lsObj},
				}
			} else {
				listenerSetResult = emptyListenerSetLoadResult()
			}

			listeners := allListeners{GatewayListeners: tc.gwListeners, ListenerSetListeners: listenerSetResult}

			mapper := listenerToRouteMapperImpl{
				listenerAttachmentHelper: &mockListenerAttachmentHelper{results: tc.attachmentResults},
				logger:                   logr.Discard(),
			}
			result, err := mapper.mapListenersAndRoutes(context.Background(), gw, listeners, tc.routes)

			assert.NoError(t, err)
			assert.ElementsMatch(t, tc.expectedAttachedListeners, result.attachedListeners)
			if tc.expectedRoutesByPort != nil {
				assert.Equal(t, len(tc.expectedRoutesByPort), len(result.routesByPort), "routesByPort length mismatch")
				for port, expectedRoutes := range tc.expectedRoutesByPort {
					assert.ElementsMatch(t, expectedRoutes, result.routesByPort[port], "port %d", port)
				}
			}
		})
	}
}
