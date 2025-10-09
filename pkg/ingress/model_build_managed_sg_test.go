package ingress

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
)

func Test_defaultModelBuildTask_buildManagedSecurityGroupTags(t *testing.T) {
	type fields struct {
		ingGroup            Group
		defaultTags         map[string]string
		externalManagedTags sets.String
	}
	tests := []struct {
		name    string
		fields  fields
		want    map[string]string
		wantErr error
	}{
		{
			name: "empty default tags, no tags annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{},
								},
							},
						},
					},
				},
				defaultTags: nil,
			},
			want: map[string]string{},
		},
		{
			name: "empty default tags, non-empty tags annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k1=v1",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k2=v2",
									},
								},
							},
						},
					},
				},
				defaultTags: nil,
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
		},
		{
			name: "non-empty default tags, empty tags annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{},
								},
							},
						},
					},
				},
				defaultTags: map[string]string{
					"k3": "v3",
					"k4": "v4",
				},
			},
			want: map[string]string{
				"k3": "v3",
				"k4": "v4",
			},
		},
		{
			name: "non-empty default tags, non-empty tags annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k1=v1,k3=v3a",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k2=v2",
									},
								},
							},
						},
					},
				},
				defaultTags: map[string]string{
					"k3": "v3",
					"k4": "v4",
				},
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
				"k3": "v3",
				"k4": "v4",
			},
		},
		{
			name: "empty default tags, conflicting tags annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k1=v1,k3=v3a",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k2=v2,k3=v3b",
									},
								},
							},
						},
					},
				},
				defaultTags: nil,
			},
			wantErr: errors.New("conflicting tag k3: v3a | v3b"),
		},
		{
			name: "non empty external managed tags, no conflicts",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k1=v1",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-2",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k2=v2",
									},
								},
							},
						},
					},
				},
				externalManagedTags: sets.NewString("k3"),
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
		},
		{
			name: "non empty external managed tags, has conflicts",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k1=v1",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-2",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k2=v2",
									},
								},
							},
						},
					},
				},
				externalManagedTags: sets.NewString("k2"),
			},
			wantErr: errors.New("failed build tags for Ingress awesome-ns/ing-2: external managed tag key k2 cannot be specified"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				ingGroup:            tt.fields.ingGroup,
				defaultTags:         tt.fields.defaultTags,
				externalManagedTags: tt.fields.externalManagedTags,
				annotationParser:    annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
				featureGates:        config.NewFeatureGates(),
			}
			got, err := task.buildManagedSecurityGroupTags(context.Background())
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildManagedSecurityGroupTags_FeatureGate(t *testing.T) {
	type fields struct {
		ingGroup            Group
		defaultTags         map[string]string
		enabledFeatureGates func() config.FeatureGates
	}
	tests := []struct {
		name    string
		fields  fields
		want    map[string]string
		wantErr error
	}{
		{
			name: "default tags take priority when feature gate disabled",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k1=v1,k2=v2,k3=v3",
									},
								},
							},
						},
					},
				},
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
			want: map[string]string{
				"k1": "v10",
				"k2": "v20",
				"k3": "v3",
			},
		},
		{
			name: "annotation tags take priority when feature gate enabled",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k1=v1,k2=v2,k3=v3",
									},
								},
							},
						},
					},
				},
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
				ingGroup:         tt.fields.ingGroup,
				defaultTags:      tt.fields.defaultTags,
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
				featureGates:     tt.fields.enabledFeatureGates(),
			}
			got, err := task.buildManagedSecurityGroupTags(context.Background())
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
