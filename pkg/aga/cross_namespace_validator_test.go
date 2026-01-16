package aga

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestGrantAllowsReference(t *testing.T) {
	validator := &ReferenceGrantValidator{}

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
			result := validator.grantAllowsReference(tt.grant, tt.fromNamespace, tt.toGroup, tt.toKind, tt.toName)
			assert.Equal(t, tt.expected, result)
		})
	}
}
