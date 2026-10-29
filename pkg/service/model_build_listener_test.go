package service

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/networking"
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
				quicPortsSet:    sets.New[string](),
			},
		},
		{
			name: "Service with QUIC enabled port",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-quic-enabled-ports": "80",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolUDP,
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
				tlsPortsSet:     sets.New[string](),
				sslPolicy:       new(string),
				backendProtocol: "",
				tcpUdpPortsSet:  sets.New[int32](),
				quicPortsSet:    sets.New[string]("80"),
			},
		},
		{
			name: "Service with QUIC enabled port, not a part of service",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-quic-enabled-ports": "500",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolUDP,
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
				tlsPortsSet:     sets.New[string](),
				sslPolicy:       new(string),
				backendProtocol: "",
				tcpUdpPortsSet:  sets.New[int32](),
				quicPortsSet:    sets.New[string]("500"),
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

func Test_defaultModelBuildTask_buildListenerTags(t *testing.T) {
	tests := []struct {
		name                string
		enabledFeatureGates func() config.FeatureGates
		defaultTags         map[string]string
		svc                 *corev1.Service
		wantTags            map[string]string
		wantErr             bool
	}{
		{
			name:                "no default tags, no annotation tags",
			enabledFeatureGates: func() config.FeatureGates { return config.NewFeatureGates() },
			defaultTags:         map[string]string{},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			wantTags: map[string]string{},
			wantErr:  false,
		},
		{
			name:                "no default tags, annotation tags",
			enabledFeatureGates: func() config.FeatureGates { return config.NewFeatureGates() },
			defaultTags:         map[string]string{},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "k1=v1,k2=v2,k3=v3",
					},
				},
			},
			wantTags: map[string]string{
				"k1": "v1",
				"k2": "v2",
				"k3": "v3",
			},
			wantErr: false,
		},
		{
			name:                "default tags, no annotation tags",
			enabledFeatureGates: func() config.FeatureGates { return config.NewFeatureGates() },
			defaultTags: map[string]string{
				"k1": "v10",
				"k2": "v20",
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			wantTags: map[string]string{
				"k1": "v10",
				"k2": "v20",
			},
			wantErr: false,
		},
		{
			name: "default tags, annotation tags, collision where default tags take priority",
			enabledFeatureGates: func() config.FeatureGates {
				featureGates := config.NewFeatureGates()
				featureGates.Disable(config.EnableDefaultTagsLowPriority)
				return featureGates
			},
			defaultTags: map[string]string{
				"k1": "v10",
				"k2": "v20",
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "k1=v1,k2=v2,k3=v3",
					},
				},
			},
			wantTags: map[string]string{
				"k1": "v10",
				"k2": "v20",
				"k3": "v3",
			},
			wantErr: false,
		},
		{
			name: "default tags, annotation tags, collision where annotation tags take priority",
			enabledFeatureGates: func() config.FeatureGates {
				featureGates := config.NewFeatureGates()
				featureGates.Enable(config.EnableDefaultTagsLowPriority)
				return featureGates
			},
			defaultTags: map[string]string{
				"k1": "v10",
				"k2": "v20",
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "k1=v1,k2=v2,k3=v3",
					},
				},
			},
			wantTags: map[string]string{
				"k1": "v1",
				"k2": "v2",
				"k3": "v3",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				featureGates:     tt.enabledFeatureGates(),
				defaultTags:      tt.defaultTags,
				annotationParser: annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io"),
				service:          tt.svc,
			}
			got, err := task.buildListenerTags(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantTags, got)
			}
			for key, value := range tt.wantTags {
				assert.Contains(t, got, key)
				assert.Equal(t, value, got[key])
			}
		})
	}
}

// newDualStackTask creates a minimal defaultModelBuildTask for dual-stack listener tests.
// It wires a mocked vpcInfoProvider and a dual-stack LB. The service uses namespace "default",
// name "mysvc", NodePort 31000 on port 80, with both IPv4 and IPv6 families.
func newDualStackTask(t *testing.T, ctrl *gomock.Controller, svcAnnotations map[string]string) (*defaultModelBuildTask, core.Stack, *elbv2model.LoadBalancer) {
	t.Helper()
	vpcInfoProvider := networking.NewMockVPCInfoProvider(ctrl)
	vpcInfoProvider.EXPECT().FetchVPCInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(networking.VPCInfo{}, nil).AnyTimes()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "mysvc",
			Namespace:   "default",
			UID:         "aaaa-bbbb-cccc",
			Annotations: svcAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type:       corev1.ServiceTypeLoadBalancer,
			IPFamilies: []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
			Ports: []corev1.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 31000},
			},
		},
	}
	stack := core.NewDefaultStack(core.StackID{Name: "test-stack"})
	lb := elbv2model.NewLoadBalancer(stack, "LB", elbv2model.LoadBalancerSpec{
		IPAddressType: elbv2model.IPAddressTypeDualStack,
	})
	task := &defaultModelBuildTask{
		clusterName:                          "my-cluster",
		annotationParser:                     annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io"),
		service:                              svc,
		loadBalancer:                         lb,
		stack:                                stack,
		logger:                               logr.Discard(),
		featureGates:                         config.NewFeatureGates(),
		tgByResID:                            make(map[string]*elbv2model.TargetGroup),
		defaultHealthCheckProtocol:           elbv2model.ProtocolTCP,
		defaultHealthCheckPort:               "traffic-port",
		defaultHealthCheckInterval:           10,
		defaultHealthCheckTimeout:            10,
		defaultHealthCheckHealthyThreshold:   3,
		defaultHealthCheckUnhealthyThreshold: 3,
		vpcInfoProvider:                      vpcInfoProvider,
		vpcID:                                "vpc-xxx",
	}
	return task, stack, lb
}

func Test_buildListener_dualStackRoutingTGs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	task, stack, _ := newDualStackTask(t, ctrl, map[string]string{
		"service.beta.kubernetes.io/aws-load-balancer-type":               "external",
		"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type":    "instance",
		"service.beta.kubernetes.io/aws-load-balancer-ip-address-type":    "dualstack",
		"service.beta.kubernetes.io/aws-load-balancer-dual-stack-routing": "true",
	})
	port := task.service.Spec.Ports[0]
	cfg := &listenerConfig{tcpUdpPortsSet: sets.New[int32](), tlsPortsSet: sets.New[string](), quicPortsSet: sets.New[string]()}

	ctx := context.Background()
	ls, err := task.buildListener(ctx, port, cfg, elbv2model.LoadBalancerSchemeInternal)
	require.NoError(t, err)
	require.NotNil(t, ls)

	// Default action should forward to the IPv6 TG
	require.Len(t, ls.Spec.DefaultActions, 1)
	assert.Equal(t, elbv2model.ActionTypeForward, ls.Spec.DefaultActions[0].Type)
	require.NotNil(t, ls.Spec.DefaultActions[0].ForwardConfig)
	require.Len(t, ls.Spec.DefaultActions[0].ForwardConfig.TargetGroups, 1)
	ipv6ARN := ls.Spec.DefaultActions[0].ForwardConfig.TargetGroups[0].TargetGroupARN

	// Verify the default TG is IPv6
	defaultResID := "default/mysvc:80"
	defaultTG, ok := task.tgByResID[defaultResID]
	require.True(t, ok, "default TG not found at resource ID %q", defaultResID)
	assert.Equal(t, elbv2model.TargetGroupIPAddressTypeIPv6, defaultTG.Spec.IPAddressType, "default TG must be IPv6")

	// A priority-1 listener rule should be registered for IPv4 traffic
	var rules []*elbv2model.ListenerRule
	require.NoError(t, stack.ListResources(&rules))
	require.Len(t, rules, 1)
	rule := rules[0]

	assert.Equal(t, int32(1), rule.Spec.Priority)
	require.Len(t, rule.Spec.Conditions, 1)
	assert.Equal(t, elbv2model.RuleConditionFieldSourceIP, rule.Spec.Conditions[0].Field)
	require.NotNil(t, rule.Spec.Conditions[0].SourceIPConfig)
	ipv4SrcType := elbv2model.SourceIPAddressTypeIPv4
	assert.Equal(t, &ipv4SrcType, rule.Spec.Conditions[0].SourceIPConfig.IpAddressType)

	require.Len(t, rule.Spec.Actions, 1)
	require.NotNil(t, rule.Spec.Actions[0].ForwardConfig)
	ipv4ARN := rule.Spec.Actions[0].ForwardConfig.TargetGroups[0].TargetGroupARN
	assert.NotEqual(t, ipv6ARN, ipv4ARN)

	// Verify the IPv4 rule TG is actually IPv4
	ipv4ResID := "default/mysvc:80:ipv4"
	ipv4TG, ok := task.tgByResID[ipv4ResID]
	require.True(t, ok, "IPv4 TG not found at resource ID %q", ipv4ResID)
	assert.Equal(t, elbv2model.TargetGroupIPAddressTypeIPv4, ipv4TG.Spec.IPAddressType, "rule TG must be IPv4")
}

func Test_buildListener_dualStackRoutingRegistersRule(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	task, stack, _ := newDualStackTask(t, ctrl, map[string]string{
		"service.beta.kubernetes.io/aws-load-balancer-type":               "external",
		"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type":    "instance",
		"service.beta.kubernetes.io/aws-load-balancer-ip-address-type":    "dualstack",
		"service.beta.kubernetes.io/aws-load-balancer-dual-stack-routing": "true",
	})

	cfg := &listenerConfig{
		tlsPortsSet:    sets.New[string](),
		quicPortsSet:   sets.New[string](),
		tcpUdpPortsSet: sets.New[int32](),
	}

	ctx := context.Background()
	port := task.service.Spec.Ports[0]
	ls, err := task.buildListener(ctx, port, cfg, elbv2model.LoadBalancerSchemeInternal)
	require.NoError(t, err)
	require.NotNil(t, ls)

	// Listener default action should point to the IPv6 TG
	require.Len(t, ls.Spec.DefaultActions, 1)
	assert.Equal(t, elbv2model.ActionTypeForward, ls.Spec.DefaultActions[0].Type)

	// Rule should be in the stack referencing the listener ARN with priority 1
	var rules []*elbv2model.ListenerRule
	require.NoError(t, stack.ListResources(&rules))
	require.Len(t, rules, 1, "expected exactly one listener rule in stack")
	rule := rules[0]

	require.NotNil(t, rule.Spec.ListenerARN)
	assert.Equal(t, ls.ListenerARN().Dependencies(), rule.Spec.ListenerARN.Dependencies())
	assert.Equal(t, int32(1), rule.Spec.Priority)
	assert.NotNil(t, rule.Spec.Tags, "listener rule must carry resource tags")
}

func Test_hasBothIPFamilies(t *testing.T) {
	tests := []struct {
		name string
		svc  *corev1.Service
		want bool
	}{
		{
			name: "IPv4 only",
			svc:  &corev1.Service{Spec: corev1.ServiceSpec{IPFamilies: []corev1.IPFamily{corev1.IPv4Protocol}}},
			want: false,
		},
		{
			name: "IPv6 only",
			svc:  &corev1.Service{Spec: corev1.ServiceSpec{IPFamilies: []corev1.IPFamily{corev1.IPv6Protocol}}},
			want: false,
		},
		{
			name: "both IPv4 and IPv6",
			svc:  &corev1.Service{Spec: corev1.ServiceSpec{IPFamilies: []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}}},
			want: true,
		},
		{
			name: "both IPv6 and IPv4 (order reversed)",
			svc:  &corev1.Service{Spec: corev1.ServiceSpec{IPFamilies: []corev1.IPFamily{corev1.IPv6Protocol, corev1.IPv4Protocol}}},
			want: true,
		},
		{
			name: "no families",
			svc:  &corev1.Service{Spec: corev1.ServiceSpec{}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasBothIPFamilies(tt.svc))
		})
	}
}

func Test_shouldEnableDualStackRouting(t *testing.T) {
	stack := core.NewDefaultStack(core.StackID{Name: "test-stack"})
	tests := []struct {
		name          string
		lbIPAddrType  elbv2model.IPAddressType
		svcIPFamilies []corev1.IPFamily
		annotations   map[string]string
		want          bool
	}{
		{
			name:          "annotation true, dual-stack LB, both families",
			lbIPAddrType:  elbv2model.IPAddressTypeDualStack,
			svcIPFamilies: []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
			annotations:   map[string]string{"service.beta.kubernetes.io/aws-load-balancer-dual-stack-routing": "true"},
			want:          true,
		},
		{
			name:          "annotation true, dualstack-without-public-ipv4 LB, both families",
			lbIPAddrType:  elbv2model.IPAddressTypeDualStackWithoutPublicIPV4,
			svcIPFamilies: []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
			annotations:   map[string]string{"service.beta.kubernetes.io/aws-load-balancer-dual-stack-routing": "true"},
			want:          true,
		},
		{
			name:          "annotation false, dual-stack LB, both families",
			lbIPAddrType:  elbv2model.IPAddressTypeDualStack,
			svcIPFamilies: []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
			annotations:   map[string]string{"service.beta.kubernetes.io/aws-load-balancer-dual-stack-routing": "false"},
			want:          false,
		},
		{
			name:          "annotation absent, dual-stack LB, both families",
			lbIPAddrType:  elbv2model.IPAddressTypeDualStack,
			svcIPFamilies: []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
			annotations:   map[string]string{},
			want:          false,
		},
		{
			name:          "annotation true, IPv4-only LB",
			lbIPAddrType:  elbv2model.IPAddressTypeIPV4,
			svcIPFamilies: []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
			annotations:   map[string]string{"service.beta.kubernetes.io/aws-load-balancer-dual-stack-routing": "true"},
			want:          false,
		},
		{
			name:          "annotation true, dual-stack LB, IPv4-only service",
			lbIPAddrType:  elbv2model.IPAddressTypeDualStack,
			svcIPFamilies: []corev1.IPFamily{corev1.IPv4Protocol},
			annotations:   map[string]string{"service.beta.kubernetes.io/aws-load-balancer-dual-stack-routing": "true"},
			want:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lb := elbv2model.NewLoadBalancer(stack, "LB", elbv2model.LoadBalancerSpec{
				IPAddressType: tt.lbIPAddrType,
			})
			task := &defaultModelBuildTask{
				annotationParser: annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io"),
				service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations},
					Spec:       corev1.ServiceSpec{IPFamilies: tt.svcIPFamilies},
				},
				loadBalancer: lb,
				logger:       logr.Discard(),
			}
			assert.Equal(t, tt.want, task.shouldEnableDualStackRouting())
		})
	}
}

func Test_buildListenerSpec_dualStackRoutingAndActionsAnnotationError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	stack := core.NewDefaultStack(core.StackID{Name: "test-stack"})
	lb := elbv2model.NewLoadBalancer(stack, "LB", elbv2model.LoadBalancerSpec{
		IPAddressType: elbv2model.IPAddressTypeDualStack,
	})
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mysvc",
			Namespace: "default",
			UID:       "aaaa-bbbb-cccc",
			Annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type":    "instance",
				"service.beta.kubernetes.io/aws-load-balancer-dual-stack-routing": "true",
				"service.beta.kubernetes.io/actions.TCP-80":                       `{"type":"forward","forwardConfig":{"baseServiceWeight":1,"targetGroups":[]}}`,
			},
		},
		Spec: corev1.ServiceSpec{
			Type:       corev1.ServiceTypeLoadBalancer,
			IPFamilies: []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
			Ports: []corev1.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 31000},
			},
		},
	}

	task := &defaultModelBuildTask{
		clusterName:      "my-cluster",
		annotationParser: annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io"),
		service:          svc,
		loadBalancer:     lb,
		stack:            stack,
		logger:           logr.Discard(),
		featureGates:     config.NewFeatureGates(),
		tgByResID:        make(map[string]*elbv2model.TargetGroup),
	}

	cfg := &listenerConfig{
		tlsPortsSet:    sets.New[string](),
		quicPortsSet:   sets.New[string](),
		tcpUdpPortsSet: sets.New[int32](),
	}

	_, err := task.buildListener(context.Background(), svc.Spec.Ports[0], cfg, elbv2model.LoadBalancerSchemeInternal)
	require.EqualError(t, err, "dual-stack routing annotation is incompatible with the actions annotation")
}

func Test_buildDualStackIPv4ListenerRule_ipTargetTypeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	task, stack, _ := newDualStackTask(t, ctrl, map[string]string{
		"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type":    "ip",
		"service.beta.kubernetes.io/aws-load-balancer-dual-stack-routing": "true",
	})
	task.enableIPTargetType = true

	ls := elbv2model.NewListener(stack, "80", elbv2model.ListenerSpec{
		LoadBalancerARN: task.loadBalancer.LoadBalancerARN(),
	})

	cfg := &listenerConfig{tcpUdpPortsSet: sets.New[int32](), tlsPortsSet: sets.New[string](), quicPortsSet: sets.New[string]()}
	err := task.buildDualStackIPv4ListenerRule(context.Background(), ls, task.service.Spec.Ports[0], cfg, elbv2model.LoadBalancerSchemeInternal)
	require.EqualError(t, err, "dual-stack routing is not supported with ip target type")
}

func Test_buildDualStackIPv4ListenerRule_udpProtocol(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	task, stack, _ := newDualStackTask(t, ctrl, map[string]string{
		"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type":    "instance",
		"service.beta.kubernetes.io/aws-load-balancer-dual-stack-routing": "true",
	})
	// UDP listeners support listener rules on dual-stack NLBs
	task.service.Spec.Ports[0].Protocol = corev1.ProtocolUDP

	ls := elbv2model.NewListener(stack, "80", elbv2model.ListenerSpec{
		LoadBalancerARN: task.loadBalancer.LoadBalancerARN(),
	})
	cfg := &listenerConfig{tcpUdpPortsSet: sets.New[int32](), tlsPortsSet: sets.New[string](), quicPortsSet: sets.New[string]()}
	err := task.buildDualStackIPv4ListenerRule(context.Background(), ls, task.service.Spec.Ports[0], cfg, elbv2model.LoadBalancerSchemeInternal)
	require.NoError(t, err)

	var rules []*elbv2model.ListenerRule
	require.NoError(t, stack.ListResources(&rules))
	require.Len(t, rules, 1)
}

// Test_dualStackRoutingTGNameStability verifies that adding the dual-stack routing annotation
// to an existing service does not rename the default target group. It builds the listener with
// two otherwise-identical tasks — one without the annotation and one with it — and asserts
// the TG at the default resource ID has the same name in both cases.
func Test_dualStackRoutingTGNameStability(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg := &listenerConfig{
		tlsPortsSet:    sets.New[string](),
		quicPortsSet:   sets.New[string](),
		tcpUdpPortsSet: sets.New[int32](),
	}
	ctx := context.Background()

	baseAnnotations := map[string]string{
		"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
		"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "instance",
		"service.beta.kubernetes.io/aws-load-balancer-ip-address-type": "dualstack",
	}
	dualStackAnnotations := make(map[string]string, len(baseAnnotations)+1)
	for k, v := range baseAnnotations {
		dualStackAnnotations[k] = v
	}
	dualStackAnnotations["service.beta.kubernetes.io/aws-load-balancer-dual-stack-routing"] = "true"

	legacyTask, _, _ := newDualStackTask(t, ctrl, baseAnnotations)
	_, err := legacyTask.buildListener(ctx, legacyTask.service.Spec.Ports[0], cfg, elbv2model.LoadBalancerSchemeInternal)
	require.NoError(t, err)

	dualStackTask, _, _ := newDualStackTask(t, ctrl, dualStackAnnotations)
	_, err = dualStackTask.buildListener(ctx, dualStackTask.service.Spec.Ports[0], cfg, elbv2model.LoadBalancerSchemeInternal)
	require.NoError(t, err)

	defaultResID := "default/mysvc:80"
	legacyTG, ok := legacyTask.tgByResID[defaultResID]
	require.True(t, ok, "legacy task: default TG not found at resource ID %q", defaultResID)
	dualStackTG, ok := dualStackTask.tgByResID[defaultResID]
	require.True(t, ok, "dual-stack task: default TG not found at resource ID %q", defaultResID)

	assert.Equal(t, legacyTG.Spec.Name, dualStackTG.Spec.Name,
		"default TG name must be identical before and after the dual-stack routing annotation is added")
	assert.Equal(t, legacyTG.ID(), dualStackTG.ID(),
		"default TG resource ID must be identical before and after the dual-stack routing annotation is added")
}
