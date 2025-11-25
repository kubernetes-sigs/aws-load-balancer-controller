package shared_utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFindIngressTwoDNSName(t *testing.T) {
	tests := []struct {
		name     string
		ingress  *networking.Ingress
		wantALB  string
		wantNLB  string
		testCase string
	}{
		{
			name: "empty ingress status",
			ingress: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
				},
				Status: networking.IngressStatus{
					LoadBalancer: networking.IngressLoadBalancerStatus{
						Ingress: []networking.IngressLoadBalancerIngress{},
					},
				},
			},
			wantALB:  "",
			wantNLB:  "",
			testCase: "No DNS entries in status",
		},
		{
			name: "only ALB DNS present",
			ingress: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
				},
				Status: networking.IngressStatus{
					LoadBalancer: networking.IngressLoadBalancerStatus{
						Ingress: []networking.IngressLoadBalancerIngress{
							{
								Hostname: "test-alb-1234567890.us-west-2.elb.amazonaws.com",
							},
						},
					},
				},
			},
			wantALB:  "test-alb-1234567890.us-west-2.elb.amazonaws.com",
			wantNLB:  "",
			testCase: "ALB DNS only",
		},
		{
			name: "only NLB DNS present",
			ingress: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
				},
				Status: networking.IngressStatus{
					LoadBalancer: networking.IngressLoadBalancerStatus{
						Ingress: []networking.IngressLoadBalancerIngress{
							{
								Hostname: "test-nlb-1234567890.us-west-2.elb.us-west-2.amazonaws.aws",
							},
						},
					},
				},
			},
			wantALB:  "",
			wantNLB:  "test-nlb-1234567890.us-west-2.elb.us-west-2.amazonaws.aws",
			testCase: "NLB DNS only",
		},
		{
			name: "both ALB and NLB DNS present",
			ingress: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
				},
				Status: networking.IngressStatus{
					LoadBalancer: networking.IngressLoadBalancerStatus{
						Ingress: []networking.IngressLoadBalancerIngress{
							{
								Hostname: "test-alb-1234567890.us-west-2.elb.amazonaws.com",
							},
							{
								Hostname: "test-nlb-1234567890.us-west-2.elb.us-west-2.amazonaws.aws",
							},
						},
					},
				},
			},
			wantALB:  "test-alb-1234567890.us-west-2.elb.amazonaws.com",
			wantNLB:  "test-nlb-1234567890.us-west-2.elb.us-west-2.amazonaws.aws",
			testCase: "Both ALB and NLB DNS",
		},
		{
			name: "multiple same type DNS entries",
			ingress: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
				},
				Status: networking.IngressStatus{
					LoadBalancer: networking.IngressLoadBalancerStatus{
						Ingress: []networking.IngressLoadBalancerIngress{
							{
								Hostname: "test-alb-1234567890.us-west-2.elb.amazonaws.com",
							},
							{
								Hostname: "another-alb-1234567890.us-west-2.elb.amazonaws.com",
							},
							{
								Hostname: "test-nlb-1234567890.us-west-2.elb.us-west-2.amazonaws.aws",
							},
							{
								Hostname: "another-nlb-1234567890.us-west-2.elb.us-west-2.amazonaws.aws",
							},
						},
					},
				},
			},
			wantALB:  "another-alb-1234567890.us-west-2.elb.amazonaws.com",
			wantNLB:  "another-nlb-1234567890.us-west-2.elb.us-west-2.amazonaws.aws",
			testCase: "Multiple entries of each type (function returns last matching hostname)",
		},
		{
			name: "empty hostnames",
			ingress: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
				},
				Status: networking.IngressStatus{
					LoadBalancer: networking.IngressLoadBalancerStatus{
						Ingress: []networking.IngressLoadBalancerIngress{
							{
								Hostname: "",
							},
							{
								Hostname: "",
							},
						},
					},
				},
			},
			wantALB:  "",
			wantNLB:  "",
			testCase: "Empty hostname entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotALB, gotNLB := FindIngressTwoDNSName(tt.ingress)
			assert.Equal(t, tt.wantALB, gotALB, "ALB DNS mismatch for %s", tt.testCase)
			assert.Equal(t, tt.wantNLB, gotNLB, "NLB DNS mismatch for %s", tt.testCase)
		})
	}
}
