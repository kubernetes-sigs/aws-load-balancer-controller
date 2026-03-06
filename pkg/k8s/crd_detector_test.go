package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// mockDiscoveryClient implements DiscoveryClient for testing.
type mockDiscoveryClient struct {
	resources *metav1.APIGroupList
	err       error
}

func (m *mockDiscoveryClient) ServerGroups() (*metav1.APIGroupList, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.resources, nil
}

func TestDetectCRDs_AllPresent(t *testing.T) {
	client := &mockDiscoveryClient{
		resources: &metav1.APIGroupList{
			TypeMeta: metav1.TypeMeta{},
			Groups: []metav1.APIGroup{
				{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "gateway.networking.k8s.io/v1",
						Kind:       "Gateway",
					},
				},
			},
		},
	}

	_, err := DetectCRDs(client, sets.New("gateway.networking.k8s.io/v1"))

	assert.NoError(t, err)
}

/*

func TestDetectCRDs_APIError(t *testing.T) {
	client := &mockDiscoveryClient{
		err: fmt.Errorf("connection refused"),
	}

	_, err := DetectCRDs(client, "gateway.networking.k8s.io/v1")

	assert.Error(t, err)
}

func TestDetectCRDs_EmptyResourceList(t *testing.T) {
	client := &mockDiscoveryClient{
		resources: map[string]*metav1.APIResourceList{
			"gateway.networking.k8s.io/v1": {
				APIResources: []metav1.APIResource{},
			},
		},
	}

	result, err := DetectCRDs(client, "gateway.networking.k8s.io/v1")

	assert.NoError(t, err)
	assert.Len(t, result.PresentKinds, 0)
}

func TestDetectCRDs_GroupVersionNotFound(t *testing.T) {
	client := &mockDiscoveryClient{
		resources: map[string]*metav1.APIResourceList{},
	}

	result, err := DetectCRDs(client, "gateway.networking.k8s.io/v1alpha2")

	assert.NoError(t, err)
	assert.Len(t, result.PresentKinds, 0)
}

*/
