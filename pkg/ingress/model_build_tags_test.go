package ingress

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"testing"
)

func Test_defaultModelBuildTask_buildIngressGroupResourceTags(t *testing.T) {
	type fields struct {
		externalManagedTags sets.String
	}
	type args struct {
		ingList []ClassifiedIngress
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    map[string]string
		wantErr error
	}{
		{
			name: "tags from multiple Ingress didn't collision",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
			args: args{
				ingList: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/tags": "tag-c=value-c,tag-d=value-d",
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
									"alb.ingress.kubernetes.io/tags": "tag-d=value-d,tag-e=value-e",
								},
							},
						},
					},
				},
			},
			want: map[string]string{
				"tag-c": "value-c",
				"tag-d": "value-d",
				"tag-e": "value-e",
			},
		},
		{
			name: "tags from multiple Ingress has collision",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
			args: args{
				ingList: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/tags": "tag-c=value-c,tag-d=value-d",
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
									"alb.ingress.kubernetes.io/tags": "tag-d=value-d1,tag-e=value-e",
								},
							},
						},
					},
				},
			},
			wantErr: errors.New("conflicting tag tag-d: value-d | value-d1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			task := &defaultModelBuildTask{
				annotationParser:    annotationParser,
				externalManagedTags: tt.fields.externalManagedTags,
			}
			got, err := task.buildIngressGroupResourceTags(tt.args.ingList)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildIngressResourceTags(t *testing.T) {
	type fields struct {
		externalManagedTags sets.String
	}
	type args struct {
		ing ClassifiedIngress
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    map[string]string
		wantErr error
	}{
		{
			name: "non-empty annotation tags from Ingress - collision with external-managed tags",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "awesome-ing",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/tags": "tag-b=value-b,tag-c=value-c",
							},
						},
					},
					IngClassConfig: ClassConfiguration{},
				},
			},
			wantErr: errors.New("failed build tags for Ingress awesome-ns/awesome-ing: external managed tag key tag-b cannot be specified"),
		},
		{
			name: "non-empty annotation tags from Ingress, non-empty IngressClass tags",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "awesome-ing",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/tags": "tag-c=value-c,tag-d=value-d",
							},
						},
					},
					IngClassConfig: ClassConfiguration{
						IngClassParams: &elbv2api.IngressClassParams{
							ObjectMeta: metav1.ObjectMeta{
								Name: "awesome-class",
							},
							Spec: elbv2api.IngressClassParamsSpec{
								Tags: []elbv2api.Tag{
									{
										Key:   "tag-d",
										Value: "value-d1",
									},
									{
										Key:   "tag-e",
										Value: "value-e",
									},
								},
							},
						},
					},
				},
			},
			want: map[string]string{
				"tag-c": "value-c",
				"tag-d": "value-d1",
				"tag-e": "value-e",
			},
		},
		{
			name: "empty tags from Ingress, empty tags from IngressClass",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
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
			want: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			task := &defaultModelBuildTask{
				annotationParser:    annotationParser,
				externalManagedTags: tt.fields.externalManagedTags,
			}
			got, err := task.buildIngressResourceTags(tt.args.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildIngressBackendResourceTags(t *testing.T) {
	type fields struct {
		externalManagedTags sets.String
	}
	type args struct {
		ing     ClassifiedIngress
		backend *corev1.Service
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    map[string]string
		wantErr error
	}{
		{
			name: "non-empty annotation tags from Ingress & Service - Service tags takes priority",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "awesome-ing",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/tags": "tag-c=value-c,tag-d=value-d,tag-e=value-e",
							},
						},
					},
					IngClassConfig: ClassConfiguration{},
				},
				backend: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-svc",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/tags": "tag-c=value-c1,tag-d=value-d1",
						},
					},
				},
			},
			want: map[string]string{
				"tag-c": "value-c1",
				"tag-d": "value-d1",
				"tag-e": "value-e",
			},
		},
		{
			name: "non-empty annotation tags from Ingress - collision with external-managed tags",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "awesome-ing",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/tags": "tag-b=value-b,tag-c=value-c",
							},
						},
					},
					IngClassConfig: ClassConfiguration{},
				},
				backend: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-svc",
					},
				},
			},
			wantErr: errors.New("failed build tags for Ingress awesome-ns/awesome-ing and Service awesome-ns/awesome-svc: external managed tag key tag-b cannot be specified"),
		},
		{
			name: "non-empty annotation tags from Ingress, non-empty IngressClass tags",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "awesome-ing",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/tags": "tag-c=value-c,tag-d=value-d",
							},
						},
					},
					IngClassConfig: ClassConfiguration{
						IngClassParams: &elbv2api.IngressClassParams{
							ObjectMeta: metav1.ObjectMeta{
								Name: "awesome-class",
							},
							Spec: elbv2api.IngressClassParamsSpec{
								Tags: []elbv2api.Tag{
									{
										Key:   "tag-d",
										Value: "value-d1",
									},
									{
										Key:   "tag-e",
										Value: "value-e",
									},
								},
							},
						},
					},
				},
				backend: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-svc",
					},
				},
			},
			want: map[string]string{
				"tag-c": "value-c",
				"tag-d": "value-d1",
				"tag-e": "value-e",
			},
		},
		{
			name: "non-empty annotation tags from Service, non-empty IngressClass tags",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "awesome-ing",
						},
					},
					IngClassConfig: ClassConfiguration{
						IngClassParams: &elbv2api.IngressClassParams{
							ObjectMeta: metav1.ObjectMeta{
								Name: "awesome-class",
							},
							Spec: elbv2api.IngressClassParamsSpec{
								Tags: []elbv2api.Tag{
									{
										Key:   "tag-d",
										Value: "value-d1",
									},
									{
										Key:   "tag-e",
										Value: "value-e",
									},
								},
							},
						},
					},
				},
				backend: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-svc",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/tags": "tag-c=value-c,tag-d=value-d",
						},
					},
				},
			},
			want: map[string]string{
				"tag-c": "value-c",
				"tag-d": "value-d1",
				"tag-e": "value-e",
			},
		},
		{
			name: "non-empty annotation tags from Ingress and Service, non-empty IngressClass tags",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
			args: args{
				ing: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "awesome-ns",
							Name:      "awesome-ing",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/tags": "tag-c=value-c,tag-d=value-d, tag-f=value-f",
							},
						},
					},
					IngClassConfig: ClassConfiguration{
						IngClassParams: &elbv2api.IngressClassParams{
							ObjectMeta: metav1.ObjectMeta{
								Name: "awesome-class",
							},
							Spec: elbv2api.IngressClassParamsSpec{
								Tags: []elbv2api.Tag{
									{
										Key:   "tag-d",
										Value: "value-d1",
									},
									{
										Key:   "tag-e",
										Value: "value-e",
									},
								},
							},
						},
					},
				},
				backend: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-svc",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/tags": "tag-c=value-c,tag-d=value-d2",
						},
					},
				},
			},
			want: map[string]string{
				"tag-c": "value-c",
				"tag-d": "value-d1",
				"tag-e": "value-e",
				"tag-f": "value-f",
			},
		},
		{
			name: "empty tags from Ingress & Service, empty tags from IngressClass",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
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
				backend: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-svc",
					},
				},
			},
			want: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			task := &defaultModelBuildTask{
				annotationParser:    annotationParser,
				externalManagedTags: tt.fields.externalManagedTags,
			}
			got, err := task.buildIngressBackendResourceTags(tt.args.ing, tt.args.backend)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildIngressClassResourceTags(t *testing.T) {
	type fields struct {
		externalManagedTags sets.String
	}
	type args struct {
		ingClassConfig ClassConfiguration
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    map[string]string
		wantErr error
	}{
		{
			name: "non-empty ingressClassParams, non-empty tags - no collision",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
			args: args{
				ingClassConfig: ClassConfiguration{
					IngClassParams: &elbv2api.IngressClassParams{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-class",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							Tags: []elbv2api.Tag{
								{
									Key:   "tag-c",
									Value: "value-c",
								},
								{
									Key:   "tag-d",
									Value: "value-d",
								},
							},
						},
					},
				},
			},
			want: map[string]string{
				"tag-c": "value-c",
				"tag-d": "value-d",
			},
		},
		{
			name: "non-empty ingressClassParams, non-empty tags - has collision",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
			args: args{
				ingClassConfig: ClassConfiguration{
					IngClassParams: &elbv2api.IngressClassParams{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-class",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							Tags: []elbv2api.Tag{
								{
									Key:   "tag-b",
									Value: "value-b",
								},
								{
									Key:   "tag-d",
									Value: "value-d",
								},
							},
						},
					},
				},
			},
			wantErr: errors.New("failed build tags for IngressClassParams awesome-class: external managed tag key tag-b cannot be specified"),
		},
		{
			name: "non-empty ingressClassParams, empty tags",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
			args: args{
				ingClassConfig: ClassConfiguration{
					IngClassParams: &elbv2api.IngressClassParams{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-class",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							Tags: nil,
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "empty ingressClassParams",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
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
			task := &defaultModelBuildTask{
				externalManagedTags: tt.fields.externalManagedTags,
			}
			got, err := task.buildIngressClassResourceTags(tt.args.ingClassConfig)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_validateTagCollisionWithExternalManagedTags(t *testing.T) {
	type fields struct {
		externalManagedTags sets.String
	}
	type args struct {
		tags map[string]string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "non-empty externalManagedTags, non-empty tags - no collision",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
			args: args{
				tags: map[string]string{
					"tag-c": "value-c",
					"tag-d": "value-d",
				},
			},
			wantErr: nil,
		},
		{
			name: "non-empty externalManagedTags, non-empty tags - has collision",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
			args: args{
				tags: map[string]string{
					"tag-b": "value-b",
					"tag-c": "value-c",
				},
			},
			wantErr: errors.New("external managed tag key tag-b cannot be specified"),
		},
		{
			name: "non-empty externalManagedTags, empty tags",
			fields: fields{
				externalManagedTags: sets.NewString("tag-a", "tag-b"),
			},
			args: args{
				tags: nil,
			},
			wantErr: nil,
		},
		{
			name: "empty externalManagedTags, non-empty tags",
			fields: fields{
				externalManagedTags: sets.NewString(),
			},
			args: args{
				tags: map[string]string{
					"tag-b": "value-b",
					"tag-c": "value-c",
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				externalManagedTags: tt.fields.externalManagedTags,
			}
			err := task.validateTagCollisionWithExternalManagedTags(tt.args.tags)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
