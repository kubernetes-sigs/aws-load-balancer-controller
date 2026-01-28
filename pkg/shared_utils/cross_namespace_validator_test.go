package shared_utils

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// mockClient implements a mock client.Client for testing
type mockClient struct {
	mock.Mock
	client.Client
}

func (m *mockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	args := m.Called(ctx, list, opts)
	return args.Error(0)
}

func TestValidateCrossNamespaceReference(t *testing.T) {

	testCases := []struct {
		name            string
		kind            string
		referenceGrants []gwv1beta1.ReferenceGrant
		fromNamespace   string
		fromGroup       string
		fromKind        string
		toGroup         string
		toKind          string
		toNamespace     string
		toName          string
		expected        bool
		expectErr       bool
		setupMock       func() client.Client
	}{
		{
			name:          "happy path - service reference",
			kind:          shared_constants.ServiceKind,
			fromNamespace: "route-namespace",
			fromGroup:     shared_constants.GatewayAPIResourcesGroup,
			fromKind:      shared_constants.HTTPRouteKind,
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toNamespace:   "svc-namespace",
			toName:        "svc-name",
			referenceGrants: []gwv1beta1.ReferenceGrant{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "svc-namespace",
						Name:      "grant1",
					},
					Spec: gwv1beta1.ReferenceGrantSpec{
						From: []gwv1beta1.ReferenceGrantFrom{
							{
								Group:     shared_constants.GatewayAPIResourcesGroup,
								Kind:      gwv1beta1.Kind(shared_constants.HTTPRouteKind),
								Namespace: "route-namespace",
							},
						},
						To: []gwv1beta1.ReferenceGrantTo{
							{
								Group: shared_constants.CoreAPIGroup,
								Kind:  shared_constants.ServiceKind,
								Name:  (*gwv1beta1.ObjectName)(aws.String("svc-name")),
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name:          "happy path - gateway reference",
			kind:          shared_constants.GatewayApiKind,
			fromNamespace: "route-namespace",
			fromGroup:     shared_constants.GatewayAPIResourcesGroup,
			fromKind:      shared_constants.HTTPRouteKind,
			toGroup:       shared_constants.GatewayAPIResourcesGroup,
			toKind:        shared_constants.GatewayApiKind,
			toNamespace:   "gw-namespace",
			toName:        "gw-name",
			referenceGrants: []gwv1beta1.ReferenceGrant{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gw-namespace",
						Name:      "grant1",
					},
					Spec: gwv1beta1.ReferenceGrantSpec{
						From: []gwv1beta1.ReferenceGrantFrom{
							{
								Group:     shared_constants.GatewayAPIResourcesGroup,
								Kind:      gwv1beta1.Kind(shared_constants.HTTPRouteKind),
								Namespace: "route-namespace",
							},
						},
						To: []gwv1beta1.ReferenceGrantTo{
							{
								Group: shared_constants.GatewayAPIResourcesGroup,
								Kind:  shared_constants.GatewayApiKind,
								Name:  (*gwv1beta1.ObjectName)(aws.String("gw-name")),
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name:          "happy path (no name equals wildcard)",
			kind:          shared_constants.ServiceKind,
			fromNamespace: "route-namespace",
			fromGroup:     shared_constants.GatewayAPIResourcesGroup,
			fromKind:      shared_constants.HTTPRouteKind,
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toNamespace:   "svc-namespace",
			toName:        "svc-name",
			referenceGrants: []gwv1beta1.ReferenceGrant{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "svc-namespace",
						Name:      "grant1",
					},
					Spec: gwv1beta1.ReferenceGrantSpec{
						From: []gwv1beta1.ReferenceGrantFrom{
							{
								Group:     shared_constants.GatewayAPIResourcesGroup,
								Kind:      gwv1beta1.Kind(shared_constants.HTTPRouteKind),
								Namespace: "route-namespace",
							},
						},
						To: []gwv1beta1.ReferenceGrantTo{
							{
								Group: shared_constants.CoreAPIGroup,
								Kind:  shared_constants.ServiceKind,
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name:          "no grants, should not allow",
			kind:          shared_constants.ServiceKind,
			fromNamespace: "route-namespace",
			fromGroup:     shared_constants.GatewayAPIResourcesGroup,
			fromKind:      shared_constants.HTTPRouteKind,
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toNamespace:   "svc-namespace",
			toName:        "svc-name",
			expected:      false,
		},
		{
			name:          "from is allowed, but not to",
			kind:          shared_constants.ServiceKind,
			fromNamespace: "route-namespace",
			fromGroup:     shared_constants.GatewayAPIResourcesGroup,
			fromKind:      shared_constants.HTTPRouteKind,
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toNamespace:   "svc-namespace",
			toName:        "svc-name",
			referenceGrants: []gwv1beta1.ReferenceGrant{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "svc-namespace",
						Name:      "grant1",
					},
					Spec: gwv1beta1.ReferenceGrantSpec{
						From: []gwv1beta1.ReferenceGrantFrom{
							{
								Group:     shared_constants.GatewayAPIResourcesGroup,
								Kind:      gwv1beta1.Kind(shared_constants.HTTPRouteKind),
								Namespace: "route-namespace",
							},
						},
						To: []gwv1beta1.ReferenceGrantTo{
							{
								Group: shared_constants.CoreAPIGroup,
								Kind:  shared_constants.ServiceKind,
								Name:  (*gwv1beta1.ObjectName)(aws.String("different-svc")),
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name:          "to is allowed, but not from",
			kind:          shared_constants.ServiceKind,
			fromNamespace: "route-namespace",
			fromGroup:     shared_constants.GatewayAPIResourcesGroup,
			fromKind:      shared_constants.HTTPRouteKind,
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toNamespace:   "svc-namespace",
			toName:        "svc-name",
			referenceGrants: []gwv1beta1.ReferenceGrant{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "svc-namespace",
						Name:      "grant1",
					},
					Spec: gwv1beta1.ReferenceGrantSpec{
						From: []gwv1beta1.ReferenceGrantFrom{
							{
								Group:     shared_constants.GatewayAPIResourcesGroup,
								Kind:      gwv1beta1.Kind("other kind"),
								Namespace: "route-namespace",
							},
						},
						To: []gwv1beta1.ReferenceGrantTo{
							{
								Group: shared_constants.CoreAPIGroup,
								Kind:  shared_constants.ServiceKind,
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name:          "reference grant is for wrong type",
			kind:          shared_constants.GatewayApiKind,
			fromNamespace: "route-namespace",
			fromGroup:     shared_constants.GatewayAPIResourcesGroup,
			fromKind:      shared_constants.HTTPRouteKind,
			toGroup:       shared_constants.GatewayAPIResourcesGroup,
			toKind:        shared_constants.GatewayApiKind,
			toNamespace:   "gw-namespace",
			toName:        "gw-name",
			referenceGrants: []gwv1beta1.ReferenceGrant{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gw-namespace",
						Name:      "grant1",
					},
					Spec: gwv1beta1.ReferenceGrantSpec{
						From: []gwv1beta1.ReferenceGrantFrom{
							{
								Group:     shared_constants.GatewayAPIResourcesGroup,
								Kind:      gwv1beta1.Kind(shared_constants.HTTPRouteKind),
								Namespace: "route-namespace",
							},
						},
						To: []gwv1beta1.ReferenceGrantTo{
							{
								Group: shared_constants.CoreAPIGroup,
								Kind:  shared_constants.ServiceKind,
								Name:  (*gwv1beta1.ObjectName)(aws.String("gw-name")),
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name:          "wrong from group - should not allow",
			kind:          shared_constants.ServiceKind,
			fromNamespace: "route-namespace",
			fromGroup:     shared_constants.GatewayAPIResourcesGroup,
			fromKind:      shared_constants.HTTPRouteKind,
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toNamespace:   "svc-namespace",
			toName:        "svc-name",
			referenceGrants: []gwv1beta1.ReferenceGrant{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "svc-namespace",
						Name:      "grant1",
					},
					Spec: gwv1beta1.ReferenceGrantSpec{
						From: []gwv1beta1.ReferenceGrantFrom{
							{
								Group:     "wrong-group",
								Kind:      gwv1beta1.Kind(shared_constants.HTTPRouteKind),
								Namespace: "route-namespace",
							},
						},
						To: []gwv1beta1.ReferenceGrantTo{
							{
								Group: shared_constants.CoreAPIGroup,
								Kind:  shared_constants.ServiceKind,
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name:          "wrong to group - should not allow",
			kind:          shared_constants.ServiceKind,
			fromNamespace: "route-namespace",
			fromGroup:     shared_constants.GatewayAPIResourcesGroup,
			fromKind:      shared_constants.HTTPRouteKind,
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toNamespace:   "svc-namespace",
			toName:        "svc-name",
			referenceGrants: []gwv1beta1.ReferenceGrant{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "svc-namespace",
						Name:      "grant1",
					},
					Spec: gwv1beta1.ReferenceGrantSpec{
						From: []gwv1beta1.ReferenceGrantFrom{
							{
								Group:     shared_constants.GatewayAPIResourcesGroup,
								Kind:      gwv1beta1.Kind(shared_constants.HTTPRouteKind),
								Namespace: "route-namespace",
							},
						},
						To: []gwv1beta1.ReferenceGrantTo{
							{
								Group: "wrong-group",
								Kind:  shared_constants.ServiceKind,
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name:          "accelerator to service reference",
			kind:          shared_constants.ServiceKind,
			fromNamespace: "accelerator-ns",
			fromGroup:     shared_constants.GlobalAcceleratorResourcesGroup,
			fromKind:      shared_constants.GlobalAcceleratorKind,
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toNamespace:   "service-ns",
			toName:        "my-service",
			referenceGrants: []gwv1beta1.ReferenceGrant{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "service-ns",
						Name:      "grant1",
					},
					Spec: gwv1beta1.ReferenceGrantSpec{
						From: []gwv1beta1.ReferenceGrantFrom{
							{
								Group:     shared_constants.GlobalAcceleratorResourcesGroup,
								Kind:      shared_constants.GlobalAcceleratorKind,
								Namespace: "accelerator-ns",
							},
						},
						To: []gwv1beta1.ReferenceGrantTo{
							{
								Group: shared_constants.CoreAPIGroup,
								Kind:  shared_constants.ServiceKind,
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name:          "missing ReferenceGrant CRD - NotFound error",
			kind:          shared_constants.ServiceKind,
			fromNamespace: "accelerator-ns",
			fromGroup:     shared_constants.GlobalAcceleratorResourcesGroup,
			fromKind:      shared_constants.GlobalAcceleratorKind,
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toNamespace:   "service-ns",
			toName:        "my-service",
			setupMock: func() client.Client {
				mockClient := &mockClient{}
				// Simulate NotFound error when the CRD is not installed
				notFoundErr := apierrors.NewNotFound(schema.GroupResource{
					Group:    "gateway.networking.k8s.io",
					Resource: "referencegrants",
				}, "referencegrants")
				mockClient.On("List", mock.Anything, mock.Anything, mock.Anything).Return(notFoundErr)
				return mockClient
			},
			expected:  false, // Should deny access when CRD is missing
			expectErr: false, // But should not return an error
		},
		{
			name:          "missing ReferenceGrant CRD - NoMatch error",
			kind:          shared_constants.ServiceKind,
			fromNamespace: "accelerator-ns",
			fromGroup:     shared_constants.GlobalAcceleratorResourcesGroup,
			fromKind:      shared_constants.GlobalAcceleratorKind,
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toNamespace:   "service-ns",
			toName:        "my-service",
			setupMock: func() client.Client {
				mockClient := &mockClient{}
				// Simulate NoMatch error when the CRD is not installed
				noMatchErr := &apierrors.StatusError{
					ErrStatus: metav1.Status{
						Status:  "Failure",
						Message: "no matches for kind \"ReferenceGrant\" in version \"gateway.networking.k8s.io/v1beta1\"",
						Reason:  "NoMatch",
						Code:    404,
					},
				}
				mockClient.On("List", mock.Anything, mock.Anything, mock.Anything).Return(noMatchErr)
				return mockClient
			},
			expected:  false, // Should deny access when CRD is missing
			expectErr: false, // But should not return an error
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var k8sClient client.Client
			
			// Setup mock client if provided
			if tc.setupMock != nil {
				k8sClient = tc.setupMock()
			} else {
				// Use normal test client
				k8sClient = testutils.GenerateTestClient()
				for _, ref := range tc.referenceGrants {
					err := k8sClient.Create(context.Background(), &ref)
					assert.NoError(t, err)
				}
			}

			// Create a context with logger to capture log output
			loggerCtx := logr.NewContext(context.Background(), log.Log)
			
			result, err := ValidateCrossNamespaceReference(
				loggerCtx,
				k8sClient,
				tc.fromNamespace, tc.fromGroup, tc.fromKind,
				tc.toGroup, tc.toKind, tc.toNamespace, tc.toName,
			)

			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGrantAllowsReference(t *testing.T) {
	tests := []struct {
		name          string
		grant         gwv1beta1.ReferenceGrant
		fromNamespace string
		toGroup       string
		toKind        string
		toName        string
		expected      bool
	}{
		{
			name: "matching grant - service reference",
			grant: gwv1beta1.ReferenceGrant{
				Spec: gwv1beta1.ReferenceGrantSpec{
					From: []gwv1beta1.ReferenceGrantFrom{
						{
							Group:     gwv1beta1.Group(shared_constants.GlobalAcceleratorResourcesGroup),
							Kind:      gwv1beta1.Kind(shared_constants.GlobalAcceleratorKind),
							Namespace: gwv1beta1.Namespace("accelerator-ns"),
						},
					},
					To: []gwv1beta1.ReferenceGrantTo{
						{
							Group: gwv1beta1.Group(shared_constants.CoreAPIGroup),
							Kind:  gwv1beta1.Kind(shared_constants.ServiceKind),
						},
					},
				},
			},
			fromNamespace: "accelerator-ns",
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toName:        "my-service",
			expected:      true,
		},
		{
			name: "matching grant - ingress reference",
			grant: gwv1beta1.ReferenceGrant{
				Spec: gwv1beta1.ReferenceGrantSpec{
					From: []gwv1beta1.ReferenceGrantFrom{
						{
							Group:     gwv1beta1.Group(shared_constants.GlobalAcceleratorResourcesGroup),
							Kind:      gwv1beta1.Kind(shared_constants.GlobalAcceleratorKind),
							Namespace: gwv1beta1.Namespace("accelerator-ns"),
						},
					},
					To: []gwv1beta1.ReferenceGrantTo{
						{
							Group: gwv1beta1.Group(shared_constants.IngressAPIGroup),
							Kind:  gwv1beta1.Kind(shared_constants.IngressKind),
						},
					},
				},
			},
			fromNamespace: "accelerator-ns",
			toGroup:       shared_constants.IngressAPIGroup,
			toKind:        shared_constants.IngressKind,
			toName:        "my-ingress",
			expected:      true,
		},
		{
			name: "matching grant - gateway reference",
			grant: gwv1beta1.ReferenceGrant{
				Spec: gwv1beta1.ReferenceGrantSpec{
					From: []gwv1beta1.ReferenceGrantFrom{
						{
							Group:     gwv1beta1.Group(shared_constants.GlobalAcceleratorResourcesGroup),
							Kind:      gwv1beta1.Kind(shared_constants.GlobalAcceleratorKind),
							Namespace: gwv1beta1.Namespace("accelerator-ns"),
						},
					},
					To: []gwv1beta1.ReferenceGrantTo{
						{
							Group: gwv1beta1.Group(shared_constants.GatewayAPIResourcesGroup),
							Kind:  gwv1beta1.Kind(shared_constants.GatewayApiKind),
						},
					},
				},
			},
			fromNamespace: "accelerator-ns",
			toGroup:       shared_constants.GatewayAPIResourcesGroup,
			toKind:        shared_constants.GatewayApiKind,
			toName:        "my-gateway",
			expected:      true,
		},
		{
			name: "matching grant - multiple from namespaces",
			grant: gwv1beta1.ReferenceGrant{
				Spec: gwv1beta1.ReferenceGrantSpec{
					From: []gwv1beta1.ReferenceGrantFrom{
						{
							Group:     gwv1beta1.Group(shared_constants.GlobalAcceleratorResourcesGroup),
							Kind:      gwv1beta1.Kind(shared_constants.GlobalAcceleratorKind),
							Namespace: gwv1beta1.Namespace("other-ns"),
						},
						{
							Group:     gwv1beta1.Group(shared_constants.GlobalAcceleratorResourcesGroup),
							Kind:      gwv1beta1.Kind(shared_constants.GlobalAcceleratorKind),
							Namespace: gwv1beta1.Namespace("accelerator-ns"),
						},
					},
					To: []gwv1beta1.ReferenceGrantTo{
						{
							Group: gwv1beta1.Group(shared_constants.CoreAPIGroup),
							Kind:  gwv1beta1.Kind(shared_constants.ServiceKind),
						},
					},
				},
			},
			fromNamespace: "accelerator-ns",
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toName:        "my-service",
			expected:      true,
		},
		{
			name: "matching grant - multiple to resources",
			grant: gwv1beta1.ReferenceGrant{
				Spec: gwv1beta1.ReferenceGrantSpec{
					From: []gwv1beta1.ReferenceGrantFrom{
						{
							Group:     gwv1beta1.Group(shared_constants.GlobalAcceleratorResourcesGroup),
							Kind:      gwv1beta1.Kind(shared_constants.GlobalAcceleratorKind),
							Namespace: gwv1beta1.Namespace("accelerator-ns"),
						},
					},
					To: []gwv1beta1.ReferenceGrantTo{
						{
							Group: gwv1beta1.Group(shared_constants.IngressAPIGroup),
							Kind:  gwv1beta1.Kind(shared_constants.IngressKind),
						},
						{
							Group: gwv1beta1.Group(shared_constants.CoreAPIGroup),
							Kind:  gwv1beta1.Kind(shared_constants.ServiceKind),
						},
					},
				},
			},
			fromNamespace: "accelerator-ns",
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toName:        "my-service",
			expected:      true,
		},
		{
			name: "non-matching - wrong from namespace",
			grant: gwv1beta1.ReferenceGrant{
				Spec: gwv1beta1.ReferenceGrantSpec{
					From: []gwv1beta1.ReferenceGrantFrom{
						{
							Group:     gwv1beta1.Group(shared_constants.GlobalAcceleratorResourcesGroup),
							Kind:      gwv1beta1.Kind(shared_constants.GlobalAcceleratorKind),
							Namespace: gwv1beta1.Namespace("other-ns"),
						},
					},
					To: []gwv1beta1.ReferenceGrantTo{
						{
							Group: gwv1beta1.Group(shared_constants.CoreAPIGroup),
							Kind:  gwv1beta1.Kind(shared_constants.ServiceKind),
						},
					},
				},
			},
			fromNamespace: "accelerator-ns",
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toName:        "my-service",
			expected:      false,
		},
		{
			name: "non-matching - wrong from group",
			grant: gwv1beta1.ReferenceGrant{
				Spec: gwv1beta1.ReferenceGrantSpec{
					From: []gwv1beta1.ReferenceGrantFrom{
						{
							Group:     gwv1beta1.Group("some-other-group"),
							Kind:      gwv1beta1.Kind(shared_constants.GlobalAcceleratorKind),
							Namespace: gwv1beta1.Namespace("accelerator-ns"),
						},
					},
					To: []gwv1beta1.ReferenceGrantTo{
						{
							Group: gwv1beta1.Group(shared_constants.CoreAPIGroup),
							Kind:  gwv1beta1.Kind(shared_constants.ServiceKind),
						},
					},
				},
			},
			fromNamespace: "accelerator-ns",
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toName:        "my-service",
			expected:      false,
		},
		{
			name: "non-matching - wrong from kind",
			grant: gwv1beta1.ReferenceGrant{
				Spec: gwv1beta1.ReferenceGrantSpec{
					From: []gwv1beta1.ReferenceGrantFrom{
						{
							Group:     gwv1beta1.Group(shared_constants.GlobalAcceleratorResourcesGroup),
							Kind:      gwv1beta1.Kind("SomeOtherKind"),
							Namespace: gwv1beta1.Namespace("accelerator-ns"),
						},
					},
					To: []gwv1beta1.ReferenceGrantTo{
						{
							Group: gwv1beta1.Group(shared_constants.CoreAPIGroup),
							Kind:  gwv1beta1.Kind(shared_constants.ServiceKind),
						},
					},
				},
			},
			fromNamespace: "accelerator-ns",
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toName:        "my-service",
			expected:      false,
		},
		{
			name: "non-matching - wrong to group",
			grant: gwv1beta1.ReferenceGrant{
				Spec: gwv1beta1.ReferenceGrantSpec{
					From: []gwv1beta1.ReferenceGrantFrom{
						{
							Group:     gwv1beta1.Group(shared_constants.GlobalAcceleratorResourcesGroup),
							Kind:      gwv1beta1.Kind(shared_constants.GlobalAcceleratorKind),
							Namespace: gwv1beta1.Namespace("accelerator-ns"),
						},
					},
					To: []gwv1beta1.ReferenceGrantTo{
						{
							Group: gwv1beta1.Group(shared_constants.IngressAPIGroup),
							Kind:  gwv1beta1.Kind(shared_constants.IngressKind),
						},
					},
				},
			},
			fromNamespace: "accelerator-ns",
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toName:        "my-service",
			expected:      false,
		},
		{
			name: "non-matching - wrong to kind",
			grant: gwv1beta1.ReferenceGrant{
				Spec: gwv1beta1.ReferenceGrantSpec{
					From: []gwv1beta1.ReferenceGrantFrom{
						{
							Group:     gwv1beta1.Group(shared_constants.GlobalAcceleratorResourcesGroup),
							Kind:      gwv1beta1.Kind(shared_constants.GlobalAcceleratorKind),
							Namespace: gwv1beta1.Namespace("accelerator-ns"),
						},
					},
					To: []gwv1beta1.ReferenceGrantTo{
						{
							Group: gwv1beta1.Group(shared_constants.CoreAPIGroup),
							Kind:  gwv1beta1.Kind("SomeOtherKind"),
						},
					},
				},
			},
			fromNamespace: "accelerator-ns",
			toGroup:       shared_constants.CoreAPIGroup,
			toKind:        shared_constants.ServiceKind,
			toName:        "my-service",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use shared constants for from params
			fromGroup := shared_constants.GlobalAcceleratorResourcesGroup
			fromKind := shared_constants.GlobalAcceleratorKind
			result := grantAllowsReference(tt.grant, tt.fromNamespace, fromGroup, fromKind, tt.toGroup, tt.toKind, tt.toName)
			assert.Equal(t, tt.expected, result)
		})
	}
}
