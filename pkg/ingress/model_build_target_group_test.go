package ingress

import (
	"context"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

func Test_defaultModelBuildTask_buildTargetGroupName(t *testing.T) {
	type args struct {
		ingKey     types.NamespacedName
		svc        *corev1.Service
		port       intstr.IntOrString
		tgPort     int64
		targetType elbv2model.TargetType
		tgProtocol elbv2model.Protocol
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "standard case",
			args: args{
				ingKey: types.NamespacedName{Namespace: "ns-1", Name: "name-1"},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "name-1",
						UID:       "my-uuid",
					},
				},
				port:       intstr.FromString("http"),
				tgPort:     8080,
				targetType: elbv2model.TargetTypeIP,
				tgProtocol: elbv2model.ProtocolHTTP,
			},
			want: "k8s-ns1-name1-59797694c2",
		},
		{
			name: "standard case - port differs",
			args: args{
				ingKey: types.NamespacedName{Namespace: "ns-1", Name: "name-1"},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "name-1",
						UID:       "my-uuid",
					},
				},
				port:       intstr.FromInt(80),
				tgPort:     8080,
				targetType: elbv2model.TargetTypeIP,
				tgProtocol: elbv2model.ProtocolHTTP,
			},
			want: "k8s-ns1-name1-70ebbeea02",
		},
		{
			name: "standard case - tgPort differs",
			args: args{
				ingKey: types.NamespacedName{Namespace: "ns-1", Name: "name-1"},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "name-1",
						UID:       "my-uuid",
					},
				},
				port:       intstr.FromString("http"),
				tgPort:     9090,
				targetType: elbv2model.TargetTypeIP,
				tgProtocol: elbv2model.ProtocolHTTP,
			},
			want: "k8s-ns1-name1-cf545f64e8",
		},
		{
			name: "standard case - targetType differs",
			args: args{
				ingKey: types.NamespacedName{Namespace: "ns-1", Name: "name-1"},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "name-1",
						UID:       "my-uuid",
					},
				},
				port:       intstr.FromString("http"),
				tgPort:     8080,
				targetType: elbv2model.TargetTypeInstance,
				tgProtocol: elbv2model.ProtocolHTTP,
			},
			want: "k8s-ns1-name1-e66dadb781",
		},
		{
			name: "standard case - protocol differs",
			args: args{
				ingKey: types.NamespacedName{Namespace: "ns-1", Name: "name-1"},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "name-1",
						UID:       "my-uuid",
					},
				},
				port:       intstr.FromString("http"),
				tgPort:     8080,
				targetType: elbv2model.TargetTypeIP,
				tgProtocol: elbv2model.ProtocolHTTPS,
			},
			want: "k8s-ns1-name1-3e1463213f",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got := task.buildTargetGroupName(context.Background(), tt.args.ingKey, tt.args.svc, tt.args.port, tt.args.tgPort, tt.args.targetType, tt.args.tgProtocol)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultModelBuildTask_buildTargetGroupPort(t *testing.T) {
	type args struct {
		targetType elbv2model.TargetType
		svcPort    corev1.ServicePort
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			name: "instance targetGroup should use nodePort as port",
			args: args{
				targetType: elbv2model.TargetTypeInstance,
				svcPort: corev1.ServicePort{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					NodePort:   32768,
				},
			},
			want: 32768,
		},
		{
			name: "ip targetGroup with numeric targetPort should use targetPort as port",
			args: args{
				targetType: elbv2model.TargetTypeIP,
				svcPort: corev1.ServicePort{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					NodePort:   32768,
				},
			},
			want: 8080,
		},
		{
			name: "ip targetGroup with literal targetPort should use 1 as port",
			args: args{
				targetType: elbv2model.TargetTypeIP,
				svcPort: corev1.ServicePort{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromString("http"),
					NodePort:   32768,
				},
			},
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got := task.buildTargetGroupPort(context.Background(), tt.args.targetType, tt.args.svcPort)
			assert.Equal(t, tt.want, got)
		})
	}
}
