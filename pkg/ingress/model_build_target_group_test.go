package ingress

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

func Test_defaultModelBuildTask_buildTargetGroupName(t *testing.T) {
	type args struct {
		ingKey            types.NamespacedName
		svc               *corev1.Service
		port              intstr.IntOrString
		tgPort            int64
		targetType        elbv2model.TargetType
		tgProtocol        elbv2model.Protocol
		tgProtocolVersion elbv2model.ProtocolVersion
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
				port:              intstr.FromString("http"),
				tgPort:            8080,
				targetType:        elbv2model.TargetTypeIP,
				tgProtocol:        elbv2model.ProtocolHTTP,
				tgProtocolVersion: elbv2model.ProtocolVersionHTTP1,
			},
			want: "k8s-ns1-name1-2c37289a00",
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
				port:              intstr.FromInt(80),
				tgPort:            8080,
				targetType:        elbv2model.TargetTypeIP,
				tgProtocol:        elbv2model.ProtocolHTTP,
				tgProtocolVersion: elbv2model.ProtocolVersionHTTP1,
			},
			want: "k8s-ns1-name1-ab859e54b5",
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
				port:              intstr.FromString("http"),
				tgPort:            9090,
				targetType:        elbv2model.TargetTypeIP,
				tgProtocol:        elbv2model.ProtocolHTTP,
				tgProtocolVersion: elbv2model.ProtocolVersionHTTP1,
			},
			want: "k8s-ns1-name1-6481032048",
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
				port:              intstr.FromString("http"),
				tgPort:            8080,
				targetType:        elbv2model.TargetTypeInstance,
				tgProtocol:        elbv2model.ProtocolHTTP,
				tgProtocolVersion: elbv2model.ProtocolVersionHTTP1,
			},
			want: "k8s-ns1-name1-f4adfdc175",
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
				port:              intstr.FromString("http"),
				tgPort:            8080,
				targetType:        elbv2model.TargetTypeIP,
				tgProtocol:        elbv2model.ProtocolHTTPS,
				tgProtocolVersion: elbv2model.ProtocolVersionHTTP1,
			},
			want: "k8s-ns1-name1-22fbce26a7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got := task.buildTargetGroupName(context.Background(), tt.args.ingKey, tt.args.svc, tt.args.port, tt.args.tgPort, tt.args.targetType, tt.args.tgProtocol, tt.args.tgProtocolVersion)
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

func Test_defaultModelBuildTask_buildTargetGroupTags(t *testing.T) {
	type fields struct {
		defaultTags         map[string]string
		externalManagedTags sets.String
	}
	type args struct {
		ing ClassifiedIngress
		svc *corev1.Service
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    map[string]string
		wantErr error
	}{
		{
			name: "empty default tags, empty annotation tags",
			fields: fields{
				defaultTags: nil,
			},
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "ing-1",
						},
					},
				},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "svc-1",
					},
				},
			},
			want: map[string]string{},
		},
		{
			name: "empty default tags, non-empty annotation tags from Ingress",
			fields: fields{
				defaultTags: nil,
			},
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "ing-1",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/tags": "k1=v1,k2=v2",
							},
						},
					},
				},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "svc-1",
					},
				},
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
		},
		{
			name: "empty default tags, non-empty annotation tags from Service",
			fields: fields{
				defaultTags: nil,
			},
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "ing-1",
						},
					},
				},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "svc-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/tags": "k1=v1,k2=v2",
						},
					},
				},
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
		},
		{
			name: "non-empty default tags, empty annotation tags",
			fields: fields{
				defaultTags: map[string]string{
					"k3": "v3",
					"k4": "v4",
				},
			},
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "ing-1",
						},
					},
				},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "svc-1",
					},
				},
			},
			want: map[string]string{
				"k3": "v3",
				"k4": "v4",
			},
		},
		{
			name: "non-empty default tags, non-empty annotation tags",
			fields: fields{
				defaultTags: map[string]string{
					"k3": "v3",
					"k4": "v4",
				},
			},
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "ing-1",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/tags": "k1=v1,k2=v2,k3=v3a",
							},
						},
					},
				},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "svc-1",
					},
				},
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
				"k3": "v3",
				"k4": "v4",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				defaultTags:         tt.fields.defaultTags,
				externalManagedTags: tt.fields.externalManagedTags,
				annotationParser:    annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
			}
			got, err := task.buildTargetGroupTags(context.Background(), tt.args.ing, tt.args.svc)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildTargetGroupHealthCheckPath(t *testing.T) {
	type fields struct {
		defaultHealthCheckPathHTTP string
		defaultHealthCheckPathGRPC string
	}
	type args struct {
		svcAndIngAnnotations map[string]string
		tgProtocolVersion    elbv2model.ProtocolVersion
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   string
	}{
		{
			name: "HTTP1, without annotation configured",
			fields: fields{
				defaultHealthCheckPathHTTP: "/",
				defaultHealthCheckPathGRPC: "/AWS.ALB/healthcheck",
			},
			args: args{
				svcAndIngAnnotations: nil,
				tgProtocolVersion:    elbv2model.ProtocolVersionHTTP1,
			},
			want: "/",
		},
		{
			name: "HTTP2, without annotation configured",
			fields: fields{
				defaultHealthCheckPathHTTP: "/",
				defaultHealthCheckPathGRPC: "/AWS.ALB/healthcheck",
			},
			args: args{
				svcAndIngAnnotations: nil,
				tgProtocolVersion:    elbv2model.ProtocolVersionHTTP2,
			},
			want: "/",
		},
		{
			name: "GRPC, without annotation configured",
			fields: fields{
				defaultHealthCheckPathHTTP: "/",
				defaultHealthCheckPathGRPC: "/AWS.ALB/healthcheck",
			},
			args: args{
				svcAndIngAnnotations: nil,
				tgProtocolVersion:    elbv2model.ProtocolVersionGRPC,
			},
			want: "/AWS.ALB/healthcheck",
		},
		{
			name: "HTTP1, with annotation configured",
			fields: fields{
				defaultHealthCheckPathHTTP: "/",
				defaultHealthCheckPathGRPC: "/AWS.ALB/healthcheck",
			},
			args: args{
				svcAndIngAnnotations: map[string]string{
					"alb.ingress.kubernetes.io/healthcheck-path": "/ping",
				},
				tgProtocolVersion: elbv2model.ProtocolVersionHTTP1,
			},
			want: "/ping",
		},
		{
			name: "HTTP2, with annotation configured",
			fields: fields{
				defaultHealthCheckPathHTTP: "/",
				defaultHealthCheckPathGRPC: "/AWS.ALB/healthcheck",
			},
			args: args{
				svcAndIngAnnotations: map[string]string{
					"alb.ingress.kubernetes.io/healthcheck-path": "/ping",
				},
				tgProtocolVersion: elbv2model.ProtocolVersionHTTP2,
			},
			want: "/ping",
		},
		{
			name: "GRPC, with annotation configured",
			fields: fields{
				defaultHealthCheckPathHTTP: "/",
				defaultHealthCheckPathGRPC: "/AWS.ALB/healthcheck",
			},
			args: args{
				svcAndIngAnnotations: map[string]string{
					"alb.ingress.kubernetes.io/healthcheck-path": "/package.service/method",
				},
				tgProtocolVersion: elbv2model.ProtocolVersionGRPC,
			},
			want: "/package.service/method",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				annotationParser:           annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
				defaultHealthCheckPathHTTP: tt.fields.defaultHealthCheckPathHTTP,
				defaultHealthCheckPathGRPC: tt.fields.defaultHealthCheckPathGRPC,
			}
			got := task.buildTargetGroupHealthCheckPath(context.Background(), tt.args.svcAndIngAnnotations, tt.args.tgProtocolVersion)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultModelBuildTask_buildTargetGroupHealthCheckMatcher(t *testing.T) {
	type fields struct {
		defaultHealthCheckMatcherHTTPCode string
		defaultHealthCheckMatcherGRPCCode string
	}
	type args struct {
		svcAndIngAnnotations map[string]string
		tgProtocolVersion    elbv2model.ProtocolVersion
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   elbv2model.HealthCheckMatcher
	}{
		{
			name: "HTTP1, without annotation configured",
			fields: fields{
				defaultHealthCheckMatcherHTTPCode: "200",
				defaultHealthCheckMatcherGRPCCode: "12",
			},
			args: args{
				svcAndIngAnnotations: nil,
				tgProtocolVersion:    elbv2model.ProtocolVersionHTTP1,
			},
			want: elbv2model.HealthCheckMatcher{
				HTTPCode: awssdk.String("200"),
			},
		},
		{
			name: "HTTP2, without annotation configured",
			fields: fields{
				defaultHealthCheckMatcherHTTPCode: "200",
				defaultHealthCheckMatcherGRPCCode: "12",
			},
			args: args{
				svcAndIngAnnotations: nil,
				tgProtocolVersion:    elbv2model.ProtocolVersionHTTP2,
			},
			want: elbv2model.HealthCheckMatcher{
				HTTPCode: awssdk.String("200"),
			},
		},
		{
			name: "GRPC, without annotation configured",
			fields: fields{
				defaultHealthCheckMatcherHTTPCode: "200",
				defaultHealthCheckMatcherGRPCCode: "12",
			},
			args: args{
				svcAndIngAnnotations: nil,
				tgProtocolVersion:    elbv2model.ProtocolVersionGRPC,
			},
			want: elbv2model.HealthCheckMatcher{
				GRPCCode: awssdk.String("12"),
			},
		},
		{
			name: "HTTP1, with annotation configured",
			fields: fields{
				defaultHealthCheckMatcherHTTPCode: "200",
				defaultHealthCheckMatcherGRPCCode: "12",
			},
			args: args{
				svcAndIngAnnotations: map[string]string{
					"alb.ingress.kubernetes.io/success-codes": "200-300",
				},
				tgProtocolVersion: elbv2model.ProtocolVersionHTTP1,
			},
			want: elbv2model.HealthCheckMatcher{
				HTTPCode: awssdk.String("200-300"),
			},
		},
		{
			name: "HTTP2, with annotation configured",
			fields: fields{
				defaultHealthCheckMatcherHTTPCode: "200",
				defaultHealthCheckMatcherGRPCCode: "12",
			},
			args: args{
				svcAndIngAnnotations: map[string]string{
					"alb.ingress.kubernetes.io/success-codes": "200-300",
				},
				tgProtocolVersion: elbv2model.ProtocolVersionHTTP2,
			},
			want: elbv2model.HealthCheckMatcher{
				HTTPCode: awssdk.String("200-300"),
			},
		},
		{
			name: "GRPC, with annotation configured",
			fields: fields{
				defaultHealthCheckMatcherHTTPCode: "200",
				defaultHealthCheckMatcherGRPCCode: "12",
			},
			args: args{
				svcAndIngAnnotations: map[string]string{
					"alb.ingress.kubernetes.io/success-codes": "0",
				},
				tgProtocolVersion: elbv2model.ProtocolVersionGRPC,
			},
			want: elbv2model.HealthCheckMatcher{
				GRPCCode: awssdk.String("0"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				annotationParser:                  annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
				defaultHealthCheckMatcherHTTPCode: tt.fields.defaultHealthCheckMatcherHTTPCode,
				defaultHealthCheckMatcherGRPCCode: tt.fields.defaultHealthCheckMatcherGRPCCode,
			}
			got := task.buildTargetGroupHealthCheckMatcher(context.Background(), tt.args.svcAndIngAnnotations, tt.args.tgProtocolVersion)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultModelBuildTask_buildTargetGroupBindingNodeSelector(t *testing.T) {
	type args struct {
		ing        ClassifiedIngress
		svc        *corev1.Service
		targetType elbv2model.TargetType
	}
	tests := []struct {
		name    string
		args    args
		want    *metav1.LabelSelector
		wantErr error
	}{
		{
			name: "no annotation",
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{},
						},
					},
				},
				svc: &corev1.Service{},
			},
			want: nil,
		},
		{
			name: "ingress has annotation",
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/target-node-labels": "key1=value1, node.label/key2=value.2",
							},
						},
					},
				},
				svc:        &corev1.Service{},
				targetType: elbv2model.TargetTypeInstance,
			},
			want: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"key1":            "value1",
					"node.label/key2": "value.2",
				},
			},
		},
		{
			name: "service annotation overrides ingress",
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/target-node-labels": "key1=value1, node.label/key2=value.2",
							},
						},
					},
				},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/target-node-labels": "service/key1=value1.service, service.node.label/key2=value.2.service",
						},
					},
				},
				targetType: elbv2model.TargetTypeInstance,
			},
			want: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"service/key1":            "value1.service",
					"service.node.label/key2": "value.2.service",
				},
			},
		},
		{
			name: "target type ip",
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/target-node-labels": "key1=value1, node.label/key2=value.2",
							},
						},
					},
				},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/target-node-labels": "service/key1=value1.service, service.node.label/key2=value.2.service",
						},
					},
				},
				targetType: elbv2model.TargetTypeIP,
			},
			want: nil,
		},
		{
			name: "annotation parse error",
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/target-node-labels": "key1",
							},
						},
					},
				},
				svc:        &corev1.Service{},
				targetType: elbv2model.TargetTypeInstance,
			},
			wantErr: errors.New("failed to parse stringMap annotation, alb.ingress.kubernetes.io/target-node-labels: key1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
			}
			got, err := task.buildTargetGroupBindingNodeSelector(context.Background(), tt.args.ing, tt.args.svc, tt.args.targetType)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
