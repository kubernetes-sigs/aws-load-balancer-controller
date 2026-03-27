package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
)

// mockDiscoveryClient implements DiscoveryClient for testing.
type mockDiscoveryClient struct {
	resources map[string]*metav1.APIResourceList
	err       error
}

func (m *mockDiscoveryClient) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	if m.err != nil {
		return nil, m.err
	}
	res, ok := m.resources[groupVersion]
	if !ok {
		notFoundErr := apierrors.NewNotFound(schema.GroupResource{
			Group:    groupVersion,
			Resource: groupVersion,
		}, groupVersion)
		return nil, notFoundErr
	}
	return res, nil
}

func TestDetectCRDs_v1(t *testing.T) {
	client := &mockDiscoveryClient{
		resources: map[string]*metav1.APIResourceList{
			"gateway.networking.k8s.io/v1": {
				APIResources: []metav1.APIResource{
					{Kind: "Gateway"},
					{Kind: "GatewayClass"},
					{Kind: "HTTPRoute"},
					{Kind: "GRPCRoute"},
				},
			},
		},
	}

	result, err := DetectCRDs(client, sets.New("gateway.networking.k8s.io/v1"))

	assert.NoError(t, err)

	assert.Equal(t, len(result), 1)
	assert.Equal(t, len(result["gateway.networking.k8s.io/v1"]), 4)
	assert.True(t, result["gateway.networking.k8s.io/v1"].Has("Gateway"))
	assert.True(t, result["gateway.networking.k8s.io/v1"].Has("GatewayClass"))
	assert.True(t, result["gateway.networking.k8s.io/v1"].Has("HTTPRoute"))
	assert.True(t, result["gateway.networking.k8s.io/v1"].Has("GRPCRoute"))
}

func TestDetectCRDs_v1_ignoreNotInInterestSet(t *testing.T) {
	client := &mockDiscoveryClient{
		resources: map[string]*metav1.APIResourceList{
			"gateway.networking.k8s.io/v1": {
				APIResources: []metav1.APIResource{
					{Kind: "Gateway"},
					{Kind: "GatewayClass"},
					{Kind: "HTTPRoute"},
					{Kind: "GRPCRoute"},
				},
			},
			"gateway.networking.k8s.io/v1alpha2": {
				APIResources: []metav1.APIResource{
					{Kind: "TCPRoute"},
					{Kind: "UDPRoute"},
				},
			},
		},
	}

	result, err := DetectCRDs(client, sets.New("gateway.networking.k8s.io/v1"))

	assert.NoError(t, err)

	assert.Equal(t, len(result), 1)
	assert.Equal(t, len(result["gateway.networking.k8s.io/v1"]), 4)
	assert.True(t, result["gateway.networking.k8s.io/v1"].Has("Gateway"))
	assert.True(t, result["gateway.networking.k8s.io/v1"].Has("GatewayClass"))
	assert.True(t, result["gateway.networking.k8s.io/v1"].Has("HTTPRoute"))
	assert.True(t, result["gateway.networking.k8s.io/v1"].Has("GRPCRoute"))
}

func TestDetectCRDs_v1_alpha2(t *testing.T) {
	client := &mockDiscoveryClient{
		resources: map[string]*metav1.APIResourceList{
			"gateway.networking.k8s.io/v1": {
				APIResources: []metav1.APIResource{
					{Kind: "Gateway"},
					{Kind: "GatewayClass"},
					{Kind: "HTTPRoute"},
					{Kind: "GRPCRoute"},
				},
			},
			"gateway.networking.k8s.io/v1alpha2": {
				APIResources: []metav1.APIResource{
					{Kind: "TCPRoute"},
					{Kind: "UDPRoute"},
				},
			},
		},
	}

	result, err := DetectCRDs(client, sets.New("gateway.networking.k8s.io/v1", "gateway.networking.k8s.io/v1alpha2"))

	assert.NoError(t, err)

	assert.Equal(t, len(result), 2)
	assert.Equal(t, len(result["gateway.networking.k8s.io/v1"]), 4)
	assert.True(t, result["gateway.networking.k8s.io/v1"].Has("Gateway"))
	assert.True(t, result["gateway.networking.k8s.io/v1"].Has("GatewayClass"))
	assert.True(t, result["gateway.networking.k8s.io/v1"].Has("HTTPRoute"))
	assert.True(t, result["gateway.networking.k8s.io/v1"].Has("GRPCRoute"))

	assert.Equal(t, len(result["gateway.networking.k8s.io/v1alpha2"]), 2)
	assert.True(t, result["gateway.networking.k8s.io/v1alpha2"].Has("TCPRoute"))
	assert.True(t, result["gateway.networking.k8s.io/v1alpha2"].Has("UDPRoute"))
}
