package networking

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_ingressValidator_checkIngressClassAnnotationUsage(t *testing.T) {
	type fields struct {
		disableIngressClassAnnotation bool
	}
	type args struct {
		ing    *networking.Ingress
		oldIng *networking.Ingress
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "ingress creation with matching ingress.class annotation - when new usage enabled",
			fields: fields{
				disableIngressClassAnnotation: false,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress creation with matching ingress.class annotation - when new usage disabled",
			fields: fields{
				disableIngressClassAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
			},
			wantErr: errors.New("new usage of `kubernetes.io/ingress.class` annotation is forbidden"),
		},
		{
			name: "ingress creation with not-matching ingress.class annotation - when new usage disabled",
			fields: fields{
				disableIngressClassAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "nginx",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress creation with non ingress.class annotation - when new usage disabled",
			fields: fields{
				disableIngressClassAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ns-1",
						Name:        "ing-1",
						Annotations: map[string]string{},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with matching ingress.class annotation - when new usage enabled",
			fields: fields{
				disableIngressClassAnnotation: false,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "nginx",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with matching ingress.class annotation - when new usage disabled",
			fields: fields{
				disableIngressClassAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "nginx",
						},
					},
				},
			},
			wantErr: errors.New("new usage of `kubernetes.io/ingress.class` annotation is forbidden"),
		},
		{
			name: "ingress updates with not-matching ingress.class annotation - when new usage disabled",
			fields: fields{
				disableIngressClassAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "envoy",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "nginx",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with non ingress.class annotation - when new usage disabled",
			fields: fields{
				disableIngressClassAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ns-1",
						Name:        "ing-1",
						Annotations: map[string]string{},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "nginx",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with matching ingress.class annotation unchanged - when new usage disabled",
			fields: fields{
				disableIngressClassAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			classAnnotationMatcher := ingress.NewDefaultClassAnnotationMatcher("alb")
			v := &ingressValidator{
				annotationParser:              annotationParser,
				classAnnotationMatcher:        classAnnotationMatcher,
				disableIngressClassAnnotation: tt.fields.disableIngressClassAnnotation,
				logger:                        &log.NullLogger{},
			}
			err := v.checkIngressClassAnnotationUsage(tt.args.ing, tt.args.oldIng)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ingressValidator_checkGroupNameAnnotationUsage(t *testing.T) {
	type fields struct {
		disableIngressGroupAnnotation bool
	}
	type args struct {
		ing    *networking.Ingress
		oldIng *networking.Ingress
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "ingress creation with group.name annotation - when new usage enabled",
			fields: fields{
				disableIngressGroupAnnotation: false,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress creation with group.name annotation - when new usage disabled",
			fields: fields{
				disableIngressGroupAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
			},
			wantErr: errors.New("new usage of `alb.ingress.kubernetes.io/group.name` annotation is forbidden"),
		},
		{
			name: "ingress creation with non group.name annotation - when new usage disabled",
			fields: fields{
				disableIngressGroupAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ns-1",
						Name:        "ing-1",
						Annotations: map[string]string{},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with group.name annotation - when new usage enabled",
			fields: fields{
				disableIngressGroupAnnotation: false,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ns-1",
						Name:        "ing-1",
						Annotations: map[string]string{},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with group.name annotation - when new usage disabled",
			fields: fields{
				disableIngressGroupAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ns-1",
						Name:        "ing-1",
						Annotations: map[string]string{},
					},
				},
			},
			wantErr: errors.New("new usage of `alb.ingress.kubernetes.io/group.name` annotation is forbidden"),
		},
		{
			name: "ingress updates with non group.name annotation - when new usage disabled",
			fields: fields{
				disableIngressGroupAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ns-1",
						Name:        "ing-1",
						Annotations: map[string]string{},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ns-1",
						Name:        "ing-1",
						Annotations: map[string]string{},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with matching group.name annotation unchanged - when new usage disabled",
			fields: fields{
				disableIngressGroupAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with matching group.name annotation changed - when new usage disabled",
			fields: fields{
				disableIngressGroupAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group-2",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group-1",
						},
					},
				},
			},
			wantErr: errors.New("new value of `alb.ingress.kubernetes.io/group.name` annotation is forbidden"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			classAnnotationMatcher := ingress.NewDefaultClassAnnotationMatcher("alb")
			v := &ingressValidator{
				annotationParser:              annotationParser,
				classAnnotationMatcher:        classAnnotationMatcher,
				disableIngressGroupAnnotation: tt.fields.disableIngressGroupAnnotation,
				logger:                        &log.NullLogger{},
			}
			err := v.checkGroupNameAnnotationUsage(tt.args.ing, tt.args.oldIng)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
