package ingress

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"testing"
)

func Test_defaultModelBuildTask_buildIngressGroupLoadBalancerAttributes(t *testing.T) {
	type args struct {
		ingList []ClassifiedIngress
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr error
	}{
		{
			name: "attributes from multiple Ingress that do not conflict",
			args: args{
				ingList: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/load-balancer-attributes": "idle_timeout.timeout_seconds=60",
								},
							},
						},
					},
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/load-balancer-attributes": "deletion_protection.enabled=false",
								},
							},
						},
					},
				},
			},
			want: map[string]string{
				"idle_timeout.timeout_seconds": "60",
				"deletion_protection.enabled":  "false",
			},
		},
		{
			name: "attributes from multiple Ingress that conflict",
			args: args{
				ingList: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/load-balancer-attributes": "idle_timeout.timeout_seconds=60, deletion_protection.enabled=true",
								},
							},
						},
					},
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/load-balancer-attributes": "deletion_protection.enabled=false",
								},
							},
						},
					},
				},
			},
			wantErr: errors.New("conflicting attributes deletion_protection.enabled: true | false"),
		},
		{
			name: "non-empty annotation attributes from single Ingress, non-empty IngressClass attributes - has overlap attributes",
			args: args{
				ingList: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/load-balancer-attributes": "idle_timeout.timeout_seconds=30",
								},
							},
						},
						IngClassConfig: ClassConfiguration{
							IngClassParams: &elbv2api.IngressClassParams{
								ObjectMeta: metav1.ObjectMeta{
									Name: "awesome-class",
								},
								Spec: elbv2api.IngressClassParamsSpec{
									LoadBalancerAttributes: []elbv2api.Attribute{
										{
											Key:   "idle_timeout.timeout_seconds",
											Value: "45",
										},
										{
											Key:   "access_logs.s3.enabled",
											Value: "true",
										},
									},
								},
							},
						},
					},
				},
			},
			want: map[string]string{
				"idle_timeout.timeout_seconds": "45",
				"access_logs.s3.enabled":       "true",
			},
		},
		{
			name: "non-empty annotation attributes from multiple Ingresses, non-empty IngressClass attributes - has overlap attributes",
			args: args{
				ingList: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/load-balancer-attributes": "idle_timeout.timeout_seconds=30",
								},
							},
						},
						IngClassConfig: ClassConfiguration{
							IngClassParams: &elbv2api.IngressClassParams{
								ObjectMeta: metav1.ObjectMeta{
									Name: "awesome-class",
								},
								Spec: elbv2api.IngressClassParamsSpec{
									LoadBalancerAttributes: []elbv2api.Attribute{
										{
											Key:   "idle_timeout.timeout_seconds",
											Value: "45",
										},
										{
											Key:   "access_logs.s3.enabled",
											Value: "true",
										},
									},
								},
							},
						},
					},
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/load-balancer-attributes": "idle_timeout.timeout_seconds=30, deletion_protection.enabled=true",
								},
							},
						},
						IngClassConfig: ClassConfiguration{
							IngClassParams: &elbv2api.IngressClassParams{
								ObjectMeta: metav1.ObjectMeta{
									Name: "awesome-class",
								},
								Spec: elbv2api.IngressClassParamsSpec{
									LoadBalancerAttributes: []elbv2api.Attribute{
										{
											Key:   "idle_timeout.timeout_seconds",
											Value: "45",
										},
										{
											Key:   "access_logs.s3.enabled",
											Value: "true",
										},
									},
								},
							},
						},
					},
				},
			},
			want: map[string]string{
				"idle_timeout.timeout_seconds": "45",
				"access_logs.s3.enabled":       "true",
				"deletion_protection.enabled":  "true",
			},
		},
		{
			name: "non-empty annotation attributes from Ingress, non-empty IngressClass attributes - no overlap attributes",
			args: args{
				ingList: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/load-balancer-attributes": "deletion_protection.enabled=true",
								},
							},
						},
						IngClassConfig: ClassConfiguration{
							IngClassParams: &elbv2api.IngressClassParams{
								ObjectMeta: metav1.ObjectMeta{
									Name: "awesome-class",
								},
								Spec: elbv2api.IngressClassParamsSpec{
									LoadBalancerAttributes: []elbv2api.Attribute{
										{
											Key:   "idle_timeout.timeout_seconds",
											Value: "45",
										},
										{
											Key:   "access_logs.s3.enabled",
											Value: "true",
										},
									},
								},
							},
						},
					},
				},
			},
			want: map[string]string{
				"idle_timeout.timeout_seconds": "45",
				"access_logs.s3.enabled":       "true",
				"deletion_protection.enabled":  "true",
			},
		},
		{
			name: "non-empty annotation attributes from multiple Ingresses, non-empty IngressClass attributes - no overlap attributes",
			args: args{
				ingList: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/load-balancer-attributes": "idle_timeout.timeout_seconds=30",
								},
							},
						},
						IngClassConfig: ClassConfiguration{
							IngClassParams: &elbv2api.IngressClassParams{
								ObjectMeta: metav1.ObjectMeta{
									Name: "awesome-class",
								},
								Spec: elbv2api.IngressClassParamsSpec{
									LoadBalancerAttributes: []elbv2api.Attribute{
										{
											Key:   "access_logs.s3.enabled",
											Value: "true",
										},
									},
								},
							},
						},
					},
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/load-balancer-attributes": "deletion_protection.enabled=true",
								},
							},
						},
						IngClassConfig: ClassConfiguration{
							IngClassParams: &elbv2api.IngressClassParams{
								ObjectMeta: metav1.ObjectMeta{
									Name: "awesome-class",
								},
								Spec: elbv2api.IngressClassParamsSpec{
									LoadBalancerAttributes: []elbv2api.Attribute{
										{
											Key:   "access_logs.s3.enabled",
											Value: "true",
										},
									},
								},
							},
						},
					},
				},
			},
			want: map[string]string{
				"access_logs.s3.enabled":       "true",
				"idle_timeout.timeout_seconds": "30",
				"deletion_protection.enabled":  "true",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			task := &defaultModelBuildTask{
				annotationParser: annotationParser,
			}
			got, err := task.buildIngressGroupLoadBalancerAttributes(tt.args.ingList)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildIngressLoadBalancerAttributes(t *testing.T) {
	type args struct {
		ing ClassifiedIngress
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr error
	}{
		{
			name: "non-empty annotation attributes from Ingress",
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "awesome-ing",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/load-balancer-attributes": "idle_timeout.timeout_seconds=30, access_logs.s3.enabled=true",
							},
						},
					},
				},
			},
			want: map[string]string{
				"idle_timeout.timeout_seconds": "30",
				"access_logs.s3.enabled":       "true",
			},
		},
		{
			name: "empty attributes from Ingress",
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "awesome-ing",
						},
					},
					IngClassConfig: ClassConfiguration{},
				},
			},
			want: map[string]string(nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			task := &defaultModelBuildTask{
				annotationParser: annotationParser,
			}
			got, err := task.buildIngressLoadBalancerAttributes(tt.args.ing)
			if tt.wantErr != nil {
				fmt.Println(err)
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildIngressClassLoadBalancerAttributes(t *testing.T) {
	type args struct {
		ingClassConfig ClassConfiguration
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "non-empty ingressClassParams, non-empty loadBalancerAttributes",
			args: args{
				ingClassConfig: ClassConfiguration{
					IngClassParams: &elbv2api.IngressClassParams{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-class",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							LoadBalancerAttributes: []elbv2api.Attribute{
								{
									Key:   "access_logs.s3.enabled",
									Value: "true",
								},
								{
									Key:   "idle_timeout.timeout_seconds",
									Value: "60",
								},
							},
						},
					},
				},
			},
			want: map[string]string{
				"access_logs.s3.enabled":       "true",
				"idle_timeout.timeout_seconds": "60",
			},
		},
		{
			name: "non-empty ingressClassParams, empty LoadBalancerAttributes",
			args: args{
				ingClassConfig: ClassConfiguration{
					IngClassParams: &elbv2api.IngressClassParams{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-class",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							LoadBalancerAttributes: nil,
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "empty ingressClassParams",
			args: args{
				ingClassConfig: ClassConfiguration{
					IngClassParams: nil,
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got, err := task.buildIngressClassLoadBalancerAttributes(tt.args.ingClassConfig)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
