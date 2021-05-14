package ingress

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_defaultReferenceIndexer_BuildServiceRefIndexes(t *testing.T) {
	type args struct {
		ing *networking.Ingress
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "standard Ingress - with default backend",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-ing",
					},
					Spec: networking.IngressSpec{
						Backend: &networking.IngressBackend{
							ServiceName: "svc-a",
							ServicePort: intstr.FromInt(80),
						},
						Rules: []networking.IngressRule{
							{
								Host: "/hostX",
								IngressRuleValue: networking.IngressRuleValue{
									HTTP: &networking.HTTPIngressRuleValue{
										Paths: []networking.HTTPIngressPath{
											{
												Path: "/pathB",
												Backend: networking.IngressBackend{
													ServiceName: "svc-b",
													ServicePort: intstr.FromInt(80),
												},
											},
											{
												Path: "/pathC",
												Backend: networking.IngressBackend{
													ServiceName: "svc-c",
													ServicePort: intstr.FromInt(80),
												},
											},
										},
									},
								},
							},
							{
								Host: "/hostY",
								IngressRuleValue: networking.IngressRuleValue{
									HTTP: &networking.HTTPIngressRuleValue{
										Paths: []networking.HTTPIngressPath{
											{
												Path: "/pathB",
												Backend: networking.IngressBackend{
													ServiceName: "svc-b",
													ServicePort: intstr.FromInt(80),
												},
											},
											{
												Path: "/pathD",
												Backend: networking.IngressBackend{
													ServiceName: "svc-d",
													ServicePort: intstr.FromInt(80),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []string{"svc-a", "svc-b", "svc-c", "svc-d"},
		},
		{
			name: "standard Ingress - without default backend",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-ing",
					},
					Spec: networking.IngressSpec{
						Backend: nil,
						Rules: []networking.IngressRule{
							{
								Host: "/hostX",
								IngressRuleValue: networking.IngressRuleValue{
									HTTP: &networking.HTTPIngressRuleValue{
										Paths: []networking.HTTPIngressPath{
											{
												Path: "/pathB",
												Backend: networking.IngressBackend{
													ServiceName: "svc-b",
													ServicePort: intstr.FromInt(80),
												},
											},
											{
												Path: "/pathC",
												Backend: networking.IngressBackend{
													ServiceName: "svc-c",
													ServicePort: intstr.FromInt(80),
												},
											},
										},
									},
								},
							},
							{
								Host: "/hostY",
								IngressRuleValue: networking.IngressRuleValue{
									HTTP: &networking.HTTPIngressRuleValue{
										Paths: []networking.HTTPIngressPath{
											{
												Path: "/pathB",
												Backend: networking.IngressBackend{
													ServiceName: "svc-b",
													ServicePort: intstr.FromInt(80),
												},
											},
											{
												Path: "/pathD",
												Backend: networking.IngressBackend{
													ServiceName: "svc-d",
													ServicePort: intstr.FromInt(80),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []string{"svc-b", "svc-c", "svc-d"},
		},
		{
			name: "empty http path are ignored",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-ing",
					},
					Spec: networking.IngressSpec{
						Rules: []networking.IngressRule{
							{
								Host: "/hostX",
								IngressRuleValue: networking.IngressRuleValue{
									HTTP: nil,
								},
							},
							{
								Host: "/hostY",
								IngressRuleValue: networking.IngressRuleValue{
									HTTP: &networking.HTTPIngressRuleValue{
										Paths: []networking.HTTPIngressPath{
											{
												Path: "/pathB",
												Backend: networking.IngressBackend{
													ServiceName: "svc-b",
													ServicePort: intstr.FromInt(80),
												},
											},
											{
												Path: "/pathD",
												Backend: networking.IngressBackend{
													ServiceName: "svc-d",
													ServicePort: intstr.FromInt(80),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []string{"svc-b", "svc-d"},
		},
		{
			name: "standard Ingress - actions configured via use-annotation",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-ing",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/actions.forward-single-svc":   `{"type":"forward","forwardConfig":{"targetGroups":[{"serviceName":"svc-a","servicePort":"80"}]}}`,
							"alb.ingress.kubernetes.io/actions.forward-multiple-svc": `{"type":"forward","forwardConfig":{"targetGroups":[{"serviceName":"svc-b","servicePort":"80","weight":20},{"serviceName":"svc-c","servicePort":"80","weight":80}]}}`,
							"alb.ingress.kubernetes.io/actions.forward-single-tg":    `{"type":"forward","forwardConfig":{"targetGroups":[{"targetGroupArn":"tg-a"}]}}`,
							"alb.ingress.kubernetes.io/actions.forward-multiple-tg":  `{"type":"forward","forwardConfig":{"targetGroups":[{"targetGroupArn":"tg-a","weight":20},{"targetGroupArn":"tg-b","weight":20}]}}`,
						},
					},
					Spec: networking.IngressSpec{
						Backend: &networking.IngressBackend{
							ServiceName: "forward-single-svc",
							ServicePort: intstr.FromString("use-annotation"),
						},
						Rules: []networking.IngressRule{
							{
								Host: "/hostX",
								IngressRuleValue: networking.IngressRuleValue{
									HTTP: &networking.HTTPIngressRuleValue{
										Paths: []networking.HTTPIngressPath{
											{
												Path: "/pathB",
												Backend: networking.IngressBackend{
													ServiceName: "forward-multiple-svc",
													ServicePort: intstr.FromString("use-annotation"),
												},
											},
											{
												Path: "/pathC",
												Backend: networking.IngressBackend{
													ServiceName: "forward-single-tg",
													ServicePort: intstr.FromString("use-annotation"),
												},
											},
											{
												Path: "/pathD",
												Backend: networking.IngressBackend{
													ServiceName: "forward-multiple-tg",
													ServicePort: intstr.FromString("use-annotation"),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []string{"svc-a", "svc-b", "svc-c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			authConfigBuilder := NewDefaultAuthConfigBuilder(annotationParser)
			enhancedBackendBuilder := NewDefaultEnhancedBackendBuilder(nil, annotationParser, nil)
			i := &defaultReferenceIndexer{
				enhancedBackendBuilder: enhancedBackendBuilder,
				authConfigBuilder:      authConfigBuilder,
				logger:                 &log.NullLogger{},
			}
			got := i.BuildServiceRefIndexes(context.Background(), tt.args.ing)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultReferenceIndexer_BuildSecretRefIndexes(t *testing.T) {
	type args struct {
		ingOrSvc metav1.Object
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "ingress with AuthOIDC annotation",
			args: args{
				ingOrSvc: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-ing",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/auth-idp-oidc": `{"issuer":"https://example.com","authorizationEndpoint":"https://authorization.example.com","tokenEndpoint":"https://token.example.com","userInfoEndpoint":"https://userinfo.example.com","secretName":"my-k8s-secret","authenticationRequestExtraParams":{"key":"value"}}`,
						},
					},
				},
			},
			want: []string{"my-k8s-secret"},
		},
		{
			name: "ingress with no annotation",
			args: args{
				ingOrSvc: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-ing",
					},
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			authConfigBuilder := NewDefaultAuthConfigBuilder(annotationParser)
			enhancedBackendBuilder := NewDefaultEnhancedBackendBuilder(nil, annotationParser, nil)
			i := &defaultReferenceIndexer{
				enhancedBackendBuilder: enhancedBackendBuilder,
				authConfigBuilder:      authConfigBuilder,
				logger:                 &log.NullLogger{},
			}
			got := i.BuildSecretRefIndexes(context.Background(), tt.args.ingOrSvc)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultReferenceIndexer_BuildIngressClassRefIndexes(t *testing.T) {
	type args struct {
		ing *networking.Ingress
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "Ingress refers no IngressClass",
			args: args{
				ing: &networking.Ingress{
					Spec: networking.IngressSpec{
						IngressClassName: nil,
					},
				},
			},
			want: nil,
		},
		{
			name: "Ingress refers one IngressClass",
			args: args{
				ing: &networking.Ingress{
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("awesome-class"),
					},
				},
			},
			want: []string{"awesome-class"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &defaultReferenceIndexer{}
			got := i.BuildIngressClassRefIndexes(context.Background(), tt.args.ing)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultReferenceIndexer_BuildIngressClassParamsRefIndexes(t *testing.T) {
	type fields struct {
		enhancedBackendBuilder EnhancedBackendBuilder
		authConfigBuilder      AuthConfigBuilder
		logger                 logr.Logger
	}
	type args struct {
		ingClass *networking.IngressClass
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []string
	}{
		{
			name: "IngressClass refers no IngressClassParams",
			args: args{
				ingClass: &networking.IngressClass{
					Spec: networking.IngressClassSpec{
						Controller: "ingress.k8s.aws/alb",
					},
				},
			},
			want: nil,
		},
		{
			name: "IngressClass isn't controlled by ALB",
			args: args{
				ingClass: &networking.IngressClass{
					Spec: networking.IngressClassSpec{
						Controller: "k8s.io/nginx",
					},
				},
			},
			want: nil,
		},
		{
			name: "IngressClass refers one IngressClassParams",
			args: args{
				ingClass: &networking.IngressClass{
					Spec: networking.IngressClassSpec{
						Controller: "ingress.k8s.aws/alb",
						Parameters: &corev1.TypedLocalObjectReference{
							APIGroup: awssdk.String("elbv2.k8s.aws"),
							Kind:     "IngressClassParams",
							Name:     "awesome-class",
						},
					},
				},
			},
			want: []string{"awesome-class"},
		},
		{
			name: "IngressClass refers wrong APIGroup",
			args: args{
				ingClass: &networking.IngressClass{
					Spec: networking.IngressClassSpec{
						Controller: "ingress.k8s.aws/alb",
						Parameters: &corev1.TypedLocalObjectReference{
							APIGroup: awssdk.String("other group"),
							Kind:     "IngressClassParams",
							Name:     "awesome-class",
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "IngressClass refers empty APIGroup",
			args: args{
				ingClass: &networking.IngressClass{
					Spec: networking.IngressClassSpec{
						Controller: "ingress.k8s.aws/alb",
						Parameters: &corev1.TypedLocalObjectReference{
							APIGroup: nil,
							Kind:     "IngressClassParams",
							Name:     "awesome-class",
						},
					},
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &defaultReferenceIndexer{
				enhancedBackendBuilder: tt.fields.enhancedBackendBuilder,
				authConfigBuilder:      tt.fields.authConfigBuilder,
				logger:                 tt.fields.logger,
			}
			got := i.BuildIngressClassParamsRefIndexes(context.Background(), tt.args.ingClass)
			assert.Equal(t, tt.want, got)
		})
	}
}
