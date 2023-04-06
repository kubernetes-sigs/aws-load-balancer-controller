package gateway

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

func Test_defaultGatewayUtils_IsGatewaySupported(t *testing.T) {
	tests := []struct {
		name                       string
		gateway                    *v1beta1.Gateway
		restrictToTypeLoadBalancer bool
		want                       bool
	}{
		{
			name: "service with nlb-ip annotation",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-ip",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type": "nlb-ip",
					},
				},
				Spec: v1beta1.GatewaySpec{
					Listeners: []v1beta1.Listener{
						{
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			want: true,
		},
		{
			name: "service with load balancer type external",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-external",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type": "external",
					},
				},
				Spec: v1beta1.GatewaySpec{
					Listeners: []v1beta1.Listener{
						{
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
		},
		{
			name: "service with load balancer type external, nlb-target-type ip",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-ip",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "ip",
					},
				},
				Spec: v1beta1.GatewaySpec{
					Listeners: []v1beta1.Listener{
						{
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			want: true,
		},
		{
			name: "service with load balancer type external, nlb-target-type instance",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-ip",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "instance",
					},
				},
				Spec: v1beta1.GatewaySpec{
					Listeners: []v1beta1.Listener{
						{
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			want: true,
		},
		{
			name: "service with load balancer type nlb-ddd, nlb-target-type instance",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-ip",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "nlb-ddd",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "instance",
					},
				},
				Spec: v1beta1.GatewaySpec{
					Listeners: []v1beta1.Listener{
						{
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
		},
		{
			name: "service without lb type annotation",
			gateway: &v1beta1.Gateway{
				Spec: v1beta1.GatewaySpec{
					Listeners: []v1beta1.Listener{
						{
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
		},
		{
			name: "deleted service",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deleted",
					Namespace: "gateway",
					DeletionTimestamp: &metav1.Time{
						Time: time.Now(),
					},
				},
			},
		},
		{
			name: "lb type ClusterIP, RestrictToLoadBalancerOnly enabled",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-ip",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "ip",
					},
				},
				Spec: v1beta1.GatewaySpec{
					Listeners: []v1beta1.Listener{
						{
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			restrictToTypeLoadBalancer: true,
		},
		{
			name: "spec.loadBalancerClass",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-ip",
					Namespace: "default",
				},
				Spec: v1beta1.GatewaySpec{
					Listeners: []v1beta1.Listener{
						{
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			restrictToTypeLoadBalancer: true,
			want:                       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			featureGates := config.NewFeatureGates()
			serviceUtils := NewGatewayUtils(annotationParser, "service.k8s.aws/resources", "service.k8s.aws/nlb", featureGates)
			got := serviceUtils.IsGatewaySupported(tt.gateway)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultGatewayUtils_IsGatewayPendingFinalization(t *testing.T) {
	tests := []struct {
		name    string
		gateway *v1beta1.Gateway
		want    bool
	}{
		{
			name: "service without finalizer",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-ip",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type": "nlb-ip",
					},
				},
			},
			want: false,
		},
		{
			name: "service with finalizer",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"other/finalizer", "service.k8s.aws/resources"},
				},
			},
			want: true,
		},
		{
			name: "service with some other finalizer",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"some.other/finalizer"},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			featureGates := config.NewFeatureGates()
			serviceUtils := NewGatewayUtils(annotationParser, "service.k8s.aws/resources", "service.k8s.aws/nlb", featureGates)
			got := serviceUtils.IsGatewayPendingFinalization(tt.gateway)
			assert.Equal(t, tt.want, got)
		})
	}
}
