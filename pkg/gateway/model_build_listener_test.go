package gateway

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

func Test_defaultModelBuilderTask_buildListenerALPNPolicy(t *testing.T) {
	tests := []struct {
		name             string
		svc              *corev1.Service
		gateway          *v1beta1.Gateway
		wantErr          string
		want             []string
		listenerProtocol elbv2model.Protocol
		targetProtocol   elbv2model.Protocol
	}{
		{
			name:             "Gateway without annotation",
			gateway:          &v1beta1.Gateway{},
			listenerProtocol: elbv2model.ProtocolTLS,
		},
		{
			name:             "Gateway without annotation, TLS target",
			gateway:          &v1beta1.Gateway{},
			listenerProtocol: elbv2model.ProtocolTLS,
			targetProtocol:   elbv2model.ProtocolTLS,
		},
		{
			name: "Gateway with annotation, non-TLS target",
			gateway: &v1beta1.Gateway{
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
			name: "Gateway with annotation, TLS target",
			gateway: &v1beta1.Gateway{
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
			name: "Gateway with invalid annotation, TLS target",
			gateway: &v1beta1.Gateway{
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
				gateway:          tt.gateway,
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
		gateway *v1beta1.Gateway
		wantErr error
		want    *listenerConfig
	}{
		{
			name: "Gateway with unused ports in the ssl-ports annotation, Unused ports provided",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-ssl-ports": "80, 85, 90, arbitrary-name",
					},
				},
				Spec: v1beta1.GatewaySpec{
					GatewayClassName: "gateway-class",
					Listeners: []v1beta1.Listener{
						{
							Name:     "gateway-listener-1",
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
						{
							Name:     "gateway-listener-2",
							Port:     83,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			wantErr: errors.New("Unused port in ssl-ports annotation [85 90 arbitrary-name]"),
		},
		{
			name: "Gateway with unused ports in the ssl-ports annotation, No unused ports",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-ssl-ports": "83",
					},
				},
				Spec: v1beta1.GatewaySpec{
					GatewayClassName: "gateway-class",
					Listeners: []v1beta1.Listener{
						{
							Name:     "gateway-listener-1",
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
						{
							Name:     "gateway-listener-2",
							Port:     83,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			want: &listenerConfig{
				certificates:    ([]elbv2model.Certificate)(nil),
				tlsPortsSet:     sets.NewString("83"),
				sslPolicy:       new(string),
				backendProtocol: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				annotationParser: parser,
				gateway:          tt.gateway,
			}
			got, err := builder.buildListenerConfig(context.Background())
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
