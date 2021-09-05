package service

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

func Test_defaultModelBuilderTask_buildListenerALPNPolicy(t *testing.T) {
	tests := []struct {
		name             string
		svc              *corev1.Service
		wantErr          error
		want             []string
		listenerProtocol elbv2model.Protocol
		targetProtocol   elbv2model.Protocol
	}{
		{
			name:             "Service without annotation",
			svc:              &corev1.Service{},
			listenerProtocol: elbv2model.ProtocolTLS,
		},
		{
			name:             "Service without annotation, TLS target",
			svc:              &corev1.Service{},
			listenerProtocol: elbv2model.ProtocolTLS,
			targetProtocol:   elbv2model.ProtocolTLS,
		},
		{
			name: "Service with annotation, non-TLS target",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-alpn-policy": "HTTP2Only",
					},
				},
			},
			want:             []string{string(elbv2model.ALPNPolicyHTTP2Only)},
			listenerProtocol: elbv2model.ProtocolTLS,
		},
		{
			name: "Service with annotation, TLS target",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-alpn-policy": "HTTP1Only",
					},
				},
			},
			want:             []string{string(elbv2model.ALPNPolicyHTTP1Only)},
			listenerProtocol: elbv2model.ProtocolTLS,
			targetProtocol:   elbv2model.ProtocolTLS,
		},
		{
			name: "Service with invalid annotation, TLS target",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-alpn-policy": "unknown",
					},
				},
			},
			wantErr:          errors.New("invalid ALPN policy unknown, policy must be one of [None, HTTP1Only, HTTP2Only, HTTP2Optional, HTTP2Preferred]"),
			listenerProtocol: elbv2model.ProtocolTLS,
			targetProtocol:   elbv2model.ProtocolTLS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				annotationParser: parser,
				service:          tt.svc,
			}
			got, err := builder.buildListenerALPNPolicy(context.Background(), tt.listenerProtocol, tt.targetProtocol)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_mergeServicePortsForListener(t *testing.T) {
	tests := []struct {
		name  string
		ports []corev1.ServicePort
		want  corev1.ServicePort
	}{
		{
			name: "one port",
			ports: []corev1.ServicePort{
				{
					Name:       "p1",
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolTCP,
					NodePort:   31223,
				},
			},
			want: corev1.ServicePort{
				Name:       "p1",
				Port:       80,
				TargetPort: intstr.FromInt(80),
				Protocol:   corev1.ProtocolTCP,
				NodePort:   31223,
			},
		},
		{
			name: "two tcp ports, different target and node ports",
			ports: []corev1.ServicePort{
				{
					Name:       "p1",
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolTCP,
					NodePort:   31223,
				},
				{
					Name:       "p2",
					Port:       80,
					TargetPort: intstr.FromInt(8888),
					Protocol:   corev1.ProtocolTCP,
					NodePort:   31224,
				},
			},
			want: corev1.ServicePort{
				Name:       "p1",
				Port:       80,
				TargetPort: intstr.FromInt(80),
				Protocol:   corev1.ProtocolTCP,
				NodePort:   31223,
			},
		},
		{
			name: "two udp ports, different target and node ports",
			ports: []corev1.ServicePort{
				{
					Name:       "p1",
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolUDP,
					NodePort:   31223,
				},
				{
					Name:       "p2",
					Port:       80,
					TargetPort: intstr.FromInt(8888),
					Protocol:   corev1.ProtocolUDP,
					NodePort:   31224,
				},
			},
			want: corev1.ServicePort{
				Name:       "p1",
				Port:       80,
				TargetPort: intstr.FromInt(80),
				Protocol:   corev1.ProtocolUDP,
				NodePort:   31223,
			},
		},
		{
			name: "one tcp and one udp, different target and node ports",
			ports: []corev1.ServicePort{
				{
					Name:       "p1",
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolTCP,
					NodePort:   31223,
				},
				{
					Name:       "p2",
					Port:       80,
					TargetPort: intstr.FromInt(8888),
					Protocol:   corev1.ProtocolUDP,
					NodePort:   31224,
				},
			},
			want: corev1.ServicePort{
				Name:       "p1",
				Port:       80,
				TargetPort: intstr.FromInt(80),
				Protocol:   corev1.ProtocolTCP,
				NodePort:   31223,
			},
		},
		{
			name: "one tcp and one udp, same target and node ports",
			ports: []corev1.ServicePort{
				{
					Name:       "p1",
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolTCP,
					NodePort:   31223,
				},
				{
					Name:       "p2",
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolUDP,
					NodePort:   31223,
				},
			},
			want: corev1.ServicePort{
				Name:       "p1",
				Port:       80,
				TargetPort: intstr.FromInt(80),
				Protocol:   corev1.Protocol("TCP_UDP"),
				NodePort:   31223,
			},
		},
		{
			name: "one udp and one tcp, same target and node ports",
			ports: []corev1.ServicePort{
				{
					Name:       "p1",
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolUDP,
					NodePort:   31223,
				},
				{
					Name:       "p2",
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolTCP,
					NodePort:   31223,
				},
			},
			want: corev1.ServicePort{
				Name:       "p1",
				Port:       80,
				TargetPort: intstr.FromInt(80),
				Protocol:   corev1.Protocol("TCP_UDP"),
				NodePort:   31223,
			},
		},
		{
			name: "one tcp and one udp, same node port, different target port",
			ports: []corev1.ServicePort{
				{
					Name:       "p1",
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolTCP,
					NodePort:   31223,
				},
				{
					Name:       "p2",
					Port:       80,
					TargetPort: intstr.FromInt(8888),
					Protocol:   corev1.ProtocolUDP,
					NodePort:   31223,
				},
			},
			want: corev1.ServicePort{
				Name:       "p1",
				Port:       80,
				TargetPort: intstr.FromInt(80),
				Protocol:   corev1.Protocol("TCP_UDP"),
				NodePort:   31223,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := mergeServicePortsForListener(tt.ports)
			assert.Equal(t, port.Name, tt.want.Name)
			assert.Equal(t, port.Port, tt.want.Port)
			assert.Equal(t, port.TargetPort.IntVal, tt.want.TargetPort.IntVal)
			assert.Equal(t, port.Protocol, tt.want.Protocol)
			assert.Equal(t, port.NodePort, tt.want.NodePort)
		})
	}

	// test that function returns new ServicePort instance
	p1 := corev1.ServicePort{
		Name:       "p1",
		Port:       80,
		TargetPort: intstr.FromInt(80),
		Protocol:   corev1.ProtocolTCP,
		NodePort:   31223,
	}
	p2 := corev1.ServicePort{
		Name:       "p2",
		Port:       80,
		TargetPort: intstr.FromInt(80),
		Protocol:   corev1.ProtocolUDP,
		NodePort:   31223,
	}
	ports := []corev1.ServicePort{p1, p2}
	mergedPort := mergeServicePortsForListener(ports)

	assert.Equal(t, corev1.ProtocolTCP, p1.Protocol)
	assert.Equal(t, corev1.Protocol("TCP_UDP"), mergedPort.Protocol)
	assert.NotEqual(t, &p1, &mergedPort)
}
