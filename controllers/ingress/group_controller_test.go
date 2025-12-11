package ingress

import (
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	"testing"
)

func TestIsIngressStatusEqual(t *testing.T) {
	testCases := []struct {
		name     string
		statusA  []networking.IngressLoadBalancerIngress
		statusB  []networking.IngressLoadBalancerIngress
		expected bool
	}{
		{
			name:     "Empty statuses",
			statusA:  []networking.IngressLoadBalancerIngress{},
			statusB:  []networking.IngressLoadBalancerIngress{},
			expected: true,
		},
		{
			name:    "Different length statuses",
			statusA: []networking.IngressLoadBalancerIngress{},
			statusB: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
				},
			},
			expected: false,
		},
		{
			name: "Same hostname with no ports",
			statusA: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
				},
			},
			statusB: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
				},
			},
			expected: true,
		},
		{
			name: "Different hostnames",
			statusA: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb1.amazonaws.com",
				},
			},
			statusB: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb2.amazonaws.com",
				},
			},
			expected: false,
		},
		{
			name: "Same hostname with same ports",
			statusA: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
						{Port: 443},
					},
				},
			},
			statusB: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
						{Port: 443},
					},
				},
			},
			expected: true,
		},
		{
			name: "Same hostname with ports in different order",
			statusA: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
						{Port: 443},
					},
				},
			},
			statusB: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 443},
						{Port: 80},
					},
				},
			},
			expected: true,
		},
		{
			name: "Same hostname with different port counts",
			statusA: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
						{Port: 443},
					},
				},
			},
			statusB: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
					},
				},
			},
			expected: false,
		},
		{
			name: "Same hostname with different port values",
			statusA: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
						{Port: 443},
					},
				},
			},
			statusB: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
						{Port: 8443},
					},
				},
			},
			expected: false,
		},
		{
			name: "Multiple entries with matching hostnames and ports",
			statusA: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
						{Port: 443},
					},
				},
				{
					Hostname: "nlb.amazonaws.com",
				},
			},
			statusB: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
						{Port: 443},
					},
				},
				{
					Hostname: "nlb.amazonaws.com",
				},
			},
			expected: true,
		},
		{
			name: "Multiple entries with different port configurations",
			statusA: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
						{Port: 443},
					},
				},
				{
					Hostname: "nlb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
					},
				},
			},
			statusB: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
						{Port: 443},
					},
				},
				{
					Hostname: "nlb.amazonaws.com",
				},
			},
			expected: false,
		},
		{
			name: "One status has empty hostname",
			statusA: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
					},
				},
				{
					Hostname: "",
				},
			},
			statusB: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
					},
				},
				{
					Hostname: "",
				},
			},
			expected: true,
		},
		{
			name: "One status has IP instead of hostname",
			statusA: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					IP:       "",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
					},
				},
			},
			statusB: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "",
					IP:       "192.168.1.1",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
					},
				},
			},
			expected: false,
		},
		{
			name: "Different entries order with same ports",
			statusA: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 80},
						{Port: 443},
					},
				},
				{
					Hostname: "nlb.amazonaws.com",
				},
			},
			statusB: []networking.IngressLoadBalancerIngress{
				{
					Hostname: "nlb.amazonaws.com",
				},
				{
					Hostname: "alb.amazonaws.com",
					Ports: []networking.IngressPortStatus{
						{Port: 443},
						{Port: 80},
					},
				},
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isIngressStatusEqual(tc.statusA, tc.statusB)
			assert.Equal(t, tc.expected, result)
		})
	}
}
