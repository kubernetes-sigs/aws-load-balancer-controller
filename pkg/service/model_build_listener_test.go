package service

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

func Test_defaultModelBuilderTask_buildListenerALPNPolicy(t *testing.T) {
	tests := []struct {
		name             string
		svc              *corev1.Service
		wantErr          string
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
			wantErr:          "invalid ALPN policy unknown, policy must be one of [None, HTTP1Only, HTTP2Only, HTTP2Optional, HTTP2Preferred]",
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
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuilderTask_buildListenerConfig(t *testing.T) {
	tests := []struct {
		name    string
		svc     *corev1.Service
		wantErr error
		want    *listenerConfig
	}{
		{
			name: "Service with unused ports in the ssl-ports annotation, Unused ports provided",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-ssl-ports": "80, 85, 90, arbitrary-name",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   31223,
						},
						{
							Name:       "alt2",
							Port:       83,
							TargetPort: intstr.FromInt(8883),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   32323,
						},
					},
				},
			},
			wantErr: errors.New("Unused port in ssl-ports annotation [85 90 arbitrary-name]"),
		},
		{
			name: "Service with unused ports in the ssl-ports annotation, No unused ports",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-ssl-ports": "83",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   31223,
						},
						{
							Name:       "alt2",
							Port:       83,
							TargetPort: intstr.FromInt(8883),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   32323,
						},
					},
				},
			},
			want: &listenerConfig{
				certificates:    ([]elbv2model.Certificate)(nil),
				tlsPortsSet:     sets.New[string]("83"),
				sslPolicy:       new(string),
				backendProtocol: "",
				tcpUdpPortsSet:  sets.New[int32](),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				annotationParser: parser,
				service:          tt.svc,
			}
			got, err := builder.buildListenerConfig(context.Background(), sets.New[int32]())
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

const tcpIdleTimeoutSeconds = "tcp.idle_timeout.seconds"

func Test_defaultModelBuilderTask_buildListenerAttributes(t *testing.T) {
	tests := []struct {
		testName  string
		svc       *corev1.Service
		wantError bool
		wantValue [][]elbv2model.ListenerAttribute
	}{
		{
			testName: "Listener attribute annotation value is not stringMap",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                       "instance",
						"service.beta.kubernetes.io/aws-load-balancer-listener-attributes.TCP-80": "tcp.idle_timeout.seconds",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   38888,
						},
					},
				},
			},
			wantError: true,
		},
		{
			testName: "Listener attribute annotation is not specified",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type": "instance",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   38888,
						},
					},
				},
			},
			wantError: false,
			wantValue: [][]elbv2model.ListenerAttribute{
				{},
			},
		},
		{
			testName: "Listener attribute annotation is specified",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                       "ip",
						"service.beta.kubernetes.io/aws-load-balancer-listener-attributes.TCP-80": "tcp.idle_timeout.seconds=400",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "test1",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   38888,
						},
						{
							Name:       "test2",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   corev1.ProtocolUDP,
							NodePort:   38888,
						},
					},
				},
			},
			wantError: false,
			wantValue: [][]elbv2model.ListenerAttribute{
				{
					{
						Key:   tcpIdleTimeoutSeconds,
						Value: "400",
					},
				},
				{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				service:          tt.svc,
				annotationParser: parser,
			}

			for index, port := range tt.svc.Spec.Ports {
				listenerAttributes, err := builder.buildListenerAttributes(context.Background(), tt.svc.Annotations, port.Port, elbv2model.Protocol(port.Protocol))

				if tt.wantError {
					assert.Error(t, err)
				} else {
					assert.ElementsMatch(t, tt.wantValue[index], listenerAttributes)
				}
			}

		})
	}
}

func Test_validateMultiProtocolUsage(t *testing.T) {
	tests := []struct {
		name        string
		ports       []corev1.ServicePort
		expectError bool
	}{
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
			expectError: true,
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
			expectError: true,
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
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolUDP,
					NodePort:   31223,
				},
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
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolUDP,
					NodePort:   31223,
				},
				{
					Name:       "p2",
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolSCTP,
					NodePort:   31223,
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMultiProtocolUsage(tt.ports)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_shouldUseTCPUDP(t *testing.T) {
	testCases := []struct {
		name         string
		svc          *corev1.Service
		defaultValue bool
		expected     bool
	}{
		{
			name:         "no annotation, use default true",
			svc:          &corev1.Service{},
			defaultValue: true,
			expected:     true,
		},
		{
			name:         "no annotation, use default false",
			svc:          &corev1.Service{},
			defaultValue: false,
			expected:     false,
		},
		{
			name: "other annotation, use default false",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-alpn-policy": "HTTP2Only",
					},
				},
			},
			defaultValue: false,
			expected:     false,
		},
		{
			name: "annotation present - true",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-enable-tcp-udp-listener": "true",
					},
				},
			},
			defaultValue: false,
			expected:     true,
		},
		{
			name: "annotation present - false",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-enable-tcp-udp-listener": "false",
					},
				},
			},
			defaultValue: false,
			expected:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := defaultModelBuildTask{
				service:             tc.svc,
				enableTCPUDPSupport: tc.defaultValue,
				annotationParser:    annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io"),
			}
			assert.Equal(t, tc.expected, task.shouldUseTCPUDP())

		})
	}
}
