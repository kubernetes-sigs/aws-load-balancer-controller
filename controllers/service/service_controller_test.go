package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestBuildPortsForStatus(t *testing.T) {
	tests := []struct {
		name     string
		service  *corev1.Service
		expected []corev1.PortStatus
	}{
		{
			name: "service with single port",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Protocol: corev1.ProtocolTCP,
							Port:     80,
						},
					},
				},
			},
			expected: []corev1.PortStatus{
				{
					Port:     80,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
		{
			name: "service with multiple ports",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
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
						{
							Name:     "dns",
							Protocol: corev1.ProtocolUDP,
							Port:     53,
						},
					},
				},
			},
			expected: []corev1.PortStatus{
				{
					Port:     80,
					Protocol: corev1.ProtocolTCP,
				},
				{
					Port:     443,
					Protocol: corev1.ProtocolTCP,
				},
				{
					Port:     53,
					Protocol: corev1.ProtocolUDP,
				},
			},
		},
		{
			name: "service with no ports",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{},
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reconciler := &serviceReconciler{}
			result := reconciler.buildPortsForStatus(tt.service)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldUpdatePorts(t *testing.T) {
	tests := []struct {
		name     string
		service  *corev1.Service
		expected bool
	}{
		{
			name: "no existing ingress entry",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Protocol: corev1.ProtocolTCP,
							Port:     80,
						},
					},
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{},
					},
				},
			},
			expected: true,
		},
		{
			name: "different port count",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
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
								Hostname: "test-nlb.elb.amazonaws.com",
								Ports: []corev1.PortStatus{
									{
										Port:     80,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "missing port",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
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
								Hostname: "test-nlb.elb.amazonaws.com",
								Ports: []corev1.PortStatus{
									{
										Port:     80,
										Protocol: corev1.ProtocolTCP,
									},
									{
										Port:     8080, // Different from spec
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "matching ports - no update needed",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
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
								Hostname: "test-nlb.elb.amazonaws.com",
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
			expected: false,
		},
		{
			name: "matching ports, order changed- no update needed",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
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
								Hostname: "test-nlb.elb.amazonaws.com",
								Ports: []corev1.PortStatus{
									{
										Port:     443,
										Protocol: corev1.ProtocolTCP,
									},
									{
										Port:     80,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reconciler := &serviceReconciler{}
			result := reconciler.shouldUpdatePorts(tt.service)
			assert.Equal(t, tt.expected, result)
		})
	}
}
