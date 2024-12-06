package ingress

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

func Test_defaultModelBuildTask_buildLoadBalancerMinimumCapacity(t *testing.T) {
	type fields struct {
		ingGroup Group
	}
	tests := []struct {
		name         string
		featureGates map[config.Feature]bool
		fields       fields
		want         *elbv2model.MinimumLoadBalancerCapacity
		wantErr      error
	}{
		{
			name: "capacity reservation feature disabled",
			featureGates: map[config.Feature]bool{
				config.LBCapacityReservation: false,
			},
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "ig-group-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "CapacityUnits=500",
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
										"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "CapacityUnits=500",
									},
								},
							},
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "capacity reservation from multiple Ingress that do not conflict",
			featureGates: map[config.Feature]bool{
				config.LBCapacityReservation: true,
			},
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "ig-group-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "CapacityUnits=500",
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
										"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "CapacityUnits=500",
									},
								},
							},
						},
					},
				},
			},
			want: &elbv2model.MinimumLoadBalancerCapacity{CapacityUnits: 500},
		},
		{
			name: "capacity reservation from multiple Ingress that conflict",
			featureGates: map[config.Feature]bool{
				config.LBCapacityReservation: true,
			},
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "ig-group-2"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "CapacityUnits=500",
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
										"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "CapacityUnits=200",
									},
								},
							},
						},
					},
				},
			},
			wantErr: errors.New("conflicting capacity reservation CapacityUnits: 500 | 200"),
		},
		{
			name: "non-empty annotation capacity reservation from multiple Ingress, non-empty IngressClass capacity reservation",
			featureGates: map[config.Feature]bool{
				config.LBCapacityReservation: true,
			},
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "ig-group-3"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "CapacityUnits=500",
									},
								},
							},
							IngClassConfig: ClassConfiguration{
								IngClassParams: &elbv2api.IngressClassParams{
									ObjectMeta: metav1.ObjectMeta{
										Name: "awesome-class",
									},
									Spec: elbv2api.IngressClassParamsSpec{
										MinimumLoadBalancerCapacity: &elbv2api.MinimumLoadBalancerCapacity{
											CapacityUnits: 1200,
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
										"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "CapacityUnits=600",
									},
								},
							},
						},
					},
				},
			},
			want: &elbv2model.MinimumLoadBalancerCapacity{CapacityUnits: 1200},
		},
		{
			name: "capacity reservation from Ingress that does not have annotation for setting capacity reservation",
			featureGates: map[config.Feature]bool{
				config.LBCapacityReservation: true,
			},
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "ig-group-4"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing",
									Annotations: map[string]string{},
								},
							},
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "invalid key to set the capacity reservation",
			featureGates: map[config.Feature]bool{
				config.LBCapacityReservation: true,
			},
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "ig-group-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "InvalidUnits=500",
									},
								},
							},
						},
					},
				},
			},
			wantErr: errors.New("invalid key to set the capacity: InvalidUnits, Expected key: CapacityUnits"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			featureGates := config.NewFeatureGates()
			for key, value := range tt.featureGates {
				if value {
					featureGates.Enable(key)
				} else {
					featureGates.Disable(key)
				}
			}
			task := &defaultModelBuildTask{
				annotationParser: annotationParser,
				ingGroup:         tt.fields.ingGroup,
				featureGates:     featureGates,
			}
			got, err := task.buildLoadBalancerMinimumCapacity(context.Background())
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildIngressGroupLoadBalancerMinimumCapacity(t *testing.T) {
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
			name: "capacity reservation from multiple Ingress that do not conflict",
			args: args{
				ingList: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "CapacityUnits=500",
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
									"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "CapacityUnits=500",
								},
							},
						},
					},
				},
			},
			want: map[string]string{
				"CapacityUnits": "500",
			},
		},
		{
			name: "capacity reservation from multiple Ingress that conflict",
			args: args{
				ingList: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "CapacityUnits=500",
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
									"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "CapacityUnits=200",
								},
							},
						},
					},
				},
			},
			wantErr: errors.New("conflicting capacity reservation CapacityUnits: 500 | 200"),
		},
		{
			name: "non-empty annotation capacity reservation from multiple Ingress, non-empty IngressClass capacity reservation",
			args: args{
				ingList: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "CapacityUnits=500",
								},
							},
						},
						IngClassConfig: ClassConfiguration{
							IngClassParams: &elbv2api.IngressClassParams{
								ObjectMeta: metav1.ObjectMeta{
									Name: "awesome-class",
								},
								Spec: elbv2api.IngressClassParamsSpec{
									MinimumLoadBalancerCapacity: &elbv2api.MinimumLoadBalancerCapacity{
										CapacityUnits: 1200,
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
									"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "CapacityUnits=600",
								},
							},
						},
					},
				},
			},
			want: map[string]string{
				"CapacityUnits": "1200",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			task := &defaultModelBuildTask{
				annotationParser: annotationParser,
			}
			got, err := task.buildIngressGroupLoadBalancerMinimumCapacity(tt.args.ingList)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildIngressLoadBalancerMinimumCapacity(t *testing.T) {
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
			name: "non-empty annotation capacity reservation from Ingress",
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "awesome-ing",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/minimum-load-balancer-capacity": "CapacityUnits=500",
							},
						},
					},
				},
			},
			want: map[string]string{
				"CapacityUnits": "500",
			},
		},
		{
			name: "empty capacity reservation from Ingress",
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
			got, err := task.buildIngressLoadBalancerMinimumCapacity(tt.args.ing)
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

func Test_defaultModelBuildTask_buildIngressClassLoadBalancerMinimumCapacity(t *testing.T) {
	type args struct {
		ingClassConfig ClassConfiguration
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "non-empty ingressClassParams, non-empty minimumLoadBalancerCapacity",
			args: args{
				ingClassConfig: ClassConfiguration{
					IngClassParams: &elbv2api.IngressClassParams{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-class",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							MinimumLoadBalancerCapacity: &elbv2api.MinimumLoadBalancerCapacity{
								CapacityUnits: 1200,
							},
						},
					},
				},
			},
			want: map[string]string{
				"CapacityUnits": "1200",
			},
		},
		{
			name: "non-empty ingressClassParams, empty minimumLoadBalancerCapacity",
			args: args{
				ingClassConfig: ClassConfiguration{
					IngClassParams: &elbv2api.IngressClassParams{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-class",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							MinimumLoadBalancerCapacity: nil,
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
			got, err := task.buildIngressClassLoadBalancerMinimumCapacity(tt.args.ingClassConfig)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
