package ingress

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func Test_updateIngressStatus(t *testing.T) {
	albDNS := "alb.example.com"
	nlbDNS := "nlb.example.com"

	tests := []struct {
		name           string
		annotations    map[string]string
		lbDNS          string
		frontendNlbDNS string
		wantHostnames  []string
	}{
		{
			name:           "no frontend NLB: only ALB in status",
			lbDNS:          albDNS,
			frontendNlbDNS: "",
			wantHostnames:  []string{albDNS},
		},
		{
			name:           "frontend NLB without annotation: ALB first then NLB",
			lbDNS:          albDNS,
			frontendNlbDNS: nlbDNS,
			wantHostnames:  []string{albDNS, nlbDNS},
		},
		{
			name: "frontend-nlb-status-only=true: only NLB in status",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/frontend-nlb-status-only": "true",
			},
			lbDNS:          albDNS,
			frontendNlbDNS: nlbDNS,
			wantHostnames:  []string{nlbDNS},
		},
		{
			name: "frontend-nlb-status-only=true but no NLB DNS: falls back to ALB",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/frontend-nlb-status-only": "true",
			},
			lbDNS:          albDNS,
			frontendNlbDNS: "",
			wantHostnames:  []string{albDNS},
		},
		{
			name: "frontend-nlb-status-only=false: ALB first then NLB",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/frontend-nlb-status-only": "false",
			},
			lbDNS:          albDNS,
			frontendNlbDNS: nlbDNS,
			wantHostnames:  []string{albDNS, nlbDNS},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "default",
					Annotations: tt.annotations,
				},
			}

			scheme := runtime.NewScheme()
			require.NoError(t, clientgoscheme.AddToScheme(scheme))

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(ing).
				WithObjects(ing).
				Build()

			r := &groupReconciler{
				k8sClient:        k8sClient,
				annotationParser: annotations.NewSuffixAnnotationParser(annotations.AnnotationPrefixIngress),
			}

			err := r.updateIngressStatus(context.Background(), tt.lbDNS, tt.frontendNlbDNS, ing, nil)
			require.NoError(t, err)

			updated := &networking.Ingress{}
			require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Name: "test-ingress", Namespace: "default"}, updated))

			var gotHostnames []string
			for _, entry := range updated.Status.LoadBalancer.Ingress {
				gotHostnames = append(gotHostnames, entry.Hostname)
			}
			assert.Equal(t, tt.wantHostnames, gotHostnames)
		})
	}
}
