package k8s

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		return nil, fmt.Errorf("group version %q not found", groupVersion)
	}
	return res, nil
}

func TestDetectCRDs_AllPresent(t *testing.T) {
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

	result := DetectCRDs(client, "gateway.networking.k8s.io/v1", []string{"Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute"})

	assert.True(t, result.AllPresent)
	assert.Empty(t, result.MissingKinds)
	assert.Equal(t, "gateway.networking.k8s.io/v1", result.GroupVersion)
}

func TestDetectCRDs_SomeMissing(t *testing.T) {
	client := &mockDiscoveryClient{
		resources: map[string]*metav1.APIResourceList{
			"gateway.networking.k8s.io/v1": {
				APIResources: []metav1.APIResource{
					{Kind: "Gateway"},
					{Kind: "GatewayClass"},
				},
			},
		},
	}

	result := DetectCRDs(client, "gateway.networking.k8s.io/v1", []string{"Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute"})

	assert.False(t, result.AllPresent)
	assert.Equal(t, []string{"HTTPRoute", "GRPCRoute"}, result.MissingKinds)
}

func TestDetectCRDs_APIError(t *testing.T) {
	client := &mockDiscoveryClient{
		err: fmt.Errorf("connection refused"),
	}

	requiredKinds := []string{"Gateway", "GatewayClass", "HTTPRoute"}
	result := DetectCRDs(client, "gateway.networking.k8s.io/v1", requiredKinds)

	assert.False(t, result.AllPresent)
	assert.Equal(t, requiredKinds, result.MissingKinds)
}

func TestDetectCRDs_EmptyResourceList(t *testing.T) {
	client := &mockDiscoveryClient{
		resources: map[string]*metav1.APIResourceList{
			"gateway.networking.k8s.io/v1": {
				APIResources: []metav1.APIResource{},
			},
		},
	}

	result := DetectCRDs(client, "gateway.networking.k8s.io/v1", []string{"Gateway", "GatewayClass"})

	assert.False(t, result.AllPresent)
	assert.Equal(t, []string{"Gateway", "GatewayClass"}, result.MissingKinds)
}

func TestDetectCRDs_ExtraKindsDoNotAffectResult(t *testing.T) {
	client := &mockDiscoveryClient{
		resources: map[string]*metav1.APIResourceList{
			"gateway.networking.k8s.io/v1": {
				APIResources: []metav1.APIResource{
					{Kind: "Gateway"},
					{Kind: "GatewayClass"},
					{Kind: "HTTPRoute"},
					{Kind: "SomeOtherResource"},
					{Kind: "AnotherResource"},
				},
			},
		},
	}

	result := DetectCRDs(client, "gateway.networking.k8s.io/v1", []string{"Gateway", "GatewayClass", "HTTPRoute"})

	assert.True(t, result.AllPresent)
	assert.Empty(t, result.MissingKinds)
}

func TestDetectCRDs_GroupVersionNotFound(t *testing.T) {
	client := &mockDiscoveryClient{
		resources: map[string]*metav1.APIResourceList{},
	}

	result := DetectCRDs(client, "gateway.networking.k8s.io/v1alpha2", []string{"TCPRoute", "UDPRoute"})

	assert.False(t, result.AllPresent)
	assert.Equal(t, []string{"TCPRoute", "UDPRoute"}, result.MissingKinds)
}

func TestDetectCRDs_EmptyRequiredKinds(t *testing.T) {
	client := &mockDiscoveryClient{
		resources: map[string]*metav1.APIResourceList{
			"gateway.networking.k8s.io/v1": {
				APIResources: []metav1.APIResource{
					{Kind: "Gateway"},
				},
			},
		},
	}

	result := DetectCRDs(client, "gateway.networking.k8s.io/v1", []string{})

	assert.True(t, result.AllPresent)
	assert.Empty(t, result.MissingKinds)
}
