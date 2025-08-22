package routeutils

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

type mockListenerAttachmentHelper struct {
	attachmentMap map[string]bool
}

func makeListenerAttachmentMapKey(listener gwv1.Listener, route preLoadRouteDescriptor) string {
	nsn := route.GetRouteNamespacedName()
	return fmt.Sprintf("%s-%d-%s-%s", listener.Name, listener.Port, nsn.Name, nsn.Namespace)
}

func (m *mockListenerAttachmentHelper) listenerAllowsAttachment(ctx context.Context, gw gwv1.Gateway, listener gwv1.Listener, route preLoadRouteDescriptor) (bool, *RouteData, error) {
	k := makeListenerAttachmentMapKey(listener, route)
	return m.attachmentMap[k], nil, nil
}

type mockRouteAttachmentHelper struct {
	routeGatewayMap  map[string]bool
	routeListenerMap map[string]bool
}

func makeRouteGatewayMapKey(gw gwv1.Gateway, route preLoadRouteDescriptor) string {
	nsn := route.GetRouteNamespacedName()
	return fmt.Sprintf("%s-%s-%s-%s", gw.Name, gw.Namespace, nsn.Name, nsn.Namespace)
}

func (m *mockRouteAttachmentHelper) doesRouteAttachToGateway(gw gwv1.Gateway, route preLoadRouteDescriptor) bool {
	k := makeRouteGatewayMapKey(gw, route)
	return m.routeGatewayMap[k]
}

func (m *mockRouteAttachmentHelper) routeAllowsAttachmentToListener(listener gwv1.Listener, route preLoadRouteDescriptor) bool {
	k := makeListenerAttachmentMapKey(listener, route)
	return m.routeListenerMap[k]
}

func Test_mapGatewayAndRoutes(t *testing.T) {

	route1 := convertHTTPRoute(gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route1",
			Namespace: "ns1",
		},
	})

	route2 := convertHTTPRoute(gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route2",
			Namespace: "ns2",
		},
	})

	route3 := convertHTTPRoute(gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route3",
			Namespace: "ns3",
		},
	})

	route4 := convertHTTPRoute(gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route4",
			Namespace: "ns4",
		},
	})

	gateway := gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw1",
			Namespace: "ns-gw",
		},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{
					Name: "section80",
					Port: gwv1.PortNumber(80),
				},
				{
					Name: "section81",
					Port: gwv1.PortNumber(81),
				},
				{
					Name: "section82",
					Port: gwv1.PortNumber(82),
				},
			},
		},
	}

	testCases := []struct {
		name                  string
		gw                    gwv1.Gateway
		routes                []preLoadRouteDescriptor
		listenerAttachmentMap map[string]bool
		routeGatewayMap       map[string]bool
		routeListenerMap      map[string]bool
		expected              map[int][]preLoadRouteDescriptor
		expectErr             bool
	}{
		{
			name:   "routes get mapped to each listener",
			gw:     gateway,
			routes: []preLoadRouteDescriptor{route1, route2, route3, route4},
			listenerAttachmentMap: map[string]bool{
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route3): true,
			},
			routeListenerMap: map[string]bool{
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route3): true,
			},
			routeGatewayMap: map[string]bool{
				makeRouteGatewayMapKey(gateway, route1): true,
				makeRouteGatewayMapKey(gateway, route2): true,
				makeRouteGatewayMapKey(gateway, route3): true,
				makeRouteGatewayMapKey(gateway, route4): true,
			},
			expected: map[int][]preLoadRouteDescriptor{
				80: {
					route1,
				},
				81: {
					route2,
				},
				82: {
					route3,
				},
			},
		},
		{
			name:   "all routes to all listeners",
			gw:     gateway,
			routes: []preLoadRouteDescriptor{route1, route2, route3, route4},
			listenerAttachmentMap: map[string]bool{
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route3): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route3): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route3): true,
			},
			routeListenerMap: map[string]bool{
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route3): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route3): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route3): true,
			},
			routeGatewayMap: map[string]bool{
				makeRouteGatewayMapKey(gateway, route1): true,
				makeRouteGatewayMapKey(gateway, route2): true,
				makeRouteGatewayMapKey(gateway, route3): true,
				makeRouteGatewayMapKey(gateway, route4): true,
			},
			expected: map[int][]preLoadRouteDescriptor{
				80: {
					route1,
					route2,
					route3,
				},
				81: {
					route1,
					route2,
					route3,
				},
				82: {
					route1,
					route2,
					route3,
				},
			},
		},
		{
			name:   "gateway doesnt allow attachment, no result",
			gw:     gateway,
			routes: []preLoadRouteDescriptor{route1, route2, route3, route4},
			listenerAttachmentMap: map[string]bool{
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route3): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route3): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route3): true,
			},
			routeListenerMap: map[string]bool{
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route3): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route3): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route3): true,
			},
			routeGatewayMap: map[string]bool{},
			expected:        map[int][]preLoadRouteDescriptor{},
		},
		{
			name:   "route allows all attachment, but listener only allows subset",
			gw:     gateway,
			routes: []preLoadRouteDescriptor{route1, route2, route3, route4},
			listenerAttachmentMap: map[string]bool{
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route3): true,
			},
			routeListenerMap: map[string]bool{
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route3): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route3): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route3): true,
			},
			routeGatewayMap: map[string]bool{
				makeRouteGatewayMapKey(gateway, route1): true,
				makeRouteGatewayMapKey(gateway, route2): true,
				makeRouteGatewayMapKey(gateway, route3): true,
				makeRouteGatewayMapKey(gateway, route4): true,
			},
			expected: map[int][]preLoadRouteDescriptor{
				80: {
					route1,
				},
				81: {
					route2,
				},
				82: {
					route3,
				},
			},
		},
		{
			name:   "listener allows all attachment, but route only allows subset",
			gw:     gateway,
			routes: []preLoadRouteDescriptor{route1, route2, route3, route4},
			listenerAttachmentMap: map[string]bool{
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route3): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route3): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route3): true,
			},
			routeListenerMap: map[string]bool{
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[0], route1): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[1], route2): true,
				makeListenerAttachmentMapKey(gateway.Spec.Listeners[2], route3): true,
			},
			routeGatewayMap: map[string]bool{
				makeRouteGatewayMapKey(gateway, route1): true,
				makeRouteGatewayMapKey(gateway, route2): true,
				makeRouteGatewayMapKey(gateway, route3): true,
				makeRouteGatewayMapKey(gateway, route4): true,
			},
			expected: map[int][]preLoadRouteDescriptor{
				80: {
					route1,
				},
				81: {
					route2,
				},
				82: {
					route3,
				},
			},
		},
		{
			name:     "no output",
			expected: make(map[int][]preLoadRouteDescriptor),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mapper := listenerToRouteMapperImpl{
				listenerAttachmentHelper: &mockListenerAttachmentHelper{
					attachmentMap: tc.listenerAttachmentMap,
				},
				routeAttachmentHelper: &mockRouteAttachmentHelper{
					routeListenerMap: tc.routeListenerMap,
					routeGatewayMap:  tc.routeGatewayMap,
				},
				logger: logr.Discard(),
			}
			result, statusUpdates, err := mapper.mapGatewayAndRoutes(context.Background(), tc.gw, tc.routes)

			if tc.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, len(tc.expected), len(result))

			assert.Equal(t, 0, len(statusUpdates))

			for k, v := range tc.expected {
				assert.ElementsMatch(t, v, result[k])
			}
		})
	}
}
