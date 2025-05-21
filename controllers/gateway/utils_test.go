package gateway

import (
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"testing"
)

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
