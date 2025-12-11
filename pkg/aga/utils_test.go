package aga

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"testing"

	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
)

func TestIsGlobalAcceleratorControllerEnabled(t *testing.T) {
	tests := []struct {
		name         string
		featureGates config.FeatureGates
		region       string
		want         bool
	}{
		{
			name: "Feature gate disabled",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Disable(config.GlobalAcceleratorController)
				return fg
			}(),
			region: "us-west-2",
			want:   false,
		},
		{
			name: "Feature gate enabled, standard region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.GlobalAcceleratorController)
				return fg
			}(),
			region: "us-west-2",
			want:   true,
		},
		{
			name: "Feature gate enabled, China region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.GlobalAcceleratorController)
				return fg
			}(),
			region: "cn-north-1",
			want:   false,
		},
		{
			name: "Feature gate enabled, GovCloud region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.GlobalAcceleratorController)
				return fg
			}(),
			region: "us-gov-west-1",
			want:   false,
		},
		{
			name: "Feature gate enabled, ISO region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.GlobalAcceleratorController)
				return fg
			}(),
			region: "us-iso-east-1",
			want:   false,
		},
		{
			name: "Feature gate enabled, ISO-E region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.GlobalAcceleratorController)
				return fg
			}(),
			region: "eu-isoe-west-1",
			want:   false,
		},
		{
			name: "Feature gate enabled, upper case region",
			featureGates: func() config.FeatureGates {
				fg := config.NewFeatureGates()
				fg.Enable(config.GlobalAcceleratorController)
				return fg
			}(),
			region: "US-WEST-2",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsGlobalAcceleratorControllerEnabled(tt.featureGates, tt.region); got != tt.want {
				t.Errorf("IsGlobalAcceleratorControllerEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanApplyAutoDiscoveryForGA(t *testing.T) {
	protocol := agaapi.GlobalAcceleratorProtocolTCP
	portRanges := []agaapi.PortRange{
		{
			FromPort: 80,
			ToPort:   80,
		},
	}

	tests := []struct {
		name            string
		ga              *agaapi.GlobalAccelerator
		loadedEndpoints []*LoadedEndpoint
		want            bool
	}{
		{
			name: "No listeners",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: nil,
				},
			},
			loadedEndpoints: []*LoadedEndpoint{},
			want:            false,
		},
		{
			name: "Empty listeners array",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{},
				},
			},
			loadedEndpoints: []*LoadedEndpoint{},
			want:            false,
		},
		{
			name: "Multiple listeners",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{}, {},
					},
				},
			},
			loadedEndpoints: []*LoadedEndpoint{},
			want:            false,
		},
		{
			name: "No endpoint groups",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							EndpointGroups: nil,
						},
					},
				},
			},
			loadedEndpoints: []*LoadedEndpoint{},
			want:            false,
		},
		{
			name: "Empty endpoint groups array",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{},
						},
					},
				},
			},
			loadedEndpoints: []*LoadedEndpoint{},
			want:            false,
		},
		{
			name: "Multiple endpoint groups",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{}, {},
							},
						},
					},
				},
			},
			loadedEndpoints: []*LoadedEndpoint{},
			want:            false,
		},
		{
			name: "No endpoints in endpoint group",
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
			loadedEndpoints: []*LoadedEndpoint{},
			want:            false,
		},
		{
			name: "Empty endpoints array",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{
									Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{},
								},
							},
						},
					},
				},
			},
			loadedEndpoints: []*LoadedEndpoint{},
			want:            false,
		},
		{
			name: "Multiple endpoints",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{
									Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
										{}, {},
									},
								},
							},
						},
					},
				},
			},
			loadedEndpoints: []*LoadedEndpoint{
				{
					Status: EndpointStatusLoaded,
				},
				{
					Status: EndpointStatusLoaded,
				},
			},
			want: false,
		},
		{
			name: "Both protocol and port ranges specified",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol:   &protocol,
							PortRanges: &portRanges,
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{
									Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
										{},
									},
								},
							},
						},
					},
				},
			},
			loadedEndpoints: []*LoadedEndpoint{
				{
					Status: EndpointStatusLoaded,
				},
			},
			want: false,
		},
		{
			name: "Failed endpoint loading",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocol,
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{
									Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
										{},
									},
								},
							},
						},
					},
				},
			},
			loadedEndpoints: []*LoadedEndpoint{
				{
					Status: EndpointStatusWarning,
					Error:  assert.AnError,
				},
			},
			want: false,
		},
		{
			name: "No loaded endpoints",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocol,
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{
									Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
										{},
									},
								},
							},
						},
					},
				},
			},
			loadedEndpoints: []*LoadedEndpoint{},
			want:            false,
		},
		{
			name: "Multiple loaded endpoints",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocol,
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{
									Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
										{}, {},
									},
								},
							},
						},
					},
				},
			},
			loadedEndpoints: []*LoadedEndpoint{
				{
					Status: EndpointStatusLoaded,
				},
				{
					Status: EndpointStatusLoaded,
				},
			},
			want: false,
		},
		{
			name: "Valid for auto-discovery - protocol specified, port ranges not specified",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocol,
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{
									Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
										{
											Type: agaapi.GlobalAcceleratorEndpointTypeService,
										},
									},
								},
							},
						},
					},
				},
			},
			loadedEndpoints: []*LoadedEndpoint{
				{
					Status:      EndpointStatusLoaded,
					Type:        agaapi.GlobalAcceleratorEndpointTypeService,
					K8sResource: &corev1.Service{}, // Add K8sResource to make the test pass
				},
			},
			want: true,
		},
		{
			name: "Valid for auto-discovery - port ranges specified, protocol not specified",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							PortRanges: &portRanges,
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{
									Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
										{
											Type: agaapi.GlobalAcceleratorEndpointTypeService,
										},
									},
								},
							},
						},
					},
				},
			},
			loadedEndpoints: []*LoadedEndpoint{
				{
					Status:      EndpointStatusLoaded,
					Type:        agaapi.GlobalAcceleratorEndpointTypeService,
					K8sResource: &corev1.Service{}, // Add K8sResource to make the test pass
				},
			},
			want: true,
		},
		{
			name: "Valid for auto-discovery - both protocol and port ranges not specified",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
								{
									Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
										{
											Type:       agaapi.GlobalAcceleratorEndpointTypeEndpointID,
											EndpointID: awssdk.String("some-arn"),
											Weight:     awssdk.Int32(112),
										},
									},
								},
							},
						},
					},
				},
			},
			loadedEndpoints: []*LoadedEndpoint{
				{
					Status: EndpointStatusLoaded,
					Type:   agaapi.GlobalAcceleratorEndpointTypeEndpointID,
					ARN:    "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-endpoint/1234567890123456", // Add ARN to make the test pass
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canApplyAutoDiscoveryForGA(tt.ga, tt.loadedEndpoints)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConsolidatePortRanges(t *testing.T) {
	tests := []struct {
		name     string
		ports    []int32
		expected []agamodel.PortRange
	}{
		{
			name:     "empty ports",
			ports:    []int32{},
			expected: nil,
		},
		{
			name:  "single port",
			ports: []int32{80},
			expected: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   80,
				},
			},
		},
		{
			name:  "consecutive ports",
			ports: []int32{80, 81, 82},
			expected: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   82,
				},
			},
		},
		{
			name:  "non-consecutive ports",
			ports: []int32{80, 443, 8080},
			expected: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   80,
				},
				{
					FromPort: 443,
					ToPort:   443,
				},
				{
					FromPort: 8080,
					ToPort:   8080,
				},
			},
		},
		{
			name:  "mixed consecutive and non-consecutive ports",
			ports: []int32{80, 81, 443, 8080, 8081, 8082},
			expected: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   81,
				},
				{
					FromPort: 443,
					ToPort:   443,
				},
				{
					FromPort: 8080,
					ToPort:   8082,
				},
			},
		},
		{
			name:  "unsorted ports",
			ports: []int32{443, 80, 8080, 81},
			expected: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   81,
				},
				{
					FromPort: 443,
					ToPort:   443,
				},
				{
					FromPort: 8080,
					ToPort:   8080,
				},
			},
		},
		{
			name:  "duplicate ports",
			ports: []int32{80, 80, 81, 443, 443},
			expected: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   81,
				},
				{
					FromPort: 443,
					ToPort:   443,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := consolidatePortRanges(tt.ports)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsPortInRanges(t *testing.T) {
	tests := []struct {
		name       string
		port       int32
		portRanges []agamodel.PortRange
		expected   bool
	}{
		{
			name: "port within single range",
			port: 85,
			portRanges: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   100,
				},
			},
			expected: true,
		},
		{
			name: "port at lower boundary",
			port: 80,
			portRanges: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   100,
				},
			},
			expected: true,
		},
		{
			name: "port at upper boundary",
			port: 100,
			portRanges: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   100,
				},
			},
			expected: true,
		},
		{
			name: "port below range",
			port: 79,
			portRanges: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   100,
				},
			},
			expected: false,
		},
		{
			name: "port above range",
			port: 101,
			portRanges: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   100,
				},
			},
			expected: false,
		},
		{
			name: "port within one of multiple ranges",
			port: 443,
			portRanges: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   100,
				},
				{
					FromPort: 443,
					ToPort:   443,
				},
				{
					FromPort: 8080,
					ToPort:   8090,
				},
			},
			expected: true,
		},
		{
			name: "port not within any of multiple ranges",
			port: 8000,
			portRanges: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   100,
				},
				{
					FromPort: 443,
					ToPort:   443,
				},
				{
					FromPort: 8080,
					ToPort:   8090,
				},
			},
			expected: false,
		},
		{
			name:       "empty port ranges",
			port:       80,
			portRanges: []agamodel.PortRange{},
			expected:   false,
		},
		{
			name:       "nil port ranges",
			port:       80,
			portRanges: nil,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPortInRanges(tt.port, tt.portRanges)
			assert.Equal(t, tt.expected, result)
		})
	}
}
