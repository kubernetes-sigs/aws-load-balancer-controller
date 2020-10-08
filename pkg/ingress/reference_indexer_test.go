package ingress

import (
	"context"
	"github.com/stretchr/testify/assert"
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
			enhancedBackendBuilder := NewDefaultEnhancedBackendBuilder(annotationParser)
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
			enhancedBackendBuilder := NewDefaultEnhancedBackendBuilder(annotationParser)
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
