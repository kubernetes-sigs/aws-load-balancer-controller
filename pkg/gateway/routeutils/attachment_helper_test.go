package routeutils

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func Test_doesResourceAttachToGateway(t *testing.T) {
	testCases := []struct {
		name              string
		parentRef         gwv1.ParentReference
		resourceNamespace string
		gw                gwv1.Gateway
		expected          bool
	}{
		{
			name: "nil kind defaults to Gateway, matching name and namespace",
			parentRef: gwv1.ParentReference{
				Name: "my-gw",
			},
			resourceNamespace: "ns1",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gw",
					Namespace: "ns1",
				},
			},
			expected: true,
		},
		{
			name: "explicit Gateway kind, matching name and explicit namespace",
			parentRef: gwv1.ParentReference{
				Name:      "my-gw",
				Kind:      (*gwv1.Kind)(awssdk.String("Gateway")),
				Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
			},
			resourceNamespace: "other-ns",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gw",
					Namespace: "ns1",
				},
			},
			expected: true,
		},
		{
			name: "non-Gateway kind returns false",
			parentRef: gwv1.ParentReference{
				Name: "my-gw",
				Kind: (*gwv1.Kind)(awssdk.String("Service")),
			},
			resourceNamespace: "ns1",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gw",
					Namespace: "ns1",
				},
			},
			expected: false,
		},
		{
			name: "name mismatch returns false",
			parentRef: gwv1.ParentReference{
				Name: "other-gw",
			},
			resourceNamespace: "ns1",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gw",
					Namespace: "ns1",
				},
			},
			expected: false,
		},
		{
			name: "namespace mismatch via explicit namespace returns false",
			parentRef: gwv1.ParentReference{
				Name:      "my-gw",
				Namespace: (*gwv1.Namespace)(awssdk.String("ns2")),
			},
			resourceNamespace: "ns1",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gw",
					Namespace: "ns1",
				},
			},
			expected: false,
		},
		{
			name: "namespace mismatch via resource namespace returns false",
			parentRef: gwv1.ParentReference{
				Name: "my-gw",
			},
			resourceNamespace: "ns2",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gw",
					Namespace: "ns1",
				},
			},
			expected: false,
		},
		{
			name: "nil kind with explicit namespace matching",
			parentRef: gwv1.ParentReference{
				Name:      "my-gw",
				Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
			},
			resourceNamespace: "different-ns",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gw",
					Namespace: "ns1",
				},
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := doesResourceAttachToGateway(tc.parentRef, tc.resourceNamespace, tc.gw)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_doesResourceAllowNamespace(t *testing.T) {
	testCases := []struct {
		name              string
		fromNamespaces    gwv1.FromNamespaces
		labelSelector     *metav1.LabelSelector
		nsSelector        namespaceSelector
		resourceNamespace string
		parentNamespace   string
		expected          bool
		expectErr         bool
	}{
		{
			name:              "NamespacesFromNone returns false",
			fromNamespaces:    gwv1.NamespacesFromNone,
			resourceNamespace: "ns1",
			parentNamespace:   "ns1",
			expected:          false,
		},
		{
			name:              "NamespacesFromSame with same namespace returns true",
			fromNamespaces:    gwv1.NamespacesFromSame,
			resourceNamespace: "ns1",
			parentNamespace:   "ns1",
			expected:          true,
		},
		{
			name:              "NamespacesFromSame with different namespace returns false",
			fromNamespaces:    gwv1.NamespacesFromSame,
			resourceNamespace: "ns2",
			parentNamespace:   "ns1",
			expected:          false,
		},
		{
			name:              "NamespacesFromAll returns true",
			fromNamespaces:    gwv1.NamespacesFromAll,
			resourceNamespace: "any-ns",
			parentNamespace:   "ns1",
			expected:          true,
		},
		{
			name:              "NamespacesFromSelector with nil selector returns false",
			fromNamespaces:    gwv1.NamespacesFromSelector,
			labelSelector:     nil,
			resourceNamespace: "ns1",
			parentNamespace:   "ns1",
			expected:          false,
		},
		{
			name:           "NamespacesFromSelector with matching namespace returns true",
			fromNamespaces: gwv1.NamespacesFromSelector,
			labelSelector:  &metav1.LabelSelector{},
			nsSelector: &mockNamespaceSelector{
				nss: sets.New("ns1", "ns3"),
			},
			resourceNamespace: "ns1",
			parentNamespace:   "gw-ns",
			expected:          true,
		},
		{
			name:           "NamespacesFromSelector with non-matching namespace returns false",
			fromNamespaces: gwv1.NamespacesFromSelector,
			labelSelector:  &metav1.LabelSelector{},
			nsSelector: &mockNamespaceSelector{
				nss: sets.New("ns3", "ns5"),
			},
			resourceNamespace: "ns1",
			parentNamespace:   "gw-ns",
			expected:          false,
		},
		{
			name:           "NamespacesFromSelector with error returns error",
			fromNamespaces: gwv1.NamespacesFromSelector,
			labelSelector:  &metav1.LabelSelector{},
			nsSelector: &mockNamespaceSelector{
				err: errors.New("k8s error"),
			},
			resourceNamespace: "ns1",
			parentNamespace:   "gw-ns",
			expectErr:         true,
		},
		{
			name:              "unknown FromNamespaces value returns false",
			fromNamespaces:    gwv1.FromNamespaces("Unknown"),
			resourceNamespace: "ns1",
			parentNamespace:   "ns1",
			expected:          false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := doesResourceAllowNamespace(context.Background(), tc.fromNamespaces, tc.labelSelector, tc.nsSelector, tc.resourceNamespace, tc.parentNamespace)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
