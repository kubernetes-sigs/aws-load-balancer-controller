package aga

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"

	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	mock_client "sigs.k8s.io/aws-load-balancer-controller/mocks/controller-runtime/client"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestDefaultListenerBuilder_Build(t *testing.T) {
	// Protocol references for direct pointer usage
	protocolTCP := agaapi.GlobalAcceleratorProtocolTCP
	protocolUDP := agaapi.GlobalAcceleratorProtocolUDP

	tests := []struct {
		name            string
		listeners       []agaapi.GlobalAcceleratorListener
		endpoints       []*LoadedEndpoint
		setupMocks      func(*gomock.Controller) (*mock_client.MockClient, services.ELBV2)
		wantListeners   int
		wantProtocol    agamodel.Protocol
		wantPortCount   int
		wantErr         bool
		isAutoDiscovery bool
	}{
		{
			name:          "with nil listeners",
			listeners:     nil,
			wantListeners: 0,
			wantErr:       false,
		},
		{
			name:          "with empty listeners",
			listeners:     []agaapi.GlobalAcceleratorListener{},
			wantListeners: 0,
			wantErr:       false,
		},
		{
			name: "with single TCP listener",
			listeners: []agaapi.GlobalAcceleratorListener{
				{
					Protocol: &protocolTCP,
					PortRanges: &[]agaapi.PortRange{
						{
							FromPort: 80,
							ToPort:   80,
						},
					},
					ClientAffinity: agaapi.ClientAffinityNone,
				},
			},
			wantListeners: 1,
			wantErr:       false,
		},
		{
			name: "with single UDP listener",
			listeners: []agaapi.GlobalAcceleratorListener{
				{
					Protocol: &protocolUDP,
					PortRanges: &[]agaapi.PortRange{
						{
							FromPort: 53,
							ToPort:   53,
						},
					},
					ClientAffinity: agaapi.ClientAffinitySourceIP,
				},
			},
			wantListeners: 1,
			wantErr:       false,
		},
		{
			name: "with multiple listeners",
			listeners: []agaapi.GlobalAcceleratorListener{
				{
					Protocol: &protocolTCP,
					PortRanges: &[]agaapi.PortRange{
						{
							FromPort: 80,
							ToPort:   80,
						},
					},
					ClientAffinity: agaapi.ClientAffinityNone,
				},
				{
					Protocol: &protocolUDP,
					PortRanges: &[]agaapi.PortRange{
						{
							FromPort: 53,
							ToPort:   53,
						},
					},
					ClientAffinity: agaapi.ClientAffinitySourceIP,
				},
			},
			wantListeners: 2,
			wantErr:       false,
		},
		{
			name: "with auto-discovery for Ingress endpoint with custom ports",
			listeners: []agaapi.GlobalAcceleratorListener{
				{
					// Both Protocol and PortRanges are nil for auto-discovery
					Protocol:   nil,
					PortRanges: nil,
					EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
						{
							Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
								{
									Type:      agaapi.GlobalAcceleratorEndpointTypeIngress,
									Name:      awssdk.String("test-ingress"),
									Namespace: awssdk.String("default"),
								},
							},
						},
					},
					ClientAffinity: agaapi.ClientAffinityNone,
				},
			},
			endpoints: []*LoadedEndpoint{
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeIngress,
					Name:      "test-ingress",
					Namespace: "default",
					Status:    EndpointStatusLoaded,
					ARN:       "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-ingress/1234567890123456",
					K8sResource: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-ingress",
							Namespace: "default",
						},
						Status: networking.IngressStatus{
							LoadBalancer: networking.IngressLoadBalancerStatus{
								Ingress: []networking.IngressLoadBalancerIngress{
									{
										Hostname: "test-alb.us-west-2.elb.amazonaws.com",
										Ports: []networking.IngressPortStatus{
											{Port: 8080},
											{Port: 8443},
										},
									},
								},
							},
						},
					},
				},
			},
			setupMocks: func(ctrl *gomock.Controller) (*mock_client.MockClient, services.ELBV2) {
				mockClient := mock_client.NewMockClient(ctrl)
				mockElbv2Client := services.NewMockELBV2(ctrl)
				// Configure mocks to handle IngressClassParams lookup
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockClient, mockElbv2Client
			},
			wantListeners:   1,
			wantProtocol:    agamodel.ProtocolTCP,
			wantPortCount:   2,
			wantErr:         false,
			isAutoDiscovery: true,
		},
		{
			name: "with auto-discovery for Service endpoint with TCP protocol",
			listeners: []agaapi.GlobalAcceleratorListener{
				{
					// Both Protocol and PortRanges are nil for auto-discovery
					Protocol:   nil,
					PortRanges: nil,
					EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
						{
							Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
								{
									Type:      agaapi.GlobalAcceleratorEndpointTypeService,
									Name:      awssdk.String("test-service"),
									Namespace: awssdk.String("default"),
								},
							},
						},
					},
					ClientAffinity: agaapi.ClientAffinityNone,
				},
			},
			endpoints: []*LoadedEndpoint{
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeService,
					Name:      "test-service",
					Namespace: "default",
					Status:    EndpointStatusLoaded,
					ARN:       "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/test-service/1234567890123456",
					K8sResource: &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-service",
							Namespace: "default",
						},
						Spec: corev1.ServiceSpec{
							Type: corev1.ServiceTypeLoadBalancer,
							Ports: []corev1.ServicePort{
								{
									Name:     "http",
									Protocol: corev1.ProtocolTCP,
									Port:     80,
								},
								{
									Name:     "https",
									Protocol: corev1.ProtocolTCP,
									Port:     443,
								},
							},
						},
						Status: corev1.ServiceStatus{
							LoadBalancer: corev1.LoadBalancerStatus{
								Ingress: []corev1.LoadBalancerIngress{
									{
										Hostname: "test-nlb.us-west-2.elb.amazonaws.com",
										Ports: []corev1.PortStatus{
											{
												Port:     80,
												Protocol: corev1.ProtocolTCP,
											},
											{
												Port:     443,
												Protocol: corev1.ProtocolTCP,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			setupMocks: func(ctrl *gomock.Controller) (*mock_client.MockClient, services.ELBV2) {
				mockClient := mock_client.NewMockClient(ctrl)
				mockElbv2Client := services.NewMockELBV2(ctrl)
				return mockClient, mockElbv2Client
			},
			wantListeners:   1,
			wantProtocol:    agamodel.ProtocolTCP,
			wantPortCount:   2,
			wantErr:         false,
			isAutoDiscovery: true,
		},
		{
			name: "with auto-discovery for Service endpoint with mixed TCP/UDP protocols",
			listeners: []agaapi.GlobalAcceleratorListener{
				{
					// Both Protocol and PortRanges are nil for auto-discovery
					Protocol:   nil,
					PortRanges: nil,
					EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
						{
							Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
								{
									Type:      agaapi.GlobalAcceleratorEndpointTypeService,
									Name:      awssdk.String("mixed-service"),
									Namespace: awssdk.String("default"),
								},
							},
						},
					},
					ClientAffinity: agaapi.ClientAffinityNone,
				},
			},
			endpoints: []*LoadedEndpoint{
				{
					Type:      agaapi.GlobalAcceleratorEndpointTypeService,
					Name:      "mixed-service",
					Namespace: "default",
					Status:    EndpointStatusLoaded,
					ARN:       "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/mixed-service/1234567890123456",
					K8sResource: &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "mixed-service",
							Namespace: "default",
						},
						Spec: corev1.ServiceSpec{
							Type: corev1.ServiceTypeLoadBalancer,
							Ports: []corev1.ServicePort{
								{
									Name:     "http",
									Protocol: corev1.ProtocolTCP,
									Port:     80,
								},
								{
									Name:     "dns",
									Protocol: corev1.ProtocolUDP,
									Port:     53,
								},
							},
						},
						Status: corev1.ServiceStatus{
							LoadBalancer: corev1.LoadBalancerStatus{
								Ingress: []corev1.LoadBalancerIngress{
									{
										Hostname: "mixed-service-nlb.us-west-2.elb.amazonaws.com",
										Ports: []corev1.PortStatus{
											{
												Port:     80,
												Protocol: corev1.ProtocolTCP,
											},
											{
												Port:     53,
												Protocol: corev1.ProtocolUDP,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			setupMocks: func(ctrl *gomock.Controller) (*mock_client.MockClient, services.ELBV2) {
				mockClient := mock_client.NewMockClient(ctrl)
				mockElbv2Client := services.NewMockELBV2(ctrl)
				return mockClient, mockElbv2Client
			},
			wantListeners:   2, // Should create 2 listeners (one for TCP, one for UDP)
			wantErr:         false,
			isAutoDiscovery: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test context
			ctx := context.Background()
			stack := core.NewDefaultStack(core.StackID{Namespace: "test-ns", Name: "test-name"})
			accelerator := createTestAccelerator(stack)

			// Create mock GA resource and loaded endpoints for the test
			ga := &agaapi.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ga",
					Namespace: "default",
				},
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &tt.listeners,
				},
			}

			// Use provided endpoints or an empty array
			loadedEndpoints := tt.endpoints
			if loadedEndpoints == nil {
				loadedEndpoints = []*LoadedEndpoint{}
			}

			// Create listener builder and build listeners
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			var mockClient *mock_client.MockClient
			var mockElbv2Client services.ELBV2

			if tt.setupMocks != nil {
				mockClient, mockElbv2Client = tt.setupMocks(ctrl)
			} else {
				mockClient = mock_client.NewMockClient(ctrl)
				mockElbv2Client = services.NewMockELBV2(ctrl)
			}

			logger := logr.New(&log.NullLogSink{})
			builder := NewListenerBuilder(mockClient, logger, mockElbv2Client)
			listeners, _, err := builder.Build(ctx, stack, accelerator, tt.listeners, ga, loadedEndpoints)

			// Check results
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.wantListeners == 0 {
					assert.Nil(t, listeners)
				} else {
					// Verify number of listeners
					assert.Equal(t, tt.wantListeners, len(listeners))

					if tt.isAutoDiscovery {

						if tt.wantPortCount > 0 {
							// For simple cases, verify port count on the first listener
							if len(listeners) > 0 {
								// Verify port ranges were auto-discovered correctly
								assert.Equal(t, tt.wantPortCount, len(listeners[0].Spec.PortRanges),
									"Incorrect number of port ranges")
							}
						}

						if tt.wantProtocol != "" {
							// Verify protocol was set correctly
							assert.Equal(t, tt.wantProtocol, listeners[0].Spec.Protocol,
								"Protocol was not auto-discovered correctly")
						}
					}
				}
			}
		})
	}
}

func TestDefaultListenerBuilder_buildListenerSpec(t *testing.T) {
	// Protocol references for direct pointer usage
	protocolTCP := agaapi.GlobalAcceleratorProtocolTCP
	protocolUDP := agaapi.GlobalAcceleratorProtocolUDP

	// Setup test context
	ctx := context.Background()
	stack := core.NewDefaultStack(core.StackID{Namespace: "test-ns", Name: "test-name"})
	accelerator := createTestAccelerator(stack)

	tests := []struct {
		name         string
		listener     agaapi.GlobalAcceleratorListener
		wantProtocol agamodel.Protocol
		wantAffinity agamodel.ClientAffinity
		wantPorts    []agamodel.PortRange
		wantErr      bool
	}{
		{
			name: "with TCP protocol",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol: &protocolTCP,
				PortRanges: &[]agaapi.PortRange{
					{
						FromPort: 80,
						ToPort:   80,
					},
				},
				ClientAffinity: agaapi.ClientAffinityNone,
			},
			wantProtocol: agamodel.ProtocolTCP,
			wantAffinity: agamodel.ClientAffinityNone,
			wantPorts: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   80,
				},
			},
			wantErr: false,
		},
		{
			name: "with UDP protocol",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol: &protocolUDP,
				PortRanges: &[]agaapi.PortRange{
					{
						FromPort: 53,
						ToPort:   53,
					},
				},
				ClientAffinity: agaapi.ClientAffinitySourceIP,
			},
			wantProtocol: agamodel.ProtocolUDP,
			wantAffinity: agamodel.ClientAffinitySourceIP,
			wantPorts: []agamodel.PortRange{
				{
					FromPort: 53,
					ToPort:   53,
				},
			},
			wantErr: false,
		},
		{
			name: "with nil protocol",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol: nil,
				PortRanges: &[]agaapi.PortRange{
					{
						FromPort: 80,
						ToPort:   80,
					},
				},
				ClientAffinity: agaapi.ClientAffinityNone,
			},
			wantProtocol: "",
			wantAffinity: agamodel.ClientAffinityNone,
			wantPorts: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   80,
				},
			},
			wantErr: true,
		},
		{
			name: "with nil port ranges",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol:       &protocolTCP,
				PortRanges:     nil,
				ClientAffinity: agaapi.ClientAffinityNone,
			},
			wantProtocol: agamodel.ProtocolTCP,
			wantAffinity: agamodel.ClientAffinityNone,
			wantPorts:    []agamodel.PortRange{},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Build listener spec
			spec, err := buildListenerSpec(ctx, accelerator, tt.listener)

			// Check results
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantProtocol, spec.Protocol)
				assert.Equal(t, tt.wantAffinity, spec.ClientAffinity)
				assert.Equal(t, tt.wantPorts, spec.PortRanges)
				// AcceleratorARN is a token that will be resolved later, not a direct string
				assert.NotNil(t, spec.AcceleratorARN)
			}
		})
	}
}

// Helper function to create a test accelerator
func createTestAccelerator(stack core.Stack) *agamodel.Accelerator {
	spec := agamodel.AcceleratorSpec{
		Name:    "test-accelerator",
		Enabled: awssdk.Bool(true),
		Tags:    map[string]string{"Key": "Value"},
	}

	accelerator := agamodel.NewAccelerator(stack, "test-accelerator", spec, &agaapi.GlobalAccelerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ga",
			Namespace: "default",
		},
	})

	// Set the accelerator status to simulate it being fulfilled
	accelerator.SetStatus(agamodel.AcceleratorStatus{
		AcceleratorARN: "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh",
		DNSName:        "a1234abcd5678efghi.awsglobalaccelerator.com",
		Status:         "DEPLOYED",
	})

	return accelerator
}

func TestBuildListenerProtocol(t *testing.T) {
	// Protocol references for direct pointer usage
	protocolTCP := agaapi.GlobalAcceleratorProtocolTCP
	protocolUDP := agaapi.GlobalAcceleratorProtocolUDP
	invalidProtocol := agaapi.GlobalAcceleratorProtocol("INVALID")

	tests := []struct {
		name          string
		listener      agaapi.GlobalAcceleratorListener
		wantProtocol  agamodel.Protocol
		wantErr       bool
		wantErrString string
	}{
		{
			name: "with nil protocol",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol: nil,
			},
			wantProtocol: "",
			wantErr:      true,
		},
		{
			name: "with TCP protocol",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol: &protocolTCP,
			},
			wantProtocol: agamodel.ProtocolTCP,
			wantErr:      false,
		},
		{
			name: "with UDP protocol",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol: &protocolUDP,
			},
			wantProtocol: agamodel.ProtocolUDP,
			wantErr:      false,
		},
		{
			name: "with invalid protocol",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol: &invalidProtocol,
			},
			wantProtocol:  "",
			wantErr:       true,
			wantErrString: "unsupported protocol: INVALID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test context
			ctx := context.Background()

			// Call function
			protocol, err := buildListenerProtocol(ctx, tt.listener)

			// Check results
			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrString != "" {
					assert.Contains(t, err.Error(), tt.wantErrString)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantProtocol, protocol)
			}
		})
	}
}

func TestBuildListenerPortRanges(t *testing.T) {
	tests := []struct {
		name      string
		listener  agaapi.GlobalAcceleratorListener
		wantPorts []agamodel.PortRange
		wantErr   bool
	}{
		{
			name: "with nil port ranges",
			listener: agaapi.GlobalAcceleratorListener{
				PortRanges: nil,
			},
			wantPorts: []agamodel.PortRange{},
			wantErr:   true,
		},
		{
			name: "with single port range",
			listener: agaapi.GlobalAcceleratorListener{
				PortRanges: &[]agaapi.PortRange{
					{
						FromPort: 443,
						ToPort:   443,
					},
				},
			},
			wantPorts: []agamodel.PortRange{
				{
					FromPort: 443,
					ToPort:   443,
				},
			},
			wantErr: false,
		},
		{
			name: "with multiple port ranges",
			listener: agaapi.GlobalAcceleratorListener{
				PortRanges: &[]agaapi.PortRange{
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
						ToPort:   8090,
					},
				},
			},
			wantPorts: []agamodel.PortRange{
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
					ToPort:   8090,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test context
			ctx := context.Background()

			// Call function
			portRanges, err := buildListenerPortRanges(ctx, tt.listener)

			// Check results
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPorts, portRanges)
			}
		})
	}
}

func TestBuildListenerClientAffinity(t *testing.T) {
	tests := []struct {
		name         string
		listener     agaapi.GlobalAcceleratorListener
		wantAffinity agamodel.ClientAffinity
	}{
		{
			name: "with NONE client affinity",
			listener: agaapi.GlobalAcceleratorListener{
				ClientAffinity: agaapi.ClientAffinityNone,
			},
			wantAffinity: agamodel.ClientAffinityNone,
		},
		{
			name: "with SOURCE_IP client affinity",
			listener: agaapi.GlobalAcceleratorListener{
				ClientAffinity: agaapi.ClientAffinitySourceIP,
			},
			wantAffinity: agamodel.ClientAffinitySourceIP,
		},
		{
			name: "with invalid client affinity (should default to NONE)",
			listener: agaapi.GlobalAcceleratorListener{
				ClientAffinity: "INVALID",
			},
			wantAffinity: agamodel.ClientAffinityNone,
		},
		{
			name: "with empty client affinity (should default to NONE)",
			listener: agaapi.GlobalAcceleratorListener{
				ClientAffinity: "",
			},
			wantAffinity: agamodel.ClientAffinityNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test context
			ctx := context.Background()

			// Call function
			clientAffinity := buildListenerClientAffinity(ctx, tt.listener)

			// Check results
			assert.Equal(t, tt.wantAffinity, clientAffinity)
		})
	}
}

// TestAutomaticEndpointDiscovery tests the auto-discovery feature for listeners
func TestAutomaticEndpointDiscovery(t *testing.T) {
	// Setup test context
	ctx := context.Background()
	stack := core.NewDefaultStack(core.StackID{Namespace: "test-ns", Name: "test-name"})
	accelerator := createTestAccelerator(stack)

	// Create a mock global accelerator resource
	ga := &agaapi.GlobalAccelerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auto-discovery-ga",
			Namespace: "default",
		},
		Spec: agaapi.GlobalAcceleratorSpec{
			Listeners: &[]agaapi.GlobalAcceleratorListener{
				{
					// Both Protocol and PortRanges are nil
					// They should be auto-discovered from the endpoint
					Protocol:   nil,
					PortRanges: nil,
					EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
						{
							Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
								{
									Type:      agaapi.GlobalAcceleratorEndpointTypeIngress,
									Name:      awssdk.String("test-ingress"),
									Namespace: awssdk.String("default"),
								},
							},
						},
					},
					ClientAffinity: agaapi.ClientAffinityNone,
				},
			},
		},
	}

	// Create a LoadedEndpoint with mock K8s resource for auto-discovery test
	endpoint := &LoadedEndpoint{
		Type:      agaapi.GlobalAcceleratorEndpointTypeIngress,
		Name:      "test-ingress",
		Namespace: "default",
		Status:    EndpointStatusLoaded,
		ARN:       "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-ingress/1234567890123456",
		K8sResource: &networking.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ingress",
				Namespace: "default",
			},
			Status: networking.IngressStatus{
				LoadBalancer: networking.IngressLoadBalancerStatus{
					Ingress: []networking.IngressLoadBalancerIngress{
						{
							Hostname: "test-alb.us-west-2.elb.amazonaws.com",
							Ports: []networking.IngressPortStatus{
								{Port: 8080},
								{Port: 8443},
							},
						},
					},
				},
			},
		},
	}

	loadedEndpoints := []*LoadedEndpoint{endpoint}

	// Verify auto-discovery is applicable
	canApply := canApplyAutoDiscoveryForGA(ga, loadedEndpoints)
	assert.True(t, canApply)

	// Create listener builder and build the listener
	ctrl := gomock.NewController(t)
	mockClient := mock_client.NewMockClient(ctrl)
	mockElbv2Client := services.NewMockELBV2(ctrl)
	logger := zap.New()

	// Configure mocks to handle IngressClassParams lookup
	mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	builder := NewListenerBuilder(mockClient, logger, mockElbv2Client)
	listeners, _, err := builder.Build(ctx, stack, accelerator, *ga.Spec.Listeners, ga, loadedEndpoints)

	// Verify build was successful
	assert.NoError(t, err)
	assert.Equal(t, 1, len(listeners))

	// Verify auto-discovered protocol and port ranges
	assert.Equal(t, agamodel.ProtocolTCP, listeners[0].Spec.Protocol)
	assert.Equal(t, 2, len(listeners[0].Spec.PortRanges))

	// Sort port ranges for consistent test results
	portRanges := listeners[0].Spec.PortRanges
	portMap := make(map[int32]bool)
	for _, pr := range portRanges {
		portMap[pr.FromPort] = true
	}

	// Verify the expected ports were discovered from the ingress annotations
	assert.True(t, portMap[8080])
	assert.True(t, portMap[8443])
}

func TestCreateNewListener(t *testing.T) {
	// Protocol references
	protocolTCP := agaapi.GlobalAcceleratorProtocolTCP
	protocolUDP := agaapi.GlobalAcceleratorProtocolUDP

	// Port ranges
	tcpPortRanges := []agamodel.PortRange{
		{
			FromPort: 80,
			ToPort:   80,
		},
		{
			FromPort: 443,
			ToPort:   443,
		},
	}

	emptyPortRanges := []agamodel.PortRange{}

	// Create template listener with full data
	templateListener := agaapi.GlobalAcceleratorListener{
		Protocol: &protocolTCP,
		PortRanges: &[]agaapi.PortRange{
			{
				FromPort: 8080,
				ToPort:   8080,
			},
		},
		ClientAffinity: agaapi.ClientAffinitySourceIP,
		EndpointGroups: &[]agaapi.GlobalAcceleratorEndpointGroup{
			{
				Region:                awssdk.String("us-west-2"),
				TrafficDialPercentage: awssdk.Int32(100),
				Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
					{
						Type:      agaapi.GlobalAcceleratorEndpointTypeService,
						Name:      awssdk.String("test-service"),
						Namespace: awssdk.String("default"),
						Weight:    awssdk.Int32(100),
					},
				},
				PortOverrides: &[]agaapi.PortOverride{
					{
						ListenerPort: 8080,
						EndpointPort: 80,
					},
				},
			},
		},
	}

	// Test cases for createNewListener function
	testCases := []struct {
		name           string
		template       agaapi.GlobalAcceleratorListener
		protocol       agaapi.GlobalAcceleratorProtocol
		portRanges     []agamodel.PortRange
		expectProtocol agaapi.GlobalAcceleratorProtocol
		expectPorts    []agaapi.PortRange
		expectAffinity agaapi.ClientAffinityType
	}{
		{
			name: "Create TCP listener with discovered ports",
			template: agaapi.GlobalAcceleratorListener{
				ClientAffinity: agaapi.ClientAffinitySourceIP,
				// No protocol, port ranges, or endpoint groups
			},
			protocol:       protocolTCP,
			portRanges:     tcpPortRanges,
			expectProtocol: protocolTCP,
			expectPorts: []agaapi.PortRange{
				{
					FromPort: 80,
					ToPort:   80,
				},
				{
					FromPort: 443,
					ToPort:   443,
				},
			},
			expectAffinity: agaapi.ClientAffinitySourceIP,
		},
		{
			name: "Create UDP listener with discovered ports",
			template: agaapi.GlobalAcceleratorListener{
				ClientAffinity: agaapi.ClientAffinitySourceIP,
				// No protocol, port ranges, or endpoint groups
			},
			protocol:       protocolUDP,
			portRanges:     tcpPortRanges, // Reuse same ports for UDP
			expectProtocol: protocolUDP,
			expectPorts: []agaapi.PortRange{
				{
					FromPort: 80,
					ToPort:   80,
				},
				{
					FromPort: 443,
					ToPort:   443,
				},
			},
			expectAffinity: agaapi.ClientAffinitySourceIP,
		},
		{
			name:           "Create listener with empty port ranges",
			template:       templateListener,
			protocol:       protocolTCP,
			portRanges:     emptyPortRanges,
			expectProtocol: protocolTCP,
			expectPorts: []agaapi.PortRange{
				{
					FromPort: 8080,
					ToPort:   8080,
				},
			}, // Uses template ports
			expectAffinity: agaapi.ClientAffinitySourceIP,
		},
		{
			name: "Create listener from minimal template",
			template: agaapi.GlobalAcceleratorListener{
				ClientAffinity: agaapi.ClientAffinityNone,
				// No protocol, port ranges, or endpoint groups
			},
			protocol:       protocolTCP,
			portRanges:     tcpPortRanges,
			expectProtocol: protocolTCP,
			expectPorts: []agaapi.PortRange{
				{
					FromPort: 80,
					ToPort:   80,
				},
				{
					FromPort: 443,
					ToPort:   443,
				},
			},
			expectAffinity: agaapi.ClientAffinityNone,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function under test
			result := createNewListener(tc.template, tc.protocol, tc.portRanges)

			// Verify protocol was set correctly
			assert.NotNil(t, result.Protocol)
			assert.Equal(t, tc.expectProtocol, *result.Protocol)

			// Verify client affinity was copied
			assert.Equal(t, tc.expectAffinity, result.ClientAffinity)

			// Verify port ranges
			if tc.expectPorts != nil {
				assert.NotNil(t, result.PortRanges)
				assert.Equal(t, len(tc.expectPorts), len(*result.PortRanges))

				// Check each port range
				for i, expectedPort := range tc.expectPorts {
					found := false
					for _, actualPort := range *result.PortRanges {
						if actualPort.FromPort == expectedPort.FromPort && actualPort.ToPort == expectedPort.ToPort {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected port range %d-%d not found", tc.expectPorts[i].FromPort, tc.expectPorts[i].ToPort)
				}
			}

			// Verify endpoint groups were copied
			if tc.template.EndpointGroups != nil {
				assert.NotNil(t, result.EndpointGroups)
				assert.Equal(t, len(*tc.template.EndpointGroups), len(*result.EndpointGroups))

				// For first endpoint group, check if fields were properly copied
				if len(*tc.template.EndpointGroups) > 0 && len(*result.EndpointGroups) > 0 {
					templateEG := (*tc.template.EndpointGroups)[0]
					resultEG := (*result.EndpointGroups)[0]

					// Check region
					if templateEG.Region != nil {
						assert.NotNil(t, resultEG.Region)
						assert.Equal(t, *templateEG.Region, *resultEG.Region)
					}

					// Check traffic dial percentage
					if templateEG.TrafficDialPercentage != nil {
						assert.NotNil(t, resultEG.TrafficDialPercentage)
						assert.Equal(t, *templateEG.TrafficDialPercentage, *resultEG.TrafficDialPercentage)
					}

					// Check endpoints
					if templateEG.Endpoints != nil && len(*templateEG.Endpoints) > 0 {
						assert.NotNil(t, resultEG.Endpoints)
						assert.Equal(t, len(*templateEG.Endpoints), len(*resultEG.Endpoints))

						// Check first endpoint
						if len(*templateEG.Endpoints) > 0 && len(*resultEG.Endpoints) > 0 {
							templateEndpoint := (*templateEG.Endpoints)[0]
							resultEndpoint := (*resultEG.Endpoints)[0]

							assert.Equal(t, templateEndpoint.Type, resultEndpoint.Type)

							if templateEndpoint.Name != nil {
								assert.NotNil(t, resultEndpoint.Name)
								assert.Equal(t, *templateEndpoint.Name, *resultEndpoint.Name)
							}

							if templateEndpoint.Namespace != nil {
								assert.NotNil(t, resultEndpoint.Namespace)
								assert.Equal(t, *templateEndpoint.Namespace, *resultEndpoint.Namespace)
							}

							assert.Equal(t, templateEndpoint.Weight, resultEndpoint.Weight)
						}
					}

					// Check port overrides
					if templateEG.PortOverrides != nil {
						assert.NotNil(t, resultEG.PortOverrides)
						assert.Equal(t, len(*templateEG.PortOverrides), len(*resultEG.PortOverrides))
					}
				}
			}
		})
	}
}
