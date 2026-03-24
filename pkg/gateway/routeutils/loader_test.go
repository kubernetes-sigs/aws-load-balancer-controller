package routeutils

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type mockMapper struct {
	t                  *testing.T
	expectedRoutes     []preLoadRouteDescriptor
	mapToReturn        map[int32][]preLoadRouteDescriptor
	listenerRouteCount map[gwv1.SectionName]int32
	routeStatusUpdates []RouteData
	matchedParentRefs  map[string][]gwv1.ParentReference
}

func (m *mockMapper) mapListenersAndRoutes(ctx context.Context, gw gwv1.Gateway, listeners allListeners, routes []preLoadRouteDescriptor) (listenerRouteMapResult, error) {
	assert.ElementsMatch(m.t, m.expectedRoutes, routes)
	matchedParentRefs := make(map[string][]gwv1.ParentReference)
	for _, routeList := range m.mapToReturn {
		for _, route := range routeList {
			routeKey := route.GetRouteIdentifier()
			matchedParentRefs[routeKey] = []gwv1.ParentReference{{
				Name:      "gw",
				Namespace: (*gwv1.Namespace)(new("gw-ns")),
			}}
		}
	}
	return listenerRouteMapResult{
		routesByPort:              m.mapToReturn,
		compatibleHostnamesByPort: make(map[int32]map[string]sets.Set[gwv1.Hostname]),
		failedRoutes:              m.routeStatusUpdates,
		matchedParentRefs:         m.matchedParentRefs,
		routesPerListener:         m.listenerRouteCount,
	}, nil
}

var _ RouteDescriptor = &mockRoute{}

type mockRoute struct {
	namespacedName            types.NamespacedName
	routeKind                 RouteKind
	generation                int64
	hostnames                 []gwv1.Hostname
	CompatibleHostnamesByPort map[int32][]gwv1.Hostname
}

func (m *mockRoute) GetCompatibleHostnamesByPort() map[int32][]gwv1.Hostname {
	return m.CompatibleHostnamesByPort
}

func (m *mockRoute) setCompatibleHostnamesByPort(hostnamesByPort map[int32][]gwv1.Hostname) {
	m.CompatibleHostnamesByPort = hostnamesByPort
}

func (m *mockRoute) loadAttachedRules(context context.Context, k8sClient client.Client, gatewayDefaultTGConfig *elbv2gw.TargetGroupConfiguration) (RouteDescriptor, []routeLoadError) {
	return m, nil
}

func (m *mockRoute) GetRouteNamespacedName() types.NamespacedName {
	return m.namespacedName
}

func (m *mockRoute) GetRouteKind() RouteKind {
	return m.routeKind
}

func (m *mockRoute) GetHostnames() []gwv1.Hostname {
	return m.hostnames
}

func (m *mockRoute) GetParentRefs() []gwv1.ParentReference {
	//TODO implement me
	panic("implement me")
}

func (m *mockRoute) GetBackendRefs() []gwv1.BackendRef {
	//TODO implement me
	panic("implement me")
}
func (m *mockRoute) GetRouteListenerRuleConfigRefs() []gwv1.LocalObjectReference {
	//TODO implement me
	panic("implement me")
}

func (m *mockRoute) GetRouteGeneration() int64 {
	return m.generation
}

func (m *mockRoute) GetRawRoute() interface{} {
	//TODO implement me
	panic("implement me")
}

func (m *mockRoute) GetAttachedRules() []RouteRule {
	//TODO implement me
	panic("implement me")
}

func (m *mockRoute) GetRouteCreateTimestamp() time.Time {
	panic("implement me")
}

func (m *mockRoute) GetRouteIdentifier() string {
	return string(m.GetRouteKind()) + "-" + m.GetRouteNamespacedName().String()
}

func Test_LoadRoutesForGateway(t *testing.T) {
	testNamespace := gwv1.Namespace("gw-ns")

	preLoadHTTPRoutes := []preLoadRouteDescriptor{
		&mockRoute{
			namespacedName: types.NamespacedName{
				Namespace: "http1-ns",
				Name:      "http1",
			},
			routeKind: HTTPRouteKind,
		},
		&mockRoute{
			namespacedName: types.NamespacedName{
				Namespace: "http2-ns",
				Name:      "http2",
			},
			routeKind: HTTPRouteKind,
		},
		&mockRoute{
			namespacedName: types.NamespacedName{
				Namespace: "http3-ns",
				Name:      "http3",
			},
			routeKind: HTTPRouteKind,
		},
	}

	loadedHTTPRoutes := make([]RouteDescriptor, 0)
	for _, preload := range preLoadHTTPRoutes {
		r, _ := preload.loadAttachedRules(nil, nil, nil)
		loadedHTTPRoutes = append(loadedHTTPRoutes, r)
	}

	preLoadTCPRoutes := []preLoadRouteDescriptor{
		&mockRoute{
			namespacedName: types.NamespacedName{
				Namespace: "tcp1-ns",
				Name:      "tcp1",
			},
			routeKind: TCPRouteKind,
		},
		&mockRoute{
			namespacedName: types.NamespacedName{
				Namespace: "tcp2-ns",
				Name:      "tcp2",
			},
			routeKind: TCPRouteKind,
		},
		&mockRoute{
			namespacedName: types.NamespacedName{
				Namespace: "tcp3-ns",
				Name:      "tcp3",
			},
			routeKind: TCPRouteKind,
		},
	}

	loadedTCPRoutes := make([]RouteDescriptor, 0)
	for _, preload := range preLoadTCPRoutes {
		r, _ := preload.loadAttachedRules(nil, nil, nil)
		loadedTCPRoutes = append(loadedTCPRoutes, r)
	}

	allRouteLoaders := map[RouteKind]func(ctx context.Context, k8sClient client.Client, opts ...client.ListOption) ([]preLoadRouteDescriptor, error){
		HTTPRouteKind: func(ctx context.Context, k8sClient client.Client, opts ...client.ListOption) ([]preLoadRouteDescriptor, error) {
			return preLoadHTTPRoutes, nil
		},
		TCPRouteKind: func(ctx context.Context, k8sClient client.Client, opts ...client.ListOption) ([]preLoadRouteDescriptor, error) {
			return preLoadTCPRoutes, nil
		},
	}

	testCases := []struct {
		name                     string
		acceptedKinds            sets.Set[RouteKind]
		expectedMap              map[int32][]RouteDescriptor
		expectedPreloadMap       map[int32][]preLoadRouteDescriptor
		parentRefs               map[string][]gwv1.ParentReference
		expectedPreMappedRoutes  []preLoadRouteDescriptor
		mapperRouteStatusUpdates []RouteData
		expectedReconcileQueue   map[string]bool // generateRouteDataCacheKey -> succeeded
		expectError              bool
	}{
		{
			name:                    "filter allows no routes",
			acceptedKinds:           make(sets.Set[RouteKind]),
			expectedPreMappedRoutes: make([]preLoadRouteDescriptor, 0),
			expectedMap:             make(map[int32][]RouteDescriptor),
			expectedReconcileQueue:  map[string]bool{},
			parentRefs:              map[string][]gwv1.ParentReference{},
		},
		{
			name:                    "filter only allows http route",
			acceptedKinds:           sets.New[RouteKind](HTTPRouteKind),
			expectedPreMappedRoutes: preLoadHTTPRoutes,
			parentRefs: map[string][]gwv1.ParentReference{
				preLoadHTTPRoutes[0].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadHTTPRoutes[1].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadHTTPRoutes[2].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
			},
			expectedPreloadMap: map[int32][]preLoadRouteDescriptor{
				80: preLoadHTTPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80: loadedHTTPRoutes,
			},
			expectedReconcileQueue: map[string]bool{
				"Gateway-http1-http1-ns-HTTPRoute-gw-gw-ns--": true,
				"Gateway-http2-http2-ns-HTTPRoute-gw-gw-ns--": true,
				"Gateway-http3-http3-ns-HTTPRoute-gw-gw-ns--": true,
			},
		},
		{
			name:                    "filter only allows http route - explicit section name",
			acceptedKinds:           sets.New[RouteKind](HTTPRouteKind),
			expectedPreMappedRoutes: preLoadHTTPRoutes,
			parentRefs: map[string][]gwv1.ParentReference{
				preLoadHTTPRoutes[0].GetRouteIdentifier(): {{
					Name:        "gw",
					Namespace:   (*gwv1.Namespace)(new("gw-ns")),
					SectionName: (*gwv1.SectionName)(new("sect1")),
				}},
				preLoadHTTPRoutes[1].GetRouteIdentifier(): {{
					Name:        "gw",
					Namespace:   (*gwv1.Namespace)(new("gw-ns")),
					SectionName: (*gwv1.SectionName)(new("sect2")),
				}},
				preLoadHTTPRoutes[2].GetRouteIdentifier(): {{
					Name:        "gw",
					Namespace:   (*gwv1.Namespace)(new("gw-ns")),
					SectionName: (*gwv1.SectionName)(new("sect3")),
				}},
			},
			expectedPreloadMap: map[int32][]preLoadRouteDescriptor{
				80: preLoadHTTPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80: loadedHTTPRoutes,
			},
			expectedReconcileQueue: map[string]bool{
				"Gateway-http1-http1-ns-HTTPRoute-gw-gw-ns--sect1": true,
				"Gateway-http2-http2-ns-HTTPRoute-gw-gw-ns--sect2": true,
				"Gateway-http3-http3-ns-HTTPRoute-gw-gw-ns--sect3": true,
			},
		},
		{
			name:                    "filter only allows http route - explicit port",
			acceptedKinds:           sets.New[RouteKind](HTTPRouteKind),
			expectedPreMappedRoutes: preLoadHTTPRoutes,
			parentRefs: map[string][]gwv1.ParentReference{
				preLoadHTTPRoutes[0].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
					Port:      new(gwv1.PortNumber(80)),
				}},
				preLoadHTTPRoutes[1].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
					Port:      new(gwv1.PortNumber(80)),
				}},
				preLoadHTTPRoutes[2].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
					Port:      new(gwv1.PortNumber(80)),
				}},
			},
			expectedPreloadMap: map[int32][]preLoadRouteDescriptor{
				80: preLoadHTTPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80: loadedHTTPRoutes,
			},
			expectedReconcileQueue: map[string]bool{
				"Gateway-http1-http1-ns-HTTPRoute-gw-gw-ns-80-": true,
				"Gateway-http2-http2-ns-HTTPRoute-gw-gw-ns-80-": true,
				"Gateway-http3-http3-ns-HTTPRoute-gw-gw-ns-80-": true,
			},
		},
		{
			name:                    "filter only allows http route - explicit port and section name",
			acceptedKinds:           sets.New[RouteKind](HTTPRouteKind),
			expectedPreMappedRoutes: preLoadHTTPRoutes,
			parentRefs: map[string][]gwv1.ParentReference{
				preLoadHTTPRoutes[0].GetRouteIdentifier(): {{
					Name:        "gw",
					Namespace:   (*gwv1.Namespace)(new("gw-ns")),
					Port:        new(gwv1.PortNumber(80)),
					SectionName: (*gwv1.SectionName)(new("sect1")),
				}},
				preLoadHTTPRoutes[1].GetRouteIdentifier(): {{
					Name:        "gw",
					Namespace:   (*gwv1.Namespace)(new("gw-ns")),
					Port:        new(gwv1.PortNumber(80)),
					SectionName: (*gwv1.SectionName)(new("sect2")),
				}},
				preLoadHTTPRoutes[2].GetRouteIdentifier(): {{
					Name:        "gw",
					Namespace:   (*gwv1.Namespace)(new("gw-ns")),
					Port:        new(gwv1.PortNumber(80)),
					SectionName: (*gwv1.SectionName)(new("sect3")),
				}},
			},
			expectedPreloadMap: map[int32][]preLoadRouteDescriptor{
				80: preLoadHTTPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80: loadedHTTPRoutes,
			},
			expectedReconcileQueue: map[string]bool{
				"Gateway-http1-http1-ns-HTTPRoute-gw-gw-ns-80-sect1": true,
				"Gateway-http2-http2-ns-HTTPRoute-gw-gw-ns-80-sect2": true,
				"Gateway-http3-http3-ns-HTTPRoute-gw-gw-ns-80-sect3": true,
			},
		},
		{
			name:                    "filter only allows http route - explicit parent ref kind - gateway",
			acceptedKinds:           sets.New[RouteKind](HTTPRouteKind),
			expectedPreMappedRoutes: preLoadHTTPRoutes,
			parentRefs: map[string][]gwv1.ParentReference{
				preLoadHTTPRoutes[0].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
					Kind:      new(gwv1.Kind(gatewayKind)),
				}},
				preLoadHTTPRoutes[1].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
					Kind:      new(gwv1.Kind(gatewayKind)),
				}},
				preLoadHTTPRoutes[2].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
					Kind:      new(gwv1.Kind(gatewayKind)),
				}},
			},
			expectedPreloadMap: map[int32][]preLoadRouteDescriptor{
				80: preLoadHTTPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80: loadedHTTPRoutes,
			},
			expectedReconcileQueue: map[string]bool{
				"Gateway-http1-http1-ns-HTTPRoute-gw-gw-ns--": true,
				"Gateway-http2-http2-ns-HTTPRoute-gw-gw-ns--": true,
				"Gateway-http3-http3-ns-HTTPRoute-gw-gw-ns--": true,
			},
		},
		{
			name:                    "filter only allows http route - explicit parent ref kind - listenerset",
			acceptedKinds:           sets.New[RouteKind](HTTPRouteKind),
			expectedPreMappedRoutes: preLoadHTTPRoutes,
			parentRefs: map[string][]gwv1.ParentReference{
				preLoadHTTPRoutes[0].GetRouteIdentifier(): {{
					Name:      "ls",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
					Kind:      new(gwv1.Kind(listenerSetKind)),
				}},
				preLoadHTTPRoutes[1].GetRouteIdentifier(): {{
					Name:      "ls",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
					Kind:      new(gwv1.Kind(listenerSetKind)),
				}},
				preLoadHTTPRoutes[2].GetRouteIdentifier(): {{
					Name:      "ls",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
					Kind:      new(gwv1.Kind(listenerSetKind)),
				}},
			},
			expectedPreloadMap: map[int32][]preLoadRouteDescriptor{
				80: preLoadHTTPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80: loadedHTTPRoutes,
			},
			expectedReconcileQueue: map[string]bool{
				"ListenerSet-http1-http1-ns-HTTPRoute-ls-gw-ns--": true,
				"ListenerSet-http2-http2-ns-HTTPRoute-ls-gw-ns--": true,
				"ListenerSet-http3-http3-ns-HTTPRoute-ls-gw-ns--": true,
			},
		},
		{
			name:                    "filter only allows http route - mixed listenerset and gateway kinds",
			acceptedKinds:           sets.New[RouteKind](HTTPRouteKind),
			expectedPreMappedRoutes: preLoadHTTPRoutes,
			parentRefs: map[string][]gwv1.ParentReference{
				preLoadHTTPRoutes[0].GetRouteIdentifier(): {
					{
						Name:      "ls",
						Namespace: (*gwv1.Namespace)(new("gw-ns")),
						Kind:      new(gwv1.Kind(listenerSetKind)),
					},
					{
						Name:      "gw",
						Namespace: (*gwv1.Namespace)(new("gw-ns")),
					},
				},
				preLoadHTTPRoutes[1].GetRouteIdentifier(): {
					{
						Name:      "ls",
						Namespace: (*gwv1.Namespace)(new("gw-ns")),
						Kind:      new(gwv1.Kind(listenerSetKind)),
					},
					{
						Name:      "gw",
						Namespace: (*gwv1.Namespace)(new("gw-ns")),
					},
				},
				preLoadHTTPRoutes[2].GetRouteIdentifier(): {
					{
						Name:      "ls",
						Namespace: (*gwv1.Namespace)(new("gw-ns")),
						Kind:      new(gwv1.Kind(listenerSetKind)),
					},
					{
						Name:      "gw",
						Namespace: (*gwv1.Namespace)(new("gw-ns")),
					},
				},
			},
			expectedPreloadMap: map[int32][]preLoadRouteDescriptor{
				80: preLoadHTTPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80: loadedHTTPRoutes,
			},
			expectedReconcileQueue: map[string]bool{
				"ListenerSet-http1-http1-ns-HTTPRoute-ls-gw-ns--": true,
				"ListenerSet-http2-http2-ns-HTTPRoute-ls-gw-ns--": true,
				"ListenerSet-http3-http3-ns-HTTPRoute-ls-gw-ns--": true,
				"Gateway-http1-http1-ns-HTTPRoute-gw-gw-ns--":     true,
				"Gateway-http2-http2-ns-HTTPRoute-gw-gw-ns--":     true,
				"Gateway-http3-http3-ns-HTTPRoute-gw-gw-ns--":     true,
			},
		},
		{
			name:                    "filter only allows http route - mixed listenerset and gateway kinds - namespaced name collision",
			acceptedKinds:           sets.New[RouteKind](HTTPRouteKind),
			expectedPreMappedRoutes: preLoadHTTPRoutes,
			parentRefs: map[string][]gwv1.ParentReference{
				preLoadHTTPRoutes[0].GetRouteIdentifier(): {
					{
						Name:      "gw",
						Namespace: (*gwv1.Namespace)(new("gw-ns")),
						Kind:      new(gwv1.Kind(listenerSetKind)),
					},
					{
						Name:      "gw",
						Namespace: (*gwv1.Namespace)(new("gw-ns")),
					},
				},
				preLoadHTTPRoutes[1].GetRouteIdentifier(): {
					{
						Name:      "gw",
						Namespace: (*gwv1.Namespace)(new("gw-ns")),
						Kind:      new(gwv1.Kind(listenerSetKind)),
					},
					{
						Name:      "gw",
						Namespace: (*gwv1.Namespace)(new("gw-ns")),
					},
				},
				preLoadHTTPRoutes[2].GetRouteIdentifier(): {
					{
						Name:      "gw",
						Namespace: (*gwv1.Namespace)(new("gw-ns")),
						Kind:      new(gwv1.Kind(listenerSetKind)),
					},
					{
						Name:      "gw",
						Namespace: (*gwv1.Namespace)(new("gw-ns")),
					},
				},
			},
			expectedPreloadMap: map[int32][]preLoadRouteDescriptor{
				80: preLoadHTTPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80: loadedHTTPRoutes,
			},
			expectedReconcileQueue: map[string]bool{
				"ListenerSet-http1-http1-ns-HTTPRoute-gw-gw-ns--": true,
				"ListenerSet-http2-http2-ns-HTTPRoute-gw-gw-ns--": true,
				"ListenerSet-http3-http3-ns-HTTPRoute-gw-gw-ns--": true,
				"Gateway-http1-http1-ns-HTTPRoute-gw-gw-ns--":     true,
				"Gateway-http2-http2-ns-HTTPRoute-gw-gw-ns--":     true,
				"Gateway-http3-http3-ns-HTTPRoute-gw-gw-ns--":     true,
			},
		},
		{
			name:                    "filter only allows http route, multiple ports",
			acceptedKinds:           sets.New[RouteKind](HTTPRouteKind),
			expectedPreMappedRoutes: preLoadHTTPRoutes,
			parentRefs: map[string][]gwv1.ParentReference{
				preLoadHTTPRoutes[0].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadHTTPRoutes[1].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadHTTPRoutes[2].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
			},
			expectedPreloadMap: map[int32][]preLoadRouteDescriptor{
				80:  preLoadHTTPRoutes,
				443: preLoadHTTPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80:  loadedHTTPRoutes,
				443: loadedHTTPRoutes,
			},
			expectedReconcileQueue: map[string]bool{
				"Gateway-http1-http1-ns-HTTPRoute-gw-gw-ns--": true,
				"Gateway-http2-http2-ns-HTTPRoute-gw-gw-ns--": true,
				"Gateway-http3-http3-ns-HTTPRoute-gw-gw-ns--": true,
			},
		},
		{
			name:                    "filter only allows tcp route",
			acceptedKinds:           sets.New[RouteKind](TCPRouteKind),
			expectedPreMappedRoutes: preLoadTCPRoutes,
			expectedPreloadMap: map[int32][]preLoadRouteDescriptor{
				80: preLoadTCPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80: loadedTCPRoutes,
			},
			parentRefs: map[string][]gwv1.ParentReference{
				preLoadTCPRoutes[0].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadTCPRoutes[1].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadTCPRoutes[2].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
			},
			expectedReconcileQueue: map[string]bool{
				"Gateway-tcp1-tcp1-ns-TCPRoute-gw-gw-ns--": true,
				"Gateway-tcp2-tcp2-ns-TCPRoute-gw-gw-ns--": true,
				"Gateway-tcp3-tcp3-ns-TCPRoute-gw-gw-ns--": true,
			},
		},
		{
			name:                    "filter allows both route kinds",
			acceptedKinds:           sets.New[RouteKind](TCPRouteKind, HTTPRouteKind),
			expectedPreMappedRoutes: append(preLoadHTTPRoutes, preLoadTCPRoutes...),
			parentRefs: map[string][]gwv1.ParentReference{
				preLoadHTTPRoutes[0].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadHTTPRoutes[1].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadHTTPRoutes[2].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadTCPRoutes[0].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadTCPRoutes[1].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadTCPRoutes[2].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
			},
			expectedPreloadMap: map[int32][]preLoadRouteDescriptor{
				80:  preLoadTCPRoutes,
				443: preLoadHTTPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80:  loadedTCPRoutes,
				443: loadedHTTPRoutes,
			},
			expectedReconcileQueue: map[string]bool{
				"Gateway-http1-http1-ns-HTTPRoute-gw-gw-ns--": true,
				"Gateway-http2-http2-ns-HTTPRoute-gw-gw-ns--": true,
				"Gateway-http3-http3-ns-HTTPRoute-gw-gw-ns--": true,
				"Gateway-tcp1-tcp1-ns-TCPRoute-gw-gw-ns--":    true,
				"Gateway-tcp2-tcp2-ns-TCPRoute-gw-gw-ns--":    true,
				"Gateway-tcp3-tcp3-ns-TCPRoute-gw-gw-ns--":    true,
			},
		},
		{
			name:                    "failed route should lead to only failed version status getting published",
			acceptedKinds:           sets.New[RouteKind](TCPRouteKind, HTTPRouteKind),
			expectedPreMappedRoutes: append(preLoadHTTPRoutes, preLoadTCPRoutes...),
			parentRefs: map[string][]gwv1.ParentReference{
				preLoadHTTPRoutes[0].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadHTTPRoutes[1].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadHTTPRoutes[2].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadTCPRoutes[0].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadTCPRoutes[1].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadTCPRoutes[2].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
			},
			expectedPreloadMap: map[int32][]preLoadRouteDescriptor{
				80:  preLoadTCPRoutes,
				443: preLoadHTTPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80:  loadedTCPRoutes,
				443: loadedHTTPRoutes,
			},
			expectedReconcileQueue: map[string]bool{
				"Gateway-http1-http1-ns-HTTPRoute-gw-gw-ns--": true,
				"Gateway-http2-http2-ns-HTTPRoute-gw-gw-ns--": true,
				"Gateway-http3-http3-ns-HTTPRoute-gw-gw-ns--": true,
				"Gateway-tcp1-tcp1-ns-TCPRoute-gw-gw-ns--":    true,
				"Gateway-tcp2-tcp2-ns-TCPRoute-gw-gw-ns--":    false,
				"Gateway-tcp3-tcp3-ns-TCPRoute-gw-gw-ns--":    true,
			},
			mapperRouteStatusUpdates: []RouteData{
				{
					RouteStatusInfo: RouteStatusInfo{
						Accepted: false,
					},
					RouteMetadata: RouteMetadata{
						RouteName:       "tcp2",
						RouteNamespace:  "tcp2-ns",
						RouteKind:       string(TCPRouteKind),
						RouteGeneration: 0,
					},
					ParentRef: gwv1.ParentReference{
						Name:      "gw",
						Namespace: &testNamespace,
					},
				},
			},
		},
		{
			name:                    "multiple failed routes",
			acceptedKinds:           sets.New[RouteKind](HTTPRouteKind),
			expectedPreMappedRoutes: preLoadHTTPRoutes,
			expectedPreloadMap: map[int32][]preLoadRouteDescriptor{
				80: preLoadHTTPRoutes,
			},
			parentRefs: map[string][]gwv1.ParentReference{
				preLoadHTTPRoutes[0].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadHTTPRoutes[1].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
				preLoadHTTPRoutes[2].GetRouteIdentifier(): {{
					Name:      "gw",
					Namespace: (*gwv1.Namespace)(new("gw-ns")),
				}},
			},
			expectedMap: map[int32][]RouteDescriptor{
				80: loadedHTTPRoutes,
			},
			expectedReconcileQueue: map[string]bool{
				"Gateway-http1-http1-ns-HTTPRoute-gw-gw-ns--": false,
				"Gateway-http2-http2-ns-HTTPRoute-gw-gw-ns--": true,
				"Gateway-http3-http3-ns-HTTPRoute-gw-gw-ns--": false,
			},
			mapperRouteStatusUpdates: []RouteData{
				{
					RouteStatusInfo: RouteStatusInfo{
						Accepted: false,
					},
					RouteMetadata: RouteMetadata{
						RouteName:       "http1",
						RouteNamespace:  "http1-ns",
						RouteKind:       string(HTTPRouteKind),
						RouteGeneration: 0,
					},
					ParentRef: gwv1.ParentReference{
						Name:      "gw",
						Namespace: &testNamespace,
					},
				},
				{
					RouteStatusInfo: RouteStatusInfo{
						Accepted: false,
					},
					RouteMetadata: RouteMetadata{
						RouteName:       "http3",
						RouteNamespace:  "http3-ns",
						RouteKind:       string(HTTPRouteKind),
						RouteGeneration: 0,
					},
					ParentRef: gwv1.ParentReference{
						Name:      "gw",
						Namespace: &testNamespace,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			routeReconciler := NewMockRouteReconciler()
			loader := loaderImpl{
				mapper: &mockMapper{
					t:                  t,
					expectedRoutes:     tc.expectedPreMappedRoutes,
					mapToReturn:        tc.expectedPreloadMap,
					routeStatusUpdates: tc.mapperRouteStatusUpdates,
					matchedParentRefs:  tc.parentRefs,
				},
				allRouteLoaders: allRouteLoaders,
				logger:          logr.Discard(),
				routeSubmitter:  routeReconciler,
				lsLoader: &mockListenerSetLoader{
					result: listenerSetLoadResult{},
				},
			}

			filter := &routeFilterImpl{acceptedKinds: tc.acceptedKinds}
			result, err := loader.LoadRoutesForGateway(context.Background(), gwv1.Gateway{ObjectMeta: v1.ObjectMeta{
				Name:      "gw",
				Namespace: "gw-ns",
			}}, filter, gateway_constants.ALBGatewayController, nil)
			if tc.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedMap, result.Routes)
			assert.Equal(t, len(tc.expectedReconcileQueue), len(routeReconciler.Enqueued))

			for _, actual := range routeReconciler.Enqueued {
				ak := generateRouteDataCacheKey(actual.RouteData)

				v, ok := tc.expectedReconcileQueue[ak]
				assert.True(t, ok, ak)
				assert.Equal(t, v, actual.RouteData.RouteStatusInfo.Accepted, ak)
			}

		})
	}
}
