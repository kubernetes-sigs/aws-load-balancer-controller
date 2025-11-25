package aga

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"testing"

	"github.com/stretchr/testify/assert"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
)

func TestGetAllEndpointsFromGA(t *testing.T) {

	tests := []struct {
		name     string
		ga       *agaapi.GlobalAccelerator
		expected []EndpointReference
	}{
		{
			name:     "Empty GA",
			ga:       &agaapi.GlobalAccelerator{},
			expected: nil,
		},
		{
			name: "GA with no listeners",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: nil,
				},
			},
			expected: nil,
		},
		{
			name: "GA with listeners but no endpoint groups",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							EndpointGroups: nil,
						},
					},
				},
			},
			expected: nil,
		},
		{
			name: "GA with endpoint groups but no endpoints",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{
									Endpoints: nil,
								},
							},
						},
					},
				},
			},
			expected: nil,
		},
		{
			name: "GA with service endpoint",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{
									Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
										{
											Type: agaapi.GlobalAcceleratorEndpointTypeService,
											Name: awssdk.String("test-service"),
										},
									},
								},
							},
						},
					},
				},
			},
			expected: []EndpointReference{
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeService,
					Name:      "test-service",
					Namespace: "",
				},
			},
		},
		{
			name: "GA with EndpointID type endpoint",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{
									Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
										{
											Type:       agaapi.GlobalAcceleratorEndpointTypeEndpointID,
											EndpointID: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-service/1234567890"),
										},
									},
								},
							},
						},
					},
				},
			},
			expected: []EndpointReference{
				{
					Type:       agaapi.GlobalAcceleratorEndpointTypeEndpointID,
					Name:       "",
					Namespace:  "",
					EndpointID: "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-service/1234567890",
				},
			},
		},
		{
			name: "GA with multiple types of endpoints",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{
									Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
										{
											Type: agaapi.GlobalAcceleratorEndpointTypeService,
											Name: awssdk.String("test-service"),
										},
										{
											Type: agaapi.GlobalAcceleratorEndpointTypeIngress,
											Name: awssdk.String("test-ingress"),
										},
										{
											Type:      agaapi.GlobalAcceleratorEndpointTypeGateway,
											Name:      awssdk.String("test-gateway"),
											Namespace: awssdk.String("custom-namespace"),
										},
									},
								},
							},
						},
					},
				},
			},
			expected: []EndpointReference{
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeService,
					Name:      "test-service",
					Namespace: "",
				},
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeIngress,
					Name:      "test-ingress",
					Namespace: "",
				},
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeGateway,
					Name:      "test-gateway",
					Namespace: "custom-namespace",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set namespace for GA
			if tt.ga != nil {
				tt.ga.Namespace = "default"

				// Update expected namespaces if they're empty (but only for non-EndpointID types)
				for i := range tt.expected {
					// Only apply default namespace for Service/Ingress/Gateway types
					if tt.expected[i].Namespace == "" && tt.expected[i].Type != agaapi.GlobalAcceleratorEndpointTypeEndpointID {
						tt.expected[i].Namespace = tt.ga.Namespace
					}
				}
			}

			result := GetAllEndpointsFromGA(tt.ga)

			// Compare lengths
			assert.Equal(t, len(tt.expected), len(result))

			// Compare contents
			if tt.expected != nil {
				for i, exp := range tt.expected {
					assert.Equal(t, exp.Type, result[i].Type)
					assert.Equal(t, exp.Name, result[i].Name)
					assert.Equal(t, exp.Namespace, result[i].Namespace)
				}
			}
		})
	}
}

func TestEndpointReferenceToResourceKey(t *testing.T) {
	// Test Service type endpoint
	t.Run("Service type endpoint", func(t *testing.T) {
		endpoint := EndpointReference{
			Type:      agaapi.GlobalAcceleratorEndpointTypeService,
			Name:      "test-service",
			Namespace: "test-namespace",
		}

		resourceKey := endpoint.ToResourceKey()
		assert.Equal(t, ResourceType(endpoint.Type), resourceKey.Type)
		assert.Equal(t, endpoint.Name, resourceKey.Name.Name)
		assert.Equal(t, endpoint.Namespace, resourceKey.Name.Namespace)
	})

	// Test EndpointID type endpoint
	t.Run("EndpointID type endpoint", func(t *testing.T) {
		endpoint := EndpointReference{
			Type:       agaapi.GlobalAcceleratorEndpointTypeEndpointID,
			EndpointID: "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-service/1234567890",
		}

		resourceKey := endpoint.ToResourceKey()
		assert.Equal(t, ResourceType(endpoint.Type), resourceKey.Type)
		assert.Equal(t, endpoint.EndpointID, resourceKey.Name.Name)
		assert.Equal(t, "", resourceKey.Name.Namespace) // Namespace should be empty for EndpointID type
	})
}
