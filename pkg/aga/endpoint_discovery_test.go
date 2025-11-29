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
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	// Test Service endpoint
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{
					Port: 80,
				},
				{
					Port: 443,
				},
			},
		},
	}

	t.Run("Service endpoint with TCP ports", func(t *testing.T) {
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

		// With the updated structure, we expect a single protocol with multiple ports
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

	// Test Service with multi-protocol ports
	svcMultiProto := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service-multi-proto",
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
					Name:     "udp-port",
					Protocol: corev1.ProtocolUDP,
					Port:     53,
				},
			},
		},
	}

	t.Run("Service with multi-protocol ports", func(t *testing.T) {
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
	}

	t.Run("Ingress endpoint with default port", func(t *testing.T) {
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
		assert.Len(t, protocolPortsInfo, 1) // Default is just HTTP port 80
		assert.Equal(t, agaapi.GlobalAcceleratorProtocolTCP, protocolPortsInfo[0].Protocol)
		assert.Len(t, protocolPortsInfo[0].Ports, 1, "Should have one port in TCP group")
		assert.Equal(t, int32(80), protocolPortsInfo[0].Ports[0], "Port should be 80")
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

	// Define test IngressClass and IngressClassParams to test certificate discovery from IngressClassParams
	ingressClass := &networkingv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "alb",
		},
		Spec: networkingv1.IngressClassSpec{
			Controller: "ingress.k8s.aws/alb",
			Parameters: &networkingv1.IngressClassParametersReference{
				APIGroup: awssdk.String("elbv2.k8s.aws"),
				Kind:     "IngressClassParams",
				Name:     "alb-class-params",
			},
		},
	}

	ingressClassParams := &elbv2api.IngressClassParams{
		ObjectMeta: metav1.ObjectMeta{
			Name: "alb-class-params",
		},
		Spec: elbv2api.IngressClassParamsSpec{
			CertificateArn: []string{"arn:aws:acm:us-west-2:123456789012:certificate/12345678-1234-1234-1234-123456789012"},
		},
	}

	testCases := []struct {
		name                string
		annotations         map[string]string
		expectedPortCount   int
		expectedPorts       map[int32]bool
		expectError         bool
		errorSubstring      string
		hasIngressClassName bool
		ingressClassName    string
	}{
		{
			name:              "Default HTTP port when no annotations",
			annotations:       map[string]string{},
			expectedPortCount: 1,
			expectedPorts:     map[int32]bool{80: true},
			expectError:       false,
		},
		{
			name: "Default HTTPS port when certificate annotation is present",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/certificate-arn": "arn:aws:acm:us-west-2:123456789012:certificate/12345678-1234-1234-1234-123456789012",
			},
			expectedPortCount: 1,
			expectedPorts:     map[int32]bool{443: true},
			expectError:       false,
		},
		{
			name:                "Default HTTPS port when certificate is in IngressClassParams",
			annotations:         map[string]string{},
			expectedPortCount:   1,
			expectedPorts:       map[int32]bool{443: true},
			expectError:         false,
			hasIngressClassName: true,
			ingressClassName:    "alb",
		},
		{
			name: "Custom listen-ports configuration",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/listen-ports": `[{"HTTP": 8080}, {"HTTPS": 8443}]`,
			},
			expectedPortCount: 2,
			expectedPorts:     map[int32]bool{8080: true, 8443: true},
			expectError:       false,
		},
		{
			name: "Multiple entries for same protocol in listen-ports configuration",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/listen-ports": `[{"HTTP": 80}, {"HTTPS": 443}, {"HTTP": 8080}, {"HTTPS": 8443}]`,
			},
			expectedPortCount: 4,
			expectedPorts:     map[int32]bool{80: true, 443: true, 8080: true, 8443: true},
			expectError:       false,
		},
		{
			name: "Invalid listen-ports configuration",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/listen-ports": `invalid-json`,
			},
			expectedPortCount: 0,
			expectError:       true,
			errorSubstring:    "failed to parse listen-ports annotation",
		},
		{
			name: "Empty listen-ports configuration",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/listen-ports": `[]`,
			},
			expectedPortCount: 0,
			expectError:       true,
			errorSubstring:    "empty listen-ports configuration",
		},
		{
			name: "Invalid protocol configuration",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/listen-ports": `{"invalid-format": true}`,
			},
			expectedPortCount: 0,
			expectError:       true,
			errorSubstring:    "failed to parse",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create Ingress resource with test case annotations
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress-" + tc.name,
					Namespace:   "default",
					Annotations: tc.annotations,
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

			// Add IngressClassName if test case requires it
			if tc.hasIngressClassName {
				ingress.Spec.IngressClassName = &tc.ingressClassName
			}

			// Create mocks
			mockClient := mock_client.NewMockClient(ctrl)
			mockElbv2Client := services.NewMockELBV2(ctrl)
			logger := zap.New()

			// Configure mocks to handle IngressClassParams lookup based on test case
			if tc.hasIngressClassName && tc.ingressClassName == "alb" {
				// Mock getting IngressClass
				mockClient.EXPECT().
					Get(gomock.Any(),
						client.ObjectKey{Name: tc.ingressClassName},
						gomock.AssignableToTypeOf(&networkingv1.IngressClass{}),
						gomock.Any()).
					DoAndReturn(func(ctx context.Context, key client.ObjectKey, obj *networkingv1.IngressClass, _ ...any) error {
						*obj = *ingressClass
						return nil
					})

				// Mock getting IngressClassParams
				mockClient.EXPECT().
					Get(gomock.Any(),
						client.ObjectKey{Name: "alb-class-params"},
						gomock.AssignableToTypeOf(&elbv2api.IngressClassParams{}),
						gomock.Any()).
					DoAndReturn(func(ctx context.Context, key client.ObjectKey, obj *elbv2api.IngressClassParams, _ ...any) error {
						*obj = *ingressClassParams
						return nil
					})
			} else {
				// For other test cases, just make sure the Get calls don't fail
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			}

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
				// The expected port count is now the number of ports, not the number of protocol groups
				// With the updated structure, we should have only one protocol group (TCP) with multiple ports
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
		name             string
		servicePorts     []corev1.ServicePort
		expectedTCPPorts []int32
		expectedUDPPorts []int32
		expectError      bool
	}{
		{
			name: "Single protocol (TCP only)",
			servicePorts: []corev1.ServicePort{
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
			expectedTCPPorts: []int32{80, 443},
			expectedUDPPorts: []int32{},
			expectError:      false,
		},
		{
			name: "Multi-protocol service (TCP + UDP)",
			servicePorts: []corev1.ServicePort{
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
			expectedTCPPorts: []int32{80},
			expectedUDPPorts: []int32{53},
			expectError:      false,
		},
		{
			name: "Service with same port but different protocols (TCP_UDP service) - not supported",
			servicePorts: []corev1.ServicePort{
				{
					Name:     "dns-tcp",
					Protocol: corev1.ProtocolTCP,
					Port:     53,
				},
				{
					Name:     "dns-udp",
					Protocol: corev1.ProtocolUDP,
					Port:     53,
				},
			},
			expectedTCPPorts: []int32{},
			expectedUDPPorts: []int32{},
			expectError:      true,
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
					Type:  corev1.ServiceTypeLoadBalancer,
					Ports: tc.servicePorts,
				},
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

func TestIngressHasCertificate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	mockClient := mock_client.NewMockClient(ctrl)
	logger := zap.New()
	mockElbv2Client := services.NewMockELBV2(ctrl)

	discovery := NewEndpointDiscovery(mockClient, logger, mockElbv2Client)

	// Define IngressClass and IngressClassParams for testing
	ingressClass := &networkingv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "alb",
		},
		Spec: networkingv1.IngressClassSpec{
			Controller: "ingress.k8s.aws/alb",
			Parameters: &networkingv1.IngressClassParametersReference{
				APIGroup: awssdk.String("elbv2.k8s.aws"),
				Kind:     "IngressClassParams",
				Name:     "alb-class-params",
			},
		},
	}

	ingressClassParams := &elbv2api.IngressClassParams{
		ObjectMeta: metav1.ObjectMeta{
			Name: "alb-class-params",
		},
		Spec: elbv2api.IngressClassParamsSpec{
			CertificateArn: []string{"arn:aws:acm:us-west-2:123456789012:certificate/12345678-1234-1234-1234-123456789012"},
		},
	}

	testCases := []struct {
		name                string
		annotations         map[string]string
		hasIngressClassName bool
		ingressClassName    string
		expectMockCalls     bool
		expectedResult      bool
		expectError         bool
		errorSubstring      string
	}{
		{
			name: "Certificate in annotations",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/certificate-arn": "arn:aws:acm:us-west-2:123456789012:certificate/12345678-1234-1234-1234-123456789012",
			},
			hasIngressClassName: false,
			expectedResult:      true,
			expectError:         false,
		},
		{
			name:                "Certificate in IngressClassParams",
			annotations:         map[string]string{},
			hasIngressClassName: true,
			ingressClassName:    "alb",
			expectMockCalls:     true,
			expectedResult:      true,
			expectError:         false,
		},
		{
			name:                "No certificate anywhere",
			annotations:         map[string]string{},
			hasIngressClassName: false,
			expectedResult:      false,
			expectError:         false,
		},
		{
			name: "Both in annotations and IngressClassParams (annotations take precedence)",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/certificate-arn": "arn:aws:acm:us-west-2:123456789012:certificate/12345678-1234-1234-1234-123456789012",
			},
			hasIngressClassName: true,
			ingressClassName:    "alb",
			expectedResult:      true,
			expectError:         false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create Ingress resource with test case parameters
			ing := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "default",
					Annotations: tc.annotations,
				},
			}

			// Add IngressClassName if test case requires it
			if tc.hasIngressClassName {
				ing.Spec.IngressClassName = &tc.ingressClassName
			}

			// Set up mock expectations if needed
			if tc.expectMockCalls {
				// First mock call to get IngressClass
				mockClient.EXPECT().
					Get(gomock.Any(),
						client.ObjectKey{Name: tc.ingressClassName},
						gomock.AssignableToTypeOf(&networkingv1.IngressClass{}),
						gomock.Any()).
					DoAndReturn(func(ctx context.Context, key client.ObjectKey, obj *networkingv1.IngressClass, _ ...any) error {
						*obj = *ingressClass
						return nil
					})

				// Second mock call to get IngressClassParams
				mockClient.EXPECT().
					Get(gomock.Any(),
						client.ObjectKey{Name: "alb-class-params"},
						gomock.AssignableToTypeOf(&elbv2api.IngressClassParams{}),
						gomock.Any()).
					DoAndReturn(func(ctx context.Context, key client.ObjectKey, obj *elbv2api.IngressClassParams, _ ...any) error {
						*obj = *ingressClassParams
						return nil
					})
			}

			// Call the function under test
			result, err := discovery.ingressHasCertificate(ctx, ing)

			// Verify error expectations
			if tc.expectError {
				assert.Error(t, err)
				if tc.errorSubstring != "" {
					assert.Contains(t, err.Error(), tc.errorSubstring)
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify the result
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestParseIngressListenPorts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock_client.NewMockClient(ctrl)
	logger := zap.New()
	mockElbv2Client := services.NewMockELBV2(ctrl)

	discovery := NewEndpointDiscovery(mockClient, logger, mockElbv2Client)

	testCases := []struct {
		name           string
		rawListenPorts string
		expectedPorts  []int32
	}{
		{
			name:           "Single HTTP port",
			rawListenPorts: `[{"HTTP": 80}]`,
			expectedPorts:  []int32{80},
		},
		{
			name:           "Single HTTPS port",
			rawListenPorts: `[{"HTTPS": 443}]`,
			expectedPorts:  []int32{443},
		},
		{
			name:           "Multiple ports",
			rawListenPorts: `[{"HTTP": 80}, {"HTTPS": 443}, {"HTTP": 8080}, {"HTTPS": 8443}]`,
			expectedPorts:  []int32{80, 443, 8080, 8443},
		},
		{
			name:           "Empty JSON array",
			rawListenPorts: `[]`,
			expectedPorts:  nil,
		},
		{
			name:           "Invalid JSON",
			rawListenPorts: `invalid-json`,
			expectedPorts:  nil,
		},
		{
			name:           "Invalid format (not array of objects)",
			rawListenPorts: `{"HTTP": 80}`,
			expectedPorts:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock ingress resource for logging purposes
			ing := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
				},
			}

			// Call the function under test
			result, err := discovery.parseIngressListenPorts(tc.rawListenPorts, ing)

			// Verify the result
			if tc.expectedPorts == nil {
				if tc.name == "Invalid JSON" || tc.name == "Invalid format (not array of objects)" {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), "failed to parse listen-ports annotation")
					assert.Empty(t, result)
				} else if tc.name == "Empty JSON array" {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), "empty listen-ports configuration")
					assert.Empty(t, result)
				}
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tc.expectedPorts, result)
			}
		})
	}
}

func TestHasCertificatesInIngressClassParams(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	mockClient := mock_client.NewMockClient(ctrl)
	logger := zap.New()
	mockElbv2Client := services.NewMockELBV2(ctrl)

	discovery := NewEndpointDiscovery(mockClient, logger, mockElbv2Client)

	// Valid IngressClass with Parameters pointing to valid IngressClassParams
	validIngressClass := &networkingv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "alb",
		},
		Spec: networkingv1.IngressClassSpec{
			Controller: "ingress.k8s.aws/alb",
			Parameters: &networkingv1.IngressClassParametersReference{
				APIGroup: awssdk.String("elbv2.k8s.aws"),
				Kind:     "IngressClassParams",
				Name:     "alb-class-params",
			},
		},
	}

	// IngressClass without Parameters
	noParamsIngressClass := &networkingv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "alb-no-params",
		},
		Spec: networkingv1.IngressClassSpec{
			Controller: "ingress.k8s.aws/alb",
			// No Parameters
		},
	}

	// IngressClass with non-elbv2 Parameters
	nonElbv2ParamsIngressClass := &networkingv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "alb-other-params",
		},
		Spec: networkingv1.IngressClassSpec{
			Controller: "ingress.k8s.aws/alb",
			Parameters: &networkingv1.IngressClassParametersReference{
				APIGroup: awssdk.String("other.k8s.aws"), // Different API group
				Kind:     "OtherParams",
				Name:     "other-params",
			},
		},
	}

	// IngressClassParams with certificate ARN
	ingressClassParamsWithCert := &elbv2api.IngressClassParams{
		ObjectMeta: metav1.ObjectMeta{
			Name: "alb-class-params",
		},
		Spec: elbv2api.IngressClassParamsSpec{
			CertificateArn: []string{"arn:aws:acm:us-west-2:123456789012:certificate/12345678-1234-1234-1234-123456789012"},
		},
	}

	// IngressClassParams without certificate ARN
	ingressClassParamsNoCert := &elbv2api.IngressClassParams{
		ObjectMeta: metav1.ObjectMeta{
			Name: "alb-class-params-no-cert",
		},
		Spec: elbv2api.IngressClassParamsSpec{
			// No CertificateArn
		},
	}

	testCases := []struct {
		name             string
		ingressClassName string
		mockResponses    []interface{}
		expectError      bool
		expectedHasCert  bool
	}{
		{
			name:             "Valid IngressClass with certificate",
			ingressClassName: "alb",
			mockResponses:    []interface{}{validIngressClass, ingressClassParamsWithCert},
			expectError:      false,
			expectedHasCert:  true,
		},
		{
			name:             "Valid IngressClass but no certificate",
			ingressClassName: "alb",
			mockResponses:    []interface{}{validIngressClass, ingressClassParamsNoCert},
			expectError:      false,
			expectedHasCert:  false,
		},
		{
			name:             "IngressClass without Parameters",
			ingressClassName: "alb-no-params",
			mockResponses:    []interface{}{noParamsIngressClass},
			expectError:      false,
			expectedHasCert:  false,
		},
		{
			name:             "IngressClass with non-elbv2 Parameters",
			ingressClassName: "alb-other-params",
			mockResponses:    []interface{}{nonElbv2ParamsIngressClass},
			expectError:      false,
			expectedHasCert:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mock expectations
			mockClient.EXPECT().
				Get(gomock.Any(),
					client.ObjectKey{Name: tc.ingressClassName},
					gomock.AssignableToTypeOf(&networkingv1.IngressClass{}),
					gomock.Any()).
				DoAndReturn(func(ctx context.Context, key client.ObjectKey, obj *networkingv1.IngressClass, _ ...any) error {
					*obj = *(tc.mockResponses[0].(*networkingv1.IngressClass))
					return nil
				})

			// If we expect more calls (to get IngressClassParams)
			if len(tc.mockResponses) > 1 {
				mockClient.EXPECT().
					Get(gomock.Any(),
						gomock.Any(), // Don't strictly match Name here as different IngressClasses may have different param names
						gomock.AssignableToTypeOf(&elbv2api.IngressClassParams{}),
						gomock.Any()).
					DoAndReturn(func(ctx context.Context, key client.ObjectKey, obj *elbv2api.IngressClassParams, _ ...any) error {
						*obj = *(tc.mockResponses[1].(*elbv2api.IngressClassParams))
						return nil
					})
			}

			// Call the function under test
			result, err := discovery.hasCertificatesInIngressClassParams(ctx, tc.ingressClassName)

			// Verify the result
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedHasCert, result)
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
