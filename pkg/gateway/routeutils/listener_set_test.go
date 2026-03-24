package routeutils

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func Test_convertListenerSetParentRef(t *testing.T) {
	loader := &listenerSetLoaderImpl{logger: logr.Discard()}

	testCases := []struct {
		name     string
		input    gwv1.ParentGatewayReference
		expected gwv1.ParentReference
	}{
		{
			name: "all fields set",
			input: gwv1.ParentGatewayReference{
				Group:     (*gwv1.Group)(awssdk.String("gateway.networking.k8s.io")),
				Kind:      (*gwv1.Kind)(awssdk.String("Gateway")),
				Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
				Name:      "my-gw",
			},
			expected: gwv1.ParentReference{
				Group:     (*gwv1.Group)(awssdk.String("gateway.networking.k8s.io")),
				Kind:      (*gwv1.Kind)(awssdk.String("Gateway")),
				Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
				Name:      "my-gw",
			},
		},
		{
			name: "only name set",
			input: gwv1.ParentGatewayReference{
				Name: "my-gw",
			},
			expected: gwv1.ParentReference{
				Name: "my-gw",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := loader.convertListenerSetParentRef(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_convertListenerSetListenerToGatewayListener(t *testing.T) {
	loader := &listenerSetLoaderImpl{logger: logr.Discard()}
	hostname := gwv1.Hostname("example.com")
	tlsMode := gwv1.TLSModeTerminate

	entry := gwv1.ListenerEntry{
		Name:     "my-listener",
		Hostname: &hostname,
		Port:     8080,
		Protocol: gwv1.HTTPSProtocolType,
		TLS: &gwv1.ListenerTLSConfig{
			Mode: &tlsMode,
		},
		AllowedRoutes: &gwv1.AllowedRoutes{
			Namespaces: &gwv1.RouteNamespaces{
				From: (*gwv1.FromNamespaces)(awssdk.String(string(gwv1.NamespacesFromAll))),
			},
		},
	}

	result := loader.convertListenerSetListenerToGatewayListener(entry)

	assert.Equal(t, gwv1.SectionName("my-listener"), result.Name)
	assert.Equal(t, &hostname, result.Hostname)
	assert.Equal(t, gwv1.PortNumber(8080), result.Port)
	assert.Equal(t, gwv1.HTTPSProtocolType, result.Protocol)
	assert.Equal(t, &tlsMode, result.TLS.Mode)
	assert.NotNil(t, result.AllowedRoutes)
}

func Test_listenerSetGatewayHandshake(t *testing.T) {
	nsSame := gwv1.NamespacesFromSame
	nsAll := gwv1.NamespacesFromAll
	nsSelector := gwv1.NamespacesFromSelector

	testCases := []struct {
		name              string
		listenerSet       gwv1.ListenerSet
		gw                gwv1.Gateway
		nsResult          sets.Set[string]
		nsErr             error
		expectedHandshake handshakeState
		expectErr         bool
	}{
		{
			name: "listenerSet does not reference gateway - irrelevant",
			listenerSet: gwv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns1"},
				Spec: gwv1.ListenerSetSpec{
					ParentRef: gwv1.ParentGatewayReference{
						Name: "other-gw",
					},
				},
			},
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "ns1"},
			},
			expectedHandshake: irrelevantResourceHandshakeState,
		},
		{
			name: "listenerSet references gateway but no AllowedListeners - defaults to None - rejected",
			listenerSet: gwv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns1"},
				Spec: gwv1.ListenerSetSpec{
					ParentRef: gwv1.ParentGatewayReference{
						Name: "my-gw",
					},
				},
			},
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "ns1"},
			},
			expectedHandshake: gatewayRejectedHandshakeState,
		},
		{
			name: "gateway allows same namespace and listenerSet is in same namespace - accepted",
			listenerSet: gwv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns1"},
				Spec: gwv1.ListenerSetSpec{
					ParentRef: gwv1.ParentGatewayReference{
						Name: "my-gw",
					},
				},
			},
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "ns1"},
				Spec: gwv1.GatewaySpec{
					AllowedListeners: &gwv1.AllowedListeners{
						Namespaces: &gwv1.ListenerNamespaces{
							From: &nsSame,
						},
					},
				},
			},
			expectedHandshake: acceptedHandshakeState,
		},
		{
			name: "gateway allows same namespace but listenerSet is in different namespace - rejected",
			listenerSet: gwv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns2"},
				Spec: gwv1.ListenerSetSpec{
					ParentRef: gwv1.ParentGatewayReference{
						Name:      "my-gw",
						Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
					},
				},
			},
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "ns1"},
				Spec: gwv1.GatewaySpec{
					AllowedListeners: &gwv1.AllowedListeners{
						Namespaces: &gwv1.ListenerNamespaces{
							From: &nsSame,
						},
					},
				},
			},
			expectedHandshake: gatewayRejectedHandshakeState,
		},
		{
			name: "gateway allows all namespaces - accepted",
			listenerSet: gwv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns2"},
				Spec: gwv1.ListenerSetSpec{
					ParentRef: gwv1.ParentGatewayReference{
						Name:      "my-gw",
						Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
					},
				},
			},
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "ns1"},
				Spec: gwv1.GatewaySpec{
					AllowedListeners: &gwv1.AllowedListeners{
						Namespaces: &gwv1.ListenerNamespaces{
							From: &nsAll,
						},
					},
				},
			},
			expectedHandshake: acceptedHandshakeState,
		},
		{
			name: "gateway allows selector and listenerSet namespace matches - accepted",
			listenerSet: gwv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns2"},
				Spec: gwv1.ListenerSetSpec{
					ParentRef: gwv1.ParentGatewayReference{
						Name:      "my-gw",
						Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
					},
				},
			},
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "ns1"},
				Spec: gwv1.GatewaySpec{
					AllowedListeners: &gwv1.AllowedListeners{
						Namespaces: &gwv1.ListenerNamespaces{
							From:     &nsSelector,
							Selector: &metav1.LabelSelector{},
						},
					},
				},
			},
			nsResult:          sets.New("ns2", "ns3"),
			expectedHandshake: acceptedHandshakeState,
		},
		{
			name: "gateway allows selector but listenerSet namespace does not match - rejected",
			listenerSet: gwv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns4"},
				Spec: gwv1.ListenerSetSpec{
					ParentRef: gwv1.ParentGatewayReference{
						Name:      "my-gw",
						Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
					},
				},
			},
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "ns1"},
				Spec: gwv1.GatewaySpec{
					AllowedListeners: &gwv1.AllowedListeners{
						Namespaces: &gwv1.ListenerNamespaces{
							From:     &nsSelector,
							Selector: &metav1.LabelSelector{},
						},
					},
				},
			},
			nsResult:          sets.New("ns2", "ns3"),
			expectedHandshake: gatewayRejectedHandshakeState,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			loader := &listenerSetLoaderImpl{
				namespaceSelector: &mockNamespaceSelector{
					nss: tc.nsResult,
					err: tc.nsErr,
				},
				logger: logr.Discard(),
			}

			result, err := loader.listenerSetGatewayHandshake(context.Background(), tc.listenerSet, tc.gw)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedHandshake, result)
		})
	}
}

func Test_getListenerSets(t *testing.T) {
	nsAll := gwv1.NamespacesFromAll

	testCases := []struct {
		name          string
		listenerSets  []*gwv1.ListenerSet
		gw            gwv1.Gateway
		expectedCount int
		expectedNames []gwv1.SectionName
		expectErr     bool
	}{
		{
			name: "no listener sets in cluster",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "ns1"},
				Spec: gwv1.GatewaySpec{
					AllowedListeners: &gwv1.AllowedListeners{
						Namespaces: &gwv1.ListenerNamespaces{
							From: &nsAll,
						},
					},
				},
			},
			expectedCount: 0,
		},
		{
			name: "one matching listener set with two listeners",
			listenerSets: []*gwv1.ListenerSet{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ls1", Namespace: "ns1"},
					Spec: gwv1.ListenerSetSpec{
						ParentRef: gwv1.ParentGatewayReference{
							Name: "my-gw",
						},
						Listeners: []gwv1.ListenerEntry{
							{
								Name:     "listener-a",
								Port:     8080,
								Protocol: gwv1.HTTPProtocolType,
							},
							{
								Name:     "listener-b",
								Port:     8443,
								Protocol: gwv1.HTTPSProtocolType,
							},
						},
					},
				},
			},
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "ns1"},
				Spec: gwv1.GatewaySpec{
					AllowedListeners: &gwv1.AllowedListeners{
						Namespaces: &gwv1.ListenerNamespaces{
							From: &nsAll,
						},
					},
				},
			},
			expectedCount: 2,
			expectedNames: []gwv1.SectionName{"listener-a", "listener-b"},
		},
		{
			name: "listener set referencing different gateway is excluded",
			listenerSets: []*gwv1.ListenerSet{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ls1", Namespace: "ns1"},
					Spec: gwv1.ListenerSetSpec{
						ParentRef: gwv1.ParentGatewayReference{
							Name: "other-gw",
						},
						Listeners: []gwv1.ListenerEntry{
							{
								Name:     "listener-a",
								Port:     8080,
								Protocol: gwv1.HTTPProtocolType,
							},
						},
					},
				},
			},
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "ns1"},
				Spec: gwv1.GatewaySpec{
					AllowedListeners: &gwv1.AllowedListeners{
						Namespaces: &gwv1.ListenerNamespaces{
							From: &nsAll,
						},
					},
				},
			},
			expectedCount: 0,
		},
		{
			name: "mixed matching and non-matching listener sets",
			listenerSets: []*gwv1.ListenerSet{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ls-match", Namespace: "ns1"},
					Spec: gwv1.ListenerSetSpec{
						ParentRef: gwv1.ParentGatewayReference{
							Name: "my-gw",
						},
						Listeners: []gwv1.ListenerEntry{
							{
								Name:     "match-listener",
								Port:     80,
								Protocol: gwv1.HTTPProtocolType,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ls-nomatch", Namespace: "ns1"},
					Spec: gwv1.ListenerSetSpec{
						ParentRef: gwv1.ParentGatewayReference{
							Name: "other-gw",
						},
						Listeners: []gwv1.ListenerEntry{
							{
								Name:     "nomatch-listener",
								Port:     80,
								Protocol: gwv1.HTTPProtocolType,
							},
						},
					},
				},
			},
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "ns1"},
				Spec: gwv1.GatewaySpec{
					AllowedListeners: &gwv1.AllowedListeners{
						Namespaces: &gwv1.ListenerNamespaces{
							From: &nsAll,
						},
					},
				},
			},
			expectedCount: 1,
			expectedNames: []gwv1.SectionName{"match-listener"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sClient := testutils.GenerateTestClient()
			for _, ls := range tc.listenerSets {
				err := k8sClient.Create(context.Background(), ls)
				assert.NoError(t, err)
			}

			loader := &listenerSetLoaderImpl{
				k8sClient:         k8sClient,
				namespaceSelector: &mockNamespaceSelector{},
				logger:            logr.Discard(),
			}

			result, err := loader.getListenerSets(context.Background(), tc.gw)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Len(t, result, tc.expectedCount)

			if tc.expectedNames != nil {
				var names []gwv1.SectionName
				for _, l := range result {
					names = append(names, l.Name)
				}
				assert.Equal(t, tc.expectedNames, names)
			}
		})
	}
}
