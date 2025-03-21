package routeutils

import (
	"context"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

type mockMapper struct {
}

func (m mockMapper) Map(context context.Context, gw *gwv1.Gateway, routes []preLoadRouteDescriptor) (map[int][]preLoadRouteDescriptor, error) {
	return map[int][]preLoadRouteDescriptor{}, nil
}

func TestLoadRoutesForGateway(t *testing.T) {

	noOpLoader := func(ctx context.Context, k8sClient client.Client, typeSpecificBackend interface{}, backendRef gwv1.BackendRef, routeIdentifier types.NamespacedName, routeKind string) (*Backend, error) {
		return &Backend{}, nil
	}

	allRouteLoaders := map[string]func(ctx context.Context, k8sClient client.Client) ([]preLoadRouteDescriptor, error){
		HTTPRouteKind: func(ctx context.Context, k8sClient client.Client) ([]preLoadRouteDescriptor, error) {
			return []preLoadRouteDescriptor{
				&httpRouteDescription{backendLoader: noOpLoader},
				&httpRouteDescription{backendLoader: noOpLoader},
				&httpRouteDescription{backendLoader: noOpLoader},
			}, nil
		},
		TCPRouteKind: func(ctx context.Context, k8sClient client.Client) ([]preLoadRouteDescriptor, error) {
			return []preLoadRouteDescriptor{
				&tcpRouteDescription{backendLoader: noOpLoader},
				&tcpRouteDescription{backendLoader: noOpLoader},
				&tcpRouteDescription{backendLoader: noOpLoader},
			}, nil
		},
	}

	loader := loaderImpl{
		mapper:          &mockMapper{},
		allRouteLoaders: allRouteLoaders,
	}

	testCases := []struct {
		name          string
		acceptedKinds sets.Set[string]
		expectedMap   map[int][]RouteDescriptor
		expectError   bool
	}{
		{
			name:          "filter allows no routes",
			acceptedKinds: make(sets.Set[string]),
			expectedMap:   make(map[int][]RouteDescriptor),
		},
		{
			name:          "filter only allows http route",
			acceptedKinds: sets.New[string](HTTPRouteKind),
			expectedMap:   make(map[int][]RouteDescriptor),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			filter := &routeFilterImpl{acceptedKinds: tc.acceptedKinds}
			result, err := loader.LoadRoutesForGateway(context.Background(), &gwv1.Gateway{}, filter)
			if tc.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedMap, result)
		})
	}
}
