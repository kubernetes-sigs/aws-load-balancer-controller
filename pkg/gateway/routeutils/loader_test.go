package routeutils

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
	"time"
)

type mockMapper struct {
	t              *testing.T
	expectedRoutes []preLoadRouteDescriptor
	mapToReturn    map[int][]preLoadRouteDescriptor
}

func (m *mockMapper) mapGatewayAndRoutes(context context.Context, gw gwv1.Gateway, routes []preLoadRouteDescriptor, routeReconciler RouteReconciler) (map[int][]preLoadRouteDescriptor, error) {
	assert.ElementsMatch(m.t, m.expectedRoutes, routes)
	return m.mapToReturn, nil
}

var _ RouteDescriptor = &mockRoute{}

type mockRoute struct {
	namespacedName types.NamespacedName
	routeKind      RouteKind
	generation     int64
	hostnames      []gwv1.Hostname
}

func (m *mockRoute) loadAttachedRules(context context.Context, k8sClient client.Client) (RouteDescriptor, []routeLoadError) {
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
func (m *mockRoute) GetListenerRuleConfigs() []gwv1.LocalObjectReference {
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

func TestLoadRoutesForGateway(t *testing.T) {
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
		r, _ := preload.loadAttachedRules(nil, nil)
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
		r, _ := preload.loadAttachedRules(nil, nil)
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
		name                    string
		acceptedKinds           sets.Set[RouteKind]
		expectedMap             map[int32][]RouteDescriptor
		expectedPreloadMap      map[int][]preLoadRouteDescriptor
		expectedPreMappedRoutes []preLoadRouteDescriptor
		expectError             bool
	}{
		{
			name:                    "filter allows no routes",
			acceptedKinds:           make(sets.Set[RouteKind]),
			expectedPreMappedRoutes: make([]preLoadRouteDescriptor, 0),
			expectedMap:             make(map[int32][]RouteDescriptor),
		},
		{
			name:                    "filter only allows http route",
			acceptedKinds:           sets.New[RouteKind](HTTPRouteKind),
			expectedPreMappedRoutes: preLoadHTTPRoutes,
			expectedPreloadMap: map[int][]preLoadRouteDescriptor{
				80: preLoadHTTPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80: loadedHTTPRoutes,
			},
		},
		{
			name:                    "filter only allows http route, multiple ports",
			acceptedKinds:           sets.New[RouteKind](HTTPRouteKind),
			expectedPreMappedRoutes: preLoadHTTPRoutes,
			expectedPreloadMap: map[int][]preLoadRouteDescriptor{
				80:  preLoadHTTPRoutes,
				443: preLoadHTTPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80:  loadedHTTPRoutes,
				443: loadedHTTPRoutes,
			},
		},
		{
			name:                    "filter only allows tcp route",
			acceptedKinds:           sets.New[RouteKind](TCPRouteKind),
			expectedPreMappedRoutes: preLoadTCPRoutes,
			expectedPreloadMap: map[int][]preLoadRouteDescriptor{
				80: preLoadTCPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80: loadedTCPRoutes,
			},
		},
		{
			name:                    "filter allows both route kinds",
			acceptedKinds:           sets.New[RouteKind](TCPRouteKind, HTTPRouteKind),
			expectedPreMappedRoutes: append(preLoadHTTPRoutes, preLoadTCPRoutes...),
			expectedPreloadMap: map[int][]preLoadRouteDescriptor{
				80:  preLoadTCPRoutes,
				443: preLoadHTTPRoutes,
			},
			expectedMap: map[int32][]RouteDescriptor{
				80:  loadedTCPRoutes,
				443: loadedHTTPRoutes,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			loader := loaderImpl{
				mapper: &mockMapper{
					t:              t,
					expectedRoutes: tc.expectedPreMappedRoutes,
					mapToReturn:    tc.expectedPreloadMap,
				},
				allRouteLoaders: allRouteLoaders,
				logger:          logr.Discard(),
			}

			filter := &routeFilterImpl{acceptedKinds: tc.acceptedKinds}
			mockReconciler := NewMockRouteReconciler()
			result, err := loader.LoadRoutesForGateway(context.Background(), gwv1.Gateway{}, filter, mockReconciler)
			if tc.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedMap, result)
		})
	}
}
