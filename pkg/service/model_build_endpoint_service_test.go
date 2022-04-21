package service

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
)

func Test_defaultModelBuildTask_buildEnabled(t *testing.T) {
	tests := []struct {
		name    string
		svc     *corev1.Service
		wantErr bool
		want    bool
	}{
		{
			name: "Service without annotation",
			svc:  &corev1.Service{},
			want: false,
		},
		{
			name: "Service with valid annotation",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-endpoint-service-enabled": "true",
					},
				},
			},
			want: true,
		},
		{
			name: "Service with invalid annotation",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-endpoint-service-enabled": "True",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				annotationParser: parser,
				service:          tt.svc,
			}
			got, err := builder.buildEnabled(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildAcceptanceRequired(t *testing.T) {
	tests := []struct {
		name    string
		svc     *corev1.Service
		wantErr bool
		want    bool
	}{
		{
			name: "Service without annotation",
			svc:  &corev1.Service{},
			want: false,
		},
		{
			name: "Service with valid annotation",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-endpoint-service-acceptance-required": "true",
					},
				},
			},
			want: true,
		},
		{
			name: "Service with invalid annotation",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-endpoint-service-acceptance-required": "True",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				annotationParser: parser,
				service:          tt.svc,
			}
			got, err := builder.buildAcceptanceRequired(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildAllowedPrinciples(t *testing.T) {
	tests := []struct {
		name string
		svc  *corev1.Service
		want []string
	}{
		{
			name: "Service without annotation",
			svc:  &corev1.Service{},
			want: []string{},
		},
		{
			name: "Service with single arn",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-endpoint-service-allowed-principals": "arn1",
					},
				},
			},
			want: []string{"arn1"},
		},
		{
			name: "Service with multiple arns",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-endpoint-service-allowed-principals": "arn1,arn2",
					},
				},
			},
			want: []string{"arn1", "arn2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				annotationParser: parser,
				service:          tt.svc,
			}
			got := builder.buildAllowedPrinciples(context.Background())
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultModelBuildTask_buildPrivateDNSName(t *testing.T) {
	tests := []struct {
		name string
		svc  *corev1.Service
		want *string
	}{
		{
			name: "Service without annotation",
			svc:  &corev1.Service{},
			want: nil,
		},
		{
			name: "Service with valid annotation",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-endpoint-service-private-dns-name": "privateDnsName",
					},
				},
			},
			want: awssdk.String("privateDnsName"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				annotationParser: parser,
				service:          tt.svc,
			}
			got := builder.buildPrivateDNSName(context.Background())
			assert.Equal(t, tt.want, got)
		})
	}
}
