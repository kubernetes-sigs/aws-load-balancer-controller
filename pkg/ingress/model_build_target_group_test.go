package ingress

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	elbv2modelk8s "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
)

func Test_defaultModelBuildTask_buildTargetGroupName(t *testing.T) {
	type args struct {
		ingKey            types.NamespacedName
		svc               *corev1.Service
		port              intstr.IntOrString
		tgPort            int32
		controlPort       *int32
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
		{
			name: "include control port",
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
				controlPort:       awssdk.Int32(80),
			},
			want: "k8s-ns1-name1-7ed91338c9",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got := task.buildTargetGroupName(context.Background(), tt.args.ingKey, tt.args.svc, tt.args.port, tt.args.tgPort, tt.args.targetType, tt.args.tgProtocol, tt.args.tgProtocolVersion, tt.args.controlPort)
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
		want int32
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
				featureGates:        config.NewFeatureGates(),
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

func Test_defaultModelBuildTask_buildTargetGroupTags_FeatureGate(t *testing.T) {
	type fields struct {
		defaultTags         map[string]string
		enabledFeatureGates func() config.FeatureGates
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
			name: "default tags take priority when feature gate disabled",
			fields: fields{
				defaultTags: map[string]string{
					"k1": "v10",
					"k2": "v20",
				},
				enabledFeatureGates: func() config.FeatureGates {
					featureGates := config.NewFeatureGates()
					featureGates.Disable(config.EnableDefaultTagsLowPriority)
					return featureGates
				},
			},
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "ing-1",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/tags": "k1=v1,k2=v2,k3=v3",
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
				"k1": "v10",
				"k2": "v20",
				"k3": "v3",
			},
		},
		{
			name: "annotation tags take priority when feature gate enabled",
			fields: fields{
				defaultTags: map[string]string{
					"k1": "v10",
					"k2": "v20",
				},
				enabledFeatureGates: func() config.FeatureGates {
					featureGates := config.NewFeatureGates()
					featureGates.Enable(config.EnableDefaultTagsLowPriority)
					return featureGates
				},
			},
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "ing-1",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/tags": "k1=v1,k2=v2,k3=v3",
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
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				defaultTags:      tt.fields.defaultTags,
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
				featureGates:     tt.fields.enabledFeatureGates(),
			}
			got, err := task.buildTargetGroupTags(context.Background(), tt.args.ing, tt.args.svc)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
			for key, value := range tt.want {
				assert.Contains(t, got, key)
				assert.Equal(t, value, got[key])
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

func Test_defaultModelBuildTask_buildTargetGroupTargetControlPort(t *testing.T) {
	type args struct {
		svcAndIngAnnotations map[string]string
		svc                  *corev1.Service
		port                 intstr.IntOrString
	}
	tests := []struct {
		name    string
		args    args
		want    *int32
		wantErr error
	}{
		{
			name: "no annotation",
			args: args{
				svcAndIngAnnotations: map[string]string{},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "svc-1",
					},
				},
			},
			want:    nil,
			wantErr: nil,
		},
		{
			name: "use port number, with annotation",
			args: args{
				svcAndIngAnnotations: map[string]string{
					"alb.ingress.kubernetes.io/target-control-port.svc-1.80": "42",
				},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "svc-1",
					},
				},
				port: intstr.FromInt32(80),
			},
			want:    awssdk.Int32(42),
			wantErr: nil,
		},
		{
			name: "invalid value",
			args: args{
				svcAndIngAnnotations: map[string]string{
					"alb.ingress.kubernetes.io/target-control-port.svc-1": "invalid",
				},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "svc-1",
					},
				},
				port: intstr.FromInt32(80),
			},
			want:    nil,
			wantErr: nil,
		},
		{
			name: "use port name, with annotation",
			args: args{
				svcAndIngAnnotations: map[string]string{
					"alb.ingress.kubernetes.io/target-control-port.svc-1.portName": "3000",
				},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "svc-1",
					},
				},
				port: intstr.FromString("portName"),
			},
			want: awssdk.Int32(3000),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
			}
			got, err := task.buildTargetGroupTargetControlPort(context.Background(), tt.args.svcAndIngAnnotations, tt.args.svc, tt.args.port)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildTargetGroupBindingMultiClusterFlag(t *testing.T) {
	tests := []struct {
		name    string
		ing     ClassifiedIngress
		svc     *corev1.Service
		want    bool
		wantErr bool
	}{
		{
			name: "no annotation",
			ing: ClassifiedIngress{
				Ing: &networking.Ingress{},
			},
			svc:  &corev1.Service{},
			want: false,
		},
		{
			name: "ing annotation",
			ing: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/multi-cluster-target-group": "false",
						},
					},
				},
			},
			svc:  &corev1.Service{},
			want: false,
		},
		{
			name: "svc annotation",
			ing: ClassifiedIngress{
				Ing: &networking.Ingress{},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/multi-cluster-target-group": "false",
					},
				},
			},
			want: false,
		},
		{
			name: "ing true annotation",
			ing: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/multi-cluster-target-group": "true",
						},
					},
				},
			},
			svc:  &corev1.Service{},
			want: true,
		},
		{
			name: "svc true annotation",
			ing: ClassifiedIngress{
				Ing: &networking.Ingress{},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/multi-cluster-target-group": "true",
					},
				},
			},
			want: true,
		},
		{
			name: "mix true annotation - ing true",
			ing: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/multi-cluster-target-group": "true",
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/multi-cluster-target-group": "false",
					},
				},
			},
			want: true,
		},
		{
			name: "mix true annotation - svc true",
			ing: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/multi-cluster-target-group": "false",
						},
					},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/multi-cluster-target-group": "true",
					},
				},
			},
			want: true,
		},
		{
			name: "not a bool svc",
			ing: ClassifiedIngress{
				Ing: &networking.Ingress{},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/multi-cluster-target-group": "cat",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "not a bool ing",
			ing: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/multi-cluster-target-group": "cat",
						},
					},
				},
			},
			svc:     &corev1.Service{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
			}
			got, err := task.buildTargetGroupBindingMultiClusterFlag(tt.ing, tt.svc)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
func Test_defaultModelBuildTask_buildTargetGroupSpec(t *testing.T) {
	type args struct {
		ing     ClassifiedIngress
		svc     *corev1.Service
		port    intstr.IntOrString
		svcPort corev1.ServicePort
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "instance target type with target control port should fail",
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/target-type":                  "instance",
								"alb.ingress.kubernetes.io/backend-protocol":             "HTTP",
								"alb.ingress.kubernetes.io/backend-protocol-version":     "HTTP1",
								"alb.ingress.kubernetes.io/target-control-port.svc-1.80": "3000",
							},
						},
					},
				},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "svc-1",
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
					},
				},
				port: intstr.FromInt32(80),
				svcPort: corev1.ServicePort{
					Port:     80,
					NodePort: 30080,
				},
			},
			wantErr: errors.New("target control port is not supported for instance target target group"),
		},
		{
			name: "ip target type with target control port should succeed",
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/target-type":                  "ip",
								"alb.ingress.kubernetes.io/backend-protocol":             "HTTP",
								"alb.ingress.kubernetes.io/backend-protocol-version":     "HTTP1",
								"alb.ingress.kubernetes.io/target-control-port.svc-1.80": "3000",
							},
						},
					},
				},
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "svc-1",
					},
				},
				port: intstr.FromInt32(80),
				svcPort: corev1.ServicePort{
					Port:       80,
					TargetPort: intstr.FromInt32(8080),
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				annotationParser:    annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
				defaultTargetType:   elbv2model.TargetTypeIP,
				enableIPTargetType:  true,
				defaultTags:         map[string]string{},
				clusterName:         "test-cluster",
				ingGroup:            Group{ID: GroupID(types.NamespacedName{Namespace: "default", Name: "test"})},
				featureGates:        config.NewFeatureGates(),
				externalManagedTags: sets.NewString(),
			}
			_, err := task.buildTargetGroupSpec(context.Background(), tt.args.ing, tt.args.svc, tt.args.port, tt.args.svcPort)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildTargetGroupBindingNetworking(t *testing.T) {
	protocolTCP := elbv2api.NetworkingProtocolTCP
	intstr80 := intstr.FromInt32(80)
	intstr8080 := intstr.FromInt32(8080)
	intstr3000 := intstr.FromInt32(3000)
	intstrTrafficPort := intstr.FromString(shared_constants.HealthCheckPortTrafficPort)
	sgBackend := "sg-backend"

	tests := []struct {
		name                     string
		disableRestrictedSGRules bool

		targetPort        intstr.IntOrString
		healthCheckPort   intstr.IntOrString
		targetControlPort *int32
		tgProtocol        elbv2.Protocol
		svcPort           corev1.ServicePort
		backendSGIDToken  core.StringToken

		expected *elbv2modelk8s.TargetGroupBindingNetworking
	}{
		{
			name:                     "tcp with restricted rules disabled",
			disableRestrictedSGRules: true,
			targetPort:               intstr80,
			healthCheckPort:          intstrTrafficPort,
			tgProtocol:               elbv2.ProtocolTCP,
			backendSGIDToken:         core.LiteralStringToken(sgBackend),
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken(sgBackend),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     nil,
							},
						},
					},
				},
			},
		},
		{
			name:             "use restricted sg rules - str hc port",
			backendSGIDToken: core.LiteralStringToken(sgBackend),
			svcPort: corev1.ServicePort{
				Protocol: corev1.ProtocolTCP,
			},
			targetPort:      intstr80,
			healthCheckPort: intstrTrafficPort,
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken(sgBackend),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr80,
							},
						},
					},
				},
			},
		},
		{
			name:             "with port restricted rules, different hc",
			backendSGIDToken: core.LiteralStringToken(sgBackend),
			svcPort: corev1.ServicePort{
				Protocol: corev1.ProtocolTCP,
			},
			targetPort:      intstr80,
			healthCheckPort: intstr8080,
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken(sgBackend),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken(sgBackend),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr8080,
							},
						},
					},
				},
			},
		},
		{
			name:             "with port restricted rules, different targetControlPort",
			backendSGIDToken: core.LiteralStringToken(sgBackend),
			svcPort: corev1.ServicePort{
				Protocol: corev1.ProtocolTCP,
			},
			targetPort:        intstr80,
			healthCheckPort:   intstr8080,
			targetControlPort: awssdk.Int32(3000),
			expected: &elbv2modelk8s.TargetGroupBindingNetworking{
				Ingress: []elbv2modelk8s.NetworkingIngressRule{
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken(sgBackend),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr80,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken(sgBackend),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr8080,
							},
						},
					},
					{
						From: []elbv2modelk8s.NetworkingPeer{
							{
								SecurityGroup: &elbv2modelk8s.SecurityGroup{
									GroupID: core.LiteralStringToken(sgBackend),
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &protocolTCP,
								Port:     &intstr3000,
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				disableRestrictedSGRules: tt.disableRestrictedSGRules,
				backendSGIDToken:         tt.backendSGIDToken,
			}
			got := task.buildTargetGroupBindingNetworking(context.Background(), tt.targetPort, tt.healthCheckPort, tt.targetControlPort)
			assert.Equal(t, tt.expected, got)
		})
	}
}
