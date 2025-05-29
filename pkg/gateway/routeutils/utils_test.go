package routeutils

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	mock_client "sigs.k8s.io/aws-load-balancer-controller/mocks/controller-runtime/client"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ RouteDescriptor = &mockPreLoadRouteDescriptor{}

// Mock implementations
type mockPreLoadRouteDescriptor struct {
	backendRefs    []gwv1.BackendRef
	namespacedName types.NamespacedName
}

func (m mockPreLoadRouteDescriptor) GetAttachedRules() []RouteRule {
	//TODO implement me
	panic("implement me")
}

func (m mockPreLoadRouteDescriptor) GetRouteNamespacedName() types.NamespacedName {
	return m.namespacedName
}

func (m mockPreLoadRouteDescriptor) GetRouteKind() RouteKind {
	//TODO implement me
	panic("implement me")
}

func (m mockPreLoadRouteDescriptor) GetHostnames() []gwv1.Hostname {
	//TODO implement me
	panic("implement me")
}

func (m mockPreLoadRouteDescriptor) GetParentRefs() []gwv1.ParentReference {
	//TODO implement me
	panic("implement me")
}

func (m mockPreLoadRouteDescriptor) GetRawRoute() interface{} {
	//TODO implement me
	panic("implement me")
}

func (m mockPreLoadRouteDescriptor) GetBackendRefs() []gwv1.BackendRef {
	return m.backendRefs
}

func (m mockPreLoadRouteDescriptor) loadAttachedRules(context context.Context, k8sClient client.Client) (RouteDescriptor, error) {
	//TODO implement me
	panic("implement me")
}

// Test ListL4Routes
func Test_ListL4Routes(t *testing.T) {
	tests := []struct {
		name           string
		mockSetup      func(*gomock.Controller) client.Client
		expectedRoutes int
		expectedErr    error
	}{
		{
			name: "Successfully lists all L4 routes",
			mockSetup: func(ctrl *gomock.Controller) client.Client {
				k8sClient := testutils.GenerateTestClient()
				k8sClient.Create(context.Background(), &gwalpha2.TCPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo1",
						Namespace: "bar1",
					},
					Spec: gwalpha2.TCPRouteSpec{
						Rules: []gwalpha2.TCPRouteRule{
							{
								BackendRefs: []gwalpha2.BackendRef{
									{},
									{},
								},
							},
							{
								BackendRefs: []gwalpha2.BackendRef{
									{},
									{},
									{},
									{},
								},
							},
							{
								BackendRefs: []gwalpha2.BackendRef{},
							},
						},
					},
				})
				k8sClient.Create(context.Background(), &gwalpha2.UDPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo1",
						Namespace: "bar1",
					},
					Spec: gwalpha2.UDPRouteSpec{
						Rules: []gwalpha2.UDPRouteRule{
							{
								BackendRefs: []gwalpha2.BackendRef{
									{},
									{},
								},
							},
							{
								BackendRefs: []gwalpha2.BackendRef{
									{},
									{},
									{},
									{},
								},
							},
							{
								BackendRefs: []gwalpha2.BackendRef{},
							},
						},
					},
				})
				k8sClient.Create(context.Background(), &gwalpha2.TLSRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo1",
						Namespace: "bar1",
					},
					Spec: gwalpha2.TLSRouteSpec{
						Hostnames: []gwv1.Hostname{
							"host1",
						},
						Rules: []gwalpha2.TLSRouteRule{
							{
								BackendRefs: []gwalpha2.BackendRef{
									{},
									{},
								},
							},
							{
								BackendRefs: []gwalpha2.BackendRef{
									{},
									{},
									{},
									{},
								},
							},
							{
								BackendRefs: []gwalpha2.BackendRef{},
							},
						},
					},
				})
				return k8sClient
			},
			expectedRoutes: 3,
		},
		{
			name: "Handles error in TCP routes",
			mockSetup: func(ctrl *gomock.Controller) client.Client {
				mockClient := mock_client.NewMockClient(ctrl)
				// Setup mock responses for TCP, UDP, and TLS routes
				mockClient.EXPECT().List(gomock.Any(), &gwalpha2.TCPRouteList{}).Return(fmt.Errorf("TCP error"))
				mockClient.EXPECT().List(gomock.Any(), &gwalpha2.UDPRouteList{}).Return(nil)
				mockClient.EXPECT().List(gomock.Any(), &gwalpha2.TLSRouteList{}).Return(nil)
				return mockClient
			},
			expectedRoutes: 0,
			expectedErr:    fmt.Errorf("failed to list L4 routes, [TCPRoute]"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := tt.mockSetup(ctrl)
			routes, err := ListL4Routes(context.Background(), client)

			if tt.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			}
			assert.Len(t, routes, tt.expectedRoutes)

		})
	}
}

// Test ListL7Routes
func Test_ListL7Routes(t *testing.T) {
	tests := []struct {
		name           string
		mockSetup      func(*gomock.Controller) client.Client
		expectedRoutes int
		expectedErr    error
	}{
		{
			name: "Successfully lists all L7 routes",
			mockSetup: func(ctrl *gomock.Controller) client.Client {
				k8sClient := testutils.GenerateTestClient()
				k8sClient.Create(context.Background(), &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo1",
						Namespace: "bar1",
					},
					Spec: gwv1.HTTPRouteSpec{
						Hostnames: []gwv1.Hostname{
							"host1",
						},
						Rules: []gwv1.HTTPRouteRule{
							{
								BackendRefs: []gwv1.HTTPBackendRef{
									{},
									{},
								},
							},
							{
								BackendRefs: []gwv1.HTTPBackendRef{
									{},
									{},
									{},
									{},
								},
							},
							{
								BackendRefs: []gwv1.HTTPBackendRef{},
							},
						},
					},
				})
				k8sClient.Create(context.Background(), &gwv1.GRPCRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo1",
						Namespace: "bar1",
					},
					Spec: gwv1.GRPCRouteSpec{
						Hostnames: []gwv1.Hostname{
							"host1",
						},
						Rules: []gwv1.GRPCRouteRule{
							{
								BackendRefs: []gwv1.GRPCBackendRef{
									{},
									{},
								},
							},
							{
								BackendRefs: []gwv1.GRPCBackendRef{
									{},
									{},
									{},
									{},
								},
							},
							{
								BackendRefs: []gwv1.GRPCBackendRef{},
							},
						},
					},
				})
				return k8sClient
			},
			expectedRoutes: 2,
			expectedErr:    nil,
		},
		{
			name: "Handles error in HTTP routes",
			mockSetup: func(ctrl *gomock.Controller) client.Client {
				mockClient := mock_client.NewMockClient(ctrl)
				mockClient.EXPECT().List(gomock.Any(), &gwv1.HTTPRouteList{}).Return(fmt.Errorf("HTTP error"))
				mockClient.EXPECT().List(gomock.Any(), &gwv1.GRPCRouteList{}).Return(nil)
				return mockClient
			},
			expectedRoutes: 0,
			expectedErr:    fmt.Errorf("failed to list L7 routes, [HTTPRoute]"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := tt.mockSetup(ctrl)
			routes, err := ListL7Routes(context.Background(), client)

			if tt.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			}
			assert.Len(t, routes, tt.expectedRoutes)

		})
	}
}

// Test FilterRoutesBySvc
func Test_FilterRoutesBySvc(t *testing.T) {
	namespace := "test-ns"
	svcName := "test-svc"

	tests := []struct {
		name          string
		routes        []preLoadRouteDescriptor
		service       *corev1.Service
		expectedCount int
	}{
		{
			name:          "Nil service returns nil",
			routes:        []preLoadRouteDescriptor{},
			service:       nil,
			expectedCount: 0,
		},
		{
			name:   "Empty routes returns nil",
			routes: []preLoadRouteDescriptor{},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: namespace,
				},
			},
			expectedCount: 0,
		},
		{
			name: "Filters matching routes",
			routes: []preLoadRouteDescriptor{
				mockPreLoadRouteDescriptor{
					backendRefs: []gwv1.BackendRef{
						{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: gwv1.ObjectName(svcName),
							},
						},
					},
					namespacedName: types.NamespacedName{
						Namespace: namespace,
						Name:      "route-1",
					},
				},
				&mockPreLoadRouteDescriptor{
					backendRefs: []gwv1.BackendRef{
						{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: gwv1.ObjectName("other-svc"),
							},
						},
					},
					namespacedName: types.NamespacedName{
						Namespace: namespace,
						Name:      "route-2",
					},
				},
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: namespace,
				},
			},
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := FilterRoutesBySvc(tt.routes, tt.service)
			assert.Len(t, filtered, tt.expectedCount)
		})
	}
}

// Test isServiceReferredByRoute
func Test_IsServiceReferredByRoute(t *testing.T) {
	tests := []struct {
		name     string
		route    preLoadRouteDescriptor
		svcID    types.NamespacedName
		expected bool
	}{
		{
			name: "Route refers to service",
			route: mockPreLoadRouteDescriptor{
				backendRefs: []gwv1.BackendRef{
					{
						BackendObjectReference: gwv1.BackendObjectReference{
							Name: gwv1.ObjectName("test-svc"),
						},
					},
				},
				namespacedName: types.NamespacedName{
					Namespace: "test-ns",
					Name:      "route-1",
				},
			},
			svcID: types.NamespacedName{
				Namespace: "test-ns",
				Name:      "test-svc",
			},
			expected: true,
		},
		{
			name: "Route does not refer to service",
			route: mockPreLoadRouteDescriptor{
				backendRefs: []gwv1.BackendRef{
					{
						BackendObjectReference: gwv1.BackendObjectReference{
							Name: gwv1.ObjectName("other-svc"),
						},
					},
				},
				namespacedName: types.NamespacedName{
					Namespace: "test-ns",
					Name:      "route-1",
				},
			},
			svcID: types.NamespacedName{
				Namespace: "test-ns",
				Name:      "test-svc",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isServiceReferredByRoute(tt.route, tt.svcID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test isHostNameInValidFormat
func TestIsHostNameInValidFormat(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		want     bool
		wantErr  string
	}{
		{
			name:     "IPv4 address",
			hostname: "192.168.1.1",
			want:     false,
			wantErr:  "hostname can not be IP address",
		},
		{
			name:     "Label too short",
			hostname: "test..com",
			want:     false,
			wantErr:  "invalid hostname label length",
		},
		{
			name:     "Label too long",
			hostname: strings.Repeat("a", 64) + ".com",
			want:     false,
			wantErr:  "invalid hostname label length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsHostNameInValidFormat(tt.hostname)
			if got != tt.want {
				t.Errorf("IsHostNameInValidFormat() got = %v, want %v", got, tt.want)
			}

			if tt.wantErr == "" && err != nil {
				t.Errorf("IsHostNameInValidFormat() unexpected error = %v", err)
			}
			if tt.wantErr != "" && err == nil {
				t.Errorf("IsHostNameInValidFormat() expected error containing %q but got nil", tt.wantErr)
			}
			if tt.wantErr != "" && err != nil && !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("IsHostNameInValidFormat() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test isHostnameCompatible
func Test_isHostnameCompatible(t *testing.T) {
	tests := []struct {
		name        string
		hostnameOne string
		hostnameTwo string
		want        bool
	}{
		{
			name:        "exact match",
			hostnameOne: "example.com",
			hostnameTwo: "example.com",
			want:        true,
		},
		{
			name:        "first hostname is wildcard - matching",
			hostnameOne: "*.example.com",
			hostnameTwo: "test.example.com",
			want:        true,
		},
		{
			name:        "second hostname is wildcard - matching",
			hostnameOne: "test.example.com",
			hostnameTwo: "*.example.com",
			want:        true,
		},
		{
			name:        "both hostnames are wildcards - matching",
			hostnameOne: "*.example.com",
			hostnameTwo: "*.test.example.com",
			want:        true,
		},
		{
			name:        "first hostname is wildcard - not matching",
			hostnameOne: "*.example.com",
			hostnameTwo: "test.different.com",
			want:        false,
		},
		{
			name:        "second hostname is wildcard - not matching",
			hostnameOne: "test.different.com",
			hostnameTwo: "*.example.com",
			want:        false,
		},
		{
			name:        "wildcard with subdomain - matching",
			hostnameOne: "*.sub.example.com",
			hostnameTwo: "test.sub.example.com",
			want:        true,
		},
		{
			name:        "empty hostnames",
			hostnameOne: "",
			hostnameTwo: "",
			want:        true,
		},
		{
			name:        "one empty hostname",
			hostnameOne: "example.com",
			hostnameTwo: "",
			want:        false,
		},
		{
			name:        "wildcard with root domain",
			hostnameOne: "*.example.com",
			hostnameTwo: "example.com",
			want:        false,
		},
		{
			name:        "multiple subdomains - matching",
			hostnameOne: "*.example.com",
			hostnameTwo: "a.b.example.com",
			want:        true,
		},
		{
			name:        "partial wildcard match - not matching",
			hostnameOne: "*.example.com",
			hostnameTwo: "testexample.com",
			want:        false,
		},
		{
			name:        "invalid wildcard format",
			hostnameOne: "*example.com",
			hostnameTwo: "test.example.com",
			want:        false,
		},
		{
			name:        "case sensitivity test",
			hostnameOne: "*.Example.com",
			hostnameTwo: "test.example.com",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHostnameCompatible(tt.hostnameOne, tt.hostnameTwo)
			if got != tt.want {
				t.Errorf("isHostnameCompatible() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test GetHostnamePrecedenceOrder
func TestGetHostnamePrecedenceOrder(t *testing.T) {
	tests := []struct {
		name        string
		hostnameOne string
		hostnameTwo string
		want        int
		description string
	}{
		{
			name:        "non-wildcard vs wildcard",
			hostnameOne: "example.com",
			hostnameTwo: "*.example.com",
			want:        -1,
			description: "non-wildcard should have higher precedence than wildcard",
		},
		{
			name:        "wildcard vs non-wildcard",
			hostnameOne: "*.example.com",
			hostnameTwo: "example.com",
			want:        1,
			description: "wildcard should have lower precedence than non-wildcard",
		},
		{
			name:        "both non-wildcard - first longer",
			hostnameOne: "test.example.com",
			hostnameTwo: "example.com",
			want:        -1,
			description: "longer hostname should have higher precedence",
		},
		{
			name:        "both non-wildcard - second longer",
			hostnameOne: "example.com",
			hostnameTwo: "test.example.com",
			want:        1,
			description: "shorter hostname should have lower precedence",
		},
		{
			name:        "both wildcard - first longer",
			hostnameOne: "*.test.example.com",
			hostnameTwo: "*.example.com",
			want:        -1,
			description: "longer wildcard hostname should have higher precedence",
		},
		{
			name:        "both wildcard - second longer",
			hostnameOne: "*.example.com",
			hostnameTwo: "*.test.example.com",
			want:        1,
			description: "shorter wildcard hostname should have lower precedence",
		},
		{
			name:        "equal length non-wildcard",
			hostnameOne: "test1.com",
			hostnameTwo: "test2.com",
			want:        0,
			description: "equal length hostnames should have equal precedence",
		},
		{
			name:        "equal length wildcard",
			hostnameOne: "*.test1.com",
			hostnameTwo: "*.test2.com",
			want:        0,
			description: "equal length wildcard hostnames should have equal precedence",
		},
		{
			name:        "empty strings",
			hostnameOne: "",
			hostnameTwo: "",
			want:        0,
			description: "empty strings should have equal precedence",
		},
		{
			name:        "one empty string - first",
			hostnameOne: "",
			hostnameTwo: "example.com",
			want:        1,
			description: "empty string should have lower precedence",
		},
		{
			name:        "one empty string - second",
			hostnameOne: "example.com",
			hostnameTwo: "",
			want:        -1,
			description: "non-empty string should have higher precedence than empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetHostnamePrecedenceOrder(tt.hostnameOne, tt.hostnameTwo)
			if got != tt.want {
				t.Errorf("GetHostnamePrecedenceOrder() = %v, want %v\nDescription: %s\nHostname1: %q\nHostname2: %q",
					got, tt.want, tt.description, tt.hostnameOne, tt.hostnameTwo)
			}
		})
	}
}
