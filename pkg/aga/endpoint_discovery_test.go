package aga

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	mock_client "sigs.k8s.io/aws-load-balancer-controller/mocks/controller-runtime/client"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestFetchEndpointProtocolPortInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock_client.NewMockClient(ctrl)
	ctx := context.TODO()

	gatewayClassName := gwv1.ObjectName("test-class")
	httpType := gwv1.HTTPProtocolType
	httpsType := gwv1.HTTPSProtocolType
	udpType := gwv1.UDPProtocolType

	// Test Service endpoint with ports in status
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Protocol: corev1.ProtocolTCP, // This protocol should NOT be used
					Port:     9999,               // This port should NOT be used
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
	}

	t.Run("Service endpoint with TCP ports in status", func(t *testing.T) {
		serviceEndpoint := &LoadedEndpoint{
			Type:        agaapi.GlobalAcceleratorEndpointTypeService,
			Name:        "test-service",
			Namespace:   "default",
			Status:      EndpointStatusLoaded,
			K8sResource: svc,
			ARN:         "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/test-nlb/1234567890123456",
		}

		mockElbv2Client := services.NewMockELBV2(ctrl)
		logger := zap.New()
		discovery := NewEndpointDiscovery(mockClient, logger, mockElbv2Client)
		protocolPortsInfo, err := discovery.FetchProtocolPortInfo(ctx, serviceEndpoint)
		assert.NoError(t, err)

		assert.Len(t, protocolPortsInfo, 1, "Should have one protocol group (TCP)")
		assert.Equal(t, agaapi.GlobalAcceleratorProtocolTCP, protocolPortsInfo[0].Protocol, "Protocol should be TCP")

		// Verify both ports are present in the TCP group
		portsFound := make(map[int32]bool)
		for _, port := range protocolPortsInfo[0].Ports {
			portsFound[port] = true
		}
		assert.True(t, portsFound[80], "Port 80 should be present")
		assert.True(t, portsFound[443], "Port 443 should be present")
	})

	// Test Service with multi-protocol ports in status
	svcMultiProto := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service-multi-proto",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{
					Port: 9999, // Should be ignored as we're using status ports
				},
			},
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{
						Hostname: "test-nlb-multi.us-west-2.elb.amazonaws.com",
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
	}

	t.Run("Service with multi-protocol ports in status", func(t *testing.T) {
		serviceMultiProtoEndpoint := &LoadedEndpoint{
			Type:        agaapi.GlobalAcceleratorEndpointTypeService,
			Name:        "test-service-multi-proto",
			Namespace:   "default",
			Status:      EndpointStatusLoaded,
			K8sResource: svcMultiProto,
			ARN:         "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/test-nlb-multi/1234567890123456",
		}

		mockElbv2Client := services.NewMockELBV2(ctrl)
		logger := zap.New()
		discovery := NewEndpointDiscovery(mockClient, logger, mockElbv2Client)
		protocolPortsInfo, err := discovery.FetchProtocolPortInfo(ctx, serviceMultiProtoEndpoint)
		assert.NoError(t, err)
		assert.Len(t, protocolPortsInfo, 2, "Should have two protocol groups (TCP and UDP)")

		// We expect one group for TCP and one group for UDP
		tcpPorts := []int32{}
		udpPorts := []int32{}

		// Extract the ports by protocol
		for _, info := range protocolPortsInfo {
			if info.Protocol == agaapi.GlobalAcceleratorProtocolTCP {
				tcpPorts = append(tcpPorts, info.Ports...)
			} else if info.Protocol == agaapi.GlobalAcceleratorProtocolUDP {
				udpPorts = append(udpPorts, info.Ports...)
			}
		}

		// Verify TCP port
		assert.Len(t, tcpPorts, 1, "Should have one TCP port")
		assert.Contains(t, tcpPorts, int32(80), "Port 80 should be TCP")

		// Verify UDP port
		assert.Len(t, udpPorts, 1, "Should have one UDP port")
		assert.Contains(t, udpPorts, int32(53), "Port 53 should be UDP")
	})

	// Test Ingress endpoint
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
		},
		Status: networkingv1.IngressStatus{
			LoadBalancer: networkingv1.IngressLoadBalancerStatus{
				Ingress: []networkingv1.IngressLoadBalancerIngress{
					{
						Hostname: "test-alb.us-west-2.elb.amazonaws.com",
						Ports: []networkingv1.IngressPortStatus{
							{Port: 80},
							{Port: 443},
						},
					},
					{
						Hostname: "test-nlb.amazonaws.com", // Non-ALB entry
					},
				},
			},
		},
	}

	t.Run("Ingress endpoint with ALB DNS and ports in status", func(t *testing.T) {
		ingressEndpoint := &LoadedEndpoint{
			Type:        agaapi.GlobalAcceleratorEndpointTypeIngress,
			Name:        "test-ingress",
			Namespace:   "default",
			Status:      EndpointStatusLoaded,
			K8sResource: ing,
			ARN:         "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-alb/1234567890123456",
		}

		mockElbv2Client := services.NewMockELBV2(ctrl)
		logger := zap.New()
		discovery := NewEndpointDiscovery(mockClient, logger, mockElbv2Client)
		protocolPortsInfo, err := discovery.FetchProtocolPortInfo(ctx, ingressEndpoint)
		assert.NoError(t, err)
		assert.Len(t, protocolPortsInfo, 1) // TCP protocol group
		assert.Equal(t, agaapi.GlobalAcceleratorProtocolTCP, protocolPortsInfo[0].Protocol)
		assert.Len(t, protocolPortsInfo[0].Ports, 2, "Should have two ports in TCP group")

		// Check if both ports are present
		ports := protocolPortsInfo[0].Ports
		assert.Contains(t, ports, int32(80), "Port 80 should be in ports")
		assert.Contains(t, ports, int32(443), "Port 443 should be in ports")
	})

	// Test Gateway endpoint
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gateway",
			Namespace: "default",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: gatewayClassName,
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Port:     80,
					Protocol: httpType,
				},
				{
					Name:     "https",
					Port:     443,
					Protocol: httpsType,
				},
				{
					Name:     "udp",
					Port:     1433,
					Protocol: udpType,
				},
			},
		},
	}

	t.Run("Gateway endpoint with mixed protocols", func(t *testing.T) {
		gatewayEndpoint := &LoadedEndpoint{
			Type:        agaapi.GlobalAcceleratorEndpointTypeGateway,
			Name:        "test-gateway",
			Namespace:   "default",
			Status:      EndpointStatusLoaded,
			K8sResource: gw,
			ARN:         "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-gateway-alb/1234567890123456",
		}

		mockElbv2Client := services.NewMockELBV2(ctrl)
		logger := zap.New()
		discovery := NewEndpointDiscovery(mockClient, logger, mockElbv2Client)
		protocolPortsInfo, err := discovery.FetchProtocolPortInfo(ctx, gatewayEndpoint)
		assert.NoError(t, err)
		assert.Len(t, protocolPortsInfo, 2, "Should have two protocol groups (TCP and UDP)")

		// We expect one group for TCP and one group for UDP
		tcpPorts := []int32{}
		udpPorts := []int32{}

		// Extract the ports by protocol
		for _, info := range protocolPortsInfo {
			if info.Protocol == agaapi.GlobalAcceleratorProtocolTCP {
				tcpPorts = append(tcpPorts, info.Ports...)
			} else if info.Protocol == agaapi.GlobalAcceleratorProtocolUDP {
				udpPorts = append(udpPorts, info.Ports...)
			}
		}

		// Verify TCP ports
		assert.Len(t, tcpPorts, 2, "Should have two TCP ports")
		assert.Contains(t, tcpPorts, int32(80), "Port 80 should be in TCP group")
		assert.Contains(t, tcpPorts, int32(443), "Port 443 should be in TCP group")

		// Verify UDP port
		assert.Len(t, udpPorts, 1, "Should have one UDP port")
		assert.Contains(t, udpPorts, int32(1433), "Port 1433 should be in UDP group")
	})
}

func TestFetchIngressProtocolPortInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()

	testCases := []struct {
		name              string
		ingressStatus     []networkingv1.IngressLoadBalancerIngress
		expectedPortCount int
		expectedPorts     map[int32]bool
		expectError       bool
		errorSubstring    string
	}{
		{
			name: "Ingress with ALB entry and ports in status",
			ingressStatus: []networkingv1.IngressLoadBalancerIngress{
				{
					Hostname: "test-alb.us-west-2.elb.amazonaws.com",
					Ports: []networkingv1.IngressPortStatus{
						{Port: 80},
						{Port: 443},
					},
				},
			},
			expectedPortCount: 2,
			expectedPorts:     map[int32]bool{80: true, 443: true},
			expectError:       false,
		},
		{
			name: "Ingress with ALB entry but no ports in status",
			ingressStatus: []networkingv1.IngressLoadBalancerIngress{
				{
					Hostname: "test-alb.us-west-2.elb.amazonaws.com",
					// No ports
				},
			},
			expectedPortCount: 0,
			expectedPorts:     map[int32]bool{},
			expectError:       true,
			errorSubstring:    "no valid ports found",
		},
		{
			name: "Ingress with ALB and NLB entries in status (should use ALB ports)",
			ingressStatus: []networkingv1.IngressLoadBalancerIngress{
				{
					Hostname: "test-alb.us-west-2.elb.amazonaws.com",
					Ports: []networkingv1.IngressPortStatus{
						{Port: 80},
						{Port: 443},
					},
				},
				{
					Hostname: "test-nlb.amazonaws.com", // NLB entry, should be ignored for port discovery
				},
			},
			expectedPortCount: 2,
			expectedPorts:     map[int32]bool{80: true, 443: true},
			expectError:       false,
		},
		{
			name: "Ingress with NLB entry first and ALB entry second in status",
			ingressStatus: []networkingv1.IngressLoadBalancerIngress{
				{
					Hostname: "test-nlb.amazonaws.com", // NLB entry, should be ignored for port discovery
				},
				{
					Hostname: "test-alb.us-west-2.elb.amazonaws.com",
					Ports: []networkingv1.IngressPortStatus{
						{Port: 443},
						{Port: 8443},
					},
				},
			},
			expectedPortCount: 2,
			expectedPorts:     map[int32]bool{443: true, 8443: true},
			expectError:       false,
		},
		{
			name: "Ingress with no ALB entry in status",
			ingressStatus: []networkingv1.IngressLoadBalancerIngress{
				{
					Hostname: "test-nlb.amazonaws.com", // Not an ALB entry
				},
			},
			expectedPortCount: 0,
			expectedPorts:     map[int32]bool{},
			expectError:       true,
			errorSubstring:    "no valid ports found",
		},
		{
			name:              "Ingress with empty status",
			ingressStatus:     []networkingv1.IngressLoadBalancerIngress{},
			expectedPortCount: 0,
			expectedPorts:     map[int32]bool{},
			expectError:       true,
			errorSubstring:    "no valid ports found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create Ingress resource with test case status
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress-" + tc.name,
					Namespace: "default",
				},
				Status: networkingv1.IngressStatus{
					LoadBalancer: networkingv1.IngressLoadBalancerStatus{
						Ingress: tc.ingressStatus,
					},
				},
			}

			// Create loaded endpoint with the ingress resource
			ingressEndpoint := &LoadedEndpoint{
				Type:        agaapi.GlobalAcceleratorEndpointTypeIngress,
				Name:        "test-ingress-" + tc.name,
				Namespace:   "default",
				Status:      EndpointStatusLoaded,
				K8sResource: ingress,
			}

			// Create mocks
			mockClient := mock_client.NewMockClient(ctrl)
			mockElbv2Client := services.NewMockELBV2(ctrl)
			logger := zap.New()

			// Call the function under test
			discovery := NewEndpointDiscovery(mockClient, logger, mockElbv2Client)
			protocolPortsInfo, err := discovery.fetchIngressProtocolPortInfo(ctx, ingressEndpoint)

			// Check error expectations
			if tc.expectError {
				assert.Error(t, err)
				if tc.errorSubstring != "" {
					assert.Contains(t, err.Error(), tc.errorSubstring)
				}
			} else {
				assert.NoError(t, err)
				assert.Len(t, protocolPortsInfo, 1, "Should have one protocol group (TCP)")
				assert.Equal(t, agaapi.GlobalAcceleratorProtocolTCP, protocolPortsInfo[0].Protocol, "Protocol should be TCP")

				// Check if all expected ports are in the TCP port group
				portsFound := make(map[int32]bool)
				for _, port := range protocolPortsInfo[0].Ports {
					portsFound[port] = true
				}

				// Verify expected ports are found
				for port := range tc.expectedPorts {
					assert.True(t, portsFound[port], "Port %d not found", port)
				}

				// Verify port count matches expected
				assert.Equal(t, tc.expectedPortCount, len(protocolPortsInfo[0].Ports), "Port count should match expected")
			}
		})
	}
}

func TestFetchServiceProtocolPortInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()

	testCases := []struct {
		name               string
		serviceStatusPorts []corev1.PortStatus
		expectedTCPPorts   []int32
		expectedUDPPorts   []int32
		expectError        bool
		errorSubstring     string
	}{
		{
			name: "Service ports from status (TCP only)",
			serviceStatusPorts: []corev1.PortStatus{
				{
					Port:     80,
					Protocol: corev1.ProtocolTCP,
				},
				{
					Port:     443,
					Protocol: corev1.ProtocolTCP,
				},
			},
			expectedTCPPorts: []int32{80, 443},
			expectedUDPPorts: []int32{},
			expectError:      false,
		},
		{
			name: "Service ports from status (TCP + UDP)",
			serviceStatusPorts: []corev1.PortStatus{
				{
					Port:     80,
					Protocol: corev1.ProtocolTCP,
				},
				{
					Port:     53,
					Protocol: corev1.ProtocolUDP,
				},
			},
			expectedTCPPorts: []int32{80},
			expectedUDPPorts: []int32{53},
			expectError:      false,
		},
		{
			name: "Error for TCP_UDP service (same port with different protocols)",
			serviceStatusPorts: []corev1.PortStatus{
				{
					Port:     53,
					Protocol: corev1.ProtocolTCP,
				},
				{
					Port:     53,
					Protocol: corev1.ProtocolUDP,
				},
			},
			expectedTCPPorts: []int32{},
			expectedUDPPorts: []int32{},
			expectError:      true,
			errorSubstring:   "auto-discovery does not support TCP_UDP services on the same port 53",
		},
		{
			name:               "Error when status has no ports",
			serviceStatusPorts: []corev1.PortStatus{},
			expectedTCPPorts:   []int32{},
			expectedUDPPorts:   []int32{},
			expectError:        true,
			errorSubstring:     "no port information available",
		},
		{
			name:               "Error when no status entry",
			serviceStatusPorts: nil,
			expectedTCPPorts:   []int32{},
			expectedUDPPorts:   []int32{},
			expectError:        true,
			errorSubstring:     "no port information available",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create Service resource with test case ports
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service-" + tc.name,
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
				},
			}

			// Add status ports if provided
			if tc.serviceStatusPorts != nil {
				svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
					{
						Hostname: "test-nlb.us-west-2.elb.amazonaws.com",
						Ports:    tc.serviceStatusPorts,
					},
				}
			}

			// Create loaded endpoint with the service resource
			serviceEndpoint := &LoadedEndpoint{
				Type:        agaapi.GlobalAcceleratorEndpointTypeService,
				Name:        "test-service-" + tc.name,
				Namespace:   "default",
				Status:      EndpointStatusLoaded,
				K8sResource: svc,
			}

			// Create mocks
			mockClient := mock_client.NewMockClient(ctrl)
			mockElbv2Client := services.NewMockELBV2(ctrl)
			logger := zap.New()

			// Call the function under test
			discovery := NewEndpointDiscovery(mockClient, logger, mockElbv2Client)
			protocolPortsInfo, err := discovery.fetchServiceProtocolPortInfo(ctx, serviceEndpoint)

			// Check error expectations
			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, protocolPortsInfo)
			} else {
				assert.NoError(t, err)

				// Extract ports by protocol
				tcpPorts := []int32{}
				udpPorts := []int32{}

				for _, info := range protocolPortsInfo {
					if info.Protocol == agaapi.GlobalAcceleratorProtocolTCP {
						tcpPorts = append(tcpPorts, info.Ports...)
					} else if info.Protocol == agaapi.GlobalAcceleratorProtocolUDP {
						udpPorts = append(udpPorts, info.Ports...)
					}
				}

				// Verify TCP ports
				assert.Equal(t, len(tc.expectedTCPPorts), len(tcpPorts), "TCP port count should match expected")
				for _, expectedPort := range tc.expectedTCPPorts {
					assert.Contains(t, tcpPorts, expectedPort, "Expected TCP port %d not found", expectedPort)
				}

				// Verify UDP ports
				assert.Equal(t, len(tc.expectedUDPPorts), len(udpPorts), "UDP port count should match expected")
				for _, expectedPort := range tc.expectedUDPPorts {
					assert.Contains(t, udpPorts, expectedPort, "Expected UDP port %d not found", expectedPort)
				}

				// Verify protocol group count
				expectedGroupCount := 0
				if len(tc.expectedTCPPorts) > 0 {
					expectedGroupCount++
				}
				if len(tc.expectedUDPPorts) > 0 {
					expectedGroupCount++
				}
				assert.Len(t, protocolPortsInfo, expectedGroupCount, "Protocol group count should match expected")
			}
		})
	}
}

func TestFetchGatewayProtocolPortInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	gatewayClassName := gwv1.ObjectName("test-class")

	testCases := []struct {
		name                   string
		listeners              []gwv1.Listener
		expectedProtocolGroups int
		expectedPorts          map[int32]agaapi.GlobalAcceleratorProtocol
		expectError            bool
	}{
		{
			name: "Gateway with mixed protocols (HTTP, HTTPS, UDP)",
			listeners: []gwv1.Listener{
				{
					Name:     "http",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
				},
				{
					Name:     "https",
					Port:     443,
					Protocol: gwv1.HTTPSProtocolType,
				},
				{
					Name:     "udp",
					Port:     1433,
					Protocol: gwv1.UDPProtocolType,
				},
			},
			// One protocol group for TCP and one for UDP
			expectedProtocolGroups: 2,
			expectedPorts: map[int32]agaapi.GlobalAcceleratorProtocol{
				80:   agaapi.GlobalAcceleratorProtocolTCP,
				443:  agaapi.GlobalAcceleratorProtocolTCP,
				1433: agaapi.GlobalAcceleratorProtocolUDP,
			},
			expectError: false,
		},
		{
			name: "Gateway with HTTP protocol only",
			listeners: []gwv1.Listener{
				{
					Name:     "http-80",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
				},
				{
					Name:     "http-8080",
					Port:     8080,
					Protocol: gwv1.HTTPProtocolType,
				},
			},
			// Only one protocol group for TCP
			expectedProtocolGroups: 1,
			expectedPorts: map[int32]agaapi.GlobalAcceleratorProtocol{
				80:   agaapi.GlobalAcceleratorProtocolTCP,
				8080: agaapi.GlobalAcceleratorProtocolTCP,
			},
			expectError: false,
		},
		{
			name: "Gateway with UDP protocol only",
			listeners: []gwv1.Listener{
				{
					Name:     "udp-53",
					Port:     53,
					Protocol: gwv1.UDPProtocolType,
				},
				{
					Name:     "udp-123",
					Port:     123,
					Protocol: gwv1.UDPProtocolType,
				},
			},
			// Only one protocol group for UDP
			expectedProtocolGroups: 1,
			expectedPorts: map[int32]agaapi.GlobalAcceleratorProtocol{
				53:  agaapi.GlobalAcceleratorProtocolUDP,
				123: agaapi.GlobalAcceleratorProtocolUDP,
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create Gateway resource with test case listeners
			gateway := &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway-" + tc.name,
					Namespace: "default",
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: gatewayClassName,
					Listeners:        tc.listeners,
				},
			}

			// Create loaded endpoint with the gateway resource
			gatewayEndpoint := &LoadedEndpoint{
				Type:        agaapi.GlobalAcceleratorEndpointTypeGateway,
				Name:        "test-gateway-" + tc.name,
				Namespace:   "default",
				Status:      EndpointStatusLoaded,
				K8sResource: gateway,
			}

			// Create mocks
			mockClient := mock_client.NewMockClient(ctrl)
			mockElbv2Client := services.NewMockELBV2(ctrl)
			logger := zap.New()

			// Call the function under test
			discovery := NewEndpointDiscovery(mockClient, logger, mockElbv2Client)
			protocolPortsInfo, err := discovery.fetchGatewayProtocolPortInfo(ctx, gatewayEndpoint)

			// Check error expectations
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Verify the number of protocol groups is as expected
				assert.Len(t, protocolPortsInfo, tc.expectedProtocolGroups, "Protocol group count should match expected")

				// Extract ports by protocol
				tcpPorts := []int32{}
				udpPorts := []int32{}
				for _, info := range protocolPortsInfo {
					if info.Protocol == agaapi.GlobalAcceleratorProtocolTCP {
						tcpPorts = append(tcpPorts, info.Ports...)
					} else if info.Protocol == agaapi.GlobalAcceleratorProtocolUDP {
						udpPorts = append(udpPorts, info.Ports...)
					}
				}

				// Count expected TCP and UDP ports
				expectedTCPPorts := []int32{}
				expectedUDPPorts := []int32{}
				for port, protocol := range tc.expectedPorts {
					if protocol == agaapi.GlobalAcceleratorProtocolTCP {
						expectedTCPPorts = append(expectedTCPPorts, port)
					} else if protocol == agaapi.GlobalAcceleratorProtocolUDP {
						expectedUDPPorts = append(expectedUDPPorts, port)
					}
				}

				// Verify port counts by protocol
				assert.Len(t, tcpPorts, len(expectedTCPPorts), "TCP port count should match expected")
				assert.Len(t, udpPorts, len(expectedUDPPorts), "UDP port count should match expected")

				// Verify each expected TCP port is present
				for _, expectedPort := range expectedTCPPorts {
					assert.Contains(t, tcpPorts, expectedPort, "Expected TCP port %d not found", expectedPort)
				}

				// Verify each expected UDP port is present
				for _, expectedPort := range expectedUDPPorts {
					assert.Contains(t, udpPorts, expectedPort, "Expected UDP port %d not found", expectedPort)
				}
			}
		})
	}
}

func TestGetProtocolPortFromELBListener(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	mockClient := mock_client.NewMockClient(ctrl)
	logger := zap.New()
	mockElbv2Client := services.NewMockELBV2(ctrl)

	discovery := NewEndpointDiscovery(mockClient, logger, mockElbv2Client)

	albARN := "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-lb/1234567890abcdef"
	nlbARN := "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/test-lb/1234567890abcdef"

	// Test case 1: HTTP and HTTPS listeners
	httpHttpsListeners := []types.Listener{
		{
			ListenerArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/app/test-lb/1234567890abcdef/http"),
			Protocol:    types.ProtocolEnumHttp,
			Port:        awssdk.Int32(80),
		},
		{
			ListenerArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/app/test-lb/1234567890abcdef/https"),
			Protocol:    types.ProtocolEnumHttps,
			Port:        awssdk.Int32(443),
		},
	}

	// Test case 2: TCP and TLS listeners
	tcpTlsListeners := []types.Listener{
		{
			ListenerArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/net/test-lb/1234567890abcdef/tcp"),
			Protocol:    types.ProtocolEnumTcp,
			Port:        awssdk.Int32(80),
		},
		{
			ListenerArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/net/test-lb/1234567890abcdef/tls"),
			Protocol:    types.ProtocolEnumTls,
			Port:        awssdk.Int32(443),
		},
	}

	// Test case 3: UDP listener
	udpListeners := []types.Listener{
		{
			ListenerArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/net/test-lb/1234567890abcdef/udp"),
			Protocol:    types.ProtocolEnumUdp,
			Port:        awssdk.Int32(53),
		},
	}

	// Test case 4: Unsupported protocol
	unsupportedListeners := []types.Listener{
		{
			ListenerArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/net/test-lb/1234567890abcdef/tcpudp"),
			Protocol:    types.ProtocolEnumTcpUdp, // Not supported by Global Accelerator
			Port:        awssdk.Int32(53),
		},
	}

	// Test case 5: Empty listener list
	emptyListeners := []types.Listener{}

	testCases := []struct {
		name               string
		lbArn              string
		listeners          []types.Listener
		expectError        bool
		expectedTCPPorts   []int32
		expectedUDPPorts   []int32
		expectedGroupCount int
	}{
		{
			name:               "HTTP and HTTPS listeners",
			lbArn:              albARN,
			listeners:          httpHttpsListeners,
			expectError:        false,
			expectedTCPPorts:   []int32{80, 443},
			expectedUDPPorts:   []int32{},
			expectedGroupCount: 1, // Only TCP group
		},
		{
			name:               "TCP and TLS listeners",
			lbArn:              nlbARN,
			listeners:          tcpTlsListeners,
			expectError:        false,
			expectedTCPPorts:   []int32{80, 443},
			expectedUDPPorts:   []int32{},
			expectedGroupCount: 1, // Only TCP group
		},
		{
			name:               "UDP listener",
			lbArn:              nlbARN,
			listeners:          udpListeners,
			expectError:        false,
			expectedTCPPorts:   []int32{},
			expectedUDPPorts:   []int32{53},
			expectedGroupCount: 1, // Only UDP group
		},
		{
			name:               "Unsupported protocol",
			lbArn:              nlbARN,
			listeners:          unsupportedListeners,
			expectError:        true,
			expectedTCPPorts:   []int32{},
			expectedUDPPorts:   []int32{},
			expectedGroupCount: 0,
		},
		{
			name:               "Empty listeners",
			lbArn:              albARN,
			listeners:          emptyListeners,
			expectError:        false,
			expectedTCPPorts:   []int32{},
			expectedUDPPorts:   []int32{},
			expectedGroupCount: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mock expectations
			mockElbv2Client.EXPECT().
				DescribeListenersAsList(gomock.Any(), gomock.Any()).
				Return(tc.listeners, nil)

			// Call the function under test
			result, err := discovery.getProtocolPortFromELBListener(ctx, tc.lbArn)

			// Verify the result
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedGroupCount, len(result), "Number of protocol groups should match expected")

				// Extract TCP and UDP ports from the result
				var tcpPorts, udpPorts []int32
				for _, info := range result {
					if info.Protocol == agaapi.GlobalAcceleratorProtocolTCP {
						tcpPorts = append(tcpPorts, info.Ports...)
					} else if info.Protocol == agaapi.GlobalAcceleratorProtocolUDP {
						udpPorts = append(udpPorts, info.Ports...)
					}
				}

				// Verify TCP ports
				assert.ElementsMatch(t, tc.expectedTCPPorts, tcpPorts, "TCP ports should match expected")

				// Verify UDP ports
				assert.ElementsMatch(t, tc.expectedUDPPorts, udpPorts, "UDP ports should match expected")
			}
		})
	}
}

func TestFetchLoadBalancerProtocolPortInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	mockClient := mock_client.NewMockClient(ctrl)
	logger := zap.New()
	mockElbv2Client := services.NewMockELBV2(ctrl)

	discovery := NewEndpointDiscovery(mockClient, logger, mockElbv2Client)

	lbARN := "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-lb/1234567890abcdef"

	// Test with mixed TCP and UDP listeners
	mixedListeners := []types.Listener{
		{
			ListenerArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/app/test-lb/1234567890abcdef/http"),
			Protocol:    types.ProtocolEnumHttp,
			Port:        awssdk.Int32(80),
		},
		{
			ListenerArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/app/test-lb/1234567890abcdef/https"),
			Protocol:    types.ProtocolEnumHttps,
			Port:        awssdk.Int32(443),
		},
		{
			ListenerArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/net/test-lb/1234567890abcdef/udp"),
			Protocol:    types.ProtocolEnumUdp,
			Port:        awssdk.Int32(53),
		},
	}

	testCases := []struct {
		name             string
		endpoint         *LoadedEndpoint
		listeners        []types.Listener
		expectError      bool
		expectedTCPPorts []int32
		expectedUDPPorts []int32
	}{
		{
			name: "Load balancer with mixed TCP and UDP listeners",
			endpoint: &LoadedEndpoint{
				Type:      agaapi.GlobalAcceleratorEndpointTypeEndpointID,
				Name:      "test-endpoint",
				Namespace: "default",
				ARN:       lbARN,
			},
			listeners:        mixedListeners,
			expectError:      false,
			expectedTCPPorts: []int32{80, 443},
			expectedUDPPorts: []int32{53},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mock expectations if needed
			if tc.endpoint.ARN != "" {
				mockElbv2Client.EXPECT().
					DescribeListenersAsList(gomock.Any(), gomock.Any()).
					Return(tc.listeners, nil)
			}

			// Call the function under test
			result, err := discovery.fetchLoadBalancerProtocolPortInfo(ctx, tc.endpoint)

			// Verify the result
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Extract TCP and UDP ports from the result for easier comparison
				var tcpPorts, udpPorts []int32
				for _, info := range result {
					if info.Protocol == agaapi.GlobalAcceleratorProtocolTCP {
						tcpPorts = append(tcpPorts, info.Ports...)
					} else if info.Protocol == agaapi.GlobalAcceleratorProtocolUDP {
						udpPorts = append(udpPorts, info.Ports...)
					}
				}

				// Verify TCP ports
				assert.ElementsMatch(t, tc.expectedTCPPorts, tcpPorts)

				// Verify UDP ports
				assert.ElementsMatch(t, tc.expectedUDPPorts, udpPorts)

				// Verify protocol groups count
				expectedGroups := 0
				if len(tc.expectedTCPPorts) > 0 {
					expectedGroups++
				}
				if len(tc.expectedUDPPorts) > 0 {
					expectedGroups++
				}
				assert.Equal(t, expectedGroups, len(result))
			}
		})
	}
}
